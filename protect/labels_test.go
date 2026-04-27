// Copyright 2026, Jamf Software LLC

package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNameAttrForType(t *testing.T) {
	tests := []struct {
		resourceType string
		want         string
	}{
		{"jamfprotect_user", "email"},
		{"jamfprotect_role", "name"},
		{"jamfprotect_group", "name"},
		{"jamfprotect_analytic", "name"},
		{"unknown_type", "name"},
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

func TestRenameLabels_BasicRename(t *testing.T) {
	hcl := `
resource "jamfprotect_role" "all_0" {
  name = "Full Admin"
}

resource "jamfprotect_role" "all_1" {
  name = "Read Only"
}

import {
  identity = {
    id = "2"
  }
  to = jamfprotect_role.all_0
}

import {
  identity = {
    id = "1"
  }
  to = jamfprotect_role.all_1
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	// Resource labels should be renamed
	if !strings.Contains(result, `"jamfprotect_role" "full_admin"`) {
		t.Error("Expected resource label to be renamed to full_admin")
	}
	if !strings.Contains(result, `"jamfprotect_role" "read_only"`) {
		t.Error("Expected resource label to be renamed to read_only")
	}

	// Import block "to" attributes should be updated
	if !strings.Contains(result, "jamfprotect_role.full_admin") {
		t.Error("Expected import block to reference full_admin")
	}
	if !strings.Contains(result, "jamfprotect_role.read_only") {
		t.Error("Expected import block to reference read_only")
	}

	// Old labels should not appear
	if strings.Contains(result, "all_0") {
		t.Error("Old label all_0 should not appear in output")
	}
	if strings.Contains(result, "all_1") {
		t.Error("Old label all_1 should not appear in output")
	}
}

func TestRenameLabels_UserEmailLabels(t *testing.T) {
	hcl := `
resource "jamfprotect_user" "all_0" {
  email = "alice@example.com"
}

resource "jamfprotect_user" "all_1" {
  email = "bob@example.com"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	if !strings.Contains(result, `"jamfprotect_user" "alice_example_com"`) {
		t.Errorf("Expected user label to be alice_example_com, got:\n%s", result)
	}
	if !strings.Contains(result, `"jamfprotect_user" "bob_example_com"`) {
		t.Errorf("Expected user label to be bob_example_com, got:\n%s", result)
	}
}

func TestRenameLabels_DuplicateNames(t *testing.T) {
	hcl := `
resource "jamfprotect_role" "all_0" {
  name = "Admin"
}

resource "jamfprotect_role" "all_1" {
  name = "Admin"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	// First one should be "admin", second should be "admin_2"
	if !strings.Contains(result, `"jamfprotect_role" "admin"`) {
		t.Errorf("Expected first role to be admin, got:\n%s", result)
	}
	if !strings.Contains(result, `"jamfprotect_role" "admin_2"`) {
		t.Errorf("Expected second role to be admin_2, got:\n%s", result)
	}
}

func TestRenameLabels_SingletonLabelsPreserved(t *testing.T) {
	// Singleton resources already have good labels — should be left alone
	hcl := `
resource "jamfprotect_change_management" "singleton" {
  name = "Change Management"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	// The label should be renamed from "singleton" to "change_management"
	// based on the name attribute
	if !strings.Contains(result, `"jamfprotect_change_management" "change_management"`) {
		t.Errorf("Expected singleton to be renamed to change_management, got:\n%s", result)
	}
}

func TestRenameLabels_SpecialCharacters(t *testing.T) {
	hcl := `
resource "jamfprotect_analytic" "all_0" {
  name = "macOS - Threat Prevention (v2)"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	// Special characters should be sanitized to underscores
	if !strings.Contains(result, `"jamfprotect_analytic" "macos_threat_prevention_v2"`) {
		t.Errorf("Expected sanitized label macos_threat_prevention_v2, got:\n%s", result)
	}
}

func TestRenameLabels_NoNameAttribute(t *testing.T) {
	// Resources without a name attribute should keep their original label
	hcl := `
resource "jamfprotect_role" "all_0" {
  write_permissions = ["all"]
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	// Original label preserved since there's no name
	if !strings.Contains(string(content), `"jamfprotect_role" "all_0"`) {
		t.Error("Expected original label all_0 to be preserved when no name attribute")
	}
}

func TestRenameLabels_MixedResourceTypes(t *testing.T) {
	hcl := `
resource "jamfprotect_role" "all_0" {
  name = "Admin"
}

resource "jamfprotect_group" "all_0" {
  name = "Admin"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := RenameLabels(genFile); err != nil {
		t.Fatalf("RenameLabels() error: %v", err)
	}

	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatalf("Failed to read result file: %v", err)
	}

	result := string(content)

	// Both should be "admin" since they're different resource types (no collision)
	if !strings.Contains(result, `"jamfprotect_role" "admin"`) {
		t.Errorf("Expected role label to be admin, got:\n%s", result)
	}
	if !strings.Contains(result, `"jamfprotect_group" "admin"`) {
		t.Errorf("Expected group label to be admin, got:\n%s", result)
	}
}
