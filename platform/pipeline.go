// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// PipelineOptions holds all parameters needed to run the Jamf Platform pipeline.
type PipelineOptions struct {
	OutputDir         string
	BaseURL           string
	ClientID          string
	ClientSecret      string
	TenantID          string // optional; passed to provider as JAMFPLATFORM_TENANT_ID
	SelectedResources map[string]bool
	SkipReferences    bool
	ProviderVersion   string
	Quiet             bool
	Verbose           bool
	StatusFunc        func(string, int, int) // optional callback: (message, current, total)
}

// RunPipeline executes the full Jamf Platform export pipeline:
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
	logStep("Generating Terraform configuration for Jamf Platform...")
	platformCreds := &importgen.PlatformCredentials{
		BaseURL:         opts.BaseURL,
		ClientID:        opts.ClientID,
		ClientSecret:    opts.ClientSecret,
		TenantID:        opts.TenantID,
		ProviderVersion: opts.ProviderVersion,
	}
	if err := importgen.GeneratePlatform(opts.OutputDir, platformCreds); err != nil {
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
	if platformCreds.ProviderVersion == "" {
		platformCreds.ResolvedVersion = terraform.ResolvedProviderVersion(opts.OutputDir, terraform.ProviderSourceJamfPlatform)
	}

	// 3. Terraform query (list resources). Capture the JSON event stream so we
	// can recover device groups' computed jamf_pro_id (dropped by
	// -generate-config-out) for classic-API scope bridging.
	status("discovering")
	logStep("Discovering and generating configuration...")
	generatedFile := filepath.Join(opts.OutputDir, "generated.tf")
	eventsFile := filepath.Join(opts.OutputDir, "query_events.json")
	platformEnv := map[string]string{
		"JAMFPLATFORM_BASE_URL":      opts.BaseURL,
		"JAMFPLATFORM_CLIENT_ID":     opts.ClientID,
		"JAMFPLATFORM_CLIENT_SECRET": opts.ClientSecret,
	}
	if opts.TenantID != "" {
		platformEnv["JAMFPLATFORM_TENANT_ID"] = opts.TenantID
	}

	if err := terraform.QueryWithEvents(opts.OutputDir, generatedFile, eventsFile, platformEnv); err != nil {
		return nil, fmt.Errorf("terraform query: %w", err)
	}

	// 4. Discover singleton settings: write import blocks, then generate their
	// config via `terraform plan -generate-config-out`, and merge into generated.tf.
	wroteSingletons, err := WriteSingletonImports(opts.OutputDir, opts.SelectedResources)
	if err != nil {
		return nil, fmt.Errorf("writing singleton imports: %w", err)
	}
	if wroteSingletons {
		logStep("Generating singleton settings configuration...")
		singletonsFile := filepath.Join(opts.OutputDir, "singletons_generated.tf")
		// The singleton plan only generates config for the singleton import
		// blocks; it does not reference the main resources. Stage the (potentially
		// very large) generated.tf out of the working directory for the duration
		// of the plan so terraform does not re-parse it. A large config with
		// inline script_contents (e.g. ~700KB compliance-benchmark scripts) makes
		// `terraform plan` pathologically slow — script extraction to file()
		// happens later in post-processing, so the inline form is still present
		// at this point.
		staged := generatedFile + ".staged"
		if err := os.Rename(generatedFile, staged); err != nil {
			return nil, fmt.Errorf("staging generated config for singleton plan: %w", err)
		}
		genErr := terraform.GenerateConfig(opts.OutputDir, singletonsFile, platformEnv)
		if err := os.Rename(staged, generatedFile); err != nil {
			return nil, fmt.Errorf("restoring generated config after singleton plan: %w", err)
		}
		if genErr != nil {
			return nil, fmt.Errorf("terraform plan (singletons): %w", genErr)
		}
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
	if err := importgen.FinalizePlatform(opts.OutputDir, platformCreds); err != nil {
		return nil, fmt.Errorf("finalizing provider config: %w", err)
	}

	// 5. Rename auto-generated labels (all_0, all_1) to friendly names, folding
	// device_type into jamfplatform_device_group labels.
	if err := RenameLabels(generatedFile); err != nil {
		return nil, fmt.Errorf("renaming labels: %w", err)
	}

	// 6. Populate registry from generated config for reference resolution, then
	// add the synthetic computer/mobile device-group subtypes keyed by jamf_pro_id.
	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		return nil, fmt.Errorf("populating registry: %w", err)
	}
	if dgInfo, err := CollectDeviceGroupInfo(generatedFile); err == nil {
		if mErr := MergeJamfProIDsFromEvents(dgInfo, eventsFile); mErr != nil && !opts.Quiet {
			fmt.Printf("  Warning: could not read device-group jamf_pro_id from query events: %v\n", mErr)
		}
		if unbridged := PopulateDeviceGroupSubtypes(reg, dgInfo); unbridged > 0 && !opts.Quiet {
			fmt.Printf("  Warning: %d device group(s) lack a recoverable jamf_pro_id; classic scope references to them stay as raw IDs\n", unbridged)
		}
	} else if !opts.Quiet {
		fmt.Printf("  Warning: could not collect device-group info: %v\n", err)
	}
	_ = os.Remove(eventsFile)

	if !opts.Quiet {
		counts, _ := CountResources(generatedFile)
		for resourceType, count := range counts {
			fmt.Printf("  Found %d %s\n", count, ResourceTypeDisplayName(resourceType))
		}
	}

	// 7. Post-process: strip nulls, rewrite references (object-attribute aware),
	// extract scripts/profiles/app-configs to support files, split per-type.
	status("post-processing")
	logStep("Post-processing generated configuration...")
	schemas, err := terraform.ProvidersSchema(opts.OutputDir)
	if err != nil && !postprocess.Quiet {
		fmt.Printf("  Warning: could not load provider schema, skipping null attribute removal: %v\n", err)
	}
	if err := postprocess.Process(opts.OutputDir, generatedFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap:           TypeToFileMap(),
		Rules:                   DefaultRules(),
		ExtractSpecs:            ExtractSpecs(),
		SkipReferences:          opts.SkipReferences,
		ProviderSchemas:         schemas,
		InjectRequiredWriteOnly: true,
	}); err != nil {
		return nil, fmt.Errorf("post-processing: %w", err)
	}

	// 8. Clean up intermediate files
	// Must happen before validation so terraform validate doesn't see duplicate
	// resource definitions in both generated.tf and the per-type split files.
	_ = os.Remove(generatedFile)
	_ = os.Remove(filepath.Join(opts.OutputDir, "query.tfquery.hcl"))
	_ = os.Remove(filepath.Join(opts.OutputDir, "singletons_import.tf"))

	// 8. Validate and auto-fix conditionally invalid attributes
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
