// Copyright 2026, Jamf Software LLC

package pro

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/pro/client"
	"github.com/Jamf-Concepts/jamformer/pro/discovery"
	"github.com/Jamf-Concepts/jamformer/pro/download"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// PipelineOptions holds all parameters needed to run the Jamf Pro pipeline.
type PipelineOptions struct {
	OutputDir            string
	URL                  string
	AuthMethod           string
	Username             string
	Password             string
	ClientID             string
	ClientSecret         string
	SelectedResources    map[string]bool
	SkipReferences       bool
	SplitByCategory      bool
	SkipPackageDownloads bool
	ProviderVersion      string
	Quiet                bool
	Verbose              bool
	ResourcesFlag        string                 // original -include-resources value (for log message)
	ExcludeFlag          string                 // original -exclude-resources value (for log message)
	StatusFunc           func(string, int, int) // optional callback: (message, current, total)
}

// IntermediateResult holds the output of RunDiscoveryAndGenerate — everything
// needed to either run the standard single-env postprocessing or feed into
// the multi-env merge pipeline.
type IntermediateResult struct {
	Registry        *registry.Registry
	Resources       *discovery.Results
	GeneratedFile   string            // path to generated.tf
	OutputDir       string            // working directory
	PackageFiles    map[string]string // filename → relative path
	PackageInfo     map[string]string // package_name → filename
	IconURLs        map[string]string // icon jamf ID → CDN URL
	ECImageFiles    map[string]string // enrollment customization jamf ID → relative path
	ImportCreds     *importgen.Credentials
	ProviderSchemas any // *tfjson.ProviderSchemas (kept as interface to avoid import in callers)
}

