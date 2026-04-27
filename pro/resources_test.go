// Copyright 2026, Jamf Software LLC

package pro

import "testing"

func TestTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	// Ensure all expected resource types have file mappings
	expected := []string{
		"jamfpro_site",
		"jamfpro_building",
		"jamfpro_category",
		"jamfpro_department",
		"jamfpro_script",
		"jamfpro_computer_extension_attribute",
		"jamfpro_package",
		"jamfpro_dock_item",
		"jamfpro_printer",
		"jamfpro_network_segment",
		"jamfpro_smart_computer_group_v2",
		"jamfpro_static_computer_group",
		"jamfpro_macos_configuration_profile_plist",
		"jamfpro_policy",
		"jamfpro_icon",
		"jamfpro_enrollment_customization",
		"jamfpro_computer_prestage_enrollment",
		"jamfpro_advanced_computer_search",
		"jamfpro_app_installer",
		"jamfpro_mac_application",
		"jamfpro_device_enrollments",
		"jamfpro_volume_purchasing_locations",
		"jamfpro_restricted_software",
		"jamfpro_smart_mobile_device_group_v1",
		"jamfpro_static_mobile_device_group",
		"jamfpro_mobile_device_configuration_profile_plist",
		"jamfpro_mobile_device_prestage_enrollment",
		"jamfpro_mobile_device_extension_attribute",
		"jamfpro_advanced_mobile_device_search",
		"jamfpro_api_integration",
		"jamfpro_api_role",
		"jamfpro_account",
		"jamfpro_webhook",
		"jamfpro_account_group",
		"jamfpro_disk_encryption_configuration",
		"jamfpro_allowed_file_extension",
		"jamfpro_ldap_server",
		"jamfpro_mobile_device_application",
		"jamfpro_user_group",
		"jamfpro_self_service_branding_macos",
		"jamfpro_self_service_branding_ios",
		"jamfpro_advanced_user_search",
		// Singleton settings
		"jamfpro_smtp_server",
		"jamfpro_activation_code",
		"jamfpro_client_checkin",
		"jamfpro_cloud_distribution_point",
		"jamfpro_reenrollment",
		"jamfpro_computer_inventory_collection_settings",
		"jamfpro_access_management_settings",
		"jamfpro_account_driven_user_enrollment_settings",
		"jamfpro_app_installer_global_settings",
		"jamfpro_device_communication_settings",
		"jamfpro_engage_settings",
		"jamfpro_impact_alert_notification_settings",
		"jamfpro_managed_software_update_feature_toggle",
		"jamfpro_self_service_plus_settings",
		"jamfpro_self_service_settings",
		"jamfpro_service_discovery_enrollment_well_known_settings",
	}

	for _, rt := range expected {
		if _, ok := m[rt]; !ok {
			t.Errorf("missing file mapping for resource type %q", rt)
		}
	}

	if len(m) != len(expected) {
		t.Errorf("TypeToFileMap has %d entries, expected %d", len(m), len(expected))
	}
}

func TestValidFilterNames(t *testing.T) {
	m := ValidFilterNames()

	// Ensure alias works
	if v, ok := m["computer_extension_attributes"]; !ok || v != "extension_attributes" {
		t.Error("expected computer_extension_attributes alias to map to extension_attributes")
	}

	// Ensure all Resources entries have filter names
	for _, r := range Resources {
		if _, ok := m[r.FilterKey]; !ok {
			t.Errorf("missing filter name for %q", r.FilterKey)
		}
	}
}

func TestDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Error("expected DefaultRules to return non-empty slice")
	}

	// Verify we have rules for key resource types
	ruleTypes := make(map[string]bool)
	for _, r := range rules {
		ruleTypes[r.ResourceType] = true
	}

	expected := []string{
		"jamfpro_script",
		"jamfpro_package",
		"jamfpro_macos_configuration_profile_plist",
		"jamfpro_policy",
		"jamfpro_smart_computer_group_v2",
		"jamfpro_static_computer_group",
		"jamfpro_restricted_software",
		"jamfpro_mobile_device_configuration_profile_plist",
		"jamfpro_mobile_device_prestage_enrollment",
		"jamfpro_smart_mobile_device_group_v1",
		"jamfpro_static_mobile_device_group",
		"jamfpro_api_integration",
		"jamfpro_account",
		"jamfpro_account_group",
		"jamfpro_mobile_device_application",
		"jamfpro_advanced_user_search",
	}
	for _, rt := range expected {
		if !ruleTypes[rt] {
			t.Errorf("expected DefaultRules to include rules for %q", rt)
		}
	}
}

