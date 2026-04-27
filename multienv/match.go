// Copyright 2026, Jamf Software LLC

package multienv

import (
	"github.com/Jamf-Concepts/jamformer/pro"
	"github.com/Jamf-Concepts/jamformer/pro/discovery"
)

// MatchResources matches resources across environments by (resource_type, label).
// Labels are derived from Jamf object names via naming.Sanitize(), so instances
// with the same resource names produce matching labels.
func MatchResources(envResults map[string]*PerEnvResult, envNames []string) []MatchedResource {
	// Key: "resourceType\x00label"
	type matchKey struct {
		resourceType string
		label        string
	}
	type entry struct {
		name string
		ids  map[string]string
	}

	lookup := make(map[matchKey]*entry)

	for _, envName := range envNames {
		result := envResults[envName]
		if result == nil {
			continue
		}
		// Walk all resource types using the Resources table
		for _, rdef := range pro.Resources {
			resources := getResourceSlice(result.Resources, rdef.FilterKey)
			for _, r := range resources {
				key := matchKey{resourceType: rdef.TFType, label: r.Label}
				e, ok := lookup[key]
				if !ok {
					e = &entry{
						name: r.Name,
						ids:  make(map[string]string),
					}
					lookup[key] = e
				}
				e.ids[envName] = r.JamfID
			}
		}
	}

	// Convert to MatchedResource slice
	numEnvs := len(envNames)
	var matches []MatchedResource
	for key, e := range lookup {
		present := make([]string, 0, len(e.ids))
		for _, name := range envNames {
			if _, ok := e.ids[name]; ok {
				present = append(present, name)
			}
		}
		matches = append(matches, MatchedResource{
			ResourceType: key.resourceType,
			Label:        key.label,
			Name:         e.name,
			IDs:          e.ids,
			Present:      present,
			AllEnvs:      len(present) == numEnvs,
		})
	}

	return matches
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
			// Match by filter key from the Resources table
			for _, rdef := range pro.Resources {
				if rdef.TFType == tfType && rdef.FilterKey == filterKey {
					return resources
				}
			}
		}
		return nil
	}
}
