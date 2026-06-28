// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/platform/client"
	"github.com/Jamf-Concepts/jamformer/platform/download"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// PipelineOptions holds all parameters needed to run the Jamf Platform pipeline.
type PipelineOptions struct {
	OutputDir            string
	BaseURL              string
	ClientID             string
	ClientSecret         string
	TenantID             string // optional; passed to provider as JAMFPLATFORM_TENANT_ID
	SelectedResources    map[string]bool
	SkipReferences       bool
	SkipPackageDownloads bool
	SplitByCategory      bool
	ProviderVersion      string
	Quiet                bool
	Verbose              bool
	StatusFunc           func(string, int, int) // optional callback: (message, current, total)
}

// IntermediateResult holds the output of RunDiscoveryAndGenerate — everything
// needed to either run the standard single-env postprocessing or feed into the
// multi-env merge pipeline. Mirrors pro.IntermediateResult.
type IntermediateResult struct {
	Registry        *registry.Registry
	GeneratedFile   string               // path to generated.tf (import + resource blocks still present)
	OutputDir       string               // working directory
	PackageFiles    map[string]string    // JCDS file_name → relative path
	Resources       []DiscoveredResource // flat (type, label, name, id) list for multi-env matching
	ImportFiles     []string             // extra import files (singletons, jamf_connect) left in place
	PlatformCreds   *importgen.PlatformCredentials
	ProviderSchemas any // *tfjson.ProviderSchemas (kept as interface to avoid import in callers)
}