func TestResourcesConsistency(t *testing.T) {
	// Every resource should have non-empty fields
	for i, r := range Resources {
		if r.FilterKey == "" {
			t.Errorf("Resources[%d] has empty FilterKey", i)
		}
		if r.DisplayName == "" {
			t.Errorf("Resources[%d] has empty DisplayName", i)
		}
		if r.TFType == "" {
			t.Errorf("Resources[%d] has empty TFType", i)
		}
		if r.OutputFile == "" {
			t.Errorf("Resources[%d] has empty OutputFile", i)
		}
		// Singletons must have an import ID
		if r.IsSingleton && r.SingletonImportID == "" {
			t.Errorf("Resources[%d] (%s) is singleton but has empty SingletonImportID", i, r.FilterKey)
		}
		// Non-singletons must not have an import ID
		if !r.IsSingleton && r.SingletonImportID != "" {
			t.Errorf("Resources[%d] (%s) is not singleton but has SingletonImportID set", i, r.FilterKey)
		}
	}
}

func TestSingletonCount(t *testing.T) {
	count := 0
	for _, r := range Resources {
		if r.IsSingleton {
			count++
		}
	}
	if count != 16 {
		t.Errorf("expected 16 singleton resources, got %d", count)
	}
}

func TestOutputFilesUnique(t *testing.T) {
	seen := make(map[string]string) // outputFile -> filterKey
	for _, r := range Resources {
		if prev, ok := seen[r.OutputFile]; ok {
			t.Errorf("duplicate OutputFile %q: used by %q and %q", r.OutputFile, prev, r.FilterKey)
		}
		seen[r.OutputFile] = r.FilterKey
	}
}

func TestTFTypesUnique(t *testing.T) {
	seen := make(map[string]string) // tfType -> filterKey
	for _, r := range Resources {
		if prev, ok := seen[r.TFType]; ok {
			t.Errorf("duplicate TFType %q: used by %q and %q", r.TFType, prev, r.FilterKey)
		}
		seen[r.TFType] = r.FilterKey
	}
}

func TestDefaultRulesReferenceValidTFTypes(t *testing.T) {
	// Build set of all known TF types from Resources table
	validTypes := make(map[string]bool, len(Resources))
	for _, r := range Resources {
		validTypes[r.TFType] = true
	}

	rules := DefaultRules()
	for i, rule := range rules {
		// Check ResourceType is a valid TF type
		if !validTypes[rule.ResourceType] {
			t.Errorf("rule[%d] ResourceType %q is not in the Resources table", i, rule.ResourceType)
		}
		// Check every TargetType is a valid TF type
		for _, tt := range rule.TargetTypes {
			if !validTypes[tt] {
				t.Errorf("rule[%d] TargetType %q (in rule for %s.%s) is not in the Resources table",
					i, tt, rule.ResourceType, rule.AttrName)
			}
		}
	}
}

func TestDefaultRulesFieldsNonEmpty(t *testing.T) {
	rules := DefaultRules()
	for i, rule := range rules {
		if rule.ResourceType == "" {
			t.Errorf("rule[%d] has empty ResourceType", i)
		}
		if rule.AttrName == "" {
			t.Errorf("rule[%d] has empty AttrName", i)
		}
		if len(rule.TargetTypes) == 0 {
			t.Errorf("rule[%d] (%s.%s) has no TargetTypes", i, rule.ResourceType, rule.AttrName)
		}
		if rule.TargetAttr == "" {
			t.Errorf("rule[%d] (%s.%s) has empty TargetAttr", i, rule.ResourceType, rule.AttrName)
		}
	}
}

func TestFilterKeysUnique(t *testing.T) {
	seen := make(map[string]int) // filterKey -> count
	for _, r := range Resources {
		seen[r.FilterKey]++
	}
	for key, count := range seen {
		if count > 1 {
			t.Errorf("duplicate FilterKey %q appears %d times", key, count)
		}
	}
}

func TestCountDiscoveryTypes(t *testing.T) {
	// nil filter means all discoverable (non-singleton) types
	allCount := countDiscoveryTypes(nil)
	discoverableCount := 0
	for _, r := range Resources {
		if !r.IsSingleton {
			discoverableCount++
		}
	}
	if allCount != discoverableCount {
		t.Errorf("countDiscoveryTypes(nil) = %d, want %d", allCount, discoverableCount)
	}

	// Filtered to 2 types
	filter := map[string]bool{"categories": true, "scripts": true}
	if got := countDiscoveryTypes(filter); got != 2 {
		t.Errorf("countDiscoveryTypes(2 types) = %d, want 2", got)
	}

	// Empty filter means nothing selected
	if got := countDiscoveryTypes(map[string]bool{}); got != 0 {
		t.Errorf("countDiscoveryTypes(empty) = %d, want 0", got)
	}
}
