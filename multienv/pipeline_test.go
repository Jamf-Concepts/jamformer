// Copyright 2026, Jamf Software LLC

package multienv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupOutputRoot(t *testing.T) {
	dir := t.TempDir()

	// Create files that should be cleaned up
	for _, name := range []string{
		"scripts_import.tf", "policies_import.tf",
		"provider.tf", "variables.tf", "terraform.tfvars",
		"locals_env.tf", "dev.tfvars", "prod.tfvars",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create support_files dir that should be removed
	if err := os.MkdirAll(filepath.Join(dir, "support_files", "dev", "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "support_files", "dev", "scripts", "test.sh"), []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create files that should NOT be cleaned up
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "modules", "jamf"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modules", "jamf", "policies.tf"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	cleanupOutputRoot(dir)

	// Verify cleanup
	for _, name := range []string{
		"scripts_import.tf", "policies_import.tf",
		"provider.tf", "variables.tf", "terraform.tfvars",
		"locals_env.tf", "dev.tfvars", "prod.tfvars",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "support_files")); !os.IsNotExist(err) {
		t.Error("support_files directory should have been removed")
	}

	// Verify preserved files
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Error(".gitignore should be preserved")
	}
	if _, err := os.Stat(filepath.Join(dir, "modules", "jamf", "policies.tf")); err != nil {
		t.Error("module files should be preserved")
	}
}
