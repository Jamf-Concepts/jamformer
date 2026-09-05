// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"testing"
)

// The Security Cloud UEM Connect read is the case this strategy exists for: the
// provider returns uem_server_url for a platform_tenant connector, whose
// required companion oauth is absent, so its own validators reject the config
// it generated. The orphan comes out.
func TestAlsoRequiresRemovesOrphanedCompanion(t *testing.T) {
	src := `resource "jamfplatform_security_cloud_uem_connect" "jamf_pro" {
  oauth = null
  platform_tenant = {
    tenant_id = "8e4d6d65-d941-45e1-a5aa-090a1793fc99"
  }
  uem_server_url = "https://example.jamfcloud.com"
  uem_vendor     = "JAMF_PRO"
}
`
	path := filepath.Join(t.TempDir(), "uem.tf")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	fix := classifyFix("Invalid Attribute Combination",
		"These attributes must be configured together: [oauth,uem_server_url]", path, 6, nil)
	if fix == nil {
		t.Fatal("expected a fix for the AlsoRequires diagnostic")
	}
	if fix.attrName != "uem_server_url" {
		t.Fatalf("want uem_server_url removed, got %q", fix.attrName)
	}
	if !removeAttributeFromFile(fix.filePath, fix.line, fix.attrName) {
		t.Fatal("removal failed")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if contains(out, "uem_server_url") {
		t.Errorf("uem_server_url survived:\n%s", out)
	}
	// The authoritative auth mode must be untouched — it is the real
	// configuration, and the orphan was the echo.
	if !contains(out, "platform_tenant") || !contains(out, "tenant_id") {
		t.Errorf("platform_tenant was damaged:\n%s", out)
	}
}

// With two of the named attributes set and one missing, there is no way to tell
// which the object wants. Guessing would delete real configuration, so the
// diagnostic is left for a human.
func TestAlsoRequiresDeclinesWhenAmbiguous(t *testing.T) {
	src := `resource "x" "y" {
  a = "one"
  b = "two"
  c = null
}
`
	path := filepath.Join(t.TempDir(), "x.tf")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if fix := classifyFix("Invalid Attribute Combination",
		"These attributes must be configured together: [a,b,c]", path, 2, nil); fix != nil {
		t.Errorf("expected no fix when the orphan is ambiguous, got %q", fix.attrName)
	}
}

// Nothing missing means nothing to fix: the attributes are already configured
// together and the diagnostic came from somewhere else.
func TestAlsoRequiresDeclinesWhenAllPresent(t *testing.T) {
	src := `resource "x" "y" {
  a = "one"
  b = "two"
}
`
	path := filepath.Join(t.TempDir(), "x.tf")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if fix := classifyFix("Invalid Attribute Combination",
		"These attributes must be configured together: [a,b]", path, 2, nil); fix != nil {
		t.Errorf("expected no fix when every attribute is present, got %q", fix.attrName)
	}
}

// A patch software title comes back with version_packages = {}, which the
// provider's own SizeAtLeast(1) validator rejects. The empty collection holds
// nothing, so it comes out.
func TestEmptyCollectionIsRemoved(t *testing.T) {
	src := `resource "jamfplatform_pro_patch_software_title" "microsoft_defender" {
  name             = "Microsoft Defender"
  version_packages = {}
}
`
	path := filepath.Join(t.TempDir(), "patch.tf")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	fix := classifyFix("Invalid Attribute Value",
		"Attribute version_packages map must contain at least 1 elements, got: 0", path, 3, nil)
	if fix == nil {
		t.Fatal("expected a fix for the empty-collection diagnostic")
	}
	if fix.attrName != "version_packages" {
		t.Fatalf("want version_packages, got %q", fix.attrName)
	}
	if !removeAttributeFromFile(fix.filePath, fix.line, fix.attrName) {
		t.Fatal("removal failed")
	}
	got, _ := os.ReadFile(path)
	if contains(string(got), "version_packages") {
		t.Errorf("version_packages survived:\n%s", got)
	}
	// hclwrite re-aligns the block after a removal, so the assertion is on the
	// value surviving rather than on its spacing.
	if !contains(string(got), `"Microsoft Defender"`) {
		t.Errorf("sibling attribute was damaged:\n%s", got)
	}
}

// A non-empty collection that trips some other validator is not this
// strategy's business — "got: 3" must not match.
func TestNonEmptyCollectionIsNotRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.tf")
	if err := os.WriteFile(path, []byte("resource \"x\" \"y\" {\n  a = 1\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if fix := classifyFix("Invalid Attribute Value",
		"Attribute a set must contain at least 5 elements, got: 3", path, 2, nil); fix != nil {
		t.Errorf("expected no fix for a non-empty collection, got %q", fix.attrName)
	}
}

func TestSplitAttrListTakesTheLeafName(t *testing.T) {
	got := splitAttrList("oauth, group_membership_mapping.mappings , uem_server_url")
	want := []string{"oauth", "mappings", "uem_server_url"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
