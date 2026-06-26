// Copyright 2026, Jamf Software LLC

package postprocess_test

import (
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
)

// objAttrRegistry builds a registry mimicking the jamfplatform provider, where
// device groups are registered under synthetic computer/mobile subtypes keyed by
// their Jamf Pro numeric ID, and other pro_* objects key on their classic id.
func objAttrRegistry() *registry.Registry {
	reg := registry.New()
	reg.Register("jamfplatform_pro_category", "5", "jamfplatform_pro_category.productivity")
	reg.Register("jamfplatform_pro_building", "3", "jamfplatform_pro_building.hq")
	reg.Register("jamfplatform_pro_script", "44", "jamfplatform_pro_script.rotate_admin")
	reg.Register("jamfplatform_device_group#computer", "12", "jamfplatform_device_group.engineering_computer")
	reg.Register("jamfplatform_device_group#mobile", "12", "jamfplatform_device_group.field_ipads_mobile")
	return reg
}

// TestObjAttrNestedSingle rewrites a single ID nested one level into an object
// attribute: general = { category_id = "5" }.
func TestObjAttrNestedSingle(t *testing.T) {
	reg := objAttrRegistry()
	f := parseHCLRef(t, `
resource "jamfplatform_pro_script" "test" {
  general = {
    name        = "test"
    category_id = "5"
  }
}
`)
	body := refBlockBody(t, f)
	rule := postprocess.ReferenceRule{
		ResourceType: "jamfplatform_pro_script",
		AttrPath:     []string{"general"},
		AttrName:     "category_id",
		TargetTypes:  []string{"jamfplatform_pro_category"},
		TargetAttr:   "id",
	}
	postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfplatform_pro_category.productivity.id") {
		t.Errorf("expected nested category reference, got:\n%s", result)
	}
}

// TestObjAttrPolymorphicElement rewrites a list-of-objects element ID whose
// target type is chosen per-element by a sibling discriminator field, mirroring
// macOS onboarding's onboarding_items[].entity_id keyed on self_service_entity_type.
func TestObjAttrPolymorphicElement(t *testing.T) {
	reg := registry.New()
	reg.Register("jamfplatform_pro_policy", "4", "jamfplatform_pro_policy.install_chrome")
	reg.Register("jamfplatform_pro_macos_configuration_profile", "11", "jamfplatform_pro_macos_configuration_profile.filevault")
	reg.Register("jamfplatform_pro_mac_app_store_app", "87", "jamfplatform_pro_mac_app_store_app.slack")

	f := parseHCLRef(t, `
resource "jamfplatform_pro_macos_onboarding" "singleton" {
  onboarding_items = [
    {
      entity_id                = "4"
      self_service_entity_type = "OS_X_POLICY"
    },
    {
      entity_id                = "11"
      self_service_entity_type = "OS_X_CONFIG_PROFILE"
    },
    {
      entity_id                = "87"
      self_service_entity_type = "OS_X_MAC_APP"
    },
  ]
}
`)
	body := refBlockBody(t, f)
	rule := postprocess.ReferenceRule{
		ResourceType:      "jamfplatform_pro_macos_onboarding",
		AttrName:          "onboarding_items",
		ElementAttr:       "entity_id",
		DiscriminatorAttr: "self_service_entity_type",
		DiscriminatorMap: map[string]string{
			"OS_X_POLICY":         "jamfplatform_pro_policy",
			"OS_X_CONFIG_PROFILE": "jamfplatform_pro_macos_configuration_profile",
			"OS_X_MAC_APP":        "jamfplatform_pro_mac_app_store_app",
		},
		TargetAttr: "id",
	}
	postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	for _, want := range []string{
		"jamfplatform_pro_policy.install_chrome.id",
		"jamfplatform_pro_macos_configuration_profile.filevault.id",
		"jamfplatform_pro_mac_app_store_app.slack.id",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in rewritten output, got:\n%s", want, result)
		}
	}
}

// TestObjAttrNestedScopeList rewrites a list of IDs two levels deep in object
// attributes, resolving to the computer device-group subtype:
// scope = { targets = { computer_group_ids = ["12"] } }.
func TestObjAttrNestedScopeList(t *testing.T) {
	reg := objAttrRegistry()
	f := parseHCLRef(t, `
resource "jamfplatform_pro_policy" "test" {
  scope = {
    targets = {
      computer_group_ids = ["12"]
      building_ids       = ["3"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	rules := []postprocess.ReferenceRule{
		{
			ResourceType: "jamfplatform_pro_policy",
			AttrPath:     []string{"scope", "targets"},
			AttrName:     "computer_group_ids",
			TargetTypes:  []string{"jamfplatform_device_group#computer"},
			TargetAttr:   "jamf_pro_id",
			IsList:       true,
		},
		{
			ResourceType: "jamfplatform_pro_policy",
			AttrPath:     []string{"scope", "targets"},
			AttrName:     "building_ids",
			TargetTypes:  []string{"jamfplatform_pro_building"},
			TargetAttr:   "id",
			IsList:       true,
		},
	}
	for _, rule := range rules {
		postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)
	}

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfplatform_device_group.engineering_computer.jamf_pro_id") {
		t.Errorf("expected computer device-group jamf_pro_id reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfplatform_pro_building.hq.id") {
		t.Errorf("expected building reference, got:\n%s", result)
	}
	// Must NOT resolve to the mobile subtype.
	if strings.Contains(result, "field_ipads_mobile") {
		t.Errorf("computer_group_ids must not resolve to a mobile device group, got:\n%s", result)
	}
}

// TestObjAttrListOfObjects rewrites the id field on each element of a
// list-of-objects nested in an object attribute:
// scripts = { scripts = [ { id = "44" } ] }.
func TestObjAttrListOfObjects(t *testing.T) {
	reg := objAttrRegistry()
	f := parseHCLRef(t, `
resource "jamfplatform_pro_policy" "test" {
  scripts = {
    scripts = [
      {
        id       = "44"
        priority = "After"
      },
    ]
  }
}
`)
	body := refBlockBody(t, f)
	rule := postprocess.ReferenceRule{
		ResourceType: "jamfplatform_pro_policy",
		AttrPath:     []string{"scripts"},
		AttrName:     "scripts",
		ElementAttr:  "id",
		TargetTypes:  []string{"jamfplatform_pro_script"},
		TargetAttr:   "id",
	}
	postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfplatform_pro_script.rotate_admin.id") {
		t.Errorf("expected script reference on list element, got:\n%s", result)
	}
	if !strings.Contains(result, `priority = "After"`) {
		t.Errorf("expected sibling attribute preserved, got:\n%s", result)
	}
}

// TestObjAttrMissingAttrNoOp verifies that a rule whose path is absent on the
// instance is a no-op (does not panic or corrupt output).
func TestObjAttrMissingAttrNoOp(t *testing.T) {
	reg := objAttrRegistry()
	f := parseHCLRef(t, `
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = "no scope here"
  }
}
`)
	body := refBlockBody(t, f)
	rule := postprocess.ReferenceRule{
		ResourceType: "jamfplatform_pro_policy",
		AttrPath:     []string{"scope", "targets"},
		AttrName:     "computer_group_ids",
		TargetTypes:  []string{"jamfplatform_device_group#computer"},
		TargetAttr:   "jamf_pro_id",
		IsList:       true,
	}
	postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, `name = "no scope here"`) {
		t.Errorf("expected unchanged output, got:\n%s", result)
	}
}
