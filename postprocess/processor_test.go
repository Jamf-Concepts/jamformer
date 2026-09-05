// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

// parseHCL is a test helper that parses HCL source into an hclwrite.File.
func parseHCL(t *testing.T, src string) *hclwrite.File {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}
	return f
}

// blockBody returns the body of the first resource block in the file.
func blockBody(t *testing.T, f *hclwrite.File) *hclwrite.Body {
	t.Helper()
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			return block.Body()
		}
	}
	t.Fatal("no resource block found")
	return nil
}

func TestIsNullValue(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  null_attr    = null
  string_attr  = "hello"
  number_attr  = 42
  bool_attr    = true
}
`)
	body := blockBody(t, f)

	tests := []struct {
		attr string
		want bool
	}{
		{"null_attr", true},
		{"string_attr", false},
		{"number_attr", false},
		{"bool_attr", false},
	}

	for _, tt := range tests {
		t.Run(tt.attr, func(t *testing.T) {
			attr := body.GetAttribute(tt.attr)
			if attr == nil {
				t.Fatalf("attribute %q not found", tt.attr)
			}
			if got := isNullValue(attr); got != tt.want {
				t.Errorf("isNullValue(%q) = %v, want %v", tt.attr, got, tt.want)
			}
		})
	}
}

func TestExtractStringValue(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  string_id  = "42"
  number_id  = 42
  null_attr  = null
}
`)
	body := blockBody(t, f)

	tests := []struct {
		attr string
		want string
	}{
		{"string_id", "42"},
		{"number_id", "42"},
		{"null_attr", ""},
	}

	for _, tt := range tests {
		t.Run(tt.attr, func(t *testing.T) {
			attr := body.GetAttribute(tt.attr)
			if attr == nil {
				t.Fatalf("attribute %q not found", tt.attr)
			}
			if got := ExtractStringValue(attr); got != tt.want {
				t.Errorf("ExtractStringValue(%q) = %q, want %q", tt.attr, got, tt.want)
			}
		})
	}
}

func TestExtractListValues(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_policy" "test" {
  string_list = ["10", "20", "30"]
  number_list = [10, 20, 30]
  empty_list  = []
}
`)
	body := blockBody(t, f)

	tests := []struct {
		attr string
		want []string
	}{
		{"string_list", []string{"10", "20", "30"}},
		{"number_list", []string{"10", "20", "30"}},
		{"empty_list", nil},
	}

	for _, tt := range tests {
		t.Run(tt.attr, func(t *testing.T) {
			attr := body.GetAttribute(tt.attr)
			if attr == nil {
				t.Fatalf("attribute %q not found", tt.attr)
			}
			got := extractListValues(attr)
			if len(got) != len(tt.want) {
				t.Fatalf("extractListValues(%q) returned %d values, want %d", tt.attr, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractListValues(%q)[%d] = %q, want %q", tt.attr, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRewriteSingleAttribute(t *testing.T) {
	reg := registry.New()
	reg.Register("jamfpro_category", "5", "jamfpro_category.productivity")

	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = "5"
}
`)
	body := blockBody(t, f)

	rule := ReferenceRule{
		ResourceType: "jamfpro_script",
		AttrName:     "category_id",
		TargetTypes:  []string{"jamfpro_category"},
		TargetAttr:   "id",
	}

	rewriteSingleAttribute(body, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected reference rewrite, got:\n%s", result)
	}
}

func TestRewriteSingleAttributeUnresolved(t *testing.T) {
	reg := registry.New()

	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  category_id = "999"
}
`)
	body := blockBody(t, f)

	rule := ReferenceRule{
		ResourceType: "jamfpro_script",
		AttrName:     "category_id",
		TargetTypes:  []string{"jamfpro_category"},
		TargetAttr:   "id",
	}

	rewriteSingleAttribute(body, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "TODO") {
		t.Errorf("expected TODO comment for unresolved reference, got:\n%s", result)
	}
}

func TestRewriteListAttribute(t *testing.T) {
	reg := registry.New()
	reg.Register("jamfpro_smart_computer_group_v2", "10", "jamfpro_smart_computer_group_v2.staff")
	reg.Register("jamfpro_static_computer_group", "20", "jamfpro_static_computer_group.lab_macs")

	f := parseHCL(t, `
