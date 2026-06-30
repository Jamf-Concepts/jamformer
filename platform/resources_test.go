// Copyright 2026, Jamf Software LLC

package platform

import "testing"

func TestPlatformTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	// A representative sample across native, listable and singleton resources.
	expected := []string{
		"jamfplatform_blueprints_blueprint",
		"jamfplatform_cbengine_benchmark",
		"jamfplatform_device_group",
		"jamfplatform_pro_policy",
		"jamfplatform_pro_macos_configuration_profile",
		"jamfplatform_pro_script",
		"jamfplatform_pro_package",
		"jamfplatform_pro_category",
		"jamfplatform_pro_smtp_server",
		"jamfplatform_pro_sso_settings",
	}

	for _, rt := range expected {
		if _, ok := m[rt]; !ok {
			t.Errorf("missing file mapping for resource type %q", rt)
		}
	}

	// 3 native + 60 listable pro_* + 27 singleton pro_* + 3 with no list
	// resource: synthesised jamfplatform_pro_icon, SDK-discovered
	// jamfplatform_pro_jamf_connect, and synthesised
	// jamfplatform_pro_self_service_branding_image = 93.
	if got := len(m); got != 93 {
		t.Errorf("TypeToFileMap has %d entries, expected 93", got)
	}
	if _, ok := m["jamfplatform_pro_icon"]; !ok {
		t.Error("missing file mapping for synthesised jamfplatform_pro_icon")
	}
	if _, ok := m["jamfplatform_pro_jamf_connect"]; !ok {
		t.Error("missing file mapping for SDK-discovered jamfplatform_pro_jamf_connect")
	}
	if _, ok := m["jamfplatform_pro_self_service_branding_image"]; !ok {
		t.Error("missing file mapping for synthesised jamfplatform_pro_self_service_branding_image")
	}
}

func TestPlatformResourceCounts(t *testing.T) {
	if got := len(ListableResources()); got != 63 {
		t.Errorf("expected 63 listable resources (3 native + 60 pro_*), got %d", got)
	}
	if got := len(SingletonResources()); got != 27 {
		t.Errorf("expected 27 singleton resources, got %d", got)
	}
}

func TestCountSelectedListableTypes(t *testing.T) {
	// nil selection means all listable types (the discovery progress denominator).
	if got, want := countSelectedListableTypes(nil), len(ListableResources()); got != want {
		t.Errorf("nil selection: got %d, want %d", got, want)
	}
	// A selection counts only listable matches, ignoring singletons and unknowns.
	sel := map[string]bool{
		"policy":      true, // listable
		"package":     true, // listable
		"smtp_server": true, // singleton — must not count
		"nonexistent": true, // unknown — must not count
	}
	if got := countSelectedListableTypes(sel); got != 2 {
		t.Errorf("selection: got %d, want 2", got)
	}
	// Empty (non-nil) selection counts nothing.
	if got := countSelectedListableTypes(map[string]bool{}); got != 0 {
		t.Errorf("empty selection: got %d, want 0", got)
	}
}

func TestPlatformResourceUniqueness(t *testing.T) {
	seenType := map[string]bool{}
	seenFile := map[string]bool{}
	seenFilter := map[string]bool{}
	for _, r := range Resources {
		if seenType[r.TFType] {
			t.Errorf("duplicate TFType %q", r.TFType)
		}
		if seenFile[r.OutputFile] {
			t.Errorf("duplicate OutputFile %q", r.OutputFile)
		}
		if seenFilter[r.FilterKey] {
			t.Errorf("duplicate FilterKey %q", r.FilterKey)
		}
		seenType[r.TFType] = true
		seenFile[r.OutputFile] = true
		seenFilter[r.FilterKey] = true
	}
}

func TestPlatformSingletonsHaveImportID(t *testing.T) {
	for _, r := range SingletonResources() {
		if r.SingletonImportID == "" {
			t.Errorf("singleton %q missing SingletonImportID", r.TFType)
		}
	}
}

func TestPlatformValidFilterNames(t *testing.T) {
	m := ValidFilterNames()

	for _, r := range Resources {
		if _, ok := m[r.FilterKey]; !ok {
			t.Errorf("missing filter name for %q", r.FilterKey)
		}
	}
	// jamf_connect is SDK-discovered (no Resources entry) but selectable.
	if _, ok := m["jamf_connect"]; !ok {
		t.Error("missing filter name for jamf_connect")
	}
}

func TestPlatformDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) < 50 {
		t.Errorf("expected the full reference-rule set (>=50), got %d", len(rules))
	}

	ruleTypes := make(map[string]bool)
	for _, r := range rules {
		ruleTypes[r.ResourceType] = true
	}

	expected := []string{
		"jamfplatform_blueprints_blueprint",
		"jamfplatform_cbengine_benchmark",
		"jamfplatform_pro_policy",
		"jamfplatform_pro_macos_configuration_profile",
		"jamfplatform_pro_mobile_device_app",
		"jamfplatform_pro_script",
		"jamfplatform_pro_computer_prestage_enrollment",
		"jamfplatform_pro_jamf_connect",
	}
	for _, rt := range expected {
		if !ruleTypes[rt] {
			t.Errorf("expected DefaultRules to include rules for %q", rt)
		}
	}
}

func TestPlatformDefaultRulesValid(t *testing.T) {
	// Every rule must name a resource type, an attribute, and a resolvable target.
	// Polymorphic rules carry their targets in DiscriminatorMap instead of TargetTypes.
	for i, r := range DefaultRules() {
		if r.ResourceType == "" {
			t.Errorf("rule %d has empty ResourceType", i)
		}
		if r.AttrName == "" {
			t.Errorf("rule %d (%s) has empty AttrName", i, r.ResourceType)
		}
		if r.TargetAttr == "" {
			t.Errorf("rule %d (%s.%s) has no TargetAttr", i, r.ResourceType, r.AttrName)
		}
		if len(r.DiscriminatorMap) > 0 {
			if r.DiscriminatorAttr == "" || r.ElementAttr == "" {
				t.Errorf("rule %d (%s.%s) has DiscriminatorMap but missing DiscriminatorAttr/ElementAttr", i, r.ResourceType, r.AttrName)
			}
			continue
		}
		if len(r.TargetTypes) == 0 {
			t.Errorf("rule %d (%s.%s) has no target", i, r.ResourceType, r.AttrName)
		}
	}
}

// TestPlatformDeviceGroupRulesUseJamfProID verifies that computer/mobile group
// scope references resolve to the synthetic device-group subtypes by jamf_pro_id.
func TestPlatformDeviceGroupRulesUseJamfProID(t *testing.T) {
	var sawComputer, sawMobile bool
	for _, r := range DefaultRules() {
		for _, tt := range r.TargetTypes {
			switch tt {
			case DeviceGroupComputerType:
				sawComputer = true
				if r.TargetAttr != "jamf_pro_id" {
					t.Errorf("computer device-group rule %s.%s must target jamf_pro_id, got %q", r.ResourceType, r.AttrName, r.TargetAttr)
				}
			case DeviceGroupMobileType:
				sawMobile = true
				if r.TargetAttr != "jamf_pro_id" {
					t.Errorf("mobile device-group rule %s.%s must target jamf_pro_id, got %q", r.ResourceType, r.AttrName, r.TargetAttr)
				}
			}
		}
	}
	if !sawComputer || !sawMobile {
		t.Errorf("expected both computer and mobile device-group rules (computer=%v mobile=%v)", sawComputer, sawMobile)
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
