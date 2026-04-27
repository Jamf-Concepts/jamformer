// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestPopulateRegistryFromGenerated_QueryImportBlocks(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `import {
  identity = {
    id = "abc-123"
  }
  to = jamfplatform_blueprints_blueprint.macos_standard
}

resource "jamfplatform_blueprints_blueprint" "macos_standard" {
  name = "macOS Standard"
}

import {
  identity = {
    id = "def-456"
  }
  to = jamfplatform_device_group.staff_macs
}

resource "jamfplatform_device_group" "staff_macs" {
  name = "Staff Macs"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	// Verify registry entries
	ref, ok := reg.Resolve("jamfplatform_blueprints_blueprint", "abc-123")
	if !ok || ref != "jamfplatform_blueprints_blueprint.macos_standard" {
		t.Errorf("expected blueprint registry entry, got %q (ok=%v)", ref, ok)
	}

	ref, ok = reg.Resolve("jamfplatform_device_group", "def-456")
	if !ok || ref != "jamfplatform_device_group.staff_macs" {
		t.Errorf("expected device group registry entry, got %q (ok=%v)", ref, ok)
	}
}

func TestCountResources(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_blueprints_blueprint" "one" {
  name = "One"
}

resource "jamfplatform_blueprints_blueprint" "two" {
  name = "Two"
}

resource "jamfplatform_device_group" "staff" {
  name = "Staff"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	counts, err := CountResources(generatedFile)
	if err != nil {
		t.Fatal(err)
	}

	if counts["jamfplatform_blueprints_blueprint"] != 2 {
		t.Errorf("expected 2 blueprints, got %d", counts["jamfplatform_blueprints_blueprint"])
	}
	if counts["jamfplatform_device_group"] != 1 {
		t.Errorf("expected 1 device group, got %d", counts["jamfplatform_device_group"])
	}
}

func TestPopulateRegistryFromGenerated_UUIDImportBlocks(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_cbengine_benchmark" "cis_macos" {
  title = "CIS macOS Benchmark"
}

import {
  identity = {
    id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
  }
  to = jamfplatform_cbengine_benchmark.cis_macos
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	ref, ok := reg.AttrReference("jamfplatform_cbengine_benchmark", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "id")
	if !ok {
		t.Error("expected benchmark UUID to be registered")
	} else if ref != "jamfplatform_cbengine_benchmark.cis_macos.id" {
		t.Errorf("expected jamfplatform_cbengine_benchmark.cis_macos.id, got %s", ref)
	}
}

func TestPopulateRegistryFromGenerated_MultipleTypes(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `import {
  identity = {
    id = "bp-1"
  }
  to = jamfplatform_blueprints_blueprint.macos
}

import {
  identity = {
    id = "bp-2"
  }
  to = jamfplatform_blueprints_blueprint.windows
}

import {
  identity = {
    id = "dg-1"
  }
  to = jamfplatform_device_group.all_devices
}

import {
  identity = {
    id = "bench-1"
  }
  to = jamfplatform_cbengine_benchmark.cis
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	// Verify all entries registered
	tests := []struct {
		resourceType string
		id           string
		wantAddr     string
	}{
		{"jamfplatform_blueprints_blueprint", "bp-1", "jamfplatform_blueprints_blueprint.macos"},
		{"jamfplatform_blueprints_blueprint", "bp-2", "jamfplatform_blueprints_blueprint.windows"},
		{"jamfplatform_device_group", "dg-1", "jamfplatform_device_group.all_devices"},
		{"jamfplatform_cbengine_benchmark", "bench-1", "jamfplatform_cbengine_benchmark.cis"},
	}

	for _, tt := range tests {
		ref, ok := reg.Resolve(tt.resourceType, tt.id)
		if !ok || ref != tt.wantAddr {
			t.Errorf("Resolve(%q, %q) = (%q, %v), want (%q, true)", tt.resourceType, tt.id, ref, ok, tt.wantAddr)
		}
	}

	// Unregistered ID should not resolve
	if _, ok := reg.Resolve("jamfplatform_blueprints_blueprint", "nonexistent"); ok {
		t.Error("expected nonexistent ID to not resolve")
	}
}

func TestPopulateRegistryFromGenerated_MissingIdentity(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Import block without identity or id should not register anything
	src := `import {
  to = jamfplatform_device_group.orphan
}

resource "jamfplatform_device_group" "orphan" {
  name = "Orphan"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	// Nothing should be registered since there's no id
	if _, ok := reg.Resolve("jamfplatform_device_group", ""); ok {
		t.Error("expected no registration for import block without identity/id")
	}
}

func TestPopulateRegistryFromGenerated_FlatIdFallback(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Flat id format (singleton style) should also work
	src := `import {
  to = jamfplatform_device_group.singleton
  id = "singleton-id-123"
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	ref, ok := reg.Resolve("jamfplatform_device_group", "singleton-id-123")
	if !ok || ref != "jamfplatform_device_group.singleton" {
		t.Errorf("expected flat id fallback to register, got %q (ok=%v)", ref, ok)
	}
}

func TestPopulateRegistryFromGenerated_MissingToAttribute(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Import block without "to" attribute should be skipped
	src := `import {
  identity = {
    id = "abc-123"
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}
	// Nothing should crash, nothing should be registered
}

func TestPopulateRegistryFromGenerated_SkipsResourceBlocks(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Only import blocks should be processed, not resource blocks
	src := `resource "jamfplatform_device_group" "staff" {
  name = "Staff"
}

import {
  identity = {
    id = "dg-1"
  }
  to = jamfplatform_device_group.staff
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}

	// Only the import block should register an entry
	ref, ok := reg.Resolve("jamfplatform_device_group", "dg-1")
	if !ok || ref != "jamfplatform_device_group.staff" {
		t.Errorf("expected device group registered from import block, got %q (ok=%v)", ref, ok)
	}
}

func TestPopulateRegistryFromGenerated_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	if err := os.WriteFile(generatedFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(generatedFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated: %v", err)
	}
}

func TestPopulateRegistryFromGenerated_MissingFile(t *testing.T) {
	reg := registry.New()
	err := PopulateRegistryFromGenerated("/nonexistent/path/generated.tf", reg)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCountResources_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	if err := os.WriteFile(generatedFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	counts, err := CountResources(generatedFile)
	if err != nil {
		t.Fatalf("CountResources: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty counts, got %v", counts)
	}
}

func TestCountResources_MissingFile(t *testing.T) {
	_, err := CountResources("/nonexistent/path/generated.tf")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCountResources_IgnoresImportBlocks(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	src := `resource "jamfplatform_device_group" "staff" {
  name = "Staff"
}

import {
  identity = {
    id = "1"
  }
  to = jamfplatform_device_group.staff
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	counts, err := CountResources(generatedFile)
	if err != nil {
		t.Fatalf("CountResources: %v", err)
	}

	if counts["jamfplatform_device_group"] != 1 {
		t.Errorf("expected 1 device group, got %d", counts["jamfplatform_device_group"])
	}
}

func TestResourceTypeDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"jamfplatform_blueprints_blueprint", "blueprints blueprint"},
		{"jamfplatform_cbengine_benchmark", "cbengine benchmark"},
		{"jamfplatform_device_group", "device group"},
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

func TestResourceTypeDisplayName_NoPrefix(t *testing.T) {
	// If input doesn't have the prefix, it just replaces underscores
	got := ResourceTypeDisplayName("some_other_type")
	if got != "some other type" {
		t.Errorf("expected 'some other type', got %q", got)
	}
}
