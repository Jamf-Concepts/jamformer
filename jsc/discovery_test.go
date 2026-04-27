// Copyright 2026, Jamf Software LLC

package jsc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDiscoveryConfig_AllResources(t *testing.T) {
	dir := t.TempDir()

	if err := generateDiscoveryConfig(dir, nil); err != nil {
		t.Fatalf("generateDiscoveryConfig() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "discovery.tf"))
	if err != nil {
		t.Fatalf("Failed to read discovery.tf: %v", err)
	}

	body := string(content)
	for _, r := range DiscoverableResources() {
		if !strings.Contains(body, r.DataSource) {
			t.Errorf("discovery.tf missing data source %q for %s", r.DataSource, r.FilterKey)
		}
	}
}

func TestGenerateDiscoveryConfig_Filtered(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"activation_profiles": true,
	}

	if err := generateDiscoveryConfig(dir, selected); err != nil {
		t.Fatalf("generateDiscoveryConfig() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "discovery.tf"))
	if err != nil {
		t.Fatalf("Failed to read discovery.tf: %v", err)
	}

	body := string(content)
	if !strings.Contains(body, "jsc_activation_profiles") {
		t.Error("discovery.tf should contain jsc_activation_profiles")
	}
	if strings.Contains(body, "jsc_entra_idps") {
		t.Error("discovery.tf should not contain jsc_entra_idps when not selected")
	}
	if strings.Contains(body, "jsc_access_policies") {
		t.Error("discovery.tf should not contain jsc_access_policies when not selected")
	}
}

// tfState is a helper that builds a minimal Terraform state JSON for testing.
type tfState struct {
	Version   int               `json:"version"`
	Resources []tfStateResource `json:"resources"`
}

type tfStateResource struct {
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Mode      string            `json:"mode"`
	Instances []tfStateInstance `json:"instances"`
}

type tfStateInstance struct {
	Attributes map[string]any `json:"attributes"`
}

func writeMockState(t *testing.T, dir string, resources []tfStateResource) {
	t.Helper()
	state := tfState{
		Version:   4,
		Resources: resources,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshalling mock state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), data, 0644); err != nil {
		t.Fatalf("writing mock state: %v", err)
	}
}

func TestParseDiscoveryState_AllResources(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{
						map[string]any{"id": "ap-1", "name": "Profile One"},
						map[string]any{"id": "ap-2", "name": "Profile Two"},
					},
				},
			}},
		},
		{
			Type: "jsc_entra_idps", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"connections": []any{
						map[string]any{"id": "entra-1", "name": "Entra Connection"},
					},
				},
			}},
		},
		{
			Type: "jsc_hostnamemappings", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"mappings": []any{
						map[string]any{"hostname": "example.com"},
					},
				},
			}},
		},
		{
			Type: "jsc_access_policies", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"policies": []any{
						map[string]any{"id": "pol-1", "name": "Block Social Media"},
					},
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 2 {
		t.Errorf("expected 2 activation profiles, got %d", len(results.ActivationProfiles))
	}
	if len(results.EntraIdps) != 1 {
		t.Errorf("expected 1 entra idp, got %d", len(results.EntraIdps))
	}
	if len(results.HostnameMappings) != 1 {
		t.Errorf("expected 1 hostname mapping, got %d", len(results.HostnameMappings))
	}
	if len(results.AccessPolicies) != 1 {
		t.Errorf("expected 1 access policy, got %d", len(results.AccessPolicies))
	}

	// Verify labels are sanitized
	if results.ActivationProfiles[0].Label == "" {
		t.Error("expected non-empty label for first activation profile")
	}
	if results.HostnameMappings[0].ID != "example.com" {
		t.Errorf("hostname mapping ID should be hostname, got %q", results.HostnameMappings[0].ID)
	}
}

func TestParseDiscoveryState_Filtered(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{
						map[string]any{"id": "ap-1", "name": "Profile One"},
					},
				},
			}},
		},
		{
			Type: "jsc_access_policies", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"policies": []any{
						map[string]any{"id": "pol-1", "name": "Policy One"},
					},
				},
			}},
		},
	})

	selected := map[string]bool{
		"activation_profiles": true,
	}

	results, err := parseDiscoveryState(dir, selected)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 1 {
		t.Errorf("expected 1 activation profile, got %d", len(results.ActivationProfiles))
	}
	if len(results.AccessPolicies) != 0 {
		t.Errorf("expected 0 access policies when filtered out, got %d", len(results.AccessPolicies))
	}
}

func TestParseDiscoveryState_SkipsNonData(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "managed",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{
						map[string]any{"id": "ap-1", "name": "Profile One"},
					},
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 0 {
		t.Errorf("expected 0 activation profiles for managed resources, got %d", len(results.ActivationProfiles))
	}
}

func TestParseDiscoveryState_EmptyEntries(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{
						map[string]any{"id": "", "name": "No ID"},
						map[string]any{"id": "ap-1", "name": "Has ID"},
					},
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 1 {
		t.Errorf("expected 1 activation profile (empty ID skipped), got %d", len(results.ActivationProfiles))
	}
}

func TestParseDiscoveryState_MissingStateFile(t *testing.T) {
	dir := t.TempDir()

	_, err := parseDiscoveryState(dir, nil)
	if err == nil {
		t.Error("expected error for missing state file")
	}
}

