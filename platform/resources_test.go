// Copyright 2026, Jamf Software LLC

package platform

import (
	"strings"
	"testing"
)

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

	// 70 listable (3 native + 1 AI Governance + 2 Jamf Account + 6 Security
	// Cloud + 58 pro_*) + 29 singletons (27 pro_* + 2 Security Cloud) + 3 with
	// no list resource: synthesised jamfplatform_pro_icon, SDK-discovered
	// jamfplatform_pro_jamf_connect, and synthesised
	// jamfplatform_pro_self_service_branding_image = 102.
	if got := len(m); got != 102 {
		t.Errorf("TypeToFileMap has %d entries, expected 102", got)
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
	// Matches the 70 list resources the provider publishes at v0.30.0:
	// 3 native Platform Services + 1 AI Governance + 2 Jamf Account +
	// 6 Security Cloud + 58 federated pro_*.
	if got := len(ListableResources()); got != 70 {
		t.Errorf("expected 70 listable resources, got %d", got)
	}
	// 27 federated Jamf Pro settings + the 2 per-tenant Security Cloud
	// Custom DNS resources (search domain, hostname mappings).
	if got := len(SingletonResources()); got != 29 {
		t.Errorf("expected 29 singleton resources, got %d", got)
	}
}

// The Platform API GA unpublished /v1/api-integrations and /v1/api-roles for
// security hardening, and the provider removed both resources along with their
// list resources. An export must not emit either: a list block for a type the
// provider no longer implements fails the whole query.
func TestRemovedAtGAAreAbsent(t *testing.T) {
	for _, removed := range []string{"jamfplatform_pro_api_client", "jamfplatform_pro_api_role"} {
		if _, ok := TypeToFileMap()[removed]; ok {
			t.Errorf("%s was removed at the Platform API GA but is still in TypeToFileMap", removed)
		}
		for _, r := range Resources {
			if r.TFType == removed {
				t.Errorf("%s was removed at the Platform API GA but is still in the Resources table", removed)
			}
		}
	}
}

// Every resource must declare the scopes it is reachable at. A zero ScopeSet
// makes a resource unreachable at every scope, which silently drops it from
// every export.
func TestEveryResourceDeclaresScopes(t *testing.T) {
	for _, r := range Resources {
		if r.Scopes == 0 {
			t.Errorf("%s (%s) declares no scopes", r.TFType, r.FilterKey)
		}
	}
}

func TestScopePartitionsResources(t *testing.T) {
	total := len(Resources)
	for _, k := range []ScopeKind{ScopeEnvironment, ScopeTenant, ScopeOrganization} {
		reachable := len(ResourcesForScope(k))
		unreachable := len(UnreachableForScope(k))
		if reachable+unreachable != total {
			t.Errorf("%s scope: %d reachable + %d unreachable != %d total", k, reachable, unreachable, total)
		}
		if reachable == 0 {
			t.Errorf("%s scope reaches nothing", k)
		}
	}
	// Organization scope reaches the Jamf Account family and nothing else.
	org := ResourcesForScope(ScopeOrganization)
	if len(org) != 2 {
		t.Errorf("organization scope: expected the 2 Jamf Account resources, got %d", len(org))
	}
	for _, r := range org {
		if !strings.HasPrefix(r.TFType, "jamfplatform_account_") {
			t.Errorf("organization scope reached a non-account resource: %s", r.TFType)
		}
	}
	// Environment scope reaches everything except Jamf Account.
	if got, want := len(ResourcesForScope(ScopeEnvironment)), total-2; got != want {
		t.Errorf("environment scope: expected %d resources, got %d", want, got)
	}
	// Tenant scope additionally loses blueprints, benchmarks and AI Governance.
	for _, key := range []string{"blueprints", "compliance_benchmarks", "ai_governance_policies"} {
		for _, r := range ResourcesForScope(ScopeTenant) {
			if r.FilterKey == key {
				t.Errorf("tenant scope must not reach %s — the provider refuses it at configure time", key)
			}
		}
	}
}

// scopedSelection must never leave a nil selection in place: downstream code
// reads nil as "no filter", which would put out-of-scope list blocks back into
// the query file.
func TestScopedSelectionNeverNil(t *testing.T) {
	for _, k := range []ScopeKind{ScopeEnvironment, ScopeTenant, ScopeOrganization} {
		got := scopedSelection(k, nil)
		if got == nil {
			t.Fatalf("%s scope: scopedSelection(nil) returned nil", k)
		}
		for _, r := range UnreachableForScope(k) {
			if got[r.FilterKey] {
				t.Errorf("%s scope: selection includes unreachable %s", k, r.FilterKey)
			}
		}
	}
	// An explicit selection is intersected, not widened.
	sel := scopedSelection(ScopeTenant, map[string]bool{"blueprints": true, "policy": true})
	if sel["blueprints"] {
		t.Error("tenant scope: blueprints must be dropped from an explicit selection")
	}
	if !sel["policy"] {
		t.Error("tenant scope: policy must survive an explicit selection")
	}
	// Organization scope reaches no pro surface, so jamf_connect is dropped.
	if scopedSelection(ScopeOrganization, nil)["jamf_connect"] {
		t.Error("organization scope must not select jamf_connect")
	}
	if !scopedSelection(ScopeEnvironment, nil)["jamf_connect"] {
		t.Error("environment scope should select jamf_connect")
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
			// A PrefixedIDs rule keys DiscriminatorMap on literal value
			// prefixes rather than a sibling field, so it carries no
			// DiscriminatorAttr.
			switch {
			case r.PrefixedIDs:
				// Valid either per-element or on a single attribute.
			case r.DiscriminatorAttr == "" || r.ElementAttr == "":
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
