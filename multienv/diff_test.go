// Copyright 2026, Jamf Software LLC

package multienv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestDiffResources(t *testing.T) {
	t.Run("detects differing attributes", func(t *testing.T) {
		dir := t.TempDir()

		// Write a postprocessed .tf file (what the source env produced)
		resourceTF := `resource "jamfpro_category" "productivity" {
  name     = "Productivity"
  priority = "5"
}
`
		if err := os.WriteFile(filepath.Join(dir, "categories.tf"), []byte(resourceTF), 0644); err != nil {
			t.Fatal(err)
		}

		// Create mock per-env results with different generated.tf content
		devDir := t.TempDir()
		devGen := `resource "jamfpro_category" "productivity" {
  name     = "Productivity"
  priority = "5"
}
`
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(devGen), 0644)

		prodDir := t.TempDir()
		prodGen := `resource "jamfpro_category" "productivity" {
  name     = "Productivity"
  priority = "10"
}
`
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(prodGen), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}

		matches := []MatchedResource{
			{ResourceType: "jamfpro_category", Label: "productivity", IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatalf("DiffResources: %v", err)
		}

		// Should find priority diff
		found := false
		for _, d := range diffs {
			if d.AttrName == "priority" {
				found = true
				if d.Values["dev"] != `"5"` {
					t.Errorf("dev value = %q, want %q", d.Values["dev"], `"5"`)
				}
				if d.Values["prod"] != `"10"` {
					t.Errorf("prod value = %q, want %q", d.Values["prod"], `"10"`)
				}
			}
		}
		if !found {
			t.Error("expected priority diff, got none")
		}

		// name should NOT be diffed (identical)
		for _, d := range diffs {
			if d.AttrName == "name" {
				t.Error("name should not be diffed (identical across envs)")
			}
		}
	})

	t.Run("skips file() attributes", func(t *testing.T) {
		dir := t.TempDir()

		// Postprocessed output has file() ref
		resourceTF := `resource "jamfpro_script" "test" {
  name            = "Test"
  script_contents = file("${path.module}/support_files/dev/scripts/test.sh")
  category_id     = "5"
}
`
		_ = os.WriteFile(filepath.Join(dir, "scripts.tf"), []byte(resourceTF), 0644)

		devDir := t.TempDir()
		devGen := `resource "jamfpro_script" "test" {
  name            = "Test"
  script_contents = "#!/bin/bash\necho dev"
  category_id     = "5"
}
`
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(devGen), 0644)

		prodDir := t.TempDir()
		prodGen := `resource "jamfpro_script" "test" {
  name            = "Test"
  script_contents = "#!/bin/bash\necho prod"
  category_id     = "10"
}
`
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(prodGen), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}

		matches := []MatchedResource{
			{ResourceType: "jamfpro_script", Label: "test", IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatal(err)
		}

		// script_contents should NOT be diffed (file() ref in postprocessed output)
		for _, d := range diffs {
			if d.AttrName == "script_contents" {
				t.Error("script_contents should not be diffed when postprocessed to file()")
			}
		}

		// category_id SHOULD be diffed
		found := false
		for _, d := range diffs {
			if d.AttrName == "category_id" {
				found = true
			}
		}
		if !found {
			t.Error("expected category_id diff")
		}
	})

	t.Run("skips partial-env resources", func(t *testing.T) {
		dir := t.TempDir()

		devDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(`resource "jamfpro_script" "dev_only" {
  name = "Dev Only"
}
`), 0644)

		prodDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(""), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}

		matches := []MatchedResource{
			{ResourceType: "jamfpro_script", Label: "dev_only", IDs: map[string]string{"dev": "1"}, Present: []string{"dev"}, AllEnvs: false},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatal(err)
		}

		if len(diffs) != 0 {
			t.Errorf("expected 0 diffs for partial-env resource, got %d", len(diffs))
		}
	})

	t.Run("no diffs when identical", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "categories.tf"), []byte(`resource "jamfpro_category" "test" {
  name = "Test"
}
`), 0644)

		gen := `resource "jamfpro_category" "test" {
  name = "Test"
}
`
		devDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(gen), 0644)
		prodDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(gen), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}
		matches := []MatchedResource{
			{ResourceType: "jamfpro_category", Label: "test", IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatal(err)
		}
		if len(diffs) != 0 {
			t.Errorf("expected 0 diffs for identical resources, got %d", len(diffs))
		}
	})
}

