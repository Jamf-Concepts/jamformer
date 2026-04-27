// Copyright 2026, Jamf Software LLC

package jsc

import "testing"

func TestJSCTypeToFileMap(t *testing.T) {
	m := TypeToFileMap()

	expected := []string{
		"jsc_ap",
		"jsc_entra_idp",
		"jsc_hostnamemapping",
		"jsc_access_policy",
		"jsc_secure_policy",
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

func TestJSCValidFilterNames(t *testing.T) {
	m := ValidFilterNames()

	for _, r := range Resources {
		if _, ok := m[r.FilterKey]; !ok {
			t.Errorf("missing filter name for %q", r.FilterKey)
		}
	}
}

func TestJSCDefaultRulesCount(t *testing.T) {
	rules := DefaultRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestJSCResourcesConsistency(t *testing.T) {
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

func TestJSCDiscoverableResources(t *testing.T) {
	discoverable := DiscoverableResources()
	if len(discoverable) != 4 {
		t.Errorf("expected 4 discoverable resources, got %d", len(discoverable))
	}
	for _, r := range discoverable {
		if r.DataSource == "" {
			t.Errorf("discoverable resource %q has empty DataSource", r.FilterKey)
		}
		if r.IsSingleton {
			t.Errorf("discoverable resource %q should not be a singleton", r.FilterKey)
		}
	}
}

func TestJSCSingletonResources(t *testing.T) {
	singletons := SingletonResources()
	if len(singletons) != 1 {
		t.Errorf("expected 1 singleton resource, got %d", len(singletons))
	}
	if len(singletons) > 0 {
		if singletons[0].TFType != "jsc_secure_policy" {
			t.Errorf("expected singleton type jsc_secure_policy, got %q", singletons[0].TFType)
		}
		if singletons[0].SingletonID == "" {
			t.Error("singleton resource has empty SingletonID")
		}
	}
}
