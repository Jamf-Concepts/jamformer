// Copyright 2026, Jamf Software LLC

package pro

import "github.com/Jamf-Concepts/jamformer/postprocess"

// ResourceDef describes a single Jamf Pro resource type.
type ResourceDef struct {
	FilterKey         string // key for -include-resources / -exclude-resources
	DisplayName       string // human-readable name for prompts and output
	TFType            string // Terraform resource type name
	OutputFile        string // filename in the output directory
	IsSingleton       bool   // true for settings resources with no list API (exactly one instance)
	SingletonImportID string // fixed import ID for singleton resources (e.g. "jamfpro_smtp_server_singleton")
}

// Resources is the ordered list of all supported Jamf Pro resource types.
// Order matches the dependency/discovery order used by the pipeline.
var Resources = []ResourceDef{
	{FilterKey: "sites", DisplayName: "Sites", TFType: "jamfpro_site", OutputFile: "sites.tf"},
	{FilterKey: "buildings", DisplayName: "Buildings", TFType: "jamfpro_building", OutputFile: "buildings.tf"},
	{FilterKey: "categories", DisplayName: "Categories", TFType: "jamfpro_category", OutputFile: "categories.tf"},
	{FilterKey: "departments", DisplayName: "Departments", TFType: "jamfpro_department", OutputFile: "departments.tf"},
	{FilterKey: "scripts", DisplayName: "Scripts", TFType: "jamfpro_script", OutputFile: "scripts.tf"},
	{FilterKey: "extension_attributes", DisplayName: "Computer Extension Attributes", TFType: "jamfpro_computer_extension_attribute", OutputFile: "computer_extension_attributes.tf"},
	{FilterKey: "packages", DisplayName: "Packages", TFType: "jamfpro_package", OutputFile: "packages.tf"},
	{FilterKey: "dock_items", DisplayName: "Dock Items", TFType: "jamfpro_dock_item", OutputFile: "dock_items.tf"},
	{FilterKey: "printers", DisplayName: "Printers", TFType: "jamfpro_printer", OutputFile: "printers.tf"},
	{FilterKey: "network_segments", DisplayName: "Network Segments", TFType: "jamfpro_network_segment", OutputFile: "network_segments.tf"},
	{FilterKey: "smart_computer_groups", DisplayName: "Smart Computer Groups", TFType: "jamfpro_smart_computer_group_v2", OutputFile: "smart_computer_groups.tf"},
	{FilterKey: "static_computer_groups", DisplayName: "Static Computer Groups", TFType: "jamfpro_static_computer_group", OutputFile: "static_computer_groups.tf"},
	{FilterKey: "macos_configuration_profiles", DisplayName: "macOS Configuration Profiles", TFType: "jamfpro_macos_configuration_profile_plist", OutputFile: "macos_configuration_profiles.tf"},
	{FilterKey: "policies", DisplayName: "Policies", TFType: "jamfpro_policy", OutputFile: "policies.tf"},
	{FilterKey: "icons", DisplayName: "Icons", TFType: "jamfpro_icon", OutputFile: "icons.tf"},
	{FilterKey: "enrollment_customizations", DisplayName: "Enrollment Customizations", TFType: "jamfpro_enrollment_customization", OutputFile: "enrollment_customizations.tf"},
	{FilterKey: "computer_prestages", DisplayName: "Computer Prestages", TFType: "jamfpro_computer_prestage_enrollment", OutputFile: "computer_prestages.tf"},
	{FilterKey: "advanced_computer_searches", DisplayName: "Advanced Computer Searches", TFType: "jamfpro_advanced_computer_search", OutputFile: "advanced_computer_searches.tf"},
	{FilterKey: "app_installers", DisplayName: "App Installers", TFType: "jamfpro_app_installer", OutputFile: "app_installers.tf"},
	{FilterKey: "mac_applications", DisplayName: "Mac Applications", TFType: "jamfpro_mac_application", OutputFile: "mac_applications.tf"},
	{FilterKey: "device_enrollments", DisplayName: "Device Enrollments (ADE)", TFType: "jamfpro_device_enrollments", OutputFile: "device_enrollments.tf"},
	{FilterKey: "volume_purchasing_locations", DisplayName: "Volume Purchasing Locations (VPP)", TFType: "jamfpro_volume_purchasing_locations", OutputFile: "volume_purchasing_locations.tf"},
	{FilterKey: "restricted_software", DisplayName: "Restricted Software", TFType: "jamfpro_restricted_software", OutputFile: "restricted_software.tf"},
	{FilterKey: "smart_mobile_device_groups", DisplayName: "Smart Mobile Device Groups", TFType: "jamfpro_smart_mobile_device_group_v1", OutputFile: "smart_mobile_device_groups.tf"},
	{FilterKey: "static_mobile_device_groups", DisplayName: "Static Mobile Device Groups", TFType: "jamfpro_static_mobile_device_group", OutputFile: "static_mobile_device_groups.tf"},
	{FilterKey: "mobile_device_configuration_profiles", DisplayName: "Mobile Device Configuration Profiles", TFType: "jamfpro_mobile_device_configuration_profile_plist", OutputFile: "mobile_device_configuration_profiles.tf"},
	{FilterKey: "mobile_device_prestages", DisplayName: "Mobile Device Prestages", TFType: "jamfpro_mobile_device_prestage_enrollment", OutputFile: "mobile_device_prestages.tf"},
	{FilterKey: "mobile_device_extension_attributes", DisplayName: "Mobile Device Extension Attributes", TFType: "jamfpro_mobile_device_extension_attribute", OutputFile: "mobile_device_extension_attributes.tf"},
	{FilterKey: "advanced_mobile_device_searches", DisplayName: "Advanced Mobile Device Searches", TFType: "jamfpro_advanced_mobile_device_search", OutputFile: "advanced_mobile_device_searches.tf"},
	{FilterKey: "api_integrations", DisplayName: "API Integrations", TFType: "jamfpro_api_integration", OutputFile: "api_integrations.tf"},
	{FilterKey: "api_roles", DisplayName: "API Roles", TFType: "jamfpro_api_role", OutputFile: "api_roles.tf"},
	{FilterKey: "accounts", DisplayName: "Accounts", TFType: "jamfpro_account", OutputFile: "accounts.tf"},
	{FilterKey: "webhooks", DisplayName: "Webhooks", TFType: "jamfpro_webhook", OutputFile: "webhooks.tf"},
	{FilterKey: "account_groups", DisplayName: "Account Groups", TFType: "jamfpro_account_group", OutputFile: "account_groups.tf"},
	{FilterKey: "disk_encryption_configurations", DisplayName: "Disk Encryption Configurations", TFType: "jamfpro_disk_encryption_configuration", OutputFile: "disk_encryption_configurations.tf"},
	{FilterKey: "allowed_file_extensions", DisplayName: "Allowed File Extensions", TFType: "jamfpro_allowed_file_extension", OutputFile: "allowed_file_extensions.tf"},
	{FilterKey: "ldap_servers", DisplayName: "LDAP Servers", TFType: "jamfpro_ldap_server", OutputFile: "ldap_servers.tf"},
	{FilterKey: "mobile_device_applications", DisplayName: "Mobile Device Applications", TFType: "jamfpro_mobile_device_application", OutputFile: "mobile_device_applications.tf"},
	{FilterKey: "user_groups", DisplayName: "User Groups", TFType: "jamfpro_user_group", OutputFile: "user_groups.tf"},
	{FilterKey: "self_service_branding_macos", DisplayName: "Self Service Branding (macOS)", TFType: "jamfpro_self_service_branding_macos", OutputFile: "self_service_branding_macos.tf"},
	{FilterKey: "self_service_branding_ios", DisplayName: "Self Service Branding (iOS)", TFType: "jamfpro_self_service_branding_ios", OutputFile: "self_service_branding_ios.tf"},
	{FilterKey: "advanced_user_searches", DisplayName: "Advanced User Searches", TFType: "jamfpro_advanced_user_search", OutputFile: "advanced_user_searches.tf"},

	// --- Singleton settings resources (no discovery, fixed import ID) ---
	{FilterKey: "smtp_server", DisplayName: "SMTP Server", TFType: "jamfpro_smtp_server", OutputFile: "smtp_server.tf", IsSingleton: true, SingletonImportID: "jamfpro_smtp_server_singleton"},
	{FilterKey: "activation_code", DisplayName: "Activation Code", TFType: "jamfpro_activation_code", OutputFile: "activation_code.tf", IsSingleton: true, SingletonImportID: "jamfpro_activation_code_singleton"},
	{FilterKey: "client_checkin", DisplayName: "Client Check-In", TFType: "jamfpro_client_checkin", OutputFile: "client_checkin.tf", IsSingleton: true, SingletonImportID: "jamfpro_client_checkin_singleton"},
	{FilterKey: "cloud_distribution_point", DisplayName: "Cloud Distribution Point", TFType: "jamfpro_cloud_distribution_point", OutputFile: "cloud_distribution_point.tf", IsSingleton: true, SingletonImportID: "jamfpro_cloud_distribution_point_singleton"},
	{FilterKey: "reenrollment", DisplayName: "Re-enrollment", TFType: "jamfpro_reenrollment", OutputFile: "reenrollment.tf", IsSingleton: true, SingletonImportID: "jamfpro_reenrollment_settings_singleton"},
	// jamfpro_sso_settings — provider doesn't implement Import (v0.35.0)
	// jamfpro_sso_certificate — provider doesn't implement Import (v0.35.0)
	// jamfpro_sso_failover — provider returns empty/invalid config on import (v0.35.0)
	{FilterKey: "computer_inventory_collection_settings", DisplayName: "Computer Inventory Collection Settings", TFType: "jamfpro_computer_inventory_collection_settings", OutputFile: "computer_inventory_collection_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_computer_inventory_collection_settings_singleton"},
	{FilterKey: "access_management_settings", DisplayName: "Access Management Settings", TFType: "jamfpro_access_management_settings", OutputFile: "access_management_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_access_management_settings_singleton"},
	{FilterKey: "account_driven_user_enrollment_settings", DisplayName: "Account-Driven User Enrollment Settings", TFType: "jamfpro_account_driven_user_enrollment_settings", OutputFile: "account_driven_user_enrollment_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_account_driven_user_enrollment_settings_singleton"},
	{FilterKey: "app_installer_global_settings", DisplayName: "App Installer Global Settings", TFType: "jamfpro_app_installer_global_settings", OutputFile: "app_installer_global_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_app_installers_global_settings_singleton"},
	{FilterKey: "device_communication_settings", DisplayName: "Device Communication Settings", TFType: "jamfpro_device_communication_settings", OutputFile: "device_communication_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_device_communication_settings_singleton"},
	{FilterKey: "engage_settings", DisplayName: "Engage Settings", TFType: "jamfpro_engage_settings", OutputFile: "engage_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_engage_settings_singleton"},
	{FilterKey: "impact_alert_notification_settings", DisplayName: "Impact Alert Notification Settings", TFType: "jamfpro_impact_alert_notification_settings", OutputFile: "impact_alert_notification_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_impact_alert_notification_settings_singleton"},
	// jamfpro_jamf_protect — provider doesn't implement Import (v0.35.0)
	// jamfpro_local_admin_password_settings — provider crashes with nil pointer on import (v0.35.0)
	// jamfpro_macos_onboarding_settings — provider doesn't implement Import (v0.35.0)
	{FilterKey: "managed_software_update_feature_toggle", DisplayName: "Managed Software Update Feature Toggle", TFType: "jamfpro_managed_software_update_feature_toggle", OutputFile: "managed_software_update_feature_toggle.tf", IsSingleton: true, SingletonImportID: "jamfpro_managed_software_update_feature_toggle_singleton"},
	{FilterKey: "self_service_plus_settings", DisplayName: "Self Service+ Settings", TFType: "jamfpro_self_service_plus_settings", OutputFile: "self_service_plus_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_self_service_plus_settings_singleton"},
	{FilterKey: "self_service_settings", DisplayName: "Self Service Settings", TFType: "jamfpro_self_service_settings", OutputFile: "self_service_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_self_service_settings_singleton"},
	{FilterKey: "service_discovery_enrollment_well_known_settings", DisplayName: "Service Discovery Enrollment Settings", TFType: "jamfpro_service_discovery_enrollment_well_known_settings", OutputFile: "service_discovery_enrollment_well_known_settings.tf", IsSingleton: true, SingletonImportID: "jamfpro_service_discovery_enrollment_well_known_settings_singleton"},
	// jamfpro_user_initiated_enrollment_settings — provider doesn't implement Import (v0.35.0)
}

