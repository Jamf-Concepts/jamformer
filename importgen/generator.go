// Copyright 2026, Jamf Software LLC

package importgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jamf-Concepts/jamformer/pro/discovery"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// Quiet suppresses progress messages.
var Quiet bool

// Credentials holds the Jamf Pro auth details for generating tfvars.
type Credentials struct {
	URL                      string
	AuthMethod               string // "basic" or "oauth2"
	Username                 string // basic auth
	Password                 string // basic auth
	ClientID                 string // oauth2
	ClientSecret             string // oauth2
	ProviderVersion          string // user-specified exact pin (empty = use latest)
	ResolvedVersion          string // version resolved by terraform init (for >= constraint)
	TokenRefreshBufferPeriod int    // oauth2: dynamically determined buffer period in seconds (0 = use provider default)
}

// Generate writes the provider.tf, variables.tf, terraform.tfvars, and per-type
// import files into the output directory.
func Generate(outputDir string, creds *Credentials, resources *discovery.Results) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := WriteGitignore(outputDir); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	// Write minimal provider.tf (required_providers only, no provider block).
	// The provider configures itself from env vars during terraform plan.
	if err := writeRequiredProviders(outputDir, creds); err != nil {
		return err
	}

	importFiles := []struct {
		filename     string
		resourceType string
		resources    []discovery.Resource
	}{
		{"sites_import.tf", "jamfpro_site", resources.Sites},
		{"buildings_import.tf", "jamfpro_building", resources.Buildings},
		{"categories_import.tf", "jamfpro_category", resources.Categories},
		{"departments_import.tf", "jamfpro_department", resources.Departments},
		{"scripts_import.tf", "jamfpro_script", resources.Scripts},
		{"computer_extension_attributes_import.tf", "jamfpro_computer_extension_attribute", resources.ComputerExtensionAttributes},
		{"packages_import.tf", "jamfpro_package", resources.Packages},
		{"dock_items_import.tf", "jamfpro_dock_item", resources.DockItems},
		{"printers_import.tf", "jamfpro_printer", resources.Printers},
		{"network_segments_import.tf", "jamfpro_network_segment", resources.NetworkSegments},
		{"smart_computer_groups_import.tf", "jamfpro_smart_computer_group_v2", resources.SmartComputerGroups},
		{"static_computer_groups_import.tf", "jamfpro_static_computer_group", resources.StaticComputerGroups},
		{"macos_configuration_profiles_import.tf", "jamfpro_macos_configuration_profile_plist", resources.MacOSConfigurationProfiles},
		{"policies_import.tf", "jamfpro_policy", resources.Policies},
		{"icons_import.tf", "jamfpro_icon", resources.Icons},
		{"enrollment_customizations_import.tf", "jamfpro_enrollment_customization", resources.EnrollmentCustomizations},
		{"computer_prestages_import.tf", "jamfpro_computer_prestage_enrollment", resources.ComputerPrestages},
		{"advanced_computer_searches_import.tf", "jamfpro_advanced_computer_search", resources.AdvancedComputerSearches},
		{"app_installers_import.tf", "jamfpro_app_installer", resources.AppInstallers},
		{"mac_applications_import.tf", "jamfpro_mac_application", resources.MacApplications},
		{"device_enrollments_import.tf", "jamfpro_device_enrollments", resources.DeviceEnrollments},
		{"volume_purchasing_locations_import.tf", "jamfpro_volume_purchasing_locations", resources.VolumePurchasingLocations},
		{"restricted_software_import.tf", "jamfpro_restricted_software", resources.RestrictedSoftware},
		{"smart_mobile_device_groups_import.tf", "jamfpro_smart_mobile_device_group_v1", resources.SmartMobileDeviceGroups},
		{"static_mobile_device_groups_import.tf", "jamfpro_static_mobile_device_group", resources.StaticMobileDeviceGroups},
		{"mobile_device_configuration_profiles_import.tf", "jamfpro_mobile_device_configuration_profile_plist", resources.MobileDeviceConfigurationProfiles},
		{"mobile_device_prestages_import.tf", "jamfpro_mobile_device_prestage_enrollment", resources.MobileDevicePrestages},
		{"mobile_device_extension_attributes_import.tf", "jamfpro_mobile_device_extension_attribute", resources.MobileDeviceExtensionAttributes},
		{"advanced_mobile_device_searches_import.tf", "jamfpro_advanced_mobile_device_search", resources.AdvancedMobileDeviceSearches},
		{"api_integrations_import.tf", "jamfpro_api_integration", resources.APIIntegrations},
		{"api_roles_import.tf", "jamfpro_api_role", resources.APIRoles},
		{"accounts_import.tf", "jamfpro_account", resources.Accounts},
		{"webhooks_import.tf", "jamfpro_webhook", resources.Webhooks},
		{"account_groups_import.tf", "jamfpro_account_group", resources.AccountGroups},
		{"disk_encryption_configurations_import.tf", "jamfpro_disk_encryption_configuration", resources.DiskEncryptionConfigurations},
		{"allowed_file_extensions_import.tf", "jamfpro_allowed_file_extension", resources.AllowedFileExtensions},
		{"ldap_servers_import.tf", "jamfpro_ldap_server", resources.LDAPServers},
		{"mobile_device_applications_import.tf", "jamfpro_mobile_device_application", resources.MobileDeviceApplications},
		{"user_groups_import.tf", "jamfpro_user_group", resources.UserGroups},
		{"self_service_branding_macos_import.tf", "jamfpro_self_service_branding_macos", resources.SelfServiceBrandingMacOS},
		{"self_service_branding_ios_import.tf", "jamfpro_self_service_branding_ios", resources.SelfServiceBrandingIOS},
		{"advanced_user_searches_import.tf", "jamfpro_advanced_user_search", resources.AdvancedUserSearches},
	}

	total := 0
	for _, f := range importFiles {
		if err := writeImportFile(outputDir, f.filename, f.resourceType, f.resources); err != nil {
			return err
		}
		total += len(f.resources)
	}

	// Handle singleton settings resources
	for tfType, singletonResources := range resources.Singletons {
		filename := strings.TrimPrefix(tfType, "jamfpro_") + "_import.tf"
		if err := writeImportFile(outputDir, filename, tfType, singletonResources); err != nil {
			return err
		}
		total += len(singletonResources)
	}

	if !Quiet {
		fmt.Printf("  Wrote %d import blocks\n", total)
	}
	return nil
}

