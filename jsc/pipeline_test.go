// Copyright 2026, Jamf Software LLC

package jsc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestWriteImportFile_BasicResources(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	resources := []DiscoveredResource{
		{ID: "abc-123", Name: "Profile Alpha", Label: "profile_alpha"},
		{ID: "def-456", Name: "Profile Beta", Label: "profile_beta"},
	}

	if err := writeImportFile(dir, "activation_profiles_import.tf", "jsc_ap", resources, reg); err != nil {
		t.Fatalf("writeImportFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "activation_profiles_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read import file: %v", err)
	}

	result := string(content)

	// Check import blocks for both resources
	if !strings.Contains(result, "jsc_ap.profile_alpha") {
		t.Error("Expected import block with to = jsc_ap.profile_alpha")
	}
	if !strings.Contains(result, `"abc-123"`) {
		t.Error("Expected id = abc-123 in import block")
	}
	if !strings.Contains(result, "jsc_ap.profile_beta") {
		t.Error("Expected import block with to = jsc_ap.profile_beta")
	}
	if !strings.Contains(result, `"def-456"`) {
		t.Error("Expected id = def-456 in import block")
	}

	// Verify registry was populated
	if addr, ok := reg.Resolve("jsc_ap", "abc-123"); !ok {
		t.Error("Expected jsc_ap abc-123 to be registered")
	} else if addr != "jsc_ap.profile_alpha" {
		t.Errorf("Expected jsc_ap.profile_alpha, got %s", addr)
	}

	if addr, ok := reg.Resolve("jsc_ap", "def-456"); !ok {
		t.Error("Expected jsc_ap def-456 to be registered")
	} else if addr != "jsc_ap.profile_beta" {
		t.Errorf("Expected jsc_ap.profile_beta, got %s", addr)
	}
}

func TestWriteImportFile_EmptyResources(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	if err := writeImportFile(dir, "empty_import.tf", "jsc_ap", nil, reg); err != nil {
		t.Fatalf("writeImportFile() error: %v", err)
	}

	// File should not be created when there are no resources
	if _, err := os.Stat(filepath.Join(dir, "empty_import.tf")); !os.IsNotExist(err) {
		t.Error("Import file should not be created for empty resource list")
	}
}

func TestWriteImportFile_SingleResource(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	resources := []DiscoveredResource{
		{ID: "host-1", Name: "example.com", Label: "example_com"},
	}

	if err := writeImportFile(dir, "hostname_mappings_import.tf", "jsc_hostnamemapping", resources, reg); err != nil {
		t.Fatalf("writeImportFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "hostname_mappings_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read import file: %v", err)
	}

	result := string(content)

	if !strings.Contains(result, "jsc_hostnamemapping.example_com") {
		t.Error("Expected import block with to = jsc_hostnamemapping.example_com")
	}
	if !strings.Contains(result, `"host-1"`) {
		t.Error("Expected id = host-1")
	}

	// Should have exactly one import block
	if strings.Count(result, "import {") != 1 {
		t.Errorf("Expected exactly 1 import block, got %d", strings.Count(result, "import {"))
	}
}

func TestWriteSingletonImportFile_Basic(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	if err := writeSingletonImportFile(dir, "secure_policy_import.tf", "jsc_secure_policy", "secure_policy", "secure_policy", reg); err != nil {
		t.Fatalf("writeSingletonImportFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "secure_policy_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read import file: %v", err)
	}

	result := string(content)

	if !strings.Contains(result, "jsc_secure_policy.secure_policy") {
		t.Error("Expected import block with to = jsc_secure_policy.secure_policy")
	}
	if !strings.Contains(result, `"secure_policy"`) {
		t.Error("Expected id = secure_policy")
	}

	// Exactly one import block
	if strings.Count(result, "import {") != 1 {
		t.Errorf("Expected exactly 1 import block, got %d", strings.Count(result, "import {"))
	}

	// Verify registry
	if addr, ok := reg.Resolve("jsc_secure_policy", "secure_policy"); !ok {
		t.Error("Expected singleton to be registered")
	} else if addr != "jsc_secure_policy.secure_policy" {
		t.Errorf("Expected jsc_secure_policy.secure_policy, got %s", addr)
	}
}

func TestWriteJSCImportFiles_AllResources(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	results := &DiscoveryResults{
		ActivationProfiles: []DiscoveredResource{
			{ID: "ap-1", Name: "Corp Profile", Label: "corp_profile"},
		},
		EntraIdps: []DiscoveredResource{
			{ID: "eid-1", Name: "Azure AD", Label: "azure_ad"},
		},
		HostnameMappings: []DiscoveredResource{
			{ID: "example.com", Name: "example.com", Label: "example_com"},
		},
		AccessPolicies: []DiscoveredResource{
			{ID: "pol-1", Name: "Block All", Label: "block_all"},
		},
	}

	// nil selectedResources = all resources
	if err := writeJSCImportFiles(dir, results, nil, reg); err != nil {
		t.Fatalf("writeJSCImportFiles() error: %v", err)
	}

	// Check all expected files were created
	expectedFiles := []string{
		"activation_profiles_import.tf",
		"entra_idps_import.tf",
		"hostname_mappings_import.tf",
		"access_policies_import.tf",
		"secure_policy_import.tf",
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("Expected file %s to exist: %v", f, err)
		}
	}

	// Verify registry has entries from all resource types
	if _, ok := reg.Resolve("jsc_ap", "ap-1"); !ok {
		t.Error("Expected activation profile to be registered")
	}
	if _, ok := reg.Resolve("jsc_entra_idp", "eid-1"); !ok {
		t.Error("Expected entra idp to be registered")
	}
	if _, ok := reg.Resolve("jsc_hostnamemapping", "example.com"); !ok {
		t.Error("Expected hostname mapping to be registered")
	}
	if _, ok := reg.Resolve("jsc_access_policy", "pol-1"); !ok {
		t.Error("Expected access policy to be registered")
	}
	if _, ok := reg.Resolve("jsc_secure_policy", "secure_policy"); !ok {
		t.Error("Expected secure policy singleton to be registered")
	}
}

func TestWriteJSCImportFiles_FilteredResources(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	results := &DiscoveryResults{
		ActivationProfiles: []DiscoveredResource{
			{ID: "ap-1", Name: "Corp Profile", Label: "corp_profile"},
		},
		EntraIdps: []DiscoveredResource{
			{ID: "eid-1", Name: "Azure AD", Label: "azure_ad"},
		},
	}

	selected := map[string]bool{
		"activation_profiles": true,
	}

	if err := writeJSCImportFiles(dir, results, selected, reg); err != nil {
		t.Fatalf("writeJSCImportFiles() error: %v", err)
	}

	// Activation profiles import file should exist
	if _, err := os.Stat(filepath.Join(dir, "activation_profiles_import.tf")); err != nil {
		t.Error("Expected activation_profiles_import.tf to exist")
	}

	// Entra IdPs should NOT be created (not selected)
	if _, err := os.Stat(filepath.Join(dir, "entra_idps_import.tf")); !os.IsNotExist(err) {
		t.Error("entra_idps_import.tf should not exist when not selected")
	}

	// Secure policy should NOT be created (not selected)
	if _, err := os.Stat(filepath.Join(dir, "secure_policy_import.tf")); !os.IsNotExist(err) {
		t.Error("secure_policy_import.tf should not exist when not selected")
	}

	// Registry should only have the selected resource
	if _, ok := reg.Resolve("jsc_ap", "ap-1"); !ok {
		t.Error("Expected activation profile to be registered")
	}
	if _, ok := reg.Resolve("jsc_entra_idp", "eid-1"); ok {
		t.Error("Entra IdP should not be registered when not selected")
	}
}

func TestWriteJSCImportFiles_EmptyResults(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	results := &DiscoveryResults{}

	if err := writeJSCImportFiles(dir, results, nil, reg); err != nil {
		t.Fatalf("writeJSCImportFiles() error: %v", err)
	}

	// Only the singleton file should exist (no discovered resources)
	if _, err := os.Stat(filepath.Join(dir, "activation_profiles_import.tf")); !os.IsNotExist(err) {
		t.Error("activation_profiles_import.tf should not exist with empty results")
	}
	if _, err := os.Stat(filepath.Join(dir, "secure_policy_import.tf")); err != nil {
		t.Error("secure_policy_import.tf should always be created when not filtered")
	}
}

func TestWriteSingletonImportFile_CustomLabel(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	if err := writeSingletonImportFile(dir, "custom_import.tf", "jsc_custom_type", "my_custom_label", "custom-import-id", reg); err != nil {
		t.Fatalf("writeSingletonImportFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "custom_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read import file: %v", err)
	}

	result := string(content)

	if !strings.Contains(result, "jsc_custom_type.my_custom_label") {
		t.Error("Expected import block with to = jsc_custom_type.my_custom_label")
	}
	if !strings.Contains(result, `"custom-import-id"`) {
		t.Error("Expected id = custom-import-id")
	}

	// Verify registry uses the importID, not the label
	if addr, ok := reg.Resolve("jsc_custom_type", "custom-import-id"); !ok {
		t.Error("Expected singleton to be registered with importID")
	} else if addr != "jsc_custom_type.my_custom_label" {
		t.Errorf("Expected jsc_custom_type.my_custom_label, got %s", addr)
	}
}

func TestWriteImportFile_ValidHCLOutput(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	resources := []DiscoveredResource{
		{ID: "id-1", Name: "Resource One", Label: "resource_one"},
		{ID: "id-2", Name: "Resource Two", Label: "resource_two"},
		{ID: "id-3", Name: "Resource Three", Label: "resource_three"},
	}

	if err := writeImportFile(dir, "test_import.tf", "jsc_test", resources, reg); err != nil {
		t.Fatalf("writeImportFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "test_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read import file: %v", err)
	}

	result := string(content)

	// Should have exactly 3 import blocks
	if count := strings.Count(result, "import {"); count != 3 {
		t.Errorf("Expected 3 import blocks, got %d", count)
	}

	// Each resource should have its own to and id
	for _, r := range resources {
		expectedTo := "jsc_test." + r.Label
		if !strings.Contains(result, expectedTo) {
			t.Errorf("Expected %s in output", expectedTo)
		}
		if !strings.Contains(result, `"`+r.ID+`"`) {
			t.Errorf("Expected id %q in output", r.ID)
		}
	}
}

func TestWriteJSCImportFiles_OnlySecurePolicy(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	results := &DiscoveryResults{}

	selected := map[string]bool{
		"secure_policy": true,
	}

	if err := writeJSCImportFiles(dir, results, selected, reg); err != nil {
		t.Fatalf("writeJSCImportFiles() error: %v", err)
	}

	// Only secure_policy_import.tf should exist
	if _, err := os.Stat(filepath.Join(dir, "secure_policy_import.tf")); err != nil {
		t.Error("Expected secure_policy_import.tf to exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "activation_profiles_import.tf")); !os.IsNotExist(err) {
		t.Error("activation_profiles_import.tf should not exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "entra_idps_import.tf")); !os.IsNotExist(err) {
		t.Error("entra_idps_import.tf should not exist")
	}

	// Verify singleton was registered
	if _, ok := reg.Resolve("jsc_secure_policy", "secure_policy"); !ok {
		t.Error("Expected secure policy to be registered")
	}
}

func TestWriteImportFile_MultipleResourceTypes(t *testing.T) {
	dir := t.TempDir()
	reg := registry.New()

	// Write different resource types to separate files and verify no collision
	apResources := []DiscoveredResource{
		{ID: "ap-1", Name: "Activation 1", Label: "activation_1"},
	}
	policyResources := []DiscoveredResource{
		{ID: "pol-1", Name: "Policy 1", Label: "policy_1"},
	}

	if err := writeImportFile(dir, "activation_profiles_import.tf", "jsc_ap", apResources, reg); err != nil {
		t.Fatalf("writeImportFile() error for ap: %v", err)
	}
	if err := writeImportFile(dir, "access_policies_import.tf", "jsc_access_policy", policyResources, reg); err != nil {
		t.Fatalf("writeImportFile() error for policies: %v", err)
	}

	// Both should be registered under different resource types
	if addr, ok := reg.Resolve("jsc_ap", "ap-1"); !ok || addr != "jsc_ap.activation_1" {
		t.Errorf("Expected jsc_ap.activation_1, got %q (found: %v)", addr, ok)
	}
	if addr, ok := reg.Resolve("jsc_access_policy", "pol-1"); !ok || addr != "jsc_access_policy.policy_1" {
		t.Errorf("Expected jsc_access_policy.policy_1, got %q (found: %v)", addr, ok)
	}
}