// TypeToFileMap returns a map of TF resource type → output filename,
// derived from the Resources table.
func TypeToFileMap() map[string]string {
	m := make(map[string]string, len(Resources))
	for _, r := range Resources {
		m[r.TFType] = r.OutputFile
	}
	return m
}

// ValidFilterNames returns a map of user-friendly filter names → canonical keys.
// Includes aliases (e.g. "computer_extension_attributes" → "extension_attributes").
func ValidFilterNames() map[string]string {
	m := make(map[string]string, len(Resources)+1)
	for _, r := range Resources {
		m[r.FilterKey] = r.FilterKey
	}
	// Aliases for backwards compatibility
	m["computer_extension_attributes"] = "extension_attributes"
	return m
}

// DefaultRules returns the reference rules for Jamf Pro resource types.
func DefaultRules() []postprocess.ReferenceRule {
	return []postprocess.ReferenceRule{
		// Script -> Category references
		{
			ResourceType: "jamfpro_script",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Package -> Category references
		{
			ResourceType: "jamfpro_package",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// macOS Configuration Profile -> Category references
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// macOS Configuration Profile -> Computer Group references in scope
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Computer Group references in scope exclusions
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Building references in scope
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Department references in scope
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// macOS Configuration Profile -> Network Segment references in scope limitations
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"scope", "limitations"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Category references
		{
			ResourceType: "jamfpro_policy",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Policy -> Script references (inside payloads.scripts block)
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"payloads", "scripts"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_script"},
			TargetAttr:   "id",
		},
		// Policy -> Computer Group references in scope (list of IDs)
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Computer Group references in scope exclusions
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Building references in scope
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Department references in scope
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Network Segment references in scope limitations
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"scope", "limitations"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Policy -> Dock Item references (inside payloads.dock_items block)
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"payloads", "dock_items"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_dock_item"},
			TargetAttr:   "id",
		},
		// Policy -> Printer references (inside payloads.printers block)
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"payloads", "printers"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_printer"},
			TargetAttr:   "id",
		},
		// Policy -> Self Service Category references
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"self_service", "self_service_category"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Policy -> Self Service Icon references
		{
			ResourceType: "jamfpro_policy",
			BlockPath:    []string{"self_service"},
			AttrName:     "self_service_icon_id",
			TargetTypes:  []string{"jamfpro_icon"},
			TargetAttr:   "id",
		},
		// macOS Configuration Profile -> Self Service Icon references
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			BlockPath:    []string{"self_service"},
			AttrName:     "self_service_icon_id",
			TargetTypes:  []string{"jamfpro_icon"},
			TargetAttr:   "id",
		},

		// --- App Installer references ---

		// App Installer -> Category
		{
			ResourceType: "jamfpro_app_installer",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// App Installer -> Smart Group
		{
			ResourceType: "jamfpro_app_installer",
			AttrName:     "smart_group_id",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2"},
			TargetAttr:   "id",
		},

		// --- Mac Application references ---

		// Mac Application -> Category
		{
			ResourceType: "jamfpro_mac_application",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Mac Application -> Computer Group references in scope
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Building references in scope
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Department references in scope
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Network Segment references in scope limitations
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope", "limitations"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Self Service Category references
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"self_service", "self_service_category"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Mac Application -> Self Service Icon references
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"self_service", "self_service_icon"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_icon"},
			TargetAttr:   "id",
		},
		// Mac Application -> VPP Admin Account (Volume Purchasing Location)
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"vpp"},
			AttrName:     "vpp_admin_account_id",
			TargetTypes:  []string{"jamfpro_volume_purchasing_locations"},
			TargetAttr:   "id",
		},
		// Mac Application -> Computer Group references in scope exclusions
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mac Application -> Network Segment references in scope exclusions
		{
			ResourceType: "jamfpro_mac_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},

		// --- Computer Prestage Enrollment references ---

		// Computer Prestage -> Device Enrollment
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "device_enrollment_program_instance_id",
			TargetTypes:  []string{"jamfpro_device_enrollments"},
			TargetAttr:   "id",
		},
		// Computer Prestage -> Enrollment Customization
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "enrollment_customization_id",
			TargetTypes:  []string{"jamfpro_enrollment_customization"},
			TargetAttr:   "id",
		},
		// Computer Prestage -> Package references (list)
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "custom_package_ids",
			TargetTypes:  []string{"jamfpro_package"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Computer Prestage -> macOS Configuration Profile references (list)
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "prestage_installed_profile_ids",
			TargetTypes:  []string{"jamfpro_macos_configuration_profile_plist"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Computer Prestage -> Department (inside location_information block)
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			BlockPath:    []string{"location_information"},
			AttrName:     "department_id",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
		},
		// Computer Prestage -> Building (inside location_information block)
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			BlockPath:    []string{"location_information"},
			AttrName:     "building_id",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
		},

		// --- site_id references (root-level, single ID) ---

		// Smart Computer Group -> Site
		{
			ResourceType: "jamfpro_smart_computer_group_v2",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Static Computer Group -> Site
		{
			ResourceType: "jamfpro_static_computer_group",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// macOS Configuration Profile -> Site
		{
			ResourceType: "jamfpro_macos_configuration_profile_plist",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Policy -> Site
		{
			ResourceType: "jamfpro_policy",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Advanced Computer Search -> Site
		{
			ResourceType: "jamfpro_advanced_computer_search",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// App Installer -> Site
		{
			ResourceType: "jamfpro_app_installer",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Mac Application -> Site
		{
			ResourceType: "jamfpro_mac_application",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Computer Prestage -> Site
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Computer Prestage -> Enrollment Site
		{
			ResourceType: "jamfpro_computer_prestage_enrollment",
			AttrName:     "enrollment_site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},

		// --- Restricted Software references ---

		// Restricted Software -> Category
		{
			ResourceType: "jamfpro_restricted_software",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Restricted Software -> Site
		{
			ResourceType: "jamfpro_restricted_software",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Restricted Software -> Computer Group references in scope
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Computer Group references in scope exclusions
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Building references in scope
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Department references in scope
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Restricted Software -> Network Segment references in scope limitations
		{
			ResourceType: "jamfpro_restricted_software",
			BlockPath:    []string{"scope", "limitations"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},

		// --- Mobile Device Configuration Profile references ---

		// Mobile Device Configuration Profile -> Category
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Mobile Device Configuration Profile -> Site
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Mobile Device Configuration Profile -> Mobile Device Group references in scope
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "mobile_device_group_ids",
			TargetTypes:  []string{"jamfpro_smart_mobile_device_group_v1", "jamfpro_static_mobile_device_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Mobile Device Group references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "mobile_device_group_ids",
			TargetTypes:  []string{"jamfpro_smart_mobile_device_group_v1", "jamfpro_static_mobile_device_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Building references in scope
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Department references in scope
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Configuration Profile -> Network Segment references in scope limitations
		{
			ResourceType: "jamfpro_mobile_device_configuration_profile_plist",
			BlockPath:    []string{"scope", "limitations"},
			AttrName:     "network_segment_ids",
			TargetTypes:  []string{"jamfpro_network_segment"},
			TargetAttr:   "id",
			IsList:       true,
		},

		// --- Mobile Device Prestage Enrollment references ---

		// Mobile Device Prestage -> Site
		{
			ResourceType: "jamfpro_mobile_device_prestage_enrollment",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Mobile Device Prestage -> Device Enrollment
		{
			ResourceType: "jamfpro_mobile_device_prestage_enrollment",
			AttrName:     "device_enrollment_program_instance_id",
			TargetTypes:  []string{"jamfpro_device_enrollments"},
			TargetAttr:   "id",
		},
		// Mobile Device Prestage -> Enrollment Customization
		{
			ResourceType: "jamfpro_mobile_device_prestage_enrollment",
			AttrName:     "enrollment_customization_id",
			TargetTypes:  []string{"jamfpro_enrollment_customization"},
			TargetAttr:   "id",
		},
		// Mobile Device Prestage -> Department (inside location_information block)
		{
			ResourceType: "jamfpro_mobile_device_prestage_enrollment",
			BlockPath:    []string{"location_information"},
			AttrName:     "department_id",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
		},
		// Mobile Device Prestage -> Building (inside location_information block)
		{
			ResourceType: "jamfpro_mobile_device_prestage_enrollment",
			BlockPath:    []string{"location_information"},
			AttrName:     "building_id",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
		},

		// --- Smart Mobile Device Group references ---

		// Smart Mobile Device Group -> Site
		{
			ResourceType: "jamfpro_smart_mobile_device_group_v1",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},

		// --- Static Mobile Device Group references ---

		// Static Mobile Device Group -> Site
		{
			ResourceType: "jamfpro_static_mobile_device_group",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},

		// --- Advanced Mobile Device Search references ---

		// Advanced Mobile Device Search -> Site
		{
			ResourceType: "jamfpro_advanced_mobile_device_search",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},

		// --- Account references ---

		// Account -> Site
		{
			ResourceType: "jamfpro_account",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Account -> Identity Server (LDAP)
		{
			ResourceType: "jamfpro_account",
			AttrName:     "identity_server_id",
			TargetTypes:  []string{"jamfpro_ldap_server"},
			TargetAttr:   "id",
		},

		// --- API Integration references ---

		// API Integration -> API Role
		{
			ResourceType: "jamfpro_api_integration",
			AttrName:     "api_role_id",
			TargetTypes:  []string{"jamfpro_api_role"},
			TargetAttr:   "id",
		},

		// --- Account Group references ---

		// Account Group -> Site
		{
			ResourceType: "jamfpro_account_group",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Account Group -> LDAP Server
		{
			ResourceType: "jamfpro_account_group",
			AttrName:     "ldap_server_id",
			TargetTypes:  []string{"jamfpro_ldap_server"},
			TargetAttr:   "id",
		},
		// Account Group -> Identity Server (LDAP)
		{
			ResourceType: "jamfpro_account_group",
			AttrName:     "identity_server_id",
			TargetTypes:  []string{"jamfpro_ldap_server"},
			TargetAttr:   "id",
		},
		// Account Group -> Account members (list of account IDs)
		{
			ResourceType: "jamfpro_account_group",
			AttrName:     "member_ids",
			TargetTypes:  []string{"jamfpro_account"},
			TargetAttr:   "id",
			IsList:       true,
		},

		// --- Mobile Device Application references ---

		// Mobile Device Application -> Category
		{
			ResourceType: "jamfpro_mobile_device_application",
			AttrName:     "category_id",
			TargetTypes:  []string{"jamfpro_category"},
			TargetAttr:   "id",
		},
		// Mobile Device Application -> Site
		{
			ResourceType: "jamfpro_mobile_device_application",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
		// Mobile Device Application -> Mobile Device Group references in scope
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope"},
			AttrName:     "mobile_device_group_ids",
			TargetTypes:  []string{"jamfpro_smart_mobile_device_group_v1", "jamfpro_static_mobile_device_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Mobile Device Group references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "mobile_device_group_ids",
			TargetTypes:  []string{"jamfpro_smart_mobile_device_group_v1", "jamfpro_static_mobile_device_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Building references in scope
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Building references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfpro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Department references in scope
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Department references in scope exclusions
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"scope", "exclusions"},
			AttrName:     "department_ids",
			TargetTypes:  []string{"jamfpro_department"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Mobile Device Application -> Self Service Icon
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"self_service", "self_service_icon"},
			AttrName:     "id",
			TargetTypes:  []string{"jamfpro_icon"},
			TargetAttr:   "id",
		},
		// Mobile Device Application -> VPP Admin Account (Volume Purchasing Location)
		{
			ResourceType: "jamfpro_mobile_device_application",
			BlockPath:    []string{"vpp"},
			AttrName:     "vpp_admin_account_id",
			TargetTypes:  []string{"jamfpro_volume_purchasing_locations"},
			TargetAttr:   "id",
		},

		// --- Advanced User Search references ---

		// Advanced User Search -> Site
		{
			ResourceType: "jamfpro_advanced_user_search",
			AttrName:     "site_id",
			TargetTypes:  []string{"jamfpro_site"},
			TargetAttr:   "id",
		},
	}
}
