// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func TestFindMatchingDelimiter(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		openPos int
		want    int
	}{
		{
			name:    "simple braces",
			src:     `{foo = "bar"}`,
			openPos: 0,
			want:    12,
		},
		{
			name:    "simple brackets",
			src:     `["a", "b"]`,
			openPos: 0,
			want:    9,
		},
		{
			name:    "nested braces",
			src:     `{outer = {inner = "val"}}`,
			openPos: 0,
			want:    24,
		},
		{
			name:    "inner nested brace",
			src:     `{outer = {inner = "val"}}`,
			openPos: 9,
			want:    23,
		},
		{
			name:    "nested brackets",
			src:     `[[1, 2], [3, 4]]`,
			openPos: 0,
			want:    15,
		},
		{
			name:    "braces in string are ignored",
			src:     `{foo = "a}b"}`,
			openPos: 0,
			want:    12,
		},
		{
			name:    "escaped quote in string",
			src:     `{foo = "a\"b"}`,
			openPos: 0,
			want:    13,
		},
		{
			name:    "unclosed brace returns -1",
			src:     `{foo = "bar"`,
			openPos: 0,
			want:    -1,
		},
		{
			name:    "unclosed bracket returns -1",
			src:     `["a", "b"`,
			openPos: 0,
			want:    -1,
		},
		{
			name:    "openPos beyond src length returns -1",
			src:     `{}`,
			openPos: 5,
			want:    -1,
		},
		{
			name:    "non-delimiter at openPos returns -1",
			src:     `abc`,
			openPos: 0,
			want:    -1,
		},
		{
			name:    "empty braces",
			src:     `{}`,
			openPos: 0,
			want:    1,
		},
		{
			name:    "empty brackets",
			src:     `[]`,
			openPos: 0,
			want:    1,
		},
		{
			name:    "deeply nested",
			src:     `{a = {b = {c = "d"}}}`,
			openPos: 0,
			want:    20,
		},
		{
			name:    "mixed delimiters - brace containing bracket",
			src:     `{list = ["a", "b"]}`,
			openPos: 0,
			want:    18,
		},
		{
			name:    "mixed delimiters - bracket containing brace",
			src:     `[{foo = "bar"}, {baz = "qux"}]`,
			openPos: 0,
			want:    29,
		},
		{
			name:    "string with escaped backslash before quote",
			src:     `{foo = "a\\"} `,
			openPos: 0,
			want:    12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMatchingDelimiter([]byte(tt.src), tt.openPos)
			if got != tt.want {
				t.Errorf("findMatchingDelimiter(%q, %d) = %d, want %d", tt.src, tt.openPos, got, tt.want)
			}
		})
	}
}

func TestStripNullAttributes_NestedBlock(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    all_computers  = true
    building_ids   = null
    department_ids = null
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	var body *hclwrite.Body
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			body = block.Body()
			break
		}
	}
	if body == nil {
		t.Fatal("no resource block found")
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_policy": {
				"": {
					"name": {Required: true},
				},
				"scope": {
					"all_computers":  {Required: true},
					"building_ids":   {Optional: true},
					"department_ids": {Optional: true},
				},
			},
		},
	}

	stripNullAttributes(body, "jamfpro_policy", "", schema)

	result := string(f.Bytes())

	// Required "name" at top level should remain
	if body.GetAttribute("name") == nil {
		t.Error("expected required attribute 'name' to remain")
	}

	// Check scope block contents
	var scopeBody *hclwrite.Body
	for _, block := range body.Blocks() {
		if block.Type() == "scope" {
			scopeBody = block.Body()
			break
		}
	}
	if scopeBody == nil {
		t.Fatal("scope block not found after stripping")
	}

	if scopeBody.GetAttribute("all_computers") == nil {
		t.Error("expected required 'all_computers' to remain in scope")
	}
	if scopeBody.GetAttribute("building_ids") != nil {
		t.Errorf("expected optional null 'building_ids' to be removed from scope, got:\n%s", result)
	}
	if scopeBody.GetAttribute("department_ids") != nil {
		t.Errorf("expected optional null 'department_ids' to be removed from scope, got:\n%s", result)
	}
}

