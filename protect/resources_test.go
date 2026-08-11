// Copyright 2026, Jamf Software LLC

package protect

import "testing"

func TestProtectTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	expected := []string{
		"jamfprotect_action_configuration",
		"jamfprotect_analytic",
		"jamfprotect_analytic_managed",
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
		"jamfprotect_unified_logging_filter_set",
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
		"jamfprotect_unified_logging_filter_set",
		"jamfprotect_plan",
	}
	for _, rt := range expected {
		if !ruleTypes[rt] {
			t.Errorf("expected DefaultRules to include rules for %q", rt)
		}
	}
}

// TestProtectDefaultRulesHaveValidTypes verifies every rule's source and target
// resource types are types the Protect pipeline actually knows about, so a typo
// in a rule fails here rather than silently never matching.
func TestProtectDefaultRulesHaveValidTypes(t *testing.T) {
	knownTypes := TypeToFileMap()
	for _, rule := range DefaultRules() {
		if _, ok := knownTypes[rule.ResourceType]; !ok {
			t.Errorf("rule references unknown source type: %q", rule.ResourceType)
		}
		for _, tt := range rule.TargetTypes {
			if _, ok := knownTypes[tt]; !ok {
				t.Errorf("rule for %s.%s references unknown target type: %q", rule.ResourceType, rule.AttrName, tt)
			}
		}
	}
}

// TestProtectListableAndSingletonTypesAreDefined verifies every resource type the
// query/singleton tables reference has a Resources entry (and therefore an output
// file), so a new listable type can't be discovered into a file that doesn't exist.
func TestProtectListableAndSingletonTypesAreDefined(t *testing.T) {
	knownTypes := TypeToFileMap()
	filterKeys := ValidFilterNames()

	for filterKey, resourceType := range listableResourceTypes {
		if _, ok := knownTypes[resourceType]; !ok {
			t.Errorf("listable type %q (%s) has no Resources entry", resourceType, filterKey)
		}
		if _, ok := filterKeys[filterKey]; !ok {
			t.Errorf("listable filter key %q has no Resources entry", filterKey)
		}
	}

	for filterKey, singleton := range singletonResources {
		if _, ok := knownTypes[singleton.ResourceType]; !ok {
			t.Errorf("singleton type %q (%s) has no Resources entry", singleton.ResourceType, filterKey)
		}
		if _, ok := filterKeys[filterKey]; !ok {
			t.Errorf("singleton filter key %q has no Resources entry", filterKey)
		}
	}

	// Every Resources entry must be reachable as either listable or singleton.
	for _, r := range Resources {
		_, listable := listableResourceTypes[r.FilterKey]
		_, singleton := singletonResources[r.FilterKey]
		if !listable && !singleton {
			t.Errorf("Resources entry %q (%s) is neither listable nor a singleton", r.FilterKey, r.TFType)
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
