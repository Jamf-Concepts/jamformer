// Copyright 2026, Jamf Software LLC

package compact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func TestConsolidateFile_Categories(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "categories.tf")

	input := `resource "jamfpro_category" "productivity" {
  name = "Productivity"
}

resource "jamfpro_category" "security" {
  name = "Security"
}

resource "jamfpro_category" "compliance" {
  name = "Compliance"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "categories")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected consolidation result, got nil")
	}

	content := string(result.content)

	if !strings.Contains(content, "locals {") {
		t.Error("expected locals block")
	}
	if !strings.Contains(content, "categories = {") {
		t.Error("expected categories key in locals")
	}

	for _, label := range []string{"compliance", "productivity", "security"} {
		if !strings.Contains(content, label+" =") {
			t.Errorf("expected label %q in locals map", label)
		}
	}

	if !strings.Contains(content, `for_each = local.categories`) {
		t.Error("expected for_each = local.categories")
	}
	if !strings.Contains(content, `each.value.name`) {
		t.Error("expected each.value.name")
	}

	// Resource label should be "all"
	if !strings.Contains(content, `resource "jamfpro_category" "all"`) {
		t.Error("expected resource label to be 'all'")
	}

	// Check rewrites use "all"
	if result.rewrites["jamfpro_category.productivity"] != `jamfpro_category.all["productivity"]` {
		t.Errorf("unexpected rewrite: %v", result.rewrites["jamfpro_category.productivity"])
	}

	if result.resourceType != "jamfpro_category" {
		t.Errorf("expected resourceType jamfpro_category, got %s", result.resourceType)
	}

	if len(result.labels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(result.labels))
	}
}

func TestConsolidateFile_MultipleAttributes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "dock_items.tf")

	input := `resource "jamfpro_dock_item" "safari" {
  name = "Safari"
  path = "/Applications/Safari.app"
  type = "App"
}

resource "jamfpro_dock_item" "slack" {
  name = "Slack"
  path = "/Applications/Slack.app"
  type = "App"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "dock_items")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected consolidation result, got nil")
	}

	content := string(result.content)

	if !strings.Contains(content, "dock_items = {") {
		t.Error("expected dock_items key in locals")
	}
	// name and path vary — should be each.value references
	if !strings.Contains(content, "each.value.name") {
		t.Error("expected each.value.name")
	}
	if !strings.Contains(content, "each.value.path") {
		t.Error("expected each.value.path")
	}
	// type is "App" on both — should be a shared literal, not each.value
	if strings.Contains(content, "each.value.type") {
		t.Error("type is shared across all instances, should be literal not each.value")
	}
	if !strings.Contains(content, `"App"`) {
		t.Error("expected shared type with value \"App\" as literal on resource block")
	}
}

func TestConsolidateFile_SingleResource(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "sites.tf")

	input := `resource "jamfpro_site" "main" {
  name = "Main Site"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "sites")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for single resource")
	}
}

func TestConsolidateFile_NonUniform(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "buildings.tf")

	input := `resource "jamfpro_building" "hq" {
  name             = "HQ"
  street_address_1 = "123 Main St"
}

resource "jamfpro_building" "remote" {
  name = "Remote"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "buildings")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for non-uniform resources")
	}
}

func TestConsolidateFile_NestedBlocks(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "policies.tf")

	input := `resource "jamfpro_policy" "a" {
  name = "Policy A"

  scope {
    all_computers = true
  }
}

resource "jamfpro_policy" "b" {
  name = "Policy B"

  scope {
    all_computers = false
  }
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "policies")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for resources with nested blocks")
	}
}

func TestConsolidateFile_NestedAttributes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "action_configurations.tf")

	// Resources with nested attributes (structural types) should not be consolidated
	input := `resource "jamfprotect_action_configuration" "default" {
  name = "Default"
  alert_data_collection = {
    binary_included_data_attributes = ["Sha1", "Sha256"]
  }
}

resource "jamfprotect_action_configuration" "custom" {
  name = "Custom"
  alert_data_collection = {
    binary_included_data_attributes = ["Sha1"]
  }
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "action_configurations")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for resources with nested attributes (structural types)")
	}
}