// RunPipeline executes the full Jamf Platform export pipeline:
// generate provider config → query file → terraform init → terraform query →
// post-process. Returns the validation fix result (may be nil) and any fatal
// error.
func RunPipeline(opts *PipelineOptions) (*postprocess.FixResult, error) {
	// Steps 1–6: discovery and config generation
	ir, err := RunDiscoveryAndGenerate(opts)
	if err != nil {
		return nil, err
	}

	logStep := func(format string, args ...any) {
		if !opts.Quiet {
			fmt.Printf(format+"\n", args...)
		}
	}
	status := func(msg string, current, total int) {
		if opts.StatusFunc != nil {
			opts.StatusFunc(msg, current, total)
		}
	}

	// 7. Post-process: strip nulls, rewrite references (object-attribute aware),
	// extract scripts/profiles/app-configs to support files, split per-type.
	status("post-processing", 0, 0)
	logStep("Post-processing generated configuration...")
	schemas, _ := ir.ProviderSchemas.(*tfjson.ProviderSchemas)
	if err := postprocess.Process(opts.OutputDir, ir.GeneratedFile, ir.Registry, &postprocess.ProcessOptions{
		TypeToFileMap:           TypeToFileMap(),
		Rules:                   DefaultRules(),
		ExtractSpecs:            ExtractSpecs(),
		SkipReferences:          opts.SkipReferences,
		SplitByCategory:         opts.SplitByCategory,
		ProviderSchemas:         schemas,
		InjectRequiredWriteOnly: true,
		PlatformPackageFiles:    ir.PackageFiles,
	}); err != nil {
		return nil, fmt.Errorf("post-processing: %w", err)
	}

	// 8. Clean up intermediate files.
	// Must happen before validation so terraform validate doesn't see duplicate
	// resource definitions in both generated.tf and the per-type split files.
	CleanupIntermediateFiles(opts.OutputDir)

	// 8b. Validate and auto-fix conditionally invalid attributes
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

// CleanupIntermediateFiles removes the working files produced during discovery
// and generation once post-processing has split them into per-type files.
func CleanupIntermediateFiles(outputDir string) {
	for _, name := range []string{"generated.tf", "query.tfquery.hcl", "singletons_import.tf", "jamf_connect_import.tf"} {
		_ = os.Remove(filepath.Join(outputDir, name))
	}
}

// RunDiscoveryAndGenerate executes steps 1–6 of the Jamf Platform pipeline
// (generate config → init → query → jamf_connect/singleton/icon/branding
// synthesis → package download → schema load) and returns the intermediate
// results without post-processing. The generated.tf and import files are left
// in place so callers can both post-process and enumerate the discovered
// resources. Used by both the single-env RunPipeline and the multi-env merge
// pipeline.
func RunDiscoveryAndGenerate(opts *PipelineOptions) (*IntermediateResult, error) {
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
	status := func(msg string, current, total int) {
		if opts.StatusFunc != nil {
			opts.StatusFunc(msg, current, total)
		}
	}

	// 1. Generate provider config + query file
	status("generating config", 0, 0)
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
	status("initialising terraform", 0, 0)
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
	status("discovering", 0, 0)
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

	// Drive a per-list-type discovery fraction: terraform query emits one
	// "list_complete" event per list block, counted against the number of
	// selected list types.
	listTypeTotal := countSelectedListableTypes(opts.SelectedResources)
	terraform.QueryProgressFunc = func(done int) {
		status("discovering", done, listTypeTotal)
	}
	queryErr := terraform.QueryWithEvents(opts.OutputDir, generatedFile, eventsFile, platformEnv)
	terraform.QueryProgressFunc = nil
	if queryErr != nil {
		return nil, fmt.Errorf("terraform query: %w", queryErr)
	}

	// 3b. Discover Jamf Connect config-profile links via the federated pro SDK.
	// jamfplatform_pro_jamf_connect has no list resource, so it is not
	// query-discoverable; the SDK enumerates the linked profiles and we write
	// import blocks now so the generate-config-out plan below materialises their
	// config. Requires a tenant ID (pro endpoints are tenant-scoped).
	wroteJamfConnect := false
	jcSelected := opts.SelectedResources == nil || opts.SelectedResources["jamf_connect"]
	if jcSelected {
		if opts.TenantID == "" {
			logStep("Skipping Jamf Connect discovery (set JAMF_TENANT_ID to enable — pro endpoints are tenant-scoped)")
		} else {
			pc := client.NewProClient(opts.BaseURL, opts.ClientID, opts.ClientSecret, opts.TenantID)
			if n, jcErr := DiscoverJamfConnect(terraform.Ctx, pc, opts.OutputDir); jcErr != nil {
				if !opts.Quiet {
					fmt.Printf("  Warning: Jamf Connect discovery failed: %v\n", jcErr)
				}
			} else if n > 0 {
				wroteJamfConnect = true
				logStep("  Discovered %d Jamf Connect config-profile link(s)", n)
			}
		}
	}

	// 4. Discover singleton settings: write import blocks, then generate their
	// config (and any Jamf Connect imports written above) via
	// `terraform plan -generate-config-out`, and merge into generated.tf.
	wroteSingletons, err := WriteSingletonImports(opts.OutputDir, opts.SelectedResources)
	if err != nil {
		return nil, fmt.Errorf("writing singleton imports: %w", err)
	}
	if wroteSingletons || wroteJamfConnect {
		logStep("Generating settings and adopted-resource configuration...")
		// Each singleton is read by `terraform plan -generate-config-out`, which
		// emits one "Refreshing state..." line per import block — reuse the plan
		// progress hook to show a per-singleton fraction.
		status("importing settings", 0, 0)
		terraform.ProgressFunc = func(current, total int) {
			status("importing settings", current, total)
		}
		defer func() { terraform.ProgressFunc = nil }()
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

	// 6a1. Strip compliance-benchmark-derived resources (policies, profiles,
	// computer EAs, device groups, scripts, categories). The benchmark engine
	// owns and reconciles these; importing them standalone would double-manage
	// and drift. Runs before icon/criteria synthesis so stripped resources are
	// not processed downstream.
	if stripped, err := StripComplianceBenchmarkArtifacts(generatedFile, reg); err != nil {
		if !opts.Quiet {
			fmt.Printf("  Warning: could not strip compliance-benchmark artifacts: %v\n", err)
		}
	} else if stripped > 0 {
		logStep("  Stripped %d compliance-benchmark-derived resource(s)", stripped)
	}

	// 6a2. Normalize prevent_activation_lock = true on Shared iPad prestages —
	// the only value Jamf permits (the UI forces it, the API rejects false). A
	// legacy prestage can read back with false; exporting that verbatim neither
	// validates nor applies.
	if n, err := NormalizeSharedIpadActivationLock(generatedFile); err != nil {
		if !opts.Quiet {
			fmt.Printf("  Warning: could not normalize Shared iPad activation-lock: %v\n", err)
		}
	} else if n > 0 {
		logStep("  Set prevent_activation_lock=true on %d Shared iPad prestage(s) (Jamf-enforced)", n)
	}

	if !opts.Quiet {
		counts, _ := CountResources(generatedFile)
		for resourceType, count := range counts {
			fmt.Printf("  Found %d %s\n", count, ResourceTypeDisplayName(resourceType))
		}
	}

	// 6a2. Register name→address indexes (device groups per device_type, EAs) and
	// resolve EA-name criterion fields on device groups (device_type-scoped, so it
	// can't be expressed as a generic rule). Device-group / user-group member-of
	// values are resolved later by the DefaultRules discriminator engine, which
	// relies on the device-group name index registered here.
	if err := PopulateCriteriaNameIndexes(generatedFile, reg); err != nil && !opts.Quiet {
		fmt.Printf("  Warning: could not build criteria name indexes: %v\n", err)
	}
	if eaCount, eaErr := ResolveCriteriaExtensionAttributes(generatedFile, reg); eaErr != nil {
		if !opts.Quiet {
			fmt.Printf("  Warning: could not resolve EA criteria references: %v\n", eaErr)
		}
	} else if eaCount > 0 {
		logStep("  Resolved extension-attribute criteria on %d device group(s)", eaCount)
	}

	// 6c. Synthesise jamfplatform_pro_icon resources from self_service_icon
	// references on policies (icons are not query-discoverable). Registers each
	// icon so the self_service_icon.id reference rewrites in post-processing.
	if iconCount, iconErr := GenerateIcons(generatedFile, reg); iconErr != nil {
		if !opts.Quiet {
			fmt.Printf("  Warning: could not synthesise icon resources: %v\n", iconErr)
		}
	} else if iconCount > 0 {
		logStep("  Generated %d Self Service icon resource(s)", iconCount)
	}

	// 6d. Synthesise jamfplatform_pro_self_service_branding_image resources from
	// the branding singletons' image-id references (no list resource). Downloads
	// each image via the federated pro SDK; requires a tenant ID (pro endpoints
	// are tenant-scoped). Registers each image so the singleton icon_id /
	// banner_image_id references rewrite in post-processing.
	if opts.TenantID != "" {
		pc := client.NewProClient(opts.BaseURL, opts.ClientID, opts.ClientSecret, opts.TenantID)
		if imgCount, imgErr := GenerateBrandingImages(terraform.Ctx, pc, generatedFile, opts.OutputDir, reg); imgErr != nil {
			if !opts.Quiet {
				fmt.Printf("  Warning: could not synthesise branding image resources: %v\n", imgErr)
			}
		} else if imgCount > 0 {
			logStep("  Generated %d Self Service branding image resource(s)", imgCount)
		}
	}

	// 6e. Drop api_role privileges not in the instance's assignable-privilege
	// catalog (the provider validates against this same list and rejects unknown
	// values). Tenant-scoped pro endpoint, so it requires a tenant ID.
	apiRoleSelected := opts.SelectedResources == nil || opts.SelectedResources["api_role"]
	if apiRoleSelected && opts.TenantID != "" {
		pc := client.NewProClient(opts.BaseURL, opts.ClientID, opts.ClientSecret, opts.TenantID)
		if n, prErr := FilterApiRolePrivileges(terraform.Ctx, pc, generatedFile); prErr != nil {
			if !opts.Quiet {
				fmt.Printf("  Warning: could not validate api_role privileges: %v\n", prErr)
			}
		} else if n > 0 {
			logStep("  Dropped %d api_role privilege(s) not valid on this instance", n)
		}
	}

	// 6b. Download package files resident in the Jamf Cloud Distribution Service.
	// Only JCDS-resident files are fetchable (and only when a tenant ID is set —
	// the pro endpoints are tenant-scoped); the rest stay as metadata + server
	// hashes, which the provider applies without a file. Skipped entirely with
	// -skip-package-downloads.
	packageFiles := make(map[string]string) // fileName → relative path
	pkgSelected := opts.SelectedResources == nil || opts.SelectedResources["package"]
	if !opts.SkipPackageDownloads && pkgSelected {
		if opts.TenantID == "" {
			logStep("Skipping package downloads (set JAMF_TENANT_ID to enable — JCDS is tenant-scoped)")
		} else {
			status("downloading packages", 0, 0)
			logStep("Downloading package files...")
			download.Quiet = opts.Quiet
			pc := client.NewProClient(opts.BaseURL, opts.ClientID, opts.ClientSecret, opts.TenantID)
			files, dlErr := download.Packages(terraform.Ctx, pc, opts.OutputDir, func(current, total int) {
				status("downloading packages", current, total)
			})
			if dlErr != nil {
				if !opts.Quiet {
					fmt.Printf("  Warning: package download failed: %v\n", dlErr)
				}
			} else {
				packageFiles = files
				logStep("  Downloaded %d package file(s)", len(files))
			}
		}
	} else if opts.SkipPackageDownloads && pkgSelected {
		logStep("Skipping package downloads (use without -skip-package-downloads to download)")
	}

	// Load the provider schema (used by post-processing for null stripping and by
	// the multi-env merge pipeline). Requires a finalized provider.tf and a
	// completed init, both done above.
	schemas, err := terraform.ProvidersSchema(opts.OutputDir)
	if err != nil && !postprocess.Quiet {
		fmt.Printf("  Warning: could not load provider schema, skipping null attribute removal: %v\n", err)
	}

	// Enumerate the discovered resources (type, label, name, id) for multi-env
	// matching. Joins resource blocks in generated.tf with their import blocks
	// (list imports live in generated.tf; singleton/jamf_connect imports live in
	// their own files, still present at this point).
	importFiles := []string{
		filepath.Join(opts.OutputDir, "singletons_import.tf"),
		filepath.Join(opts.OutputDir, "jamf_connect_import.tf"),
	}
	discovered, err := CollectResourceRefs(generatedFile, importFiles...)
	if err != nil && !opts.Quiet {
		fmt.Printf("  Warning: could not enumerate discovered resources: %v\n", err)
	}

	return &IntermediateResult{
		Registry:        reg,
		GeneratedFile:   generatedFile,
		OutputDir:       opts.OutputDir,
		PackageFiles:    packageFiles,
		Resources:       discovered,
		ImportFiles:     importFiles,
		PlatformCreds:   platformCreds,
		ProviderSchemas: schemas,
	}, nil
}
