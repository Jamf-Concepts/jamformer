// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// setupPrefixRegistry registers the two device-group flavours keyed by their
// Jamf Pro ID, mirroring what platform.RegisterDeviceGroupSubtypes does.
func setupPrefixRegistry() *registry.Registry {
	reg := registry.New()
	reg.Register("jamfplatform_device_group#computer", "12", "jamfplatform_device_group.executives")
	reg.Register("jamfplatform_device_group#mobile", "7", "jamfplatform_device_group.field_staff")
	return reg
}

func prefixRule() ReferenceRule {
	return ReferenceRule{
		ResourceType: "jamfplatform_security_cloud_uem_connect",
		AttrPath:     []string{"group_membership_mapping"},
		AttrName:     "mappings",
		ElementAttr:  "uem_group_id",
		PrefixedIDs:  true,
		DiscriminatorMap: map[string]string{
			"computer_": "jamfplatform_device_group#computer",
			"mobile_":   "jamfplatform_device_group#mobile",
		},
		TargetAttr: "jamf_pro_id",
	}
}

func TestPrefixedIDsRewritesElements(t *testing.T) {
	src := `resource "jamfplatform_security_cloud_uem_connect" "jamf_pro" {
  group_membership_mapping = {
    enabled = true
    mappings = [
      {
        uem_group_id            = "computer_12"
        security_cloud_group_id = "aaa"
      },
      {
        uem_group_id            = "mobile_7"
        security_cloud_group_id = "bbb"
      },
    ]
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "t.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("parse: %v", diags)
	}
	body := f.Body().Blocks()[0].Body()
	rule := prefixRule()
	RewriteBlockForTest(body, rule.BlockPath, rule, setupPrefixRegistry())
	got := string(f.Bytes())
	for _, want := range []string{
		`"computer_${jamfplatform_device_group.executives.jamf_pro_id}"`,
		`"mobile_${jamfplatform_device_group.field_staff.jamf_pro_id}"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	// The sibling ID field must be untouched by this rule.
	if !strings.Contains(got, `security_cloud_group_id = "aaa"`) {
		t.Errorf("sibling attribute was modified:\n%s", got)
	}
}

func TestPrefixedIDsLeavesUnknownPrefixAndUnresolvedTail(t *testing.T) {
	src := `resource "jamfplatform_security_cloud_uem_connect" "jamf_pro" {
  group_membership_mapping = {
    mappings = [
      { uem_group_id = "school_3" },
      { uem_group_id = "computer_999" },
      { uem_group_id = "computer_" },
    ]
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "t.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("parse: %v", diags)
	}
	body := f.Body().Blocks()[0].Body()
	rule := prefixRule()
	RewriteBlockForTest(body, rule.BlockPath, rule, setupPrefixRegistry())
	got := string(f.Bytes())
	// No prefix match, no registry entry, and an empty tail: all left verbatim,
	// and none of them marked as a broken reference.
	for _, want := range []string{`"school_3"`, `"computer_999"`, `"computer_"`} {
		if !strings.Contains(got, want) {
			t.Errorf("value %s was altered:\n%s", want, got)
		}
	}
	if strings.Contains(got, "TODO") {
		t.Errorf("a prefixed value must not be marked unresolved:\n%s", got)
	}
}