func TestConsolidateFile_WithLifecycleBlock(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "icons.tf")

	input := `resource "jamfpro_icon" "chrome" {
  icon_file_web_source = "https://cdn.example.com/chrome.png"

  lifecycle {
    ignore_changes = [icon_file_web_source]
  }
}

resource "jamfpro_icon" "firefox" {
  icon_file_web_source = "https://cdn.example.com/firefox.png"

  lifecycle {
    ignore_changes = [icon_file_web_source]
  }
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "icons")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected consolidation result, got nil (lifecycle blocks should be allowed)")
	}

	content := string(result.content)
	if !strings.Contains(content, "lifecycle") {
		t.Error("expected lifecycle block in output")
	}
}

func TestConsolidateFile_WithReferences(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "scripts.tf")

	input := `resource "jamfpro_script" "install_chrome" {
  category_id = jamfpro_category.productivity.id
  name        = "Install Chrome"
}

resource "jamfpro_script" "install_slack" {
  category_id = jamfpro_category.productivity.id
  name        = "Install Slack"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "scripts")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected consolidation result, got nil")
	}

	content := string(result.content)
	if !strings.Contains(content, "jamfpro_category.productivity.id") {
		t.Error("expected reference expression preserved in locals map")
	}
}

func TestConsolidateFile_SharedAttributes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "volume_purchasing_locations.tf")

	input := `resource "jamfpro_volume_purchasing_locations" "abm" {
  auto_register_managed_users               = true
  automatically_populate_purchased_content  = true
  name                                      = "Apple Business Manager"
  send_notification_when_no_longer_assigned = false
  service_token                             = file("abm.vpptoken")
}

resource "jamfpro_volume_purchasing_locations" "abm_2" {
  auto_register_managed_users               = true
  automatically_populate_purchased_content  = true
  name                                      = "Apple Business Manager 2"
  send_notification_when_no_longer_assigned = false
  service_token                             = file("abm2.vpptoken")
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "volume_purchasing_locations")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected consolidation result, got nil")
	}

	content := string(result.content)

	// Shared attributes should be literals on the resource block, not each.value
	if strings.Contains(content, "each.value.auto_register_managed_users") {
		t.Error("shared attr auto_register_managed_users should be literal, not each.value")
	}
	if strings.Contains(content, "each.value.automatically_populate_purchased_content") {
		t.Error("shared attr automatically_populate_purchased_content should be literal, not each.value")
	}
	if strings.Contains(content, "each.value.send_notification_when_no_longer_assigned") {
		t.Error("shared attr send_notification_when_no_longer_assigned should be literal, not each.value")
	}

	// Varying attributes should use each.value
	if !strings.Contains(content, "each.value.name") {
		t.Error("expected each.value.name for varying attribute")
	}
	if !strings.Contains(content, "each.value.service_token") {
		t.Error("expected each.value.service_token for varying attribute")
	}
}

func TestConsolidateFile_AllIdentical(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "things.tf")

	// All attributes identical across all resources — no point in for_each
	input := `resource "jamfpro_category" "a" {
  name = "Same"
}

resource "jamfpro_category" "b" {
  name = "Same"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "things")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result when all attributes are identical (no varying attrs)")
	}
}

func TestConsolidateFile_MultipleResourceTypes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "mixed.tf")

	// File with two different resource types should not be consolidated
	input := `resource "jamfpro_category" "a" {
  name = "A"
}

resource "jamfpro_department" "b" {
  name = "B"
}
`
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := consolidateFile(filePath, "mixed")
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil result for file with multiple resource types")
	}
}

func TestRewriteReferences(t *testing.T) {
	dir := t.TempDir()

	input := `resource "jamfpro_policy" "chrome" {
  category_id = jamfpro_category.productivity.id
  name        = "Install Chrome"
}
`
	filePath := filepath.Join(dir, "policies.tf")
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	rewrites := map[string]string{
		"jamfpro_category.productivity": `jamfpro_category.all["productivity"]`,
	}

	if err := rewriteReferences(dir, rewrites); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, `jamfpro_category.all["productivity"].id`) {
		t.Errorf("expected rewritten reference, got:\n%s", content)
	}
}

