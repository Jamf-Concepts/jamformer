// Copyright 2026, Jamf Software LLC

package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvedProviderVersion(t *testing.T) {
	lockContent := `# This file is maintained automatically by "terraform init".

provider "registry.terraform.io/deploymenttheory/jamfpro" {
  version     = "0.35.1"
  constraints = "0.35.1"
  hashes = [
    "h1:abc123=",
  ]
}

provider "registry.terraform.io/jamf-concepts/jamfprotect" {
  version = "0.1.1"
  hashes = [
    "h1:def456=",
  ]
}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(lockContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		source string
		want   string
	}{
		{"deploymenttheory/jamfpro", "0.35.1"},
		{"Jamf-Concepts/jamfprotect", "0.1.1"},
		{"nonexistent/provider", ""},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := ResolvedProviderVersion(dir, tt.source)
			if got != tt.want {
				t.Errorf("ResolvedProviderVersion(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestResolvedProviderVersion_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	got := ResolvedProviderVersion(dir, "deploymenttheory/jamfpro")
	if got != "" {
		t.Errorf("expected empty string when no lock file, got %q", got)
	}
}

func TestFormatVersionConstraint(t *testing.T) {
	tests := []struct {
		name     string
		pinned   string
		resolved string
		want     string
	}{
		{"pinned version", "1.2.3", "1.0.0", "\n      version = \"1.2.3\""},
		{"resolved only", "", "0.35.1", "\n      version = \">= 0.35.1\""},
		{"neither", "", "", ""},
		{"pinned takes precedence", "2.0.0", "1.5.0", "\n      version = \"2.0.0\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatVersionConstraint(tt.pinned, tt.resolved)
			if got != tt.want {
				t.Errorf("FormatVersionConstraint(%q, %q) = %q, want %q", tt.pinned, tt.resolved, got, tt.want)
			}
		})
	}
}
