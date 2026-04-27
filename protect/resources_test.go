// Copyright 2026, Jamf Software LLC

package protect

import "testing"

func TestProtectTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	expected := []string{
		"jamfprotect_action_configuration",
		"jamfprotect_analytic",
		"jamfprotect_analytic_set",
		"jamfprotect_api_client",
		"jamfprotect_custom_prevent_list",
		"jamfprotect_exception_set",
		"jamfprotect_group",
		"jamfprotect_plan",
		"jamfprotect_removable_storage_control_set",
		"jamfprotect_role",
		"jamfprotect_telemetry",
		"jamfprotect_unified_logging_filter",
		"jamfprotect_user",
		"jamfprotect_change_management",
		"jamfprotect_data_forwarding",
		"jamfprotect_data_retention",
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

func TestProtectValidFilterNames(t *testing.T) {
	m := ValidFilterNames()

	for _, r := range Resources {
		if _, ok := m[r.FilterKey]; !ok {
			t.Errorf("missing filter name for %q", r.FilterKey)
		}
	}
}

func TestProtectDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Error("expected DefaultRules to return non-empty slice")
	}

	ruleTypes := make(map[string]bool)
	for _, r := range rules {
		ruleTypes[r.ResourceType] = true
	}

	expected := []string{
		"jamfprotect_group",
		"jamfprotect_user",
		"jamfprotect_api_client",
		"jamfprotect_analytic_set",
		"jamfprotect_plan",
	}
	for _, rt := range expected {
		if !ruleTypes[rt] {
			t.Errorf("expected DefaultRules to include rules for %q", rt)
		}
	}
}

func TestProtectResourcesConsistency(t *testing.T) {
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
	}
}
