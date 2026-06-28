// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"
	"os"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/pro"
	"github.com/Jamf-Concepts/jamformer/pro/discovery"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// proProvider implements Provider for the community deploymenttheory/jamfpro
// provider.
type proProvider struct{}

func (proProvider) Name() string           { return "jamfpro" }
func (proProvider) ProviderSource() string { return terraform.ProviderSourceJamfPro }
func (proProvider) TypeToFileMap() map[string]string {
	return pro.TypeToFileMap()
}

func (proProvider) DiscoverAndGenerate(env EnvConfig, opts *Options) (*PerEnvResult, error) {
	tempDir, err := os.MkdirTemp("", "jamformer-"+env.Name+"-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	ir, err := pro.RunDiscoveryAndGenerate(&pro.PipelineOptions{
		OutputDir:            tempDir,
		URL:                  env.URL,
		AuthMethod:           env.AuthMethod,
		Username:             env.Username,
		Password:             env.Password,
		ClientID:             env.ClientID,
		ClientSecret:         env.ClientSecret,
		SelectedResources:    opts.SelectedResources,
		SkipReferences:       false, // references must be resolved for diffing
		SkipPackageDownloads: opts.SkipPackageDownloads,
		ProviderVersion:      opts.ProviderVersion,
		Quiet:                opts.Quiet,
		Verbose:              opts.Verbose,
		ResourcesFlag:        opts.ResourcesFlag,
		ExcludeFlag:          opts.ExcludeFlag,
		StatusFunc:           opts.StatusFunc,
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	schemas, _ := ir.ProviderSchemas.(*tfjson.ProviderSchemas)

	// Apply the validation auto-fix in place (temp dir still init'd) so the merged
	// module and env roots inherit a plannable config.
	applyValidationFixes(tempDir, schemas)

	tokenRefresh := 0
	if ir.ImportCreds != nil {
		tokenRefresh = ir.ImportCreds.TokenRefreshBufferPeriod
	}

	return &PerEnvResult{
		EnvName:   env.Name,
		Registry:  ir.Registry,
		Resources: proResourceRefs(ir.Resources),
		OutputDir: tempDir,
		ProcessOptions: &postprocess.ProcessOptions{
			TypeToFileMap:                     pro.TypeToFileMap(),
			Rules:                             pro.DefaultRules(),
			PackageFiles:                      ir.PackageFiles,
			PackageInfo:                       ir.PackageInfo,
			IconURLs:                          ir.IconURLs,
			EnrollmentCustomizationImageFiles: ir.ECImageFiles,
			ProviderSchemas:                   schemas,
		},
		TokenRefreshPeriod: tokenRefresh,
	}, nil
}

// proResourceRefs flattens the typed discovery.Results into a flat ResourceRef
// slice by walking the Resources table.
func proResourceRefs(results *discovery.Results) []ResourceRef {
	if results == nil {
		return nil
	}
	var refs []ResourceRef
	for _, rdef := range pro.Resources {
		for _, r := range getResourceSlice(results, rdef.FilterKey) {
			refs = append(refs, ResourceRef{
				TFType: rdef.TFType,
				Label:  r.Label,
				Name:   r.Name,
				JamfID: r.JamfID,
			})
		}
	}
	return refs
}

// getResourceSlice returns the discovery.Resource slice for a given filter key
// from a Results struct. This maps filter keys to the corresponding struct fields.
func getResourceSlice(r *discovery.Results, filterKey string) []discovery.Resource {
	switch filterKey {
	case "sites":
		return r.Sites
	case "buildings":
		return r.Buildings
	case "categories":
		return r.Categories
	case "departments":
		return r.Departments
	case "scripts":
		return r.Scripts
	case "extension_attributes":
		return r.ComputerExtensionAttributes
	case "packages":
		return r.Packages
	case "dock_items":
		return r.DockItems
	case "printers":
		return r.Printers
	case "network_segments":
		return r.NetworkSegments
	case "smart_computer_groups":
		return r.SmartComputerGroups
	case "static_computer_groups":
		return r.StaticComputerGroups
	case "macos_configuration_profiles":
		return r.MacOSConfigurationProfiles
	case "policies":
		return r.Policies
	case "icons":
		return r.Icons
	case "enrollment_customizations":
		return r.EnrollmentCustomizations
	case "computer_prestages":
		return r.ComputerPrestages
	case "advanced_computer_searches":
		return r.AdvancedComputerSearches
	case "app_installers":
		return r.AppInstallers
	case "mac_applications":
		return r.MacApplications
	case "device_enrollments":
		return r.DeviceEnrollments
	case "volume_purchasing_locations":
		return r.VolumePurchasingLocations
	case "restricted_software":
		return r.RestrictedSoftware
	case "smart_mobile_device_groups":
		return r.SmartMobileDeviceGroups
	case "static_mobile_device_groups":
		return r.StaticMobileDeviceGroups
	case "mobile_device_configuration_profiles":
		return r.MobileDeviceConfigurationProfiles
	case "mobile_device_prestages":
		return r.MobileDevicePrestages
	case "mobile_device_extension_attributes":
		return r.MobileDeviceExtensionAttributes
	case "advanced_mobile_device_searches":
		return r.AdvancedMobileDeviceSearches
	case "api_integrations":
		return r.APIIntegrations
	case "api_roles":
		return r.APIRoles
	case "accounts":
		return r.Accounts
	case "webhooks":
		return r.Webhooks
	case "account_groups":
		return r.AccountGroups
	case "disk_encryption_configurations":
		return r.DiskEncryptionConfigurations
	case "allowed_file_extensions":
		return r.AllowedFileExtensions
	case "ldap_servers":
		return r.LDAPServers
	case "mobile_device_applications":
		return r.MobileDeviceApplications
	case "user_groups":
		return r.UserGroups
	case "self_service_branding_macos":
		return r.SelfServiceBrandingMacOS
	case "self_service_branding_ios":
		return r.SelfServiceBrandingIOS
	case "advanced_user_searches":
		return r.AdvancedUserSearches
	default:
		// Check singletons
		for tfType, resources := range r.Singletons {
			for _, rdef := range pro.Resources {
				if rdef.TFType == tfType && rdef.FilterKey == filterKey {
					return resources
				}
			}
		}
		return nil
	}
}

func (proProvider) ModuleProvidersBlock(versionLine string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}
`, versionLine)
}

func (proProvider) EnvProviderHeader(env EnvConfig, versionLine string, tokenRefreshPeriod int) string {
	var providerAttrs string
	if env.AuthMethod == "oauth2" {
		providerAttrs = `  client_id             = var.jamfpro_client_id
  client_secret         = var.jamfpro_client_secret`
		if tokenRefreshPeriod > 0 {
			providerAttrs += fmt.Sprintf("\n  token_refresh_buffer_period_seconds = %d", tokenRefreshPeriod)
		}
	} else {
		providerAttrs = `  basic_auth_username   = var.jamfpro_basic_auth_username
  basic_auth_password   = var.jamfpro_basic_auth_password`
	}

	return fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}

provider "jamfpro" {
  jamfpro_instance_fqdn = var.jamfpro_instance_url
  auth_method           = var.jamfpro_auth_method
%s
}
`, versionLine, providerAttrs)
}