// Finalize writes the user-facing provider.tf (with var.* references),
// variables.tf, and terraform.tfvars into the output directory. Call this
// after terraform plan/query has completed.
func Finalize(outputDir string, creds *Credentials) error {
	if err := writeProviderFile(outputDir, creds); err != nil {
		return err
	}
	if err := writeVariablesFile(outputDir, creds); err != nil {
		return err
	}
	return writeTfvarsFile(outputDir, creds)
}

// ProtectCredentials holds Jamf Protect auth details for generating tfvars.
type ProtectCredentials struct {
	URL             string
	ClientID        string
	ClientSecret    string
	ProviderVersion string // user-specified exact pin (empty = use latest)
	ResolvedVersion string // version resolved by terraform init (for >= constraint)
}

// GenerateProtect writes a minimal provider.tf (required_providers only)
// for a Jamf Protect project. The provider configures itself from env vars
// during terraform plan/query. Call FinalizeProtect after terraform completes.
func GenerateProtect(outputDir string, creds *ProtectCredentials) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := WriteGitignore(outputDir); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	// Phase 1: exact pin if user specified, otherwise no constraint (get latest)
	versionLine := formatVersionLine(creds.ProviderVersion, "")

	providerTF := fmt.Sprintf(`terraform {
  required_providers {
    jamfprotect = {
      source = "Jamf-Concepts/jamfprotect"%s
    }
  }
}
`, versionLine)
	return os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(providerTF), 0644)
}