func TestInferVarType(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{`"hello"`, "string"},
		{`"5"`, "string"},
		{`5`, "string"},
		{`["a", "b"]`, "list(string)"},
		{`["Unassigned"]`, "list(string)"},
		{`{"key" = "val"}`, "map(string)"},
		{`true`, "string"},
	}
	for _, tt := range tests {
		got := inferVarType(tt.expr)
		if got != tt.want {
			t.Errorf("inferVarType(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}

func TestIsResourceReference(t *testing.T) {
	tests := []struct {
		expr string
		want bool
	}{
		{"jamfpro_category.browsers.id", true},
		{"jamfpro_script.install_agent.id", true},
		{`"238"`, false},
		{"var.category_id", false},
		{"local.env_urls", false},
		{"file(\"path\")", false},
		{`[jamfpro_smart_computer_group_v2.all_managed.id]`, true},
		{"5", false},
		{"true", false},
	}
	for _, tt := range tests {
		got := isResourceReference(tt.expr)
		if got != tt.want {
			t.Errorf("isResourceReference(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestDiffResources_SkipsResourceReferences(t *testing.T) {
	dir := t.TempDir()

	// Postprocessed output has a resource reference for category_id
	resourceTF := `resource "jamfpro_script" "test" {
  name        = "Test"
  category_id = jamfpro_category.provisioning.id
}
`
	_ = os.WriteFile(filepath.Join(dir, "scripts.tf"), []byte(resourceTF), 0644)

	devDir := t.TempDir()
	devGen := `resource "jamfpro_script" "test" {
  name        = "Test"
  category_id = "100"
}
`
	_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(devGen), 0644)

	prodDir := t.TempDir()
	prodGen := `resource "jamfpro_script" "test" {
  name        = "Test"
  category_id = "200"
}
`
	_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(prodGen), 0644)

	envResults := map[string]*PerEnvResult{
		"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
		"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
	}
	matches := []MatchedResource{
		{ResourceType: "jamfpro_script", Label: "test", IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
	}

	diffs, err := DiffResources(dir, envResults, matches, "dev")
	if err != nil {
		t.Fatal(err)
	}

	// category_id should NOT be diffed because postprocessing resolved it to a resource reference
	for _, d := range diffs {
		if d.AttrName == "category_id" {
			t.Error("category_id should not be diffed when postprocessed to a resource reference")
		}
	}
}

func TestDiffResources_NullHandling(t *testing.T) {
	t.Run("-1 vs null kept as optional", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "prestages.tf"), []byte(`resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  name                  = "Test"
  rts_config_profile_id = "-1"
}
`), 0644)

		devDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(`resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  name                  = "Test"
  rts_config_profile_id = "-1"
}
`), 0644)
		prodDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(`resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  name                  = "Test"
  rts_config_profile_id = null
}
`), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}
		matches := []MatchedResource{
			{ResourceType: "jamfpro_mobile_device_prestage_enrollment", Label: "test",
				IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range diffs {
			if d.AttrName == "rts_config_profile_id" {
				found = true
				if d.VarType != "optional(string)" {
					t.Errorf("VarType = %q, want %q", d.VarType, "optional(string)")
				}
			}
		}
		if !found {
			t.Error("-1 vs null should be kept as a diff")
		}
	})

	t.Run("real value vs null kept as optional", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "profiles.tf"), []byte(`resource "jamfpro_macos_configuration_profile_plist" "test" {
  name        = "Test"
  description = "A profile"
}
`), 0644)

		devDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(`resource "jamfpro_macos_configuration_profile_plist" "test" {
  name        = "Test"
  description = null
}
`), 0644)
		prodDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(`resource "jamfpro_macos_configuration_profile_plist" "test" {
  name        = "Test"
  description = "A profile"
}
`), 0644)

		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
			"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
		}
		matches := []MatchedResource{
			{ResourceType: "jamfpro_macos_configuration_profile_plist", Label: "test",
				IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
		}

		diffs, err := DiffResources(dir, envResults, matches, "dev")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range diffs {
			if d.AttrName == "description" {
				found = true
				if d.VarType != "optional(string)" {
					t.Errorf("VarType = %q, want %q", d.VarType, "optional(string)")
				}
				if d.Values["dev"] != "null" {
					t.Errorf("dev value = %q, want %q", d.Values["dev"], "null")
				}
			}
		}
		if !found {
			t.Error("description diff should be kept (real value vs null)")
		}
	})
}

