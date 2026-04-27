// Copyright 2026, Jamf Software LLC

package protect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestPopulateRegistryFromGenerated_QueryImportBlocks(t *testing.T) {
	// Simulates terraform query output: import blocks with nested identity { id = "..." }
	hcl := `
resource "jamfprotect_role" "full_admin" {
  name = "Full Admin"
}

import {
  identity = {
    id = "2"
  }
  to = jamfprotect_role.full_admin
}

resource "jamfprotect_group" "all_users" {
  name = "All Users"
}

import {
  identity = {
    id = "1"
  }
  to = jamfprotect_group.all_users
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(genFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated() error: %v", err)
	}

	// Role ID 2 should resolve to jamfprotect_role.full_admin
	if ref, ok := reg.Resolve("jamfprotect_role", "2"); !ok {
		t.Error("Expected role ID 2 to be registered")
	} else if ref != "jamfprotect_role.full_admin" {
		t.Errorf("Expected jamfprotect_role.full_admin, got %s", ref)
	}

	// Group ID 1 should resolve to jamfprotect_group.all_users
	if ref, ok := reg.Resolve("jamfprotect_group", "1"); !ok {
		t.Error("Expected group ID 1 to be registered")
	} else if ref != "jamfprotect_group.all_users" {
		t.Errorf("Expected jamfprotect_group.all_users, got %s", ref)
	}

	// Unregistered ID should not resolve
	if _, ok := reg.Resolve("jamfprotect_role", "999"); ok {
		t.Error("Expected role ID 999 to not be registered")
	}
}

func TestPopulateRegistryFromGenerated_SingletonImportBlocks(t *testing.T) {
	// Simulates singleton import blocks with flat id = "..."
	hcl := `
import {
  to = jamfprotect_change_management.singleton
  id = "change_management_singleton"
}

import {
  to = jamfprotect_data_forwarding.singleton
  id = "data_forwarding_singleton"
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "singletons.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(genFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated() error: %v", err)
	}

	if ref, ok := reg.Resolve("jamfprotect_change_management", "change_management_singleton"); !ok {
		t.Error("Expected change_management singleton to be registered")
	} else if ref != "jamfprotect_change_management.singleton" {
		t.Errorf("Expected jamfprotect_change_management.singleton, got %s", ref)
	}
}

func TestPopulateRegistryFromGenerated_UUIDImportBlocks(t *testing.T) {
	// Simulates analytics with UUID IDs
	hcl := `
resource "jamfprotect_analytic" "macos_threat_prevention" {
  name = "macOS - Threat Prevention"
}

import {
  identity = {
    id = "3c8a88ef-277a-4238-a695-ebaa6eee0921"
  }
  to = jamfprotect_analytic.macos_threat_prevention
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(genFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated() error: %v", err)
	}

	if ref, ok := reg.AttrReference("jamfprotect_analytic", "3c8a88ef-277a-4238-a695-ebaa6eee0921", "id"); !ok {
		t.Error("Expected analytic UUID to be registered")
	} else if ref != "jamfprotect_analytic.macos_threat_prevention.id" {
		t.Errorf("Expected jamfprotect_analytic.macos_threat_prevention.id, got %s", ref)
	}
}

func TestCountResources(t *testing.T) {
	hcl := `
resource "jamfprotect_role" "admin" {
  name = "Admin"
}

resource "jamfprotect_role" "read_only" {
  name = "Read Only"
}

resource "jamfprotect_user" "alice" {
  email = "alice@example.com"
}

import {
  identity = {
    id = "1"
  }
  to = jamfprotect_role.admin
}
`
	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(hcl), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	counts, err := CountResources(genFile)
	if err != nil {
		t.Fatalf("CountResources() error: %v", err)
	}

	if counts["jamfprotect_role"] != 2 {
		t.Errorf("Expected 2 roles, got %d", counts["jamfprotect_role"])
	}
	if counts["jamfprotect_user"] != 1 {
		t.Errorf("Expected 1 user, got %d", counts["jamfprotect_user"])
	}
}

func TestResourceTypeDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"jamfprotect_role", "role"},
		{"jamfprotect_analytic_set", "analytic set"},
		{"jamfprotect_removable_storage_control_set", "removable storage control set"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResourceTypeDisplayName(tt.input)
			if got != tt.want {
				t.Errorf("ResourceTypeDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
