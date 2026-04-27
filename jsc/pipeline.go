// Copyright 2026, Jamf Software LLC

package jsc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// Quiet suppresses progress messages.
var Quiet bool

// PipelineOptions holds all parameters needed to run the JSC pipeline.
type PipelineOptions struct {
	OutputDir         string
	URL               string // radar.wandera.com or custom domain
	Username          string
	Password          string
	SelectedResources map[string]bool
	SkipReferences    bool
	ProviderVersion   string
	Quiet             bool
	Verbose           bool
	StatusFunc        func(string, int, int) // optional callback: (message, current, total)
}

// RunPipeline executes the full JSC export pipeline:
// generate provider config → discover resources → generate imports → terraform plan → post-process.
// Returns the validation fix result (may be nil) and any fatal error.
func RunPipeline(opts *PipelineOptions) (*postprocess.FixResult, error) {
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

	// 1. Generate provider config
	status("generating config")
	logStep("Generating Terraform configuration for JSC...")
	jscCreds := &importgen.JSCCredentials{
		URL:             opts.URL,
		Username:        opts.Username,
		Password:        opts.Password,
		ProviderVersion: opts.ProviderVersion,
	}
	if err := importgen.GenerateJSC(opts.OutputDir, jscCreds); err != nil {
		return nil, fmt.Errorf("generating provider config: %w", err)
	}

	// 2. Generate discovery config (data sources)
	logStep("Generating resource discovery configuration...")
	if err := generateDiscoveryConfig(opts.OutputDir, opts.SelectedResources); err != nil {
		return nil, fmt.Errorf("generating discovery config: %w", err)
	}

	// 3. Terraform init
	// With dev_overrides in .terraformrc, init will fail because Terraform
	// still queries the registry. We detect this specific error and continue.
	status("initialising terraform")
	logStep("Initialising Terraform provider...")
	if err := terraform.Init(opts.OutputDir); err != nil {
		errStr := err.Error()
		// Check if this is a "provider not found" error (dev_overrides scenario)
		if strings.Contains(errStr, "does not have a provider named") ||
			strings.Contains(errStr, "Failed to query available provider packages") {
			if !opts.Quiet {
				fmt.Println("  Using local provider (dev_overrides detected)")
			}
		} else {
			return nil, fmt.Errorf("terraform init: %w", err)
		}
	}

	// Resolve provider version from lock file and rewrite provider.tf
	if jscCreds.ProviderVersion == "" {
		jscCreds.ResolvedVersion = terraform.ResolvedProviderVersion(opts.OutputDir, terraform.ProviderSourceJSC)
		if jscCreds.ResolvedVersion != "" {
			if err := importgen.GenerateJSC(opts.OutputDir, jscCreds); err != nil {
				return nil, fmt.Errorf("rewriting provider config with resolved version: %w", err)
			}
		}
	}

	// 4. Run terraform apply to populate data sources
	// Credentials are passed via terraform.tfvars, not env vars
	status("discovering")
	logStep("Discovering resources...")
	if err := terraform.Apply(opts.OutputDir, nil); err != nil {
		return nil, fmt.Errorf("terraform apply (discovery): %w", err)
	}

	// 5. Parse state to get discovered resources
	results, err := parseDiscoveryState(opts.OutputDir, opts.SelectedResources)
	if err != nil {
		return nil, fmt.Errorf("parsing discovery state: %w", err)
	}

	// Print counts
	if !opts.Quiet {
		if len(results.ActivationProfiles) > 0 {
			fmt.Printf("  Found %d Activation Profiles\n", len(results.ActivationProfiles))
		}
		if len(results.EntraIdps) > 0 {
			fmt.Printf("  Found %d Entra IdP Connections\n", len(results.EntraIdps))
		}
		if len(results.HostnameMappings) > 0 {
			fmt.Printf("  Found %d Hostname Mappings\n", len(results.HostnameMappings))
		}
		if len(results.AccessPolicies) > 0 {
			fmt.Printf("  Found %d Access Policies\n", len(results.AccessPolicies))
		}
	}

	// 6. Clean up discovery files
	cleanupDiscoveryFiles(opts.OutputDir)

	// 7. Generate import blocks
	logStep("Generating import blocks...")
	reg := registry.New()

	if err := writeJSCImportFiles(opts.OutputDir, results, opts.SelectedResources, reg); err != nil {
		return nil, fmt.Errorf("writing import files: %w", err)
	}

	// 8. Run terraform plan -generate-config-out
	status("generating config")
	logStep("Generating resource configuration...")
	generatedFile := filepath.Join(opts.OutputDir, "generated.tf")
	if err := terraform.GenerateConfig(opts.OutputDir, generatedFile, nil); err != nil {
		return nil, fmt.Errorf("terraform plan: %w", err)
	}

	// 9. Post-process: strip nulls, rewrite references, split into per-type files
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

	// 10. Clean up intermediate files
	// Must happen before validation so terraform validate doesn't see duplicate
	// resource definitions in both generated.tf and the per-type split files.
	_ = os.Remove(generatedFile)

	// 11. Validate and auto-fix conditionally invalid attributes
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

// writeJSCImportFiles writes import blocks for all discovered resources.
func writeJSCImportFiles(outputDir string, results *DiscoveryResults, selectedResources map[string]bool, reg *registry.Registry) error {
	// Activation Profiles
	if selectedResources == nil || selectedResources["activation_profiles"] {
		if err := writeImportFile(outputDir, "activation_profiles_import.tf", "jsc_ap", results.ActivationProfiles, reg); err != nil {
			return err
		}
	}

	// Entra IdPs
	if selectedResources == nil || selectedResources["entra_idps"] {
		if err := writeImportFile(outputDir, "entra_idps_import.tf", "jsc_entra_idp", results.EntraIdps, reg); err != nil {
			return err
		}
	}

	// Hostname Mappings
	if selectedResources == nil || selectedResources["hostname_mappings"] {
		if err := writeImportFile(outputDir, "hostname_mappings_import.tf", "jsc_hostnamemapping", results.HostnameMappings, reg); err != nil {
			return err
		}
	}

	// Access Policies
	if selectedResources == nil || selectedResources["access_policies"] {
		if err := writeImportFile(outputDir, "access_policies_import.tf", "jsc_access_policy", results.AccessPolicies, reg); err != nil {
			return err
		}
	}

	// Singletons
	if selectedResources == nil || selectedResources["secure_policy"] {
		if err := writeSingletonImportFile(outputDir, "secure_policy_import.tf", "jsc_secure_policy", "secure_policy", "secure_policy", reg); err != nil {
			return err
		}
	}

	return nil
}

func writeImportFile(outputDir, filename, resourceType string, resources []DiscoveredResource, reg *registry.Registry) error {
	if len(resources) == 0 {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for i, r := range resources {
		if i > 0 {
			body.AppendNewline()
		}

		block := body.AppendNewBlock("import", nil)
		blockBody := block.Body()

		addr := fmt.Sprintf("%s.%s", resourceType, r.Label)
		toTokens := hclwrite.Tokens{
			{Type: 9, Bytes: []byte(addr)}, // hclsyntax.TokenIdent = 9
		}
		blockBody.SetAttributeRaw("to", toTokens)
		blockBody.SetAttributeValue("id", cty.StringVal(r.ID))

		// Register for reference resolution
		reg.Register(resourceType, r.ID, addr)
	}

	return os.WriteFile(filepath.Join(outputDir, filename), f.Bytes(), 0644)
}

func writeSingletonImportFile(outputDir, filename, resourceType, label, importID string, reg *registry.Registry) error {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	block := body.AppendNewBlock("import", nil)
	blockBody := block.Body()

	addr := fmt.Sprintf("%s.%s", resourceType, label)
	toTokens := hclwrite.Tokens{
		{Type: 9, Bytes: []byte(addr)},
	}
	blockBody.SetAttributeRaw("to", toTokens)
	blockBody.SetAttributeValue("id", cty.StringVal(importID))

	reg.Register(resourceType, importID, addr)

	return os.WriteFile(filepath.Join(outputDir, filename), f.Bytes(), 0644)
}