resource "jamfpro_policy" "test" {
  computer_group_ids = ["10", "20"]
}
`)
	body := blockBody(t, f)

	rule := ReferenceRule{
		ResourceType: "jamfpro_policy",
		AttrName:     "computer_group_ids",
		TargetTypes:  []string{"jamfpro_smart_computer_group_v2", "jamfpro_static_computer_group"},
		TargetAttr:   "id",
		IsList:       true,
	}

	rewriteListAttribute(body, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.staff.id") {
		t.Errorf("expected smart group reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_computer_group.lab_macs.id") {
		t.Errorf("expected static group reference, got:\n%s", result)
	}
}

func TestRewriteBlockNestedPath(t *testing.T) {
	reg := registry.New()
	reg.Register("jamfpro_script", "42", "jamfpro_script.my_script")

	f := parseHCL(t, `
resource "jamfpro_policy" "test" {
  payloads {
    scripts {
      id = "42"
    }
  }
}
`)
	body := blockBody(t, f)

	rule := ReferenceRule{
		ResourceType: "jamfpro_policy",
		BlockPath:    []string{"payloads", "scripts"},
		AttrName:     "id",
		TargetTypes:  []string{"jamfpro_script"},
		TargetAttr:   "id",
	}

	rewriteBlock(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_script.my_script.id") {
		t.Errorf("expected nested reference rewrite, got:\n%s", result)
	}
}

func TestCategoryIdMinusOneRemoval(t *testing.T) {
	tests := []struct {
		name    string
		hcl     string
		removed bool
	}{
		{
			name: "numeric -1",
			hcl: `
resource "jamfpro_script" "test" {
  category_id = -1
}`,
			removed: true,
		},
		{
			name: "string -1",
			hcl: `
resource "jamfpro_script" "test" {
  category_id = "-1"
}`,
			removed: true,
		},
		{
			name: "valid category id",
			hcl: `
resource "jamfpro_script" "test" {
  category_id = "5"
}`,
			removed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseHCL(t, tt.hcl)
			body := blockBody(t, f)

			if attr := body.GetAttribute("category_id"); attr != nil {
				exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if exprBytes == "-1" || exprBytes == "\"-1\"" {
					body.RemoveAttribute("category_id")
				}
			}

			attr := body.GetAttribute("category_id")
			if tt.removed && attr != nil {
				t.Error("expected category_id to be removed")
			}
			if !tt.removed && attr == nil {
				t.Error("expected category_id to be kept")
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Script", "My Script"},
		{"path/to/script", "path_to_script"},
		{"file:with:colons", "file_with_colons"},
		{"question?mark", "question_mark"},
		{"", "unnamed_script"},
		{"...dots...", "dots"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGuessScriptExtension(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"#!/bin/bash\necho hello", ".sh"},
		{"#!/bin/sh\necho hello", ".sh"},
		{"#!/usr/bin/env python3\nimport os", ".py"},
		{"#!/usr/bin/env ruby\nputs 'hi'", ".rb"},
		{"#!/bin/zsh\necho hello", ".zsh"},
		{"#!/usr/bin/perl\nprint 'hi'", ".pl"},
		{"no shebang here", ".sh"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := guessScriptExtension(tt.content)
			if got != tt.want {
				t.Errorf("guessScriptExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractFullStringValue(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  script_contents = "#!/bin/bash\necho hello"
  name            = "my_script"
}
`)
	body := blockBody(t, f)

	attr := body.GetAttribute("script_contents")
	if attr == nil {
		t.Fatal("script_contents not found")
	}

	got := extractFullStringValue(attr)
	if !strings.Contains(got, "#!/bin/bash") {
		t.Errorf("extractFullStringValue() = %q, expected to contain shebang", got)
	}
}

func TestStripNullAttributes(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name            = "hello"
  category_id     = null
  script_contents = null
}
`)
	body := blockBody(t, f)

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_script": {
				"": {
					"name":            {Required: true},
					"category_id":     {Optional: true},
					"script_contents": {Optional: true, Computed: true},
				},
			},
		},
	}

	stripNullAttributes(body, "jamfpro_script", "", schema)

	// Required attribute should remain
	if body.GetAttribute("name") == nil {
		t.Error("expected required attribute 'name' to remain")
	}
	// Optional null should be removed
	if body.GetAttribute("category_id") != nil {
		t.Error("expected optional null 'category_id' to be removed")
	}
	// Optional+computed null should be removed
	if body.GetAttribute("script_contents") != nil {
		t.Error("expected optional+computed null 'script_contents' to be removed")
	}
}

func TestSiteIdMinusOneRemoval(t *testing.T) {
	tests := []struct {
		name    string
		hcl     string
		removed bool
	}{
		{
			name: "numeric -1",
			hcl: `
resource "jamfpro_policy" "test" {
  site_id = -1
}`,
			removed: true,
		},
		{
			name: "string -1",
			hcl: `