// FinalizeProtect rewrites provider.tf with var.* references and writes
// variables.tf and terraform.tfvars. Call after terraform plan/query.
func FinalizeProtect(outputDir string, creds *ProtectCredentials) error {
	versionLine := formatVersionLine(creds.ProviderVersion, creds.ResolvedVersion)

	providerTF := fmt.Sprintf(`terraform {
  required_providers {
    jamfprotect = {
      source = "Jamf-Concepts/jamfprotect"%s
    }
  }
}

provider "jamfprotect" {
  url           = var.jamfprotect_url
  client_id     = var.jamfprotect_client_id
  client_secret = var.jamfprotect_client_secret
}
`, versionLine)
	if err := os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(providerTF), 0644); err != nil {
		return fmt.Errorf("writing provider.tf: %w", err)
	}

	variablesTF := `variable "jamfprotect_url" {
  description = "Jamf Protect instance URL (e.g. https://your-tenant.protect.jamfcloud.com)"
  type        = string
}

variable "jamfprotect_client_id" {
  description = "Jamf Protect API client ID"
  type        = string
  sensitive   = true
}

variable "jamfprotect_client_secret" {
  description = "Jamf Protect API client secret"
  type        = string
  sensitive   = true
}
`
	if err := os.WriteFile(filepath.Join(outputDir, "variables.tf"), []byte(variablesTF), 0644); err != nil {
		return fmt.Errorf("writing variables.tf: %w", err)
	}

	tfvars := fmt.Sprintf("jamfprotect_url = %q\n", creds.URL)
	return os.WriteFile(filepath.Join(outputDir, "terraform.tfvars"), []byte(tfvars), 0644)
}

// PlatformCredentials holds Jamf Platform auth details for generating tfvars.
type PlatformCredentials struct {
	BaseURL         string
	ClientID        string
	ClientSecret    string
	ProviderVersion string // user-specified exact pin (empty = use latest)
	ResolvedVersion string // version resolved by terraform init (for >= constraint)
}

// JSCCredentials holds JSC (Jamf Security Cloud) auth details for generating tfvars.
type JSCCredentials struct {
	URL             string // radar.wandera.com or custom domain
	Username        string
	Password        string
	ProviderVersion string // user-specified exact pin (empty = use latest)
	ResolvedVersion string // version resolved by terraform init (for >= constraint)
}

// GenerateJSC writes the provider.tf, variables.tf, and terraform.tfvars
// for a JSC project into the output directory.
func GenerateJSC(outputDir string, creds *JSCCredentials) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := WriteGitignore(outputDir); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	versionLine := formatVersionLine(creds.ProviderVersion, creds.ResolvedVersion)

	providerTF := fmt.Sprintf(`terraform {
  required_providers {
    jsc = {
      source = "Jamf-Concepts/jsctfprovider"%s
    }
  }
}

provider "jsc" {
  username = var.jsc_username
  password = var.jsc_password
}
`, versionLine)
	if err := os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(providerTF), 0644); err != nil {
		return fmt.Errorf("writing provider.tf: %w", err)
	}

	variablesTF := `variable "jsc_username" {
  description = "JSC username (email address)"
  type        = string
}

variable "jsc_password" {
  description = "JSC password"
  type        = string
  sensitive   = true
}
`
	if err := os.WriteFile(filepath.Join(outputDir, "variables.tf"), []byte(variablesTF), 0644); err != nil {
		return fmt.Errorf("writing variables.tf: %w", err)
	}

	tfvars := fmt.Sprintf(`jsc_username = %q
jsc_password = %q
`, creds.Username, creds.Password)
	if err := os.WriteFile(filepath.Join(outputDir, "terraform.tfvars"), []byte(tfvars), 0644); err != nil {
		return fmt.Errorf("writing terraform.tfvars: %w", err)
	}

	return nil
}

