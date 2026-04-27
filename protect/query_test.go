// Copyright 2026, Jamf Software LLC

package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateQueryFile_AllResources(t *testing.T) {
	dir := t.TempDir()

	if err := GenerateQueryFile(dir, nil); err != nil {
		t.Fatalf("GenerateQueryFile() error: %v", err)
	}

	// Check query file was created
	queryFile := filepath.Join(dir, "query.tfquery.hcl")
	content, err := os.ReadFile(queryFile)
	if err != nil {
		t.Fatalf("Failed to read query file: %v", err)
	}

	// All listable resource types should appear as list blocks
	for filterKey, resourceType := range listableResourceTypes {
		if !strings.Contains(string(content), resourceType) {
			t.Errorf("Query file missing list block for %s (%s)", filterKey, resourceType)
		}
	}

	// Singleton import file should NOT be created by GenerateQueryFile
	if _, err := os.Stat(filepath.Join(dir, "singletons_import.tf")); !os.IsNotExist(err) {
		t.Error("GenerateQueryFile should not create singletons_import.tf (use WriteSingletonImports)")
	}
}

func TestWriteSingletonImports_AllResources(t *testing.T) {
	dir := t.TempDir()

	if err := WriteSingletonImports(dir, nil, false); err != nil {
		t.Fatalf("WriteSingletonImports() error: %v", err)
	}

	sContent, err := os.ReadFile(filepath.Join(dir, "singletons_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read singleton import file: %v", err)
	}

	for _, singleton := range singletonResources {
		if !strings.Contains(string(sContent), singleton.ResourceType) {
			t.Errorf("Singleton import file missing %s", singleton.ResourceType)
		}
	}
}

func TestGenerateQueryFile_FilteredResources(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"roles":  true,
		"groups": true,
	}

	if err := GenerateQueryFile(dir, selected); err != nil {
		t.Fatalf("GenerateQueryFile() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "query.tfquery.hcl"))
	if err != nil {
		t.Fatalf("Failed to read query file: %v", err)
	}

	if !strings.Contains(string(content), "jamfprotect_role") {
		t.Error("Query file should contain jamfprotect_role")
	}
	if !strings.Contains(string(content), "jamfprotect_group") {
		t.Error("Query file should contain jamfprotect_group")
	}
	if strings.Contains(string(content), "jamfprotect_plan") {
		t.Error("Query file should not contain jamfprotect_plan when not selected")
	}
}

func TestWriteSingletonImports_Filtered(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"change_management": true,
	}

	if err := WriteSingletonImports(dir, selected, false); err != nil {
		t.Fatalf("WriteSingletonImports() error: %v", err)
	}

	sContent, err := os.ReadFile(filepath.Join(dir, "singletons_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read singleton import file: %v", err)
	}
	if !strings.Contains(string(sContent), "jamfprotect_change_management") {
		t.Error("Singleton file should contain jamfprotect_change_management")
	}
	if strings.Contains(string(sContent), "jamfprotect_data_forwarding") {
		t.Error("Singleton file should not contain jamfprotect_data_forwarding when not selected")
	}
}

func TestWriteSingletonImports_NoneSelected(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"roles": true,
	}

	if err := WriteSingletonImports(dir, selected, false); err != nil {
		t.Fatalf("WriteSingletonImports() error: %v", err)
	}

	// No singletons selected, file should not be created
	if _, err := os.Stat(filepath.Join(dir, "singletons_import.tf")); !os.IsNotExist(err) {
		t.Error("Singleton import file should not be created when no singletons selected")
	}
}

func TestWriteSingletonImports_SkipDataForwarding(t *testing.T) {
	dir := t.TempDir()

	oldQuiet := Quiet
	Quiet = true
	defer func() { Quiet = oldQuiet }()

	if err := WriteSingletonImports(dir, nil, true); err != nil {
		t.Fatalf("WriteSingletonImports() error: %v", err)
	}

	sContent, err := os.ReadFile(filepath.Join(dir, "singletons_import.tf"))
	if err != nil {
		t.Fatalf("Failed to read singleton import file: %v", err)
	}

	if strings.Contains(string(sContent), "jamfprotect_data_forwarding") {
		t.Error("Singleton import file should not contain data_forwarding when skipDataForwarding is true")
	}
	// Other singletons should still be present
	if !strings.Contains(string(sContent), "jamfprotect_change_management") {
		t.Error("Singleton import file should still contain change_management")
	}
	if !strings.Contains(string(sContent), "jamfprotect_data_retention") {
		t.Error("Singleton import file should still contain data_retention")
	}
}