resource "jamfpro_policy" "test" {
  site_id = "-1"
}`,
			removed: true,
		},
		{
			name: "valid site id",
			hcl: `
resource "jamfpro_policy" "test" {
  site_id = "5"
}`,
			removed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseHCL(t, tt.hcl)
			body := blockBody(t, f)

			if attr := body.GetAttribute("site_id"); attr != nil {
				exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if exprBytes == "-1" || exprBytes == "\"-1\"" {
					body.RemoveAttribute("site_id")
				}
			}

			attr := body.GetAttribute("site_id")
			if tt.removed && attr != nil {
				t.Error("expected site_id to be removed")
			}
			if !tt.removed && attr == nil {
				t.Error("expected site_id to be kept")
			}
		})
	}
}

func TestStripNullNestedTypeSingle(t *testing.T) {
	f := parseHCL(t, `
resource "jamfprotect_action_configuration" "test" {
  jamf_protect_cloud_endpoint = {
    collect_alerts     = ["high"]
    collect_logs       = []
    destination_filter = null
  }
  name = "test"
}
`)
	body := blockBody(t, f)

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfprotect_action_configuration": {
				"": {
					"name":                        {Required: true},
					"jamf_protect_cloud_endpoint": {Optional: true, NestingMode: "single"},
				},
				"jamf_protect_cloud_endpoint": {
					"collect_alerts":     {Optional: true},
					"collect_logs":       {Optional: true},
					"destination_filter": {Optional: true, NestingMode: "single"},
				},
			},
		},
	}

	stripNullAttributes(body, "jamfprotect_action_configuration", "", schema)

	result := string(f.Bytes())
	if strings.Contains(result, "destination_filter") {
		t.Errorf("expected optional null 'destination_filter' to be removed from nested object, got:\n%s", result)
	}
	if !strings.Contains(result, "collect_alerts") {
		t.Errorf("expected 'collect_alerts' to remain, got:\n%s", result)
	}
	if !strings.Contains(result, "name") {
		t.Errorf("expected 'name' to remain, got:\n%s", result)
	}
}

func TestStripNullNestedTypeList(t *testing.T) {
	f := parseHCL(t, `
resource "example_resource" "test" {
  name = "test"
  endpoints = [{
    url         = "https://example.com"
    auth_header = null
  }]
}
`)
	body := blockBody(t, f)

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"example_resource": {
				"": {
					"name":      {Required: true},
					"endpoints": {Optional: true, NestingMode: "list"},
				},
				"endpoints": {
					"url":         {Required: true},
					"auth_header": {Optional: true},
				},
			},
		},
	}

	stripNullAttributes(body, "example_resource", "", schema)

	result := string(f.Bytes())
	if strings.Contains(result, "auth_header") {
		t.Errorf("expected optional null 'auth_header' to be removed from list object, got:\n%s", result)
	}
	if !strings.Contains(result, "url") {
		t.Errorf("expected required 'url' to remain, got:\n%s", result)
	}
}

func TestStripNullNestedTypePreservesRequired(t *testing.T) {
	f := parseHCL(t, `
resource "example_resource" "test" {
  config = {
    required_field = null
    optional_field = null
  }
}
`)
	body := blockBody(t, f)

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"example_resource": {
				"": {
					"config": {Optional: true, NestingMode: "single"},
				},
				"config": {
					"required_field": {Required: true},
					"optional_field": {Optional: true},
				},
			},
		},
	}

	stripNullAttributes(body, "example_resource", "", schema)

	result := string(f.Bytes())
	if !strings.Contains(result, "required_field") {
		t.Errorf("expected required null 'required_field' to remain, got:\n%s", result)
	}
	if strings.Contains(result, "optional_field") {
		t.Errorf("expected optional null 'optional_field' to be removed, got:\n%s", result)
	}
}

// TestProfilePostProcessing tests redeploy_on_update and payload_validate injection.
func TestProfilePostProcessing(t *testing.T) {
	f := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name     = "Test Profile"
  payloads = "<plist>content</plist>"
}
`)
	body := blockBody(t, f)

	// Simulate the processor's profile handling
	if attr := body.GetAttribute("redeploy_on_update"); attr == nil || isNullValue(attr) {
		body.SetAttributeValue("redeploy_on_update", cty.StringVal("Newly Assigned"))
	}
	if body.GetAttribute("payload_validate") == nil {
		body.SetAttributeValue("payload_validate", cty.False)
	}

	result := string(f.Bytes())
	if !strings.Contains(result, "Newly Assigned") {
		t.Error("expected redeploy_on_update to be injected")
	}
	if !strings.Contains(result, "payload_validate") {
		t.Errorf("expected payload_validate to be injected, got:\n%s", result)
	}
	if strings.Contains(result, "payload_validate = true") {
		t.Error("expected payload_validate to be false, not true")
	}
}