// GeneratePlatform writes a minimal provider.tf (required_providers only)
// for a Jamf Platform project. The provider configures itself from env vars
// during terraform plan/query. Call FinalizePlatform after terraform completes.
func GeneratePlatform(outputDir string, creds *PlatformCredentials) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := WriteGitignore(outputDir); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	// Phase 1: exact pin if user specified, otherwise no constraint (get latest)
	versionLine := formatVersionLine(creds.ProviderVersion, "")

	providerTF := fmt.Sprintf(`terraform {
  required_providers {
    jamfplatform = {
      source = "Jamf-Concepts/jamfplatform"%s
    }
  }
}
`, versionLine)
	return os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(providerTF), 0644)
}

// FinalizePlatform rewrites provider.tf with var.* references and writes
// variables.tf and terraform.tfvars. Call after terraform plan/query.
func FinalizePlatform(outputDir string, creds *PlatformCredentials) error {
	versionLine := formatVersionLine(creds.ProviderVersion, creds.ResolvedVersion)

	providerTF := fmt.Sprintf(`terraform {
  required_providers {
    jamfplatform = {
      source = "Jamf-Concepts/jamfplatform"%s
    }
  }
}

provider "jamfplatform" {
  base_url      = var.jamfplatform_base_url
  client_id     = var.jamfplatform_client_id
  client_secret = var.jamfplatform_client_secret
}
`, versionLine)
	if err := os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(providerTF), 0644); err != nil {
		return fmt.Errorf("writing provider.tf: %w", err)
	}

	variablesTF := `variable "jamfplatform_base_url" {
  description = "Jamf Platform API gateway base URL (e.g. https://us.apigw.jamf.com)"
  type        = string
}

variable "jamfplatform_client_id" {
  description = "Jamf Platform API client ID"
  type        = string
  sensitive   = true
}

variable "jamfplatform_client_secret" {
  description = "Jamf Platform API client secret"
  type        = string
  sensitive   = true
}
`
	if err := os.WriteFile(filepath.Join(outputDir, "variables.tf"), []byte(variablesTF), 0644); err != nil {
		return fmt.Errorf("writing variables.tf: %w", err)
	}

	tfvars := fmt.Sprintf("jamfplatform_base_url = %q\n", creds.BaseURL)
	return os.WriteFile(filepath.Join(outputDir, "terraform.tfvars"), []byte(tfvars), 0644)
}

// writeRequiredProviders writes a minimal provider.tf with just the
// required_providers block. The provider configures itself from env vars.
// If a non-default token refresh buffer period is set, a provider block is
// included so the provider uses it during terraform plan (this attribute
// has no env var fallback in the provider).
func writeRequiredProviders(outputDir string, creds *Credentials) error {
	// Phase 1: exact pin if user specified, otherwise no constraint (get latest)
	versionLine := formatVersionLine(creds.ProviderVersion, "")

	var providerBlock string
	if creds.TokenRefreshBufferPeriod > 0 {
		providerBlock = fmt.Sprintf(`

provider "jamfpro" {
  token_refresh_buffer_period_seconds = %d
}
`, creds.TokenRefreshBufferPeriod)
	}

	content := fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}
%s`, versionLine, providerBlock)
	return os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(content), 0644)
}

func writeProviderFile(outputDir string, creds *Credentials) error {
	var bufferLine string
	if creds.TokenRefreshBufferPeriod > 0 {
		bufferLine = fmt.Sprintf("\n  token_refresh_buffer_period_seconds = %d", creds.TokenRefreshBufferPeriod)
	}

	var providerBlock string
	if creds.AuthMethod == "oauth2" {
		providerBlock = fmt.Sprintf(`provider "jamfpro" {
  jamfpro_instance_fqdn = var.jamfpro_instance_fqdn
  auth_method           = var.jamfpro_auth_method
  client_id             = var.jamfpro_client_id
  client_secret         = var.jamfpro_client_secret%s
}`, bufferLine)
	} else {
		providerBlock = fmt.Sprintf(`provider "jamfpro" {
  jamfpro_instance_fqdn = var.jamfpro_instance_fqdn
  auth_method           = var.jamfpro_auth_method
  basic_auth_username   = var.jamfpro_basic_auth_username
  basic_auth_password   = var.jamfpro_basic_auth_password%s
}`, bufferLine)
	}

	versionLine := formatVersionLine(creds.ProviderVersion, creds.ResolvedVersion)

	content := fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}

%s
`, versionLine, providerBlock)
	return os.WriteFile(filepath.Join(outputDir, "provider.tf"), []byte(content), 0644)
}