func TestRewriteImportFiles(t *testing.T) {
	dir := t.TempDir()

	input := `import {
  to = jamfpro_category.productivity
  id = "abc-123"
}

import {
  to = jamfpro_category.security
  id = "def-456"
}
`
	filePath := filepath.Join(dir, "categories_import.tf")
	if err := os.WriteFile(filePath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	rewrites := map[string]string{
		"jamfpro_category.productivity": `jamfpro_category.all["productivity"]`,
		"jamfpro_category.security":     `jamfpro_category.all["security"]`,
	}

	if err := rewriteImportFiles(dir, rewrites); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, `jamfpro_category.all["productivity"]`) {
		t.Errorf("expected rewritten import, got:\n%s", content)
	}
	if !strings.Contains(content, `jamfpro_category.all["security"]`) {
		t.Errorf("expected rewritten import, got:\n%s", content)
	}
}

func TestRun_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	Quiet = true
	defer func() { Quiet = false }()

	categories := `resource "jamfpro_category" "productivity" {
  name = "Productivity"
}

resource "jamfpro_category" "security" {
  name = "Security"
}
`
	if err := os.WriteFile(filepath.Join(dir, "categories.tf"), []byte(categories), 0644); err != nil {
		t.Fatal(err)
	}

	policies := `resource "jamfpro_policy" "chrome" {
  category_id = jamfpro_category.productivity.id
  name        = "Install Chrome"
}
`
	if err := os.WriteFile(filepath.Join(dir, "policies.tf"), []byte(policies), 0644); err != nil {
		t.Fatal(err)
	}

	imports := `import {
  to = jamfpro_category.productivity
  id = "abc-123"
}

import {
  to = jamfpro_category.security
  id = "def-456"
}
`
	if err := os.WriteFile(filepath.Join(dir, "categories_import.tf"), []byte(imports), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Run(dir, nil); err != nil {
		t.Fatal(err)
	}

	// Check categories.tf was consolidated
	catData, err := os.ReadFile(filepath.Join(dir, "categories.tf"))
	if err != nil {
		t.Fatal(err)
	}
	catContent := string(catData)
	if !strings.Contains(catContent, "for_each") {
		t.Error("expected for_each in consolidated categories")
	}
	if !strings.Contains(catContent, `"all"`) {
		t.Error("expected resource label 'all'")
	}

	// Check policies.tf references were rewritten
	polData, err := os.ReadFile(filepath.Join(dir, "policies.tf"))
	if err != nil {
		t.Fatal(err)
	}
	polContent := string(polData)
	if !strings.Contains(polContent, `jamfpro_category.all["productivity"].id`) {
		t.Errorf("expected rewritten reference in policies, got:\n%s", polContent)
	}

	// Check import file was rewritten
	importData, err := os.ReadFile(filepath.Join(dir, "categories_import.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(importData), `jamfpro_category.all["productivity"]`) {
		t.Errorf("expected rewritten import, got:\n%s", string(importData))
	}
}

func TestRun_IncludeFilter(t *testing.T) {
	dir := t.TempDir()
	Quiet = true
	defer func() { Quiet = false }()

	// Write two eligible files
	for _, name := range []string{"categories", "departments"} {
		content := fmt.Sprintf(`resource "jamfpro_%[1]s" "a" {
  name = "A"
}

resource "jamfpro_%[1]s" "b" {
  name = "B"
}
`, strings.TrimSuffix(name, "s")) // crude singular for test type names
		// Actually, let's just use a valid type name pattern
		if err := os.WriteFile(filepath.Join(dir, name+".tf"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Only compact categories
	opts := &Options{Include: map[string]bool{"categories": true}}
	if err := Run(dir, opts); err != nil {
		t.Fatal(err)
	}

	// categories should be compacted
	catData, _ := os.ReadFile(filepath.Join(dir, "categories.tf"))
	if !strings.Contains(string(catData), "for_each") {
		t.Error("expected categories to be compacted")
	}

	// departments should NOT be compacted
	deptData, _ := os.ReadFile(filepath.Join(dir, "departments.tf"))
	if strings.Contains(string(deptData), "for_each") {
		t.Error("expected departments to NOT be compacted (not in include list)")
	}
}

func TestRun_ExcludeFilter(t *testing.T) {
	dir := t.TempDir()
	Quiet = true
	defer func() { Quiet = false }()

	for _, name := range []string{"categories", "departments"} {
		content := fmt.Sprintf(`resource "jamfpro_%[1]s" "a" {
  name = "A"
}

resource "jamfpro_%[1]s" "b" {
  name = "B"
}
`, strings.TrimSuffix(name, "s"))
		if err := os.WriteFile(filepath.Join(dir, name+".tf"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Exclude departments
	opts := &Options{Exclude: map[string]bool{"departments": true}}
	if err := Run(dir, opts); err != nil {
		t.Fatal(err)
	}

	catData, _ := os.ReadFile(filepath.Join(dir, "categories.tf"))
	if !strings.Contains(string(catData), "for_each") {
		t.Error("expected categories to be compacted")
	}

	deptData, _ := os.ReadFile(filepath.Join(dir, "departments.tf"))
	if strings.Contains(string(deptData), "for_each") {
		t.Error("expected departments to NOT be compacted (excluded)")
	}
}

func TestRun_SkipsInfraFiles(t *testing.T) {
	dir := t.TempDir()
	Quiet = true
	defer func() { Quiet = false }()

	// provider.tf should never be touched
	provider := `terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "provider.tf"), []byte(provider), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Run(dir, nil); err != nil {
		t.Fatal(err)
	}

	// provider.tf should be unchanged
	data, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if string(data) != provider {
		t.Error("provider.tf was modified")
	}
}

func TestRun_SkipsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	Quiet = true
	defer func() { Quiet = false }()

	if err := Run(dir, nil); err != nil {
		t.Fatal(err)
	}
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		filename string
		skip     bool
	}{
		{"provider.tf", true},
		{"variables.tf", true},
		{"terraform.tfvars", true},
		{"moved.tf", true},
		{"locals.tf", true},
		{"categories_import.tf", true},
		{"categories.tf", false},
		{"policies.tf", false},
	}

	for _, tt := range tests {
		if got := shouldSkipFile(tt.filename); got != tt.skip {
			t.Errorf("shouldSkipFile(%q) = %v, want %v", tt.filename, got, tt.skip)
		}
	}
}

func TestReplaceAddress(t *testing.T) {
	tests := []struct {
		name    string
		content string
		old     string
		new     string
		want    string
	}{
		{
			name:    "simple replace",
			content: `scope = jamfpro_category.productivity.id`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `scope = jamfpro_category.all["productivity"].id`,
		},
		{
			name:    "does not match prefix of longer identifier",
			content: `scope = jamfpro_category.productivity_2.id`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `scope = jamfpro_category.productivity_2.id`,
		},
		{
			name:    "matches when followed by dot",
			content: `jamfpro_category.productivity.id`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `jamfpro_category.all["productivity"].id`,
		},
		{
			name:    "matches when followed by newline",
			content: "jamfpro_category.productivity\n",
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    "jamfpro_category.all[\"productivity\"]\n",
		},
		{
			name:    "matches at end of string",
			content: `to = jamfpro_category.productivity`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `to = jamfpro_category.all["productivity"]`,
		},
		{
			name:    "multiple occurrences",
			content: `a = jamfpro_category.productivity.id, b = jamfpro_category.productivity.name`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `a = jamfpro_category.all["productivity"].id, b = jamfpro_category.all["productivity"].name`,
		},
		{
			name:    "no match",
			content: `scope = jamfpro_category.security.id`,
			old:     "jamfpro_category.productivity",
			new:     `jamfpro_category.all["productivity"]`,
			want:    `scope = jamfpro_category.security.id`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceAddress(tt.content, tt.old, tt.new)
			if got != tt.want {
				t.Errorf("replaceAddress() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestContainsObjectLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple string value", `"hello"`, false},
		{"number value", `42`, false},
		{"object literal", `{ key = "value" }`, true},
		{"reference", `var.something`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, diags := hclwrite.ParseConfig(fmt.Appendf(nil, `x = %s`, tt.input), "", hcl.Pos{})
			if diags.HasErrors() {
				t.Fatalf("parse error: %s", diags.Error())
			}
			attr := f.Body().GetAttribute("x")
			got := containsObjectLiteral(attr.Expr().BuildTokens(nil))
			if got != tt.want {
				t.Errorf("containsObjectLiteral() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEqualStringSlices(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both empty", nil, nil, true},
		{"equal", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a", "b"}, []string{"a", "c"}, false},
		{"one nil one empty", nil, []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := equalStringSlices(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("equalStringSlices(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