func TestParseDiscoveryState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "terraform.tfstate"), []byte("not json"), 0644); err != nil {
		t.Fatalf("writing invalid state: %v", err)
	}

	_, err := parseDiscoveryState(dir, nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseDiscoveryState_NoInstances(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type:      "jsc_activation_profiles",
			Name:      "all",
			Mode:      "data",
			Instances: []tfStateInstance{},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 0 {
		t.Errorf("expected 0 activation profiles for empty instances, got %d", len(results.ActivationProfiles))
	}
}

func TestParseDiscoveryState_MalformedAttributes(t *testing.T) {
	dir := t.TempDir()

	// Profiles attribute is wrong type (not a list)
	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": "not a list",
				},
			}},
		},
		{
			Type: "jsc_entra_idps", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"connections": "not a list",
				},
			}},
		},
		{
			Type: "jsc_hostnamemappings", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"mappings": "not a list",
				},
			}},
		},
		{
			Type: "jsc_access_policies", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"policies": "not a list",
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	// All should be empty since the attributes are wrong types
	if len(results.ActivationProfiles) != 0 {
		t.Errorf("expected 0 activation profiles, got %d", len(results.ActivationProfiles))
	}
	if len(results.EntraIdps) != 0 {
		t.Errorf("expected 0 entra idps, got %d", len(results.EntraIdps))
	}
	if len(results.HostnameMappings) != 0 {
		t.Errorf("expected 0 hostname mappings, got %d", len(results.HostnameMappings))
	}
	if len(results.AccessPolicies) != 0 {
		t.Errorf("expected 0 access policies, got %d", len(results.AccessPolicies))
	}
}

func TestParseDiscoveryState_MalformedListEntries(t *testing.T) {
	dir := t.TempDir()

	// List entries are not maps
	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{"not a map", 42},
				},
			}},
		},
		{
			Type: "jsc_entra_idps", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"connections": []any{"not a map"},
				},
			}},
		},
		{
			Type: "jsc_hostnamemappings", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"mappings": []any{123},
				},
			}},
		},
		{
			Type: "jsc_access_policies", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"policies": []any{true},
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 0 {
		t.Errorf("expected 0 activation profiles, got %d", len(results.ActivationProfiles))
	}
	if len(results.EntraIdps) != 0 {
		t.Errorf("expected 0 entra idps, got %d", len(results.EntraIdps))
	}
	if len(results.HostnameMappings) != 0 {
		t.Errorf("expected 0 hostname mappings, got %d", len(results.HostnameMappings))
	}
	if len(results.AccessPolicies) != 0 {
		t.Errorf("expected 0 access policies, got %d", len(results.AccessPolicies))
	}
}

func TestParseDiscoveryState_EmptyHostname(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_hostnamemappings", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"mappings": []any{
						map[string]any{"hostname": ""},
						map[string]any{"hostname": "valid.example.com"},
					},
				},
			}},
		},
	})

	results, err := parseDiscoveryState(dir, nil)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.HostnameMappings) != 1 {
		t.Errorf("expected 1 hostname mapping (empty hostname skipped), got %d", len(results.HostnameMappings))
	}
}

func TestParseDiscoveryState_AllResourceTypesFiltered(t *testing.T) {
	dir := t.TempDir()

	writeMockState(t, dir, []tfStateResource{
		{
			Type: "jsc_activation_profiles", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"profiles": []any{
						map[string]any{"id": "ap-1", "name": "Profile"},
					},
				},
			}},
		},
		{
			Type: "jsc_entra_idps", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"connections": []any{
						map[string]any{"id": "eid-1", "name": "Entra"},
					},
				},
			}},
		},
		{
			Type: "jsc_hostnamemappings", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"mappings": []any{
						map[string]any{"hostname": "host.example.com"},
					},
				},
			}},
		},
		{
			Type: "jsc_access_policies", Name: "all", Mode: "data",
			Instances: []tfStateInstance{{
				Attributes: map[string]any{
					"policies": []any{
						map[string]any{"id": "pol-1", "name": "Policy"},
					},
				},
			}},
		},
	})

	// Filter that selects no resource types
	selected := map[string]bool{
		"something_else": true,
	}

	results, err := parseDiscoveryState(dir, selected)
	if err != nil {
		t.Fatalf("parseDiscoveryState() error: %v", err)
	}

	if len(results.ActivationProfiles) != 0 {
		t.Error("expected 0 activation profiles when all filtered")
	}
	if len(results.EntraIdps) != 0 {
		t.Error("expected 0 entra idps when all filtered")
	}
	if len(results.HostnameMappings) != 0 {
		t.Error("expected 0 hostname mappings when all filtered")
	}
	if len(results.AccessPolicies) != 0 {
		t.Error("expected 0 access policies when all filtered")
	}
}

func TestCleanupDiscoveryFiles(t *testing.T) {
	dir := t.TempDir()

	// Create the files that should be cleaned up
	for _, name := range []string{"discovery.tf", "terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}

	cleanupDiscoveryFiles(dir)

	for _, name := range []string{"discovery.tf", "terraform.tfstate", "terraform.tfstate.backup"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}
}