func writeVariablesFile(outputDir string, creds *Credentials) error {
	var authVars string
	if creds.AuthMethod == "oauth2" {
		authVars = `variable "jamfpro_client_id" {
  description = "Jamf Pro API client ID for OAuth2 authentication"
  type        = string
  sensitive   = true
}

variable "jamfpro_client_secret" {
  description = "Jamf Pro API client secret for OAuth2 authentication"
  type        = string
  sensitive   = true
}`
	} else {
		authVars = `variable "jamfpro_basic_auth_username" {
  description = "Jamf Pro username for basic authentication"
  type        = string
}

variable "jamfpro_basic_auth_password" {
  description = "Jamf Pro password for basic authentication"
  type        = string
  sensitive   = true
}`
	}

	content := fmt.Sprintf(`variable "jamfpro_instance_fqdn" {
  description = "Jamf Pro instance FQDN (e.g. https://yourinstance.jamfcloud.com)"
  type        = string
}

variable "jamfpro_auth_method" {
  description = "Authentication method for the Jamf Pro provider"
  type        = string
  default     = %q
}

%s
`, creds.AuthMethod, authVars)
	return os.WriteFile(filepath.Join(outputDir, "variables.tf"), []byte(content), 0644)
}

func writeTfvarsFile(outputDir string, creds *Credentials) error {
	content := fmt.Sprintf(`jamfpro_instance_fqdn = %q
jamfpro_auth_method   = %q
`, creds.URL, creds.AuthMethod)

	return os.WriteFile(filepath.Join(outputDir, "terraform.tfvars"), []byte(content), 0644)
}

func writeImportFile(outputDir, filename, resourceType string, resources []discovery.Resource) error {
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

		// to = jamfpro_script.my_label (a traversal, not a string)
		toTokens := hclwrite.Tokens{
			{Type: 9, Bytes: fmt.Appendf(nil, "%s.%s", resourceType, r.Label)}, // hclsyntax.TokenIdent = 9
		}
		blockBody.SetAttributeRaw("to", toTokens)
		blockBody.SetAttributeValue("id", cty.StringVal(r.JamfID))
	}

	return os.WriteFile(filepath.Join(outputDir, filename), f.Bytes(), 0644)
}

// WriteGitignore writes a .gitignore to the output directory to prevent
// accidentally committing secrets, state, and terraform internals.
// Skips writing if the file already exists.
func WriteGitignore(outputDir string) error {
	path := filepath.Join(outputDir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	content := `# Terraform state
*.tfstate
*.tfstate.*

# Terraform variables (may contain secrets)
*.tfvars

# Terraform internals
.terraform/
.terraform.lock.hcl

# OS
.DS_Store
`
	return os.WriteFile(path, []byte(content), 0644)
}

// formatVersionLine returns the version attribute for a required_providers block.
// If pinnedVersion is set, produces an exact pin. If resolvedVersion is set
// (and no pin), produces a >= minimum constraint. Otherwise returns empty.
func formatVersionLine(pinnedVersion, resolvedVersion string) string {
	switch {
	case pinnedVersion != "":
		return fmt.Sprintf("\n      version = %q", pinnedVersion)
	case resolvedVersion != "":
		return fmt.Sprintf("\n      version = %q", ">= "+resolvedVersion)
	default:
		return ""
	}
}