func (proProvider) EnvAuthVariables(env EnvConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "variable \"jamfpro_instance_url\" {\n")
	fmt.Fprintf(&b, "  description = \"Jamf Pro instance URL\"\n")
	fmt.Fprintf(&b, "  type        = string\n")
	fmt.Fprintf(&b, "  default     = %q\n", env.URL)
	fmt.Fprintf(&b, "}\n\n")

	fmt.Fprintf(&b, "variable \"jamfpro_auth_method\" {\n")
	fmt.Fprintf(&b, "  description = \"Authentication method for the Jamf Pro provider\"\n")
	fmt.Fprintf(&b, "  type        = string\n")
	fmt.Fprintf(&b, "  default     = %q\n", env.AuthMethod)
	fmt.Fprintf(&b, "}\n\n")

	if env.AuthMethod == "oauth2" {
		b.WriteString(`variable "jamfpro_client_id" {
  description = "Jamf Pro API client ID for OAuth2 authentication"
  type        = string
  sensitive   = true
}

variable "jamfpro_client_secret" {
  description = "Jamf Pro API client secret for OAuth2 authentication"
  type        = string
  sensitive   = true
}

`)
	} else {
		b.WriteString(`variable "jamfpro_basic_auth_username" {
  description = "Jamf Pro username for basic authentication"
  type        = string
}

variable "jamfpro_basic_auth_password" {
  description = "Jamf Pro password for basic authentication"
  type        = string
  sensitive   = true
}

`)
	}
	return b.String()
}
