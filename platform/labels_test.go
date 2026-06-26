// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameLabels_BasicRename(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `import {
  identity = {
    id = "1"
  }
  to = jamfplatform_blueprints_blueprint.all_0
}

resource "jamfplatform_blueprints_blueprint" "all_0" {
  name = "macOS Standard"
}

import {
  identity = {
    id = "2"
  }
  to = jamfplatform_device_group.all_0
}

resource "jamfplatform_device_group" "all_0" {
  name = "Staff Macs"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_blueprints_blueprint" "macos_standard"`) {
		t.Errorf("expected blueprint renamed to macos_standard, got:\n%s", body)
	}
	if !strings.Contains(body, `"jamfplatform_device_group" "staff_macs"`) {
		t.Errorf("expected device group renamed to staff_macs, got:\n%s", body)
	}
	// Import blocks should also be updated
	if !strings.Contains(body, "jamfplatform_blueprints_blueprint.macos_standard") {
		t.Errorf("expected import block updated for blueprint, got:\n%s", body)
	}
	if !strings.Contains(body, "jamfplatform_device_group.staff_macs") {
		t.Errorf("expected import block updated for device group, got:\n%s", body)
	}
}

func TestRenameLabels_DuplicateNames(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_device_group" "all_0" {
  name = "Staff"
}

resource "jamfplatform_device_group" "all_1" {
  name = "Staff"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"staff"`) {
		t.Errorf("expected first resource renamed to staff, got:\n%s", body)
	}
	if !strings.Contains(body, `"staff_2"`) {
		t.Errorf("expected second resource renamed to staff_2, got:\n%s", body)
	}
}

func TestRenameLabels_SpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_blueprints_blueprint" "all_0" {
  name = "macOS - Production (v2.1)"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_blueprints_blueprint" "macos_production_v2_1"`) {
		t.Errorf("expected sanitized label macos_production_v2_1, got:\n%s", body)
	}
}

func TestRenameLabels_BenchmarkUsesTitle(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Benchmarks should use "title" not "name"
	src := `resource "jamfplatform_cbengine_benchmark" "all_0" {
  title = "CIS macOS Level 1"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_cbengine_benchmark" "cis_macos_level_1"`) {
		t.Errorf("expected benchmark renamed using title attribute, got:\n%s", body)
	}
}

func TestRenameLabels_NoNameAttribute(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Resource without a name attribute should keep its original label
	src := `resource "jamfplatform_device_group" "all_0" {
  description = "some group"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_device_group" "all_0"`) {
		t.Errorf("expected original label preserved when no name attribute, got:\n%s", body)
	}
}

func TestRenameLabels_MixedResourceTypes(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Same name across different resource types should not collide
	src := `resource "jamfplatform_blueprints_blueprint" "all_0" {
  name = "Production"
}

resource "jamfplatform_device_group" "all_0" {
  name = "Production"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	// Both should be "production" since they are different resource types
	if !strings.Contains(body, `"jamfplatform_blueprints_blueprint" "production"`) {
		t.Errorf("expected blueprint renamed to production, got:\n%s", body)
	}
	if !strings.Contains(body, `"jamfplatform_device_group" "production"`) {
		t.Errorf("expected device group renamed to production, got:\n%s", body)
	}
}

func TestRenameLabels_MissingFile(t *testing.T) {
	err := RenameLabels("/nonexistent/path/generated.tf")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRenameLabels_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	if err := os.WriteFile(generatedFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not error on empty file
	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}
}

func TestRenameLabels_NoRenamesNeeded(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Resource that already has the correct label - no rename needed
	src := `resource "jamfplatform_device_group" "staff_macs" {
  name = "Staff Macs"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_device_group" "staff_macs"`) {
		t.Errorf("expected label to remain staff_macs, got:\n%s", body)
	}
}

func TestRenameLabels_ProTypeUsesGeneralName(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Federated pro_* objecty types carry their display name at nested
	// general.name, not a top-level name attribute.
	src := `import {
  identity = {
    id = "10"
  }
  to = jamfplatform_pro_policy.all_0
}

resource "jamfplatform_pro_policy" "all_0" {
  general = {
    name    = "Install Chrome"
    enabled = true
  }
}

resource "jamfplatform_pro_macos_configuration_profile" "all_1" {
  general = {
    name = "FileVault Settings"
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `"jamfplatform_pro_policy" "install_chrome"`) {
		t.Errorf("expected policy renamed from general.name to install_chrome, got:\n%s", body)
	}
	if !strings.Contains(body, "jamfplatform_pro_policy.install_chrome") {
		t.Errorf("expected import block updated to install_chrome, got:\n%s", body)
	}
	if !strings.Contains(body, `"jamfplatform_pro_macos_configuration_profile" "filevault_settings"`) {
		t.Errorf("expected profile renamed from general.name to filevault_settings, got:\n%s", body)
	}
}

// TestRenameLabels_TopLevelNameStillWins covers flat pro_* types (category,
// account, building, …) whose name remains a top-level attribute.
func TestRenameLabels_TopLevelNameStillWins(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_pro_category" "all_0" {
  name = "Productivity Apps"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	if body := string(result); !strings.Contains(body, `"jamfplatform_pro_category" "productivity_apps"`) {
		t.Errorf("expected category renamed from top-level name, got:\n%s", body)
	}
}

// TestRenameLabels_PackageUsesDisplayName covers jamfplatform_pro_package, whose
// human label lives in display_name (it has no top-level name attribute).
func TestRenameLabels_PackageUsesDisplayName(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_pro_package" "all_0" {
  display_name = "Google Chrome.pkg"
  file_name    = "Google Chrome.pkg"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	if err := RenameLabels(generatedFile); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	result, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	if body := string(result); !strings.Contains(body, `"jamfplatform_pro_package" "google_chrome_pkg"`) {
		t.Errorf("expected package renamed from display_name, got:\n%s", body)
	}
}

func TestNameAttrForType(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		{"jamfplatform_cbengine_benchmark", "title"},
		{"jamfplatform_pro_package", "display_name"},
		{"jamfplatform_blueprints_blueprint", "name"},
		{"jamfplatform_device_group", "name"},
		{"some_unknown_type", "name"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			got := nameAttrForType(tt.resourceType)
			if got != tt.want {
				t.Errorf("nameAttrForType(%q) = %q, want %q", tt.resourceType, got, tt.want)
			}
		})
	}
}