func TestStripNullAttributes_NilSchema(t *testing.T) {
	src := `
resource "jamfpro_script" "test" {
  name     = "hello"
  priority = null
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	var body *hclwrite.Body
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			body = block.Body()
			break
		}
	}

	// With a nil schema, canStripNull returns false, so nothing should be removed
	stripNullAttributes(body, "jamfpro_script", "", nil)

	if body.GetAttribute("priority") == nil {
		t.Error("expected 'priority' to remain when schema is nil")
	}
}

func TestStripNullAttributes_UnknownResourceType(t *testing.T) {
	src := `
resource "unknown_resource" "test" {
  name     = "hello"
  optional = null
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	var body *hclwrite.Body
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			body = block.Body()
			break
		}
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"other_resource": {
				"": {
					"optional": {Optional: true},
				},
			},
		},
	}

	// Resource type not in schema, nothing should be stripped
	stripNullAttributes(body, "unknown_resource", "", schema)

	if body.GetAttribute("optional") == nil {
		t.Error("expected 'optional' to remain when resource type is not in schema")
	}
}

func TestStripNullAttributes_NonNullNotStripped(t *testing.T) {
	src := `
resource "jamfpro_script" "test" {
  name        = "hello"
  category_id = "5"
  priority    = "AFTER"
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	var body *hclwrite.Body
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			body = block.Body()
			break
		}
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_script": {
				"": {
					"name":        {Required: true},
					"category_id": {Optional: true},
					"priority":    {Optional: true},
				},
			},
		},
	}

	stripNullAttributes(body, "jamfpro_script", "", schema)

	// Non-null attributes should remain regardless of optional/required
	if body.GetAttribute("name") == nil {
		t.Error("expected 'name' to remain")
	}
	if body.GetAttribute("category_id") == nil {
		t.Error("expected 'category_id' to remain (non-null)")
	}
	if body.GetAttribute("priority") == nil {
		t.Error("expected 'priority' to remain (non-null)")
	}
}

func TestCanStripNull(t *testing.T) {
	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_script": {
				"": {
					"name":        {Required: true},
					"category_id": {Optional: true},
					"notes":       {Optional: true, Computed: true},
				},
				"nested": {
					"inner_req": {Required: true},
					"inner_opt": {Optional: true},
				},
			},
		},
	}

	tests := []struct {
		name         string
		resourceType string
		blockPath    string
		attrName     string
		want         bool
	}{
		{"required top-level", "jamfpro_script", "", "name", false},
		{"optional top-level", "jamfpro_script", "", "category_id", true},
		{"optional+computed top-level", "jamfpro_script", "", "notes", true},
		{"required nested", "jamfpro_script", "nested", "inner_req", false},
		{"optional nested", "jamfpro_script", "nested", "inner_opt", true},
		{"unknown resource type", "unknown_type", "", "foo", false},
		{"unknown block path", "jamfpro_script", "unknown", "foo", false},
		{"unknown attr name", "jamfpro_script", "", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schema.canStripNull(tt.resourceType, tt.blockPath, tt.attrName)
			if got != tt.want {
				t.Errorf("canStripNull(%q, %q, %q) = %v, want %v",
					tt.resourceType, tt.blockPath, tt.attrName, got, tt.want)
			}
		})
	}

	// Test nil schema
	t.Run("nil schema", func(t *testing.T) {
		var nilSchema *ProviderSchema
		if nilSchema.canStripNull("any", "", "any") {
			t.Error("expected nil schema to return false")
		}
	})
}

func TestIsSensitive(t *testing.T) {
	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_smtp_server": {
				"": {
					"host": {Required: true},
				},
				"basic_auth_credentials": {
					"password": {Required: true, Sensitive: true},
					"username": {Required: true},
				},
			},
		},
	}

	tests := []struct {
		name         string
		resourceType string
		blockPath    string
		attrName     string
		want         bool
	}{
		{"sensitive attribute", "jamfpro_smtp_server", "basic_auth_credentials", "password", true},
		{"non-sensitive attribute", "jamfpro_smtp_server", "basic_auth_credentials", "username", false},
		{"non-sensitive top-level", "jamfpro_smtp_server", "", "host", false},
		{"unknown resource type", "unknown", "", "foo", false},
		{"unknown block path", "jamfpro_smtp_server", "unknown", "foo", false},
		{"unknown attr", "jamfpro_smtp_server", "basic_auth_credentials", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schema.isSensitive(tt.resourceType, tt.blockPath, tt.attrName)
			if got != tt.want {
				t.Errorf("isSensitive(%q, %q, %q) = %v, want %v",
					tt.resourceType, tt.blockPath, tt.attrName, got, tt.want)
			}
		})
	}

	t.Run("nil schema", func(t *testing.T) {
		var nilSchema *ProviderSchema
		if nilSchema.isSensitive("any", "", "any") {
			t.Error("expected nil schema to return false")
		}
	})
}

func TestZeroValue(t *testing.T) {
	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"test_resource": {
				"": {
					"str_attr":  {Type: cty.String},
					"bool_attr": {Type: cty.Bool},
					"num_attr":  {Type: cty.Number},
				},
			},
		},
	}

	t.Run("string zero value", func(t *testing.T) {
		val := schema.zeroValue("test_resource", "", "str_attr")
		if val.AsString() != "" {
			t.Errorf("expected empty string, got %q", val.AsString())
		}
	})

	t.Run("bool zero value", func(t *testing.T) {
		val := schema.zeroValue("test_resource", "", "bool_attr")
		if val.True() {
			t.Error("expected false")
		}
	})

	t.Run("number zero value", func(t *testing.T) {
		val := schema.zeroValue("test_resource", "", "num_attr")
		bf := val.AsBigFloat()
		f, _ := bf.Float64()
		if f != 0 {
			t.Errorf("expected 0, got %v", f)
		}
	})

	t.Run("unknown attr falls back to empty string", func(t *testing.T) {
		val := schema.zeroValue("test_resource", "", "unknown_attr")
		if val.AsString() != "" {
			t.Errorf("expected empty string fallback, got %q", val.AsString())
		}
	})

	t.Run("nil schema falls back to empty string", func(t *testing.T) {
		var nilSchema *ProviderSchema
		val := nilSchema.zeroValue("any", "", "any")
		if val.AsString() != "" {
			t.Errorf("expected empty string fallback for nil schema, got %q", val.AsString())
		}
	})
}

func TestNestingMode(t *testing.T) {
	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"test_resource": {
				"": {
					"single_nested": {NestingMode: "single"},
					"list_nested":   {NestingMode: "list"},
					"plain_attr":    {},
				},
			},
		},
	}

	tests := []struct {
		name     string
		attrName string
		want     string
	}{
		{"single nesting", "single_nested", "single"},
		{"list nesting", "list_nested", "list"},
		{"plain attr", "plain_attr", ""},
		{"unknown attr", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schema.nestingMode("test_resource", "", tt.attrName)
			if got != tt.want {
				t.Errorf("nestingMode(%q) = %q, want %q", tt.attrName, got, tt.want)
			}
		})
	}

	t.Run("nil schema", func(t *testing.T) {
		var nilSchema *ProviderSchema
		if got := nilSchema.nestingMode("any", "", "any"); got != "" {
			t.Errorf("expected empty string for nil schema, got %q", got)
		}
	})

	t.Run("unknown resource type", func(t *testing.T) {
		if got := schema.nestingMode("unknown_type", "", "foo"); got != "" {
			t.Errorf("expected empty string for unknown resource type, got %q", got)
		}
	})

	t.Run("unknown block path", func(t *testing.T) {
		if got := schema.nestingMode("test_resource", "unknown", "foo"); got != "" {
			t.Errorf("expected empty string for unknown block path, got %q", got)
		}
	})
}