// RunDiscoveryAndGenerate executes steps 1–6 of the Jamf Pro pipeline
// (authenticate → discover → download → import gen → terraform init → terraform plan)
// and returns the intermediate results without postprocessing.
// This is used by both the single-env RunPipeline and the multi-env merge pipeline.
func RunDiscoveryAndGenerate(opts *PipelineOptions) (*IntermediateResult, error) {
	// Set quiet/verbose flags for sub-packages
	discovery.Quiet = opts.Quiet
	download.Quiet = opts.Quiet
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

	// 1. Authenticate
	status("authenticating", 0, 0)
	logStep("Verifying Jamf Pro authentication...")
	jamfClient, err := client.VerifyAuth(&client.AuthConfig{
		URL:          opts.URL,
		AuthMethod:   opts.AuthMethod,
		Username:     opts.Username,
		Password:     opts.Password,
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// 2. Discover resources
	status("discovering", 0, 0)
	logStep("Discovering resources...")
	if opts.SelectedResources != nil && !opts.Quiet {
		if opts.ExcludeFlag != "" {
			fmt.Printf("  Excluding: %s\n", opts.ExcludeFlag)
		} else {
			fmt.Printf("  Filtering to: %s\n", opts.ResourcesFlag)
		}
	}
	reg := registry.New()
	packageInfo := make(map[string]string) // package_name -> filename

	// Count selected discovery types to track progress
	discoveryTotal := countDiscoveryTypes(opts.SelectedResources)
	var discoveryDone atomic.Int32
	discoveryProgress := func() {
		n := int(discoveryDone.Add(1))
		status("discovering", n, discoveryTotal)
	}

	resources, err := discovery.DiscoverAll(jamfClient, reg, packageInfo, opts.SelectedResources, discoveryProgress)
	if err != nil {
		return nil, fmt.Errorf("discovering resources: %w", err)
	}

	// Populate singleton settings resources (no API call needed)
	selected := func(name string) bool {
		return opts.SelectedResources == nil || opts.SelectedResources[name]
	}
	resources.Singletons = make(map[string][]discovery.Resource)
	for _, r := range Resources {
		if !r.IsSingleton || !selected(r.FilterKey) {
			continue
		}
		label := "settings"
		resources.Singletons[r.TFType] = []discovery.Resource{
			{JamfID: r.SingletonImportID, Name: r.DisplayName, Label: label},
		}
	}

	if !opts.Quiet {
		printCount := func(name string, filterKey string, count int) {
			if opts.SelectedResources == nil || opts.SelectedResources[filterKey] {
				fmt.Printf("  Found %d %s\n", count, name)
			}
		}
		printCount("sites", "sites", len(resources.Sites))
		printCount("buildings", "buildings", len(resources.Buildings))
		printCount("categories", "categories", len(resources.Categories))
		printCount("departments", "departments", len(resources.Departments))
		printCount("scripts", "scripts", len(resources.Scripts))
		printCount("computer extension attributes", "extension_attributes", len(resources.ComputerExtensionAttributes))
		printCount("packages", "packages", len(resources.Packages))
		printCount("dock items", "dock_items", len(resources.DockItems))
		printCount("printers", "printers", len(resources.Printers))
		printCount("network segments", "network_segments", len(resources.NetworkSegments))
		printCount("smart computer groups", "smart_computer_groups", len(resources.SmartComputerGroups))
		printCount("static computer groups", "static_computer_groups", len(resources.StaticComputerGroups))
		printCount("macOS configuration profiles", "macos_configuration_profiles", len(resources.MacOSConfigurationProfiles))
		printCount("policies", "policies", len(resources.Policies))
		printCount("icons", "icons", len(resources.Icons))
		printCount("enrollment customizations", "enrollment_customizations", len(resources.EnrollmentCustomizations))
		printCount("computer prestages", "computer_prestages", len(resources.ComputerPrestages))
		printCount("advanced computer searches", "advanced_computer_searches", len(resources.AdvancedComputerSearches))
		printCount("app installers", "app_installers", len(resources.AppInstallers))
		printCount("mac applications", "mac_applications", len(resources.MacApplications))
		printCount("device enrollments", "device_enrollments", len(resources.DeviceEnrollments))
		printCount("volume purchasing locations", "volume_purchasing_locations", len(resources.VolumePurchasingLocations))
		printCount("restricted software", "restricted_software", len(resources.RestrictedSoftware))
		printCount("smart mobile device groups", "smart_mobile_device_groups", len(resources.SmartMobileDeviceGroups))
		printCount("static mobile device groups", "static_mobile_device_groups", len(resources.StaticMobileDeviceGroups))
		printCount("mobile device configuration profiles", "mobile_device_configuration_profiles", len(resources.MobileDeviceConfigurationProfiles))
		printCount("mobile device prestages", "mobile_device_prestages", len(resources.MobileDevicePrestages))
		printCount("mobile device extension attributes", "mobile_device_extension_attributes", len(resources.MobileDeviceExtensionAttributes))
		printCount("advanced mobile device searches", "advanced_mobile_device_searches", len(resources.AdvancedMobileDeviceSearches))
		printCount("API integrations", "api_integrations", len(resources.APIIntegrations))
		printCount("API roles", "api_roles", len(resources.APIRoles))
		printCount("accounts", "accounts", len(resources.Accounts))
		printCount("webhooks", "webhooks", len(resources.Webhooks))
		printCount("account groups", "account_groups", len(resources.AccountGroups))
		printCount("disk encryption configurations", "disk_encryption_configurations", len(resources.DiskEncryptionConfigurations))
		printCount("allowed file extensions", "allowed_file_extensions", len(resources.AllowedFileExtensions))
		printCount("LDAP servers", "ldap_servers", len(resources.LDAPServers))
		printCount("mobile device applications", "mobile_device_applications", len(resources.MobileDeviceApplications))
		printCount("user groups", "user_groups", len(resources.UserGroups))
		printCount("self-service branding (macOS)", "self_service_branding_macos", len(resources.SelfServiceBrandingMacOS))
		printCount("self-service branding (iOS)", "self_service_branding_ios", len(resources.SelfServiceBrandingIOS))
		printCount("advanced user searches", "advanced_user_searches", len(resources.AdvancedUserSearches))

		// Print singleton settings count
		singletonCount := len(resources.Singletons)
		if singletonCount > 0 {
			fmt.Printf("  Including %d settings resources\n", singletonCount)
		}
	}

	// 3. Download package files (unless skipped)
	packageFiles := make(map[string]string) // filename -> relative path
	if !opts.SkipPackageDownloads && len(resources.Packages) > 0 {
		status("downloading packages", 0, 0)
		logStep("Downloading package files...")
		pkgFiles, err := download.Packages(jamfClient, opts.OutputDir, func(current, total int) {
			status("downloading packages", current, total)
		})
		if err != nil {
			return nil, fmt.Errorf("downloading packages: %w", err)
		}
		for _, pf := range pkgFiles {
			packageFiles[pf.FileName] = pf.FilePath
		}
		logStep("  Downloaded %d/%d package files", len(pkgFiles), len(resources.Packages))
	} else if opts.SkipPackageDownloads && len(resources.Packages) > 0 {
		logStep("Skipping package downloads (use without -skip-package-downloads to download)")
	}

	// 3b. Collect icon CDN URLs for icon_file_web_source
	iconURLs := make(map[string]string) // icon jamf ID -> CDN URL
	for id, info := range resources.IconInfo {
		if info.URL != "" {
			iconURLs[id] = info.URL
		}
	}

	// 3c. Download enrollment customization branding images
	ecImageFiles := make(map[string]string) // enrollment customization jamf ID -> relative path
	if len(resources.EnrollmentCustomizations) > 0 && resources.EnrollmentCustomizationInfo != nil {
		status("downloading images", 0, 0)
		logStep("Downloading enrollment customization images...")
		dlECInfo := make(map[string]download.EnrollmentCustomizationImageInfo)
		for id, info := range resources.EnrollmentCustomizationInfo {
			dlECInfo[id] = download.EnrollmentCustomizationImageInfo{
				ID:      info.ID,
				Name:    info.Name,
				IconURL: info.IconURL,
			}
		}
		ecDLFiles, err := download.EnrollmentCustomizationImages(opts.OutputDir, dlECInfo, func(current, total int) {
			status("downloading images", current, total)
		})
		if err != nil {
			return nil, fmt.Errorf("downloading enrollment customization images: %w", err)
		}
		for _, f := range ecDLFiles {
			ecImageFiles[f.JamfID] = f.FilePath
		}
		logStep("  Downloaded %d/%d enrollment customization images", len(ecDLFiles), len(resources.EnrollmentCustomizationInfo))
	}

	// 4. Generate import blocks + provider config + variables + tfvars
	status("generating imports", 0, 0)
	logStep("Generating Terraform import configuration...")
	importCreds := &importgen.Credentials{
		URL:                      opts.URL,
		AuthMethod:               opts.AuthMethod,
		Username:                 opts.Username,
		Password:                 opts.Password,
		ClientID:                 opts.ClientID,
		ClientSecret:             opts.ClientSecret,
		ProviderVersion:          opts.ProviderVersion,
		TokenRefreshBufferPeriod: client.TokenRefreshBufferPeriod(),
	}
	if err := importgen.Generate(opts.OutputDir, importCreds, resources); err != nil {
		return nil, fmt.Errorf("generating import config: %w", err)
	}

	// 5. Run terraform init + plan -generate-config-out
	status("initialising terraform", 0, 0)
	logStep("Initialising Terraform provider...")
	if err := terraform.Init(opts.OutputDir); err != nil {
		return nil, fmt.Errorf("terraform init: %w", err)
	}

	// Resolve provider version from lock file for >= constraint
	if importCreds.ProviderVersion == "" {
		importCreds.ResolvedVersion = terraform.ResolvedProviderVersion(opts.OutputDir, terraform.ProviderSourceJamfPro)
	}

	status("generating config", 0, 0)
	logStep("Generating Terraform configuration...")

	// Wire per-resource progress into the spinner for interactive runs.
	terraform.ProgressFunc = func(current, total int) {
		status("generating config", current, total)
	}
	defer func() { terraform.ProgressFunc = nil }()

	generatedFile := filepath.Join(opts.OutputDir, "generated.tf")
	proEnv := map[string]string{
		"JAMFPRO_INSTANCE_FQDN": opts.URL,
		"JAMFPRO_AUTH_METHOD":   opts.AuthMethod,
	}
	if opts.AuthMethod == "oauth2" {
		proEnv["JAMFPRO_CLIENT_ID"] = opts.ClientID
		proEnv["JAMFPRO_CLIENT_SECRET"] = opts.ClientSecret
	} else {
		proEnv["JAMFPRO_BASIC_USERNAME"] = opts.Username
		proEnv["JAMFPRO_BASIC_PASSWORD"] = opts.Password
	}
	if err := terraform.GenerateConfig(opts.OutputDir, generatedFile, proEnv); err != nil {
		return nil, fmt.Errorf("terraform generate-config: %w", err)
	}

	// Load provider schema while we still have the terraform working dir
	schemas, schemaErr := terraform.ProvidersSchema(opts.OutputDir)
	if schemaErr != nil && !postprocess.Quiet {
		fmt.Printf("  Warning: could not load provider schema, skipping null attribute removal: %v\n", schemaErr)
	}

	return &IntermediateResult{
		Registry:        reg,
		Resources:       resources,
		GeneratedFile:   generatedFile,
		OutputDir:       opts.OutputDir,
		PackageFiles:    packageFiles,
		PackageInfo:     packageInfo,
		IconURLs:        iconURLs,
		ECImageFiles:    ecImageFiles,
		ImportCreds:     importCreds,
		ProviderSchemas: schemas,
	}, nil
}

// RunPipeline executes the full Jamf Pro export pipeline:
// authenticate → discover → download → import gen → terraform plan → post-process.
// Returns the validation fix result (may be nil) and any fatal error.
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

	// Write the user-facing provider.tf (with var.* refs), variables.tf, terraform.tfvars
	if err := importgen.Finalize(opts.OutputDir, ir.ImportCreds); err != nil {
		return nil, fmt.Errorf("finalizing provider config: %w", err)
	}

	// 7. Post-process: rewrite references + split into per-type files
	status("post-processing", 0, 0)
	logStep("Post-processing generated configuration...")
	schemas, _ := ir.ProviderSchemas.(*tfjson.ProviderSchemas)
	if err := postprocess.Process(opts.OutputDir, ir.GeneratedFile, ir.Registry, &postprocess.ProcessOptions{
		TypeToFileMap:                     TypeToFileMap(),
		Rules:                             DefaultRules(),
		PackageFiles:                      ir.PackageFiles,
		PackageInfo:                       ir.PackageInfo,
		IconURLs:                          ir.IconURLs,
		EnrollmentCustomizationImageFiles: ir.ECImageFiles,
		SkipReferences:                    opts.SkipReferences,
		SplitByCategory:                   opts.SplitByCategory,
		ProviderSchemas:                   schemas,
	}); err != nil {
		return nil, fmt.Errorf("post-processing: %w", err)
	}

	// 8. Clean up intermediate file and orphan import files
	_ = os.Remove(ir.GeneratedFile)
	cleanupOrphanImportFiles(opts.OutputDir)

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

// countDiscoveryTypes returns the number of discoverable resource types that will
// be processed, based on the selected resources filter.
func countDiscoveryTypes(selected map[string]bool) int {
	count := 0
	for _, r := range Resources {
		if r.IsSingleton {
			continue
		}
		if selected == nil || selected[r.FilterKey] {
			count++
		}
	}
	return count
}

// cleanupOrphanImportFiles removes *_import.tf files that have no corresponding
// resource file. This happens when the provider doesn't support import for a
// singleton resource — the import file is generated but no config is produced.
func cleanupOrphanImportFiles(dir string) {
	importFiles, err := filepath.Glob(filepath.Join(dir, "*_import.tf"))
	if err != nil {
		return
	}
	for _, f := range importFiles {
		base := filepath.Base(f)
		resourceFile := strings.TrimSuffix(base, "_import.tf") + ".tf"
		if _, err := os.Stat(filepath.Join(dir, resourceFile)); os.IsNotExist(err) {
			_ = os.Remove(f)
		}
	}
}