func TestDiffResources_ListType(t *testing.T) {
	dir := t.TempDir()

	resourceTF := `resource "jamfpro_computer_extension_attribute" "restrictions" {
  name              = "Restrictions"
  popup_menu_choices = ["Unassigned"]
}
`
	_ = os.WriteFile(filepath.Join(dir, "computer_extension_attributes.tf"), []byte(resourceTF), 0644)

	devDir := t.TempDir()
	devGen := `resource "jamfpro_computer_extension_attribute" "restrictions" {
  name              = "Restrictions"
  popup_menu_choices = ["Unassigned"]
}
`
	_ = os.WriteFile(filepath.Join(devDir, "generated.tf"), []byte(devGen), 0644)

	prodDir := t.TempDir()
	prodGen := `resource "jamfpro_computer_extension_attribute" "restrictions" {
  name              = "Restrictions"
  popup_menu_choices = ["Unassigned", "Restricted"]
}
`
	_ = os.WriteFile(filepath.Join(prodDir, "generated.tf"), []byte(prodGen), 0644)

	envResults := map[string]*PerEnvResult{
		"dev":  {EnvName: "dev", Registry: registry.New(), OutputDir: devDir},
		"prod": {EnvName: "prod", Registry: registry.New(), OutputDir: prodDir},
	}
	matches := []MatchedResource{
		{ResourceType: "jamfpro_computer_extension_attribute", Label: "restrictions",
			IDs: map[string]string{"dev": "1", "prod": "2"}, AllEnvs: true},
	}

	diffs, err := DiffResources(dir, envResults, matches, "dev")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, d := range diffs {
		if d.AttrName == "popup_menu_choices" {
			found = true
			if d.VarType != "list(string)" {
				t.Errorf("VarType = %q, want %q", d.VarType, "list(string)")
			}
		}
	}
	if !found {
		t.Error("expected popup_menu_choices diff")
	}
}

func TestApplyDiffs(t *testing.T) {
	dir := t.TempDir()

	resourceTF := `resource "jamfpro_category" "productivity" {
  name     = "Productivity"
  priority = "5"
}
`
	if err := os.WriteFile(filepath.Join(dir, "categories.tf"), []byte(resourceTF), 0644); err != nil {
		t.Fatal(err)
	}

	diffs := []AttrDiff{
		{ResourceType: "jamfpro_category", Label: "productivity", AttrName: "priority", VarName: "category_productivity_priority"},
	}

	if err := applyDiffs(dir, diffs); err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "categories.tf"))
	s := string(content)
	if !strings.Contains(s, "var.category_productivity_priority") {
		t.Errorf("expected var reference, got:\n%s", s)
	}
	if strings.Contains(s, `"5"`) {
		t.Error("expected literal value to be replaced")
	}
}