func TestSetNullToZeroValue_String(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tf")
	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  display_name         = "Test Prestage"
  authentication_prompt = null
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_computer_prestage_enrollment": {
				"": {
					"authentication_prompt": {Required: true, Type: cty.String},
				},
			},
		},
	}

	zero, ok := setNullToZeroValue(filePath, []byte(content), "jamfpro_computer_prestage_enrollment", "test", "authentication_prompt", schema)
	if !ok {
		t.Fatal("expected true")
	}
	if zero != `""` {
		t.Errorf("expected the written value reported back as %q, got %q", `""`, zero)
	}

	result, _ := os.ReadFile(filePath)
	if strings.Contains(string(result), "null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(string(result), `authentication_prompt = ""`) {
		t.Errorf("expected empty string, got:\n%s", result)
	}
}

func TestSetNullToZeroValue_Bool(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tf")
	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  display_name  = "Test Prestage"
  mdm_removable = null
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_computer_prestage_enrollment": {
				"": {
					"mdm_removable": {Required: true, Type: cty.Bool},
				},
			},
		},
	}

	zero, ok := setNullToZeroValue(filePath, []byte(content), "jamfpro_computer_prestage_enrollment", "test", "mdm_removable", schema)
	if !ok {
		t.Fatal("expected true")
	}
	if zero != "false" {
		t.Errorf("expected the written value reported back as \"false\", got %q", zero)
	}

	result, _ := os.ReadFile(filePath)
	if strings.Contains(string(result), "null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(string(result), "mdm_removable = false") {
		t.Errorf("expected false, got:\n%s", result)
	}
}

func TestSetNullToZeroValue_Nested(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tf")
	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  display_name = "Test Prestage"
  account_settings {
    admin_username = null
    hidden_admin   = false
  }
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_computer_prestage_enrollment": {
				"account_settings": {
					"admin_username": {Required: true, Type: cty.String},
				},
			},
		},
	}

	if _, ok := setNullToZeroValue(filePath, []byte(content), "jamfpro_computer_prestage_enrollment", "test", "account_settings.0.admin_username", schema); !ok {
		t.Fatal("expected true")
	}

	result, _ := os.ReadFile(filePath)
	if strings.Contains(string(result), "admin_username = null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(string(result), `admin_username = ""`) {
		t.Errorf("expected empty string, got:\n%s", result)
	}
}

