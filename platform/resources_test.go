// Copyright 2026, Jamf Software LLC

package platform

import "testing"

func TestPlatformTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	expected := []string{
		"jamfplatform_blueprints_blueprint",
		"jamfplatform_cbengine_benchmark",
		"jamfplatform_device_group",
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

func TestPlatformValidFilterNames(t *testing.T) {
	m := ValidFilterNames()

	for _, r := range Resources {
		if _, ok := m[r.FilterKey]; !ok {
			t.Errorf("missing filter name for %q", r.FilterKey)
		}
	}
}

func TestPlatformDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}

	ruleTypes := make(map[string]bool)
	for _, r := range rules {
		ruleTypes[r.ResourceType] = true
	}

	expected := []string{
		"jamfplatform_blueprints_blueprint",
		"jamfplatform_cbengine_benchmark",
	}
	for _, rt := range expected {
		if !ruleTypes[rt] {
			t.Errorf("expected DefaultRules to include rules for %q", rt)
		}
	}
}

func TestPlatformResourcesConsistency(t *testing.T) {
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
