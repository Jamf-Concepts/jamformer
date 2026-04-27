// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestCategorySplitTypes(t *testing.T) {
	rules := []ReferenceRule{
		{ResourceType: "jamfpro_script", AttrName: "category_id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
		{ResourceType: "jamfpro_policy", AttrName: "category_id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
		// Nested category ref — should NOT be included
		{ResourceType: "jamfpro_policy", BlockPath: []string{"self_service", "self_service_categories"}, AttrName: "id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
		// Non-category rule
		{ResourceType: "jamfpro_policy", BlockPath: []string{"scope"}, AttrName: "computer_group_ids", TargetTypes: []string{"jamfpro_smart_computer_group_v2"}, TargetAttr: "id", IsList: true},
	}

	got := categorySplitTypes(rules)

	if !got["jamfpro_script"] {
		t.Error("expected jamfpro_script to be a category split type")
	}
	if !got["jamfpro_policy"] {
		t.Error("expected jamfpro_policy to be a category split type")
	}
	if len(got) != 2 {
		t.Errorf("expected 2 types, got %d", len(got))
	}
}

func TestCategorySplitTypesEmpty(t *testing.T) {
	got := categorySplitTypes(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 types for nil rules, got %d", len(got))
	}
}

func TestExtractCategoryLabel(t *testing.T) {
	t.Run("rewritten reference", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = jamfpro_category.productivity.id
}`)
		body := blockBody(t, f)

		got := extractCategoryLabel(body, nil)
		if got != "productivity" {
			t.Errorf("got %q, want %q", got, "productivity")
		}
	})

	t.Run("raw ID with registry lookup", func(t *testing.T) {
		reg := registry.New()
		reg.Register("jamfpro_category", "5", "jamfpro_category.testing")

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = "5"
}`)
		body := blockBody(t, f)

		got := extractCategoryLabel(body, reg)
		if got != "testing" {
			t.Errorf("got %q, want %q", got, "testing")
		}
	})

	t.Run("no category_id attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name = "hello"
}`)
		body := blockBody(t, f)

		got := extractCategoryLabel(body, nil)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("unresolved reference no registry match", func(t *testing.T) {
		reg := registry.New()

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = "999"
}`)
		body := blockBody(t, f)

		got := extractCategoryLabel(body, reg)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("numeric ID with registry lookup", func(t *testing.T) {
		reg := registry.New()
		reg.Register("jamfpro_category", "42", "jamfpro_category.production")

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = 42
}`)
		body := blockBody(t, f)

		got := extractCategoryLabel(body, reg)
		if got != "production" {
			t.Errorf("got %q, want %q", got, "production")
		}
	})
}

func TestSplitImportFileByCategory(t *testing.T) {
	t.Run("splits into per-category files", func(t *testing.T) {
		dir := t.TempDir()

		importContent := `import {
  to = jamfpro_policy.install_chrome
  id = "42"
}

import {
  to = jamfpro_policy.update_office
  id = "43"
}

import {
  to = jamfpro_policy.no_category
  id = "44"
}
`
		if err := os.WriteFile(filepath.Join(dir, "policies_import.tf"), []byte(importContent), 0644); err != nil {
			t.Fatal(err)
		}

		labelCategories := map[string]string{
			"install_chrome": "production",
			"update_office":  "testing",
		}

		if err := splitImportFileByCategory(dir, "policies.tf", labelCategories); err != nil {
			t.Fatal(err)
		}

		// Original should be deleted
		if _, err := os.Stat(filepath.Join(dir, "policies_import.tf")); !os.IsNotExist(err) {
			t.Error("expected original import file to be deleted")
		}

		prodContent, err := os.ReadFile(filepath.Join(dir, "policies_production_import.tf"))
		if err != nil {
			t.Fatalf("expected policies_production_import.tf: %v", err)
		}
		if !strings.Contains(string(prodContent), "install_chrome") {
			t.Error("expected install_chrome in production import file")
		}
		if strings.Contains(string(prodContent), "update_office") {
			t.Error("production import file should not contain update_office")
		}

		testContent, err := os.ReadFile(filepath.Join(dir, "policies_testing_import.tf"))
		if err != nil {
			t.Fatalf("expected policies_testing_import.tf: %v", err)
		}
		if !strings.Contains(string(testContent), "update_office") {
			t.Error("expected update_office in testing import file")
		}

		uncatContent, err := os.ReadFile(filepath.Join(dir, "policies_uncategorised_import.tf"))
		if err != nil {
			t.Fatalf("expected policies_uncategorised_import.tf: %v", err)
		}
		if !strings.Contains(string(uncatContent), "no_category") {
			t.Error("expected no_category in uncategorised import file")
		}
	})

	t.Run("missing import file returns nil", func(t *testing.T) {
		dir := t.TempDir()

		if err := splitImportFileByCategory(dir, "policies.tf", map[string]string{}); err != nil {
			t.Fatalf("expected no error for missing file, got: %v", err)
		}
	})

	t.Run("all same category", func(t *testing.T) {
		dir := t.TempDir()

		importContent := `import {
  to = jamfpro_script.a
  id = "1"
}

import {
  to = jamfpro_script.b
  id = "2"
}
`
		if err := os.WriteFile(filepath.Join(dir, "scripts_import.tf"), []byte(importContent), 0644); err != nil {
			t.Fatal(err)
		}

		labelCategories := map[string]string{
			"a": "production",
			"b": "production",
		}

		if err := splitImportFileByCategory(dir, "scripts.tf", labelCategories); err != nil {
			t.Fatal(err)
		}

		// Should produce only one category file
		content, err := os.ReadFile(filepath.Join(dir, "scripts_production_import.tf"))
		if err != nil {
			t.Fatalf("expected scripts_production_import.tf: %v", err)
		}
		if !strings.Contains(string(content), "jamfpro_script.a") || !strings.Contains(string(content), "jamfpro_script.b") {
			t.Error("expected both scripts in production import file")
		}

		// Original should be deleted
		if _, err := os.Stat(filepath.Join(dir, "scripts_import.tf")); !os.IsNotExist(err) {
			t.Error("expected original to be deleted")
		}
	})
}

func TestProcessCategorySplitting(t *testing.T) {
	dir := t.TempDir()

	generatedContent := `resource "jamfpro_script" "script_prod" {
  name        = "Production Script"
  category_id = "5"
}

resource "jamfpro_script" "script_test" {
  name        = "Testing Script"
  category_id = "10"
}

resource "jamfpro_script" "script_none" {
  name = "No Category Script"
}

resource "jamfpro_category" "production" {
  name = "Production"
}

resource "jamfpro_category" "testing" {
  name = "Testing"
}
`
	if err := os.WriteFile(filepath.Join(dir, "generated.tf"), []byte(generatedContent), 0644); err != nil {
		t.Fatal(err)
	}

	importContent := `import {
  to = jamfpro_script.script_prod
  id = "1"
}

import {
  to = jamfpro_script.script_test
  id = "2"
}

import {
  to = jamfpro_script.script_none
  id = "3"
}
`
	if err := os.WriteFile(filepath.Join(dir, "scripts_import.tf"), []byte(importContent), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	reg.Register("jamfpro_category", "5", "jamfpro_category.production")
	reg.Register("jamfpro_category", "10", "jamfpro_category.testing")

	typeMap := map[string]string{
		"jamfpro_script":   "scripts.tf",
		"jamfpro_category": "categories.tf",
	}

	rules := []ReferenceRule{
		{ResourceType: "jamfpro_script", AttrName: "category_id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
	}

	Quiet = true
	defer func() { Quiet = false }()

	if err := Process(dir, filepath.Join(dir, "generated.tf"), reg, &ProcessOptions{
		TypeToFileMap:   typeMap,
		Rules:           rules,
		SplitByCategory: true,
	}); err != nil {
		t.Fatal(err)
	}

	// scripts.tf should not exist or be empty (all routed to per-category files)
	if data, err := os.ReadFile(filepath.Join(dir, "scripts.tf")); err == nil {
		if len(strings.TrimSpace(string(data))) > 0 {
			t.Errorf("expected scripts.tf to be empty or absent, got:\n%s", data)
		}
	}

	// Per-category resource files
	prodContent, err := os.ReadFile(filepath.Join(dir, "scripts_production.tf"))
	if err != nil {
		t.Fatalf("expected scripts_production.tf: %v", err)
	}
	if !strings.Contains(string(prodContent), "script_prod") {
		t.Error("expected script_prod in production file")
	}
	if !strings.Contains(string(prodContent), "jamfpro_category.production.id") {
		t.Error("expected rewritten category reference in production file")
	}

	testContent, err := os.ReadFile(filepath.Join(dir, "scripts_testing.tf"))
	if err != nil {
		t.Fatalf("expected scripts_testing.tf: %v", err)
	}
	if !strings.Contains(string(testContent), "script_test") {
		t.Error("expected script_test in testing file")
	}

	uncatContent, err := os.ReadFile(filepath.Join(dir, "scripts_uncategorised.tf"))
	if err != nil {
		t.Fatalf("expected scripts_uncategorised.tf: %v", err)
	}
	if !strings.Contains(string(uncatContent), "script_none") {
		t.Error("expected script_none in uncategorised file")
	}

	// Categories file should exist (not a split type)
	catContent, err := os.ReadFile(filepath.Join(dir, "categories.tf"))
	if err != nil {
		t.Fatalf("expected categories.tf: %v", err)
	}
	if !strings.Contains(string(catContent), "jamfpro_category") {
		t.Error("expected categories in categories.tf")
	}

	// Import files should be split
	if _, err := os.Stat(filepath.Join(dir, "scripts_import.tf")); !os.IsNotExist(err) {
		t.Error("expected original scripts_import.tf to be deleted")
	}

	prodImport, err := os.ReadFile(filepath.Join(dir, "scripts_production_import.tf"))
	if err != nil {
		t.Fatalf("expected scripts_production_import.tf: %v", err)
	}
	if !strings.Contains(string(prodImport), "script_prod") {
		t.Error("expected script_prod in production import file")
	}

	testImport, err := os.ReadFile(filepath.Join(dir, "scripts_testing_import.tf"))
	if err != nil {
		t.Fatalf("expected scripts_testing_import.tf: %v", err)
	}
	if !strings.Contains(string(testImport), "script_test") {
		t.Error("expected script_test in testing import file")
	}

	uncatImport, err := os.ReadFile(filepath.Join(dir, "scripts_uncategorised_import.tf"))
	if err != nil {
		t.Fatalf("expected scripts_uncategorised_import.tf: %v", err)
	}
	if !strings.Contains(string(uncatImport), "script_none") {
		t.Error("expected script_none in uncategorised import file")
	}
}

func TestProcessCategorySplittingMinusOne(t *testing.T) {
	dir := t.TempDir()

	generatedContent := `resource "jamfpro_script" "categorised" {
  name        = "Has Category"
  category_id = "5"
}

resource "jamfpro_script" "uncategorised" {
  name        = "No Category"
  category_id = -1
}
`
	if err := os.WriteFile(filepath.Join(dir, "generated.tf"), []byte(generatedContent), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	reg.Register("jamfpro_category", "5", "jamfpro_category.production")

	typeMap := map[string]string{
		"jamfpro_script": "scripts.tf",
	}

	rules := []ReferenceRule{
		{ResourceType: "jamfpro_script", AttrName: "category_id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
	}

	Quiet = true
	defer func() { Quiet = false }()

	if err := Process(dir, filepath.Join(dir, "generated.tf"), reg, &ProcessOptions{
		TypeToFileMap:   typeMap,
		Rules:           rules,
		SplitByCategory: true,
	}); err != nil {
		t.Fatal(err)
	}

	prodContent, err := os.ReadFile(filepath.Join(dir, "scripts_production.tf"))
	if err != nil {
		t.Fatalf("expected scripts_production.tf: %v", err)
	}
	if !strings.Contains(string(prodContent), "categorised") {
		t.Error("expected categorised script in production file")
	}

	uncatContent, err := os.ReadFile(filepath.Join(dir, "scripts_uncategorised.tf"))
	if err != nil {
		t.Fatalf("expected scripts_uncategorised.tf: %v", err)
	}
	if !strings.Contains(string(uncatContent), "uncategorised") {
		t.Error("expected uncategorised script in uncategorised file")
	}

	// category_id = -1 should be removed from the uncategorised block
	if strings.Contains(string(uncatContent), "category_id") {
		t.Error("expected category_id = -1 to be removed")
	}
}

func TestProcessNoCategorySplitWhenNoCategoriesDiscovered(t *testing.T) {
	dir := t.TempDir()

	generatedContent := `resource "jamfpro_script" "my_script" {
  name        = "My Script"
  category_id = "5"
}
`
	if err := os.WriteFile(filepath.Join(dir, "generated.tf"), []byte(generatedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Empty registry — no categories discovered
	reg := registry.New()

	typeMap := map[string]string{
		"jamfpro_script": "scripts.tf",
	}

	rules := []ReferenceRule{
		{ResourceType: "jamfpro_script", AttrName: "category_id", TargetTypes: []string{"jamfpro_category"}, TargetAttr: "id"},
	}

	Quiet = true
	defer func() { Quiet = false }()

	if err := Process(dir, filepath.Join(dir, "generated.tf"), reg, &ProcessOptions{
		TypeToFileMap:   typeMap,
		Rules:           rules,
		SplitByCategory: true, // enabled, but no categories in registry → no split
	}); err != nil {
		t.Fatal(err)
	}

	// Should fall back to monolithic file — no per-category split
	content, err := os.ReadFile(filepath.Join(dir, "scripts.tf"))
	if err != nil {
		t.Fatalf("expected scripts.tf: %v", err)
	}
	if !strings.Contains(string(content), "my_script") {
		t.Error("expected my_script in scripts.tf")
	}

	// No per-category files should exist
	matches, _ := filepath.Glob(filepath.Join(dir, "scripts_*.tf"))
	for _, m := range matches {
		t.Errorf("unexpected per-category file: %s", filepath.Base(m))
	}
}

func TestProcessNoCategorySplitWhenNoRules(t *testing.T) {
	dir := t.TempDir()

	generatedContent := `resource "jamfpro_building" "office" {
  name = "Head Office"
}
`
	if err := os.WriteFile(filepath.Join(dir, "generated.tf"), []byte(generatedContent), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()

	typeMap := map[string]string{
		"jamfpro_building": "buildings.tf",
	}

	Quiet = true
	defer func() { Quiet = false }()

	if err := Process(dir, filepath.Join(dir, "generated.tf"), reg, &ProcessOptions{
		TypeToFileMap: typeMap,
	}); err != nil {
		t.Fatal(err)
	}

	// buildings.tf should exist as normal
	content, err := os.ReadFile(filepath.Join(dir, "buildings.tf"))
	if err != nil {
		t.Fatalf("expected buildings.tf: %v", err)
	}
	if !strings.Contains(string(content), "Head Office") {
		t.Error("expected building in buildings.tf")
	}
}
