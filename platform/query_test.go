// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateQueryFile_AllResources(t *testing.T) {
	dir := t.TempDir()

	if err := GenerateQueryFile(dir, nil); err != nil {
		t.Fatalf("GenerateQueryFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "query.tfquery.hcl"))
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}

	body := string(content)

	// All listable resource types should appear; singletons must not.
	for _, r := range ListableResources() {
		if !strings.Contains(body, r.TFType) {
			t.Errorf("query file missing resource type %q (filter key %q)", r.TFType, r.FilterKey)
		}
	}
	for _, r := range SingletonResources() {
		if strings.Contains(body, "list \""+r.TFType+"\"") {
			t.Errorf("query file must not contain a list block for singleton %q", r.TFType)
		}
	}

	// Should contain list blocks with correct provider
	if !strings.Contains(body, "provider = jamfplatform") {
		t.Error("query file missing 'provider = jamfplatform'")
	}
}

func TestGenerateQueryFile_FilteredResources(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"blueprints": true,
	}

	if err := GenerateQueryFile(dir, selected); err != nil {
		t.Fatalf("GenerateQueryFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "query.tfquery.hcl"))
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}

	body := string(content)

	if !strings.Contains(body, "jamfplatform_blueprints_blueprint") {
		t.Error("query file should contain blueprints")
	}
	if strings.Contains(body, "jamfplatform_device_group") {
		t.Error("query file should not contain device_groups when filtered out")
	}
}

func TestGenerateQueryFile_MultipleFiltered(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"blueprints":    true,
		"device_groups": true,
	}

	if err := GenerateQueryFile(dir, selected); err != nil {
		t.Fatalf("GenerateQueryFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "query.tfquery.hcl"))
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}

	body := string(content)

	if !strings.Contains(body, "jamfplatform_blueprints_blueprint") {
		t.Error("query file should contain blueprints")
	}
	if !strings.Contains(body, "jamfplatform_device_group") {
		t.Error("query file should contain device_groups")
	}
	if strings.Contains(body, "jamfplatform_cbengine_benchmark") {
		t.Error("query file should not contain benchmarks when not selected")
	}
}

func TestGenerateQueryFile_NoneSelected(t *testing.T) {
	dir := t.TempDir()

	// Select a filter key that doesn't match any listable type
	selected := map[string]bool{
		"nonexistent": true,
	}

	if err := GenerateQueryFile(dir, selected); err != nil {
		t.Fatalf("GenerateQueryFile: %v", err)
	}

	// Query file should not be created when no types match
	if _, err := os.Stat(filepath.Join(dir, "query.tfquery.hcl")); !os.IsNotExist(err) {
		t.Error("query file should not be created when no resource types match the filter")
	}
}

func TestGenerateQueryFile_QueryFileStructure(t *testing.T) {
	dir := t.TempDir()

	selected := map[string]bool{
		"compliance_benchmarks": true,
	}

	if err := GenerateQueryFile(dir, selected); err != nil {
		t.Fatalf("GenerateQueryFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "query.tfquery.hcl"))
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}

	body := string(content)

	// Verify list block structure
	if !strings.Contains(body, `list "jamfplatform_cbengine_benchmark" "all"`) {
		t.Error("expected list block with correct resource type and label")
	}
	if !strings.Contains(body, "provider = jamfplatform") {
		t.Error("expected provider = jamfplatform in list block")
	}
	if !strings.Contains(body, "limit    = 10000") {
		t.Error("expected limit = 10000 in list block")
	}
}
