// Copyright 2026, Jamf Software LLC

package protect

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// PipelineOptions holds all parameters needed to run the Jamf Protect pipeline.
type PipelineOptions struct {
	OutputDir          string
	URL                string
	ClientID           string
	ClientSecret       string
	SelectedResources  map[string]bool
	SkipReferences     bool
	ProviderVersion    string
	Quiet              bool
	Verbose            bool
	SkipDataForwarding bool                   // pre-discovered: no forwarding services enabled
	StatusFunc         func(string, int, int) // optional callback: (message, current, total)
}

// RunPipeline executes the full Jamf Protect export pipeline:
// generate provider config → query file → terraform init → terraform query → post-process.
// Returns the validation fix result (may be nil) and any fatal error.
func RunPipeline(opts *PipelineOptions) (*postprocess.FixResult, error) {
	// Set quiet/verbose flags for sub-packages
	Quiet = opts.Quiet
	importgen.Quiet = opts.Quiet
	postprocess.Quiet = opts.Quiet
	terraform.Quiet = opts.Quiet
	terraform.Verbose = opts.Verbose

	logStep := func(format string, args ...any) {
		if !opts.Quiet {
			fmt.Printf(format+"\n", args...)
		}
	}
	status := func(msg string) {
		if opts.StatusFunc != nil {
			opts.StatusFunc(msg, 0, 0)
		}
	}

	// 1. Generate provider config + query file
	status("generating config")
	logStep("Generating Terraform configuration for Jamf Protect...")
	protectCreds := &importgen.ProtectCredentials{
		URL:             opts.URL,
		ClientID:        opts.ClientID,
		ClientSecret:    opts.ClientSecret,
		ProviderVersion: opts.ProviderVersion,
	}
	if err := importgen.GenerateProtect(opts.OutputDir, protectCreds); err != nil {
		return nil, fmt.Errorf("generating provider config: %w", err)
	}
	if err := GenerateQueryFile(opts.OutputDir, opts.SelectedResources); err != nil {
		return nil, fmt.Errorf("generating query file: %w", err)
	}

	// 2. Terraform init
	status("initialising terraform")
	logStep("Initialising Terraform provider...")
	if err := terraform.Init(opts.OutputDir); err != nil {
		return nil, fmt.Errorf("terraform init: %w", err)
	}

	// Resolve provider version from lock file for >= constraint
	if protectCreds.ProviderVersion == "" {
		protectCreds.ResolvedVersion = terraform.ResolvedProviderVersion(opts.OutputDir, terraform.ProviderSourceJamfProtect)
	}

	// 3. Terraform query (list resources). Capture the JSON event stream so
	// we can derive labels from display_name for resource types whose name
	// attribute is computed and therefore omitted from the generated HCL
	// (e.g. jamfprotect_analytic_managed).
	status("discovering")
	logStep("Discovering and generating configuration...")
	generatedFile := filepath.Join(opts.OutputDir, "generated.tf")
	eventsFile := filepath.Join(opts.OutputDir, "query_events.json")
	protectEnv := map[string]string{
		"JAMFPROTECT_URL":           opts.URL,
		"JAMFPROTECT_CLIENT_ID":     opts.ClientID,
		"JAMFPROTECT_CLIENT_SECRET": opts.ClientSecret,
	}
	if err := terraform.QueryWithEvents(opts.OutputDir, generatedFile, eventsFile, protectEnv); err != nil {
		return nil, fmt.Errorf("terraform query: %w", err)
	}
	idToName, err := ParseQueryEvents(eventsFile)
	if err != nil {
		if !opts.Quiet {
			fmt.Printf("  Warning: could not parse query events: %v\n", err)
		}
		idToName = nil
	}
	_ = os.Remove(eventsFile)

	// 4. Write singleton import blocks (after query, so they don't interfere)
	if err := WriteSingletonImports(opts.OutputDir, opts.SelectedResources, opts.SkipDataForwarding); err != nil {
		return nil, fmt.Errorf("writing singleton imports: %w", err)
	}

	// 5. Generate singleton resource blocks from import blocks via terraform plan
	singletonsFile := filepath.Join(opts.OutputDir, "singletons_generated.tf")
	if _, statErr := os.Stat(filepath.Join(opts.OutputDir, "singletons_import.tf")); statErr == nil {
		logStep("Generating singleton resource configuration...")
		if err := terraform.GenerateConfig(opts.OutputDir, singletonsFile, protectEnv); err != nil {
			return nil, fmt.Errorf("terraform plan (singletons): %w", err)
		}

		// Merge singleton resources into main generated file
		if singletonData, err := os.ReadFile(singletonsFile); err == nil {
			mainData, err := os.ReadFile(generatedFile)
			if err != nil {
				return nil, fmt.Errorf("reading generated config for singleton merge: %w", err)
			}
			if err := os.WriteFile(generatedFile, append(mainData, singletonData...), 0644); err != nil {
				return nil, fmt.Errorf("merging singleton config: %w", err)
			}
			_ = os.Remove(singletonsFile)
		}
	}

	// Write the user-facing provider.tf (with var.* refs), variables.tf, terraform.tfvars
	if err := importgen.FinalizeProtect(opts.OutputDir, protectCreds); err != nil {
		return nil, fmt.Errorf("finalizing provider config: %w", err)
	}

	// 5. Rename auto-generated labels (all_0, all_1) to friendly names
	if err := RenameLabelsWithEvents(generatedFile, idToName); err != nil {
		return nil, fmt.Errorf("renaming labels: %w", err)
	}

	// 6. Populate registry from generated config for reference resolution
	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		return nil, fmt.Errorf("populating registry: %w", err)
	}

	if !opts.Quiet {
		counts, _ := CountResources(generatedFile)
		for resourceType, count := range counts {
			fmt.Printf("  Found %d %s\n", count, ResourceTypeDisplayName(resourceType))
		}
	}

	// 7. Post-process: strip nulls, rewrite references, split into per-type files
	status("post-processing")
	logStep("Post-processing generated configuration...")
	schemas, err := terraform.ProvidersSchema(opts.OutputDir)
	if err != nil && !postprocess.Quiet {
		fmt.Printf("  Warning: could not load provider schema, skipping null attribute removal: %v\n", err)
	}
	if err := postprocess.Process(opts.OutputDir, generatedFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap:   TypeToFileMap(),
		Rules:           DefaultRules(),
		SkipReferences:  opts.SkipReferences,
		ProviderSchemas: schemas,
	}); err != nil {
		return nil, fmt.Errorf("post-processing: %w", err)
	}

	// 8. Clean up intermediate files
	// Must happen before validation so terraform validate doesn't see duplicate
	// resource definitions in both generated.tf and the per-type split files.
	_ = os.Remove(generatedFile)
	_ = os.Remove(filepath.Join(opts.OutputDir, "query.tfquery.hcl"))

	// 9. Validate and auto-fix conditionally invalid attributes
	var providerSchema *postprocess.ProviderSchema
	if schemas != nil {
		providerSchema = postprocess.LoadProviderSchema(schemas)
	}
	fixResult, err := postprocess.FixValidationErrors(opts.OutputDir, providerSchema)
	if err != nil {
		if !postprocess.Quiet {
			fmt.Printf("  Warning: validation fix failed: %v\n", err)
		}
	} else {
		if fixResult.Fixed > 0 {
			logStep("  Fixed %d conditionally invalid attributes", fixResult.Fixed)
		}
		for _, v := range fixResult.RequiredVars {
			logStep("  ⚠ %s: replaced null with var.%s (sensitive, value required)", v.Resource, v.VarName)
		}
	}

	return fixResult, nil
}