func TestLoadProviderSchema(t *testing.T) {
	schemas := &tfjson.ProviderSchemas{
		FormatVersion: "1.0",
		Schemas: map[string]*tfjson.ProviderSchema{
			"registry.terraform.io/example/test": {
				ResourceSchemas: map[string]*tfjson.Schema{
					"test_resource": {
						Block: &tfjson.SchemaBlock{
							Attributes: map[string]*tfjson.SchemaAttribute{
								"name":     {Required: true},
								"optional": {Optional: true},
								"computed": {Optional: true, Computed: true},
								"nested_single": {
									Optional: true,
									AttributeNestedType: &tfjson.SchemaNestedAttributeType{
										NestingMode: tfjson.SchemaNestingModeSingle,
										Attributes: map[string]*tfjson.SchemaAttribute{
											"inner_req": {Required: true},
											"inner_opt": {Optional: true},
										},
									},
								},
								"nested_list": {
									Optional: true,
									AttributeNestedType: &tfjson.SchemaNestedAttributeType{
										NestingMode: tfjson.SchemaNestingModeList,
										Attributes: map[string]*tfjson.SchemaAttribute{
											"item": {Optional: true},
										},
									},
								},
							},
							NestedBlocks: map[string]*tfjson.SchemaBlockType{
								"settings": {
									NestingMode: tfjson.SchemaNestingModeSingle,
									Block: &tfjson.SchemaBlock{
										Attributes: map[string]*tfjson.SchemaAttribute{
											"enabled": {Optional: true},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ps := LoadProviderSchema(schemas)

	// Top-level attributes
	if !ps.canStripNull("test_resource", "", "optional") {
		t.Error("expected optional attr to be strippable")
	}
	if ps.canStripNull("test_resource", "", "name") {
		t.Error("expected required attr to NOT be strippable")
	}
	if !ps.canStripNull("test_resource", "", "computed") {
		t.Error("expected optional+computed attr to be strippable")
	}

	// Nested type nesting modes
	if nm := ps.nestingMode("test_resource", "", "nested_single"); nm != "single" {
		t.Errorf("expected nested_single nesting mode 'single', got %q", nm)
	}
	if nm := ps.nestingMode("test_resource", "", "nested_list"); nm != "list" {
		t.Errorf("expected nested_list nesting mode 'list', got %q", nm)
	}

	// Nested type child attributes
	if ps.canStripNull("test_resource", "nested_single", "inner_req") {
		t.Error("expected nested required attr to NOT be strippable")
	}
	if !ps.canStripNull("test_resource", "nested_single", "inner_opt") {
		t.Error("expected nested optional attr to be strippable")
	}
	if !ps.canStripNull("test_resource", "nested_list", "item") {
		t.Error("expected nested list item attr to be strippable")
	}

	// Block type child attributes
	if !ps.canStripNull("test_resource", "settings", "enabled") {
		t.Error("expected block child attr to be strippable")
	}
}

func TestExtractTokenVar(t *testing.T) {
	t.Run("replaces attribute with file(var.xxx)", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_device_enrollments" "my_enrollment" {
  name          = "My Enrollment"
  encoded_token = "base64data"
}`)
		body := blockBody(t, f)
		tv := extractTokenVar(body, "encoded_token", "dep_token_path_", "my_enrollment", ".p7m")
		if tv == nil {
			t.Fatal("expected TokenVar, got nil")
		}
		if tv.VarName != "dep_token_path_my_enrollment" {
			t.Errorf("VarName = %q, want %q", tv.VarName, "dep_token_path_my_enrollment")
		}
		if !strings.Contains(tv.Description, ".p7m") {
			t.Errorf("Description should mention extension, got %q", tv.Description)
		}
		if !strings.Contains(tv.Description, "My Enrollment") {
			t.Errorf("Description should mention resource name, got %q", tv.Description)
		}

		// Check the attribute was rewritten
		attr := body.GetAttribute("encoded_token")
		if attr == nil {
			t.Fatal("expected encoded_token attribute")
		}
		raw := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
		if raw != "file(var.dep_token_path_my_enrollment)" {
			t.Errorf("attribute = %q, want file(var.dep_token_path_my_enrollment)", raw)
		}
	})

	t.Run("VPP token", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_volume_purchasing_locations" "neils_vpp" {
  name          = "Neil's VPP"
  service_token = "tokendata"
}`)
		body := blockBody(t, f)
		tv := extractTokenVar(body, "service_token", "vpp_token_path_", "neils_vpp", ".vpptoken")
		if tv == nil {
			t.Fatal("expected TokenVar, got nil")
		}
		if tv.VarName != "vpp_token_path_neils_vpp" {
			t.Errorf("VarName = %q, want %q", tv.VarName, "vpp_token_path_neils_vpp")
		}

		attr := body.GetAttribute("service_token")
		raw := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
		if raw != "file(var.vpp_token_path_neils_vpp)" {
			t.Errorf("attribute = %q, want file(var.vpp_token_path_neils_vpp)", raw)
		}
	})

	t.Run("returns nil when no name attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_device_enrollments" "unnamed" {
  encoded_token = "data"
}`)
		body := blockBody(t, f)
		tv := extractTokenVar(body, "encoded_token", "dep_token_path_", "unnamed", ".p7m")
		if tv != nil {
			t.Errorf("expected nil for missing name, got %+v", tv)
		}
	})

	t.Run("returns nil when name is empty string", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_device_enrollments" "empty" {
  name          = ""
  encoded_token = "data"
}`)
		body := blockBody(t, f)
		tv := extractTokenVar(body, "encoded_token", "dep_token_path_", "empty", ".p7m")
		if tv != nil {
			t.Errorf("expected nil for empty name, got %+v", tv)
		}
	})
}

func TestAppendTokenVars(t *testing.T) {
	t.Run("appends variables to existing file", func(t *testing.T) {
		dir := t.TempDir()
		initial := "variable \"existing\" {\n  type = string\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(initial), 0644); err != nil {
			t.Fatal(err)
		}

		appendTokenVars(dir, []TokenVar{
			{VarName: "dep_token_path_enrollment_one", Description: "Path to token file (.p7m) for \"Enrollment One\""},
		})

		content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(content)
		if !strings.Contains(s, `variable "existing"`) {
			t.Error("expected original content preserved")
		}
		if !strings.Contains(s, `variable "dep_token_path_enrollment_one"`) {
			t.Error("expected token variable appended")
		}
		if !strings.Contains(s, "type        = string") {
			t.Error("expected string type")
		}
	})

	t.Run("creates file if missing", func(t *testing.T) {
		dir := t.TempDir()

		appendTokenVars(dir, []TokenVar{
			{VarName: "vpp_token_path_my_vpp", Description: "Path to token file (.vpptoken) for \"My VPP\""},
		})

		content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `variable "vpp_token_path_my_vpp"`) {
			t.Error("expected token variable in new file")
		}
	})

	t.Run("skips duplicates", func(t *testing.T) {
		dir := t.TempDir()
		initial := "variable \"dep_token_path_dup\" {\n  type = string\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(initial), 0644); err != nil {
			t.Fatal(err)
		}

		appendTokenVars(dir, []TokenVar{
			{VarName: "dep_token_path_dup", Description: "duplicate"},
		})

		content, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
		if count := strings.Count(string(content), "dep_token_path_dup"); count != 1 {
			t.Errorf("expected 1 occurrence, got %d", count)
		}
	})

	t.Run("appends multiple variables", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		appendTokenVars(dir, []TokenVar{
			{VarName: "dep_token_path_one", Description: "first"},
			{VarName: "vpp_token_path_two", Description: "second"},
		})

		content, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
		s := string(content)
		if !strings.Contains(s, `variable "dep_token_path_one"`) {
			t.Error("expected first variable")
		}
		if !strings.Contains(s, `variable "vpp_token_path_two"`) {
			t.Error("expected second variable")
		}
	})
}

func TestExtractScriptContents(t *testing.T) {
	t.Run("bash script with shebang", func(t *testing.T) {
		dir := t.TempDir()
		scriptsDir := filepath.Join(dir, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			t.Fatal(err)
		}

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name            = "My Bash Script"
  script_contents = "#!/bin/bash\necho hello world"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractScriptContents(body, scriptsDir, "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}

		// Check that the script file was created with correct extension
		scriptContent, err := os.ReadFile(filepath.Join(scriptsDir, "My Bash Script.sh"))
		if err != nil {
			t.Fatalf("expected script file to be created: %v", err)
		}
		if !strings.Contains(string(scriptContent), "#!/bin/bash") {
			t.Error("expected script content to contain shebang")
		}
		if !strings.Contains(string(scriptContent), "echo hello world") {
			t.Error("expected script content to contain body")
		}

		// Check that the attribute was replaced with file() reference
		attr := body.GetAttribute("script_contents")
		if attr == nil {
			t.Fatal("expected script_contents attribute to exist")
		}
		raw := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
		if !strings.Contains(raw, `file("${path.module}/support_files/scripts/My Bash Script.sh")`) {
			t.Errorf("expected file() reference, got: %s", raw)
		}
	})

	t.Run("python script", func(t *testing.T) {
		dir := t.TempDir()
		scriptsDir := filepath.Join(dir, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			t.Fatal(err)
		}

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name            = "Python Script"
  script_contents = "#!/usr/bin/env python3\nimport os\nprint('hello')"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractScriptContents(body, scriptsDir, "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}

		// Check .py extension
		if _, err := os.Stat(filepath.Join(scriptsDir, "Python Script.py")); err != nil {
			t.Errorf("expected .py extension, file not found: %v", err)
		}
	})

	t.Run("no script_contents attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name = "No Script"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractScriptContents(body, "/tmp", "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}
		// Should be a no-op, no error
	})

	t.Run("no name attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  script_contents = "#!/bin/bash\necho test"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractScriptContents(body, "/tmp", "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}
		// Should be a no-op with no name attr
	})

	t.Run("filename collision handling", func(t *testing.T) {
		dir := t.TempDir()
		scriptsDir := filepath.Join(dir, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			t.Fatal(err)
		}

		fileNames := make(map[string]int)

		// First script
		f1 := parseHCL(t, `
resource "jamfpro_script" "test1" {
  name            = "Duplicate Name"
  script_contents = "#!/bin/bash\necho first"
}
`)
		body1 := blockBody(t, f1)
		if err := extractScriptContents(body1, scriptsDir, "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}

		// Second script with same name
		f2 := parseHCL(t, `
resource "jamfpro_script" "test2" {
  name            = "Duplicate Name"
  script_contents = "#!/bin/bash\necho second"
}
`)
		body2 := blockBody(t, f2)
		if err := extractScriptContents(body2, scriptsDir, "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}

		// First file should be the original name
		if _, err := os.Stat(filepath.Join(scriptsDir, "Duplicate Name.sh")); err != nil {
			t.Errorf("expected first script file: %v", err)
		}

		// Second file should have a _2 suffix
		if _, err := os.Stat(filepath.Join(scriptsDir, "Duplicate Name_2.sh")); err != nil {
			t.Errorf("expected second script file with _2 suffix: %v", err)
		}
	})

	t.Run("script name already has extension", func(t *testing.T) {
		dir := t.TempDir()
		scriptsDir := filepath.Join(dir, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			t.Fatal(err)
		}

		f := parseHCL(t, `
resource "jamfpro_script" "test" {
  name            = "my_script.sh"
  script_contents = "#!/bin/bash\necho test"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractScriptContents(body, scriptsDir, "support_files/scripts", fileNames); err != nil {
			t.Fatal(err)
		}

		// Should not double-append .sh
		if _, err := os.Stat(filepath.Join(scriptsDir, "my_script.sh")); err != nil {
			t.Errorf("expected file without double extension: %v", err)
		}
		if _, err := os.Stat(filepath.Join(scriptsDir, "my_script.sh.sh")); err == nil {
			t.Error("should not have double .sh extension")
		}
	})
}

func TestExtractProfilePayloads(t *testing.T) {
	t.Run("extracts mobileconfig payload", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "macos_configuration_profiles")
		if err := os.MkdirAll(profilesDir, 0755); err != nil {
			t.Fatal(err)
		}

		f := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name     = "WiFi Profile"
  payloads = "<?xml version=\"1.0\"?><plist><dict></dict></plist>"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractProfilePayloads(body, profilesDir, "support_files", "macos_configuration_profiles", fileNames); err != nil {
			t.Fatal(err)
		}

		// Check file was created with .mobileconfig extension
		content, err := os.ReadFile(filepath.Join(profilesDir, "WiFi Profile.mobileconfig"))
		if err != nil {
			t.Fatalf("expected profile file to be created: %v", err)
		}
		if !strings.Contains(string(content), "<plist>") {
			t.Error("expected profile content to contain plist data")
		}

		// Check attribute was replaced with file() reference
		attr := body.GetAttribute("payloads")
		if attr == nil {
			t.Fatal("expected payloads attribute to exist")
		}
		raw := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
		if !strings.Contains(raw, `file("${path.module}/support_files/macos_configuration_profiles/WiFi Profile.mobileconfig")`) {
			t.Errorf("expected file() reference, got: %s", raw)
		}
	})

	t.Run("no payloads attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "No Payload"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractProfilePayloads(body, "/tmp", "support_files", "macos_configuration_profiles", fileNames); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no name attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  payloads = "<plist></plist>"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractProfilePayloads(body, "/tmp", "support_files", "macos_configuration_profiles", fileNames); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("filename collision handling", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "macos_configuration_profiles")
		if err := os.MkdirAll(profilesDir, 0755); err != nil {
			t.Fatal(err)
		}

		fileNames := make(map[string]int)

		f1 := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test1" {
  name     = "Same Profile"
  payloads = "<plist>first</plist>"
}
`)
		body1 := blockBody(t, f1)
		if err := extractProfilePayloads(body1, profilesDir, "support_files", "macos_configuration_profiles", fileNames); err != nil {
			t.Fatal(err)
		}

		f2 := parseHCL(t, `
resource "jamfpro_macos_configuration_profile_plist" "test2" {
  name     = "Same Profile"
  payloads = "<plist>second</plist>"
}
`)
		body2 := blockBody(t, f2)
		if err := extractProfilePayloads(body2, profilesDir, "support_files", "macos_configuration_profiles", fileNames); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(profilesDir, "Same Profile.mobileconfig")); err != nil {
			t.Errorf("expected first profile: %v", err)
		}
		if _, err := os.Stat(filepath.Join(profilesDir, "Same Profile_2.mobileconfig")); err != nil {
			t.Errorf("expected second profile with _2 suffix: %v", err)
		}
	})
}

func TestExtractAppConfiguration(t *testing.T) {
	t.Run("extracts preferences to XML file", func(t *testing.T) {
		dir := t.TempDir()
		configsDir := filepath.Join(dir, "app_configurations")
		if err := os.MkdirAll(configsDir, 0755); err != nil {
			t.Fatal(err)
		}

		f := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "My MDM App"

  app_configuration {
    preferences = "<dict><key>ServerURL</key><string>https://example.com</string></dict>"
  }
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractAppConfiguration(body, configsDir, "support_files", fileNames); err != nil {
			t.Fatal(err)
		}

		// Check file was created with .xml extension
		content, err := os.ReadFile(filepath.Join(configsDir, "My MDM App.xml"))
		if err != nil {
			t.Fatalf("expected app config file to be created: %v", err)
		}
		if !strings.Contains(string(content), "ServerURL") {
			t.Error("expected app config content to contain XML data")
		}

		// Check attribute in app_configuration block was replaced
		appConfigBlock := body.FirstMatchingBlock("app_configuration", nil)
		if appConfigBlock == nil {
			t.Fatal("expected app_configuration block to exist")
		}
		attr := appConfigBlock.Body().GetAttribute("preferences")
		if attr == nil {
			t.Fatal("expected preferences attribute to exist")
		}
		raw := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
		if !strings.Contains(raw, `file("${path.module}/support_files/app_configurations/My MDM App.xml")`) {
			t.Errorf("expected file() reference, got: %s", raw)
		}
	})

	t.Run("no app_configuration block", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "Simple App"
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractAppConfiguration(body, "/tmp", "support_files", fileNames); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("app_configuration without preferences", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "No Prefs App"

  app_configuration {
  }
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractAppConfiguration(body, "/tmp", "support_files", fileNames); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("no name attribute", func(t *testing.T) {
		f := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test" {
  app_configuration {
    preferences = "<dict></dict>"
  }
}
`)
		body := blockBody(t, f)
		fileNames := make(map[string]int)

		if err := extractAppConfiguration(body, "/tmp", "support_files", fileNames); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("filename collision handling", func(t *testing.T) {
		dir := t.TempDir()
		configsDir := filepath.Join(dir, "app_configurations")
		if err := os.MkdirAll(configsDir, 0755); err != nil {
			t.Fatal(err)
		}

		fileNames := make(map[string]int)

		f1 := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test1" {
  name = "Same App"
  app_configuration {
    preferences = "<dict>first</dict>"
  }
}
`)
		body1 := blockBody(t, f1)
		if err := extractAppConfiguration(body1, configsDir, "support_files", fileNames); err != nil {
			t.Fatal(err)
		}

		f2 := parseHCL(t, `
resource "jamfpro_mobile_device_application" "test2" {
  name = "Same App"
  app_configuration {
    preferences = "<dict>second</dict>"
  }
}
`)
		body2 := blockBody(t, f2)
		if err := extractAppConfiguration(body2, configsDir, "support_files", fileNames); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(configsDir, "Same App.xml")); err != nil {
			t.Errorf("expected first config file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(configsDir, "Same App_2.xml")); err != nil {
			t.Errorf("expected second config file with _2 suffix: %v", err)
		}
	})
}

// provider_name is a closed enum today, and the skip that depends on it is the
// only thing standing between an unhydrated cloud identity provider and a
// project that will not plan — the credentials block it is missing carries
// secrets no read exposes, so nothing downstream can repair it. A third IdP
// type must therefore be reported as unknown rather than reading as an
// all-clear, which is what a silent fall-through gave.
func TestMissingCloudIdPBlock(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		wantMissing string
		wantUnknown bool
	}{
		{
			name: "entra with credentials is complete",
			src: `resource "r" "l" {
  provider_name = "ENTRA_ID"
  entra_id = {
    client_id = "abc"
  }
}`,
		},
		{
			name: "entra without credentials is skipped",
			src: `resource "r" "l" {
  provider_name = "ENTRA_ID"
}`,
			wantMissing: "entra_id",
		},
		{
			name: "google with a null block is skipped",
			src: `resource "r" "l" {
  provider_name = "GOOGLE"
  google        = null
}`,
			wantMissing: "google",
		},
		{
			name: "a third IdP type is reported unknown",
			src: `resource "r" "l" {
  provider_name = "OKTA"
}`,
			wantUnknown: true,
		},
		{
			name: "a renamed discriminator is reported unknown",
			src: `resource "r" "l" {
  provider_name = "ENTRA"
  entra_id = {
    client_id = "abc"
  }
}`,
			wantUnknown: true,
		},
		{
			// No discriminator to read: provider_name is Required, so the
			// validation auto-fix reports that in its own terms.
			name: "absent provider_name is not this check's business",
			src: `resource "r" "l" {
  display_name = "x"
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := parseHCL(t, tt.src).Body().Blocks()[0].Body()
			missing, unknown := missingCloudIdPBlock(body)
			if missing != tt.wantMissing {
				t.Errorf("missing = %q, want %q", missing, tt.wantMissing)
			}
			if unknown != tt.wantUnknown {
				t.Errorf("unknown = %v, want %v", unknown, tt.wantUnknown)
			}
		})
	}
}
