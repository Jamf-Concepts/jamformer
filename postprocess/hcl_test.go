// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func attrFromSrc(t *testing.T, src string) *hclwrite.Attribute {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(src), "t.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}
	for _, b := range f.Body().Blocks() {
		if a := b.Body().GetAttribute("x"); a != nil {
			return a
		}
	}
	t.Fatal("attr x not found")
	return nil
}

func TestExtractStringValue_NegativeNumberKeepsSign(t *testing.T) {
	cases := map[string]string{
		"resource \"r\" \"l\" {\n  x = -1\n}\n":        "-1",
		"resource \"r\" \"l\" {\n  x = -2\n}\n":        "-2",
		"resource \"r\" \"l\" {\n  x = 42\n}\n":        "42",
		"resource \"r\" \"l\" {\n  x = \"31\"\n}\n":    "31",
		"resource \"r\" \"l\" {\n  x = -1 # note\n}\n": "-1",
	}
	for src, want := range cases {
		if got := ExtractStringValue(attrFromSrc(t, src)); got != want {
			t.Errorf("ExtractStringValue(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestRewriteSingleAttribute_NegativeSentinelLeftAlone(t *testing.T) {
	// site_id = -1 (no site) must be recognised as a sentinel and left untouched,
	// not mangled into a positive "1".
	f, diags := hclwrite.ParseConfig([]byte(`resource "jamfplatform_pro_account" "a" {
  site_id = -1
}
`), "t.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	block := f.Body().Blocks()[0]
	rule := ReferenceRule{ResourceType: "jamfplatform_pro_account", AttrName: "site_id", TargetTypes: []string{"jamfplatform_pro_site"}, TargetAttr: "id"}
	RewriteBlockForTest(block.Body(), nil, rule, registry.New())
	got := string(block.Body().GetAttribute("site_id").Expr().BuildTokens(nil).Bytes())
	if got != " -1" && got != "-1" {
		t.Errorf("site_id should stay -1, got %q", got)
	}
}
