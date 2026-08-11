// Copyright 2026, Jamf Software LLC

package postprocess_test

import (
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/pro"
	"github.com/Jamf-Concepts/jamformer/protect"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// setupRegistry creates a registry with entries for all supported resource types.
func setupRegistry() *registry.Registry {
	reg := registry.New()

	// Sites
	reg.Register("jamfpro_site", "1", "jamfpro_site.main_site")

	// Buildings
	reg.Register("jamfpro_building", "1", "jamfpro_building.headquarters")

	// Categories
	reg.Register("jamfpro_category", "5", "jamfpro_category.productivity")
	reg.Register("jamfpro_category", "10", "jamfpro_category.security")

	// Departments
	reg.Register("jamfpro_department", "3", "jamfpro_department.engineering")

	// Scripts
	reg.Register("jamfpro_script", "42", "jamfpro_script.disable_bluetooth")
	reg.Register("jamfpro_script", "43", "jamfpro_script.install_chrome")

	// Extension attributes
	reg.Register("jamfpro_computer_extension_attribute", "7", "jamfpro_computer_extension_attribute.serial_number")

	// Packages
	reg.Register("jamfpro_package", "100", "jamfpro_package.firefox_pkg")

	// Dock items
	reg.Register("jamfpro_dock_item", "15", "jamfpro_dock_item.safari")

	// Printers
	reg.Register("jamfpro_printer", "20", "jamfpro_printer.office_laser")

	// Network segments
	reg.Register("jamfpro_network_segment", "8", "jamfpro_network_segment.office_wifi")

	// Smart computer groups
	reg.Register("jamfpro_smart_computer_group_v2", "50", "jamfpro_smart_computer_group_v2.staff_macs")
	reg.Register("jamfpro_smart_computer_group_v2", "51", "jamfpro_smart_computer_group_v2.developer_macs")

	// Static computer groups
	reg.Register("jamfpro_static_computer_group", "60", "jamfpro_static_computer_group.lab_macs")

	// Icons
	reg.Register("jamfpro_icon", "200", "jamfpro_icon.firefox_icon")

	// Enrollment customizations
	reg.Register("jamfpro_enrollment_customization", "300", "jamfpro_enrollment_customization.default_branding")

	// Device enrollments
	reg.Register("jamfpro_device_enrollments", "400", "jamfpro_device_enrollments.ade_server")

	// Smart mobile device groups
	reg.Register("jamfpro_smart_mobile_device_group_v1", "70", "jamfpro_smart_mobile_device_group.staff_ipads")

	// Static mobile device groups
	reg.Register("jamfpro_static_mobile_device_group", "80", "jamfpro_static_mobile_device_group.lab_ipads")

	// API roles
	reg.Register("jamfpro_api_role", "500", "jamfpro_api_role.auditor")

	// Accounts (for account group member_ids)
	reg.Register("jamfpro_account", "233", "jamfpro_account.admin_user")
	reg.Register("jamfpro_account", "238", "jamfpro_account.service_account")

	// LDAP servers
	reg.Register("jamfpro_ldap_server", "10", "jamfpro_ldap_server.corporate_ldap")

	// Volume purchasing locations
	reg.Register("jamfpro_volume_purchasing_locations", "600", "jamfpro_volume_purchasing_locations.main_vpp")

	return reg
}

// --- helpers ---

func parseHCLRef(t *testing.T, src string) *hclwrite.File {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse HCL: %s", diags.Error())
	}
	return f
}

func refBlockBody(t *testing.T, f *hclwrite.File) *hclwrite.Body {
	t.Helper()
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			return block.Body()
		}
	}
	t.Fatal("no resource block found")
	return nil
}

func applyRules(body *hclwrite.Body, resourceType string, rules []postprocess.ReferenceRule, reg *registry.Registry) {
	for _, rule := range rules {
		if rule.ResourceType != resourceType {
			continue
		}
		postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)
	}
}

// TestScriptCategoryReference tests Script -> Category reference rewriting.
func TestScriptCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_script" "test" {
  name        = "test"
  category_id = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_script", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

// TestPackageCategoryReference tests Package -> Category reference rewriting.
func TestPackageCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_package" "test" {
  package_name = "test"
  category_id  = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_package", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

// TestProfileCategoryReference tests macOS Configuration Profile -> Category reference.
func TestProfileCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name        = "test"
  category_id = "10"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.security.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

// TestProfileScopeComputerGroupIds tests Profile -> scope.computer_group_ids reference.
func TestProfileScopeComputerGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    computer_group_ids = ["50", "60"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.staff_macs.id") {
		t.Errorf("expected smart group reference in scope, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_computer_group.lab_macs.id") {
		t.Errorf("expected static group reference in scope, got:\n%s", result)
	}
}

// TestProfileScopeExclusionComputerGroupIds tests Profile -> scope.exclusions.computer_group_ids.
func TestProfileScopeExclusionComputerGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    exclusions {
      computer_group_ids = ["51"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.developer_macs.id") {
		t.Errorf("expected exclusion group reference, got:\n%s", result)
	}
}

// TestProfileScopeBuildingIds tests Profile -> scope.building_ids reference.
func TestProfileScopeBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    building_ids = ["1"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected building reference, got:\n%s", result)
	}
}

// TestProfileScopeDepartmentIds tests Profile -> scope.department_ids reference.
func TestProfileScopeDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    department_ids = ["3"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected department reference, got:\n%s", result)
	}
}

// TestProfileScopeExclusionBuildingIds tests Profile -> scope.exclusions.building_ids.
func TestProfileScopeExclusionBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    exclusions {
      building_ids = ["1"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected exclusion building reference, got:\n%s", result)
	}
}

// TestProfileScopeExclusionDepartmentIds tests Profile -> scope.exclusions.department_ids.
func TestProfileScopeExclusionDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    exclusions {
      department_ids = ["3"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected exclusion department reference, got:\n%s", result)
	}
}

// TestProfileScopeLimitationsNetworkSegmentIds tests Profile -> scope.limitations.network_segment_ids.
func TestProfileScopeLimitationsNetworkSegmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "test"
  scope {
    limitations {
      network_segment_ids = ["8"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_network_segment.office_wifi.id") {
		t.Errorf("expected network segment reference, got:\n%s", result)
	}
}

// TestProfileSiteReference tests Profile -> site_id reference.
func TestProfileSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_macos_configuration_profile_plist" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_macos_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// TestPolicyCategoryReference tests Policy -> category_id reference.
func TestPolicyCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name        = "test"
  category_id = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

// TestPolicyScriptReference tests Policy -> payloads.scripts.id reference.
func TestPolicyScriptReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  payloads {
    scripts {
      id = "42"
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_script.disable_bluetooth.id") {
		t.Errorf("expected script reference, got:\n%s", result)
	}
}

// TestPolicyScopeComputerGroupIds tests Policy -> scope.computer_group_ids reference.
func TestPolicyScopeComputerGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    computer_group_ids = ["50", "60"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.staff_macs.id") {
		t.Errorf("expected smart group reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_computer_group.lab_macs.id") {
		t.Errorf("expected static group reference, got:\n%s", result)
	}
}

// TestPolicyScopeExclusionComputerGroupIds tests Policy -> scope.exclusions.computer_group_ids.
func TestPolicyScopeExclusionComputerGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    exclusions {
      computer_group_ids = ["51"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.developer_macs.id") {
		t.Errorf("expected exclusion group reference, got:\n%s", result)
	}
}

// TestPolicyScopeBuildingIds tests Policy -> scope.building_ids reference.
func TestPolicyScopeBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    building_ids = ["1"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected building reference, got:\n%s", result)
	}
}

// TestPolicyScopeExclusionBuildingIds tests Policy -> scope.exclusions.building_ids.
func TestPolicyScopeExclusionBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    exclusions {
      building_ids = ["1"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected exclusion building reference, got:\n%s", result)
	}
}

// TestPolicyScopeDepartmentIds tests Policy -> scope.department_ids reference.
func TestPolicyScopeDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    department_ids = ["3"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected department reference, got:\n%s", result)
	}
}

// TestPolicyScopeExclusionDepartmentIds tests Policy -> scope.exclusions.department_ids.
func TestPolicyScopeExclusionDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    exclusions {
      department_ids = ["3"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected exclusion department reference, got:\n%s", result)
	}
}

// TestPolicyScopeLimitationsNetworkSegmentIds tests Policy -> scope.limitations.network_segment_ids.
func TestPolicyScopeLimitationsNetworkSegmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    limitations {
      network_segment_ids = ["8"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_network_segment.office_wifi.id") {
		t.Errorf("expected network segment reference, got:\n%s", result)
	}
}

// TestPolicyDockItemReference tests Policy -> payloads.dock_items.id reference.
func TestPolicyDockItemReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  payloads {
    dock_items {
      id = "15"
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_dock_item.safari.id") {
		t.Errorf("expected dock item reference, got:\n%s", result)
	}
}

// TestPolicyPrinterReference tests Policy -> payloads.printers.id reference.
func TestPolicyPrinterReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  payloads {
    printers {
      id = "20"
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_printer.office_laser.id") {
		t.Errorf("expected printer reference, got:\n%s", result)
	}
}

// TestPolicySelfServiceCategoryReference tests Policy -> self_service.self_service_category.id.
func TestPolicySelfServiceCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  self_service {
    self_service_category {
      id = "5"
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected self-service category reference, got:\n%s", result)
	}
}

// TestPolicySiteReference tests Policy -> site_id reference.
func TestPolicySiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// TestSmartComputerGroupSiteReference tests Smart Computer Group -> site_id reference.
func TestSmartComputerGroupSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_smart_computer_group_v2" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_smart_computer_group_v2", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// TestStaticComputerGroupSiteReference tests Static Computer Group -> site_id reference.
func TestStaticComputerGroupSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_static_computer_group" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_static_computer_group", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// TestUnresolvedReferenceHasTODO verifies unresolved IDs get a TODO comment.
func TestUnresolvedReferenceHasTODO(t *testing.T) {
	reg := registry.New() // empty registry
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_script" "test" {
  name        = "test"
  category_id = "999"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_script", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "TODO") {
		t.Errorf("expected TODO comment for unresolved reference, got:\n%s", result)
	}
	if !strings.Contains(result, "999") {
		t.Errorf("expected original value preserved, got:\n%s", result)
	}
}

// TestUnresolvedListReferenceHasTODO verifies unresolved list IDs get TODO comments.
func TestUnresolvedListReferenceHasTODO(t *testing.T) {
	reg := registry.New() // empty registry
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    computer_group_ids = ["999"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "TODO") {
		t.Errorf("expected TODO comment for unresolved list reference, got:\n%s", result)
	}
}

// TestMixedResolvedUnresolvedList verifies a list with both resolved and unresolved IDs.
func TestMixedResolvedUnresolvedList(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    computer_group_ids = ["50", "999"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_policy", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.staff_macs.id") {
		t.Errorf("expected resolved reference, got:\n%s", result)
	}
	if !strings.Contains(result, "TODO") {
		t.Errorf("expected TODO for unresolved reference, got:\n%s", result)
	}
}

// --- Restricted Software reference tests ---

func TestRestrictedSoftwareCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_restricted_software" "test" {
  name        = "test"
  category_id = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_restricted_software", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

func TestRestrictedSoftwareSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_restricted_software" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_restricted_software", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

func TestRestrictedSoftwareScopeComputerGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_restricted_software" "test" {
  name = "test"
  scope {
    computer_group_ids = ["50", "60"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_restricted_software", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_computer_group_v2.staff_macs.id") {
		t.Errorf("expected smart group reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_computer_group.lab_macs.id") {
		t.Errorf("expected static group reference, got:\n%s", result)
	}
}

func TestRestrictedSoftwareScopeExclusionBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_restricted_software" "test" {
  name = "test"
  scope {
    exclusions {
      building_ids = ["1"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_restricted_software", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected building reference in exclusions, got:\n%s", result)
	}
}

// --- Mobile Device Configuration Profile reference tests ---

func TestMobileDeviceProfileCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_configuration_profile_plist" "test" {
  name        = "test"
  category_id = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

func TestMobileDeviceProfileScopeMobileDeviceGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_configuration_profile_plist" "test" {
  name = "test"
  scope {
    mobile_device_group_ids = ["70", "80"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_mobile_device_group.staff_ipads.id") {
		t.Errorf("expected smart mobile device group reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_mobile_device_group.lab_ipads.id") {
		t.Errorf("expected static mobile device group reference, got:\n%s", result)
	}
}

func TestMobileDeviceProfileScopeExclusionMobileDeviceGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_configuration_profile_plist" "test" {
  name = "test"
  scope {
    exclusions {
      mobile_device_group_ids = ["70"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_configuration_profile_plist", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_mobile_device_group.staff_ipads.id") {
		t.Errorf("expected mobile device group reference in exclusions, got:\n%s", result)
	}
}

// --- Mobile Device Prestage reference tests ---

func TestMobileDevicePrestageSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  display_name = "test"
  site_id      = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_prestage_enrollment", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

func TestMobileDevicePrestageDeviceEnrollmentReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  display_name                        = "test"
  device_enrollment_program_instance_id = "400"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_prestage_enrollment", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_device_enrollments.ade_server.id") {
		t.Errorf("expected device enrollment reference, got:\n%s", result)
	}
}

func TestMobileDevicePrestageEnrollmentCustomizationReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_prestage_enrollment" "test" {
  display_name                = "test"
  enrollment_customization_id = "300"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_prestage_enrollment", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_enrollment_customization.default_branding.id") {
		t.Errorf("expected enrollment customization reference, got:\n%s", result)
	}
}

// --- API Integration reference tests ---

func TestAPIIntegrationRoleReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_api_integration" "test" {
  display_name = "test"
  api_role_id  = "500"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_api_integration", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_api_role.auditor.id") {
		t.Errorf("expected API role reference, got:\n%s", result)
	}
}

// --- Smart Mobile Device Group reference tests ---

func TestSmartMobileDeviceGroupSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_smart_mobile_device_group_v1" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_smart_mobile_device_group_v1", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// --- Account reference tests ---

func TestAccountSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

func TestAccountIdentityServerReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account" "test" {
  name               = "test"
  identity_server_id = "10"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_ldap_server.corporate_ldap.id") {
		t.Errorf("expected LDAP server reference for identity_server_id, got:\n%s", result)
	}
}

// --- Account Group reference tests ---

func TestAccountGroupSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account_group" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account_group", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

func TestAccountGroupMemberIdsReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account_group" "test" {
  name       = "test"
  member_ids = ["233", "238"]
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account_group", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_account.admin_user.id") {
		t.Errorf("expected account reference for member 233, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_account.service_account.id") {
		t.Errorf("expected account reference for member 238, got:\n%s", result)
	}
}

func TestAccountGroupIdentityServerReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account_group" "test" {
  name               = "test"
  identity_server_id = "10"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account_group", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_ldap_server.corporate_ldap.id") {
		t.Errorf("expected LDAP server reference for identity_server_id, got:\n%s", result)
	}
}

func TestAccountGroupLdapServerReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_account_group" "test" {
  name           = "test"
  ldap_server_id = "10"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_account_group", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_ldap_server.corporate_ldap.id") {
		t.Errorf("expected LDAP server reference, got:\n%s", result)
	}
}

// --- Mobile Device Application reference tests ---

func TestMobileDeviceApplicationCategoryReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name        = "test"
  category_id = "5"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_category.productivity.id") {
		t.Errorf("expected category reference, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeMobileDeviceGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    mobile_device_group_ids = ["70", "80"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_mobile_device_group.staff_ipads.id") {
		t.Errorf("expected smart mobile device group reference, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfpro_static_mobile_device_group.lab_ipads.id") {
		t.Errorf("expected static mobile device group reference, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeExclusionMobileDeviceGroupIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    exclusions {
      mobile_device_group_ids = ["70"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_smart_mobile_device_group.staff_ipads.id") {
		t.Errorf("expected mobile device group reference in exclusions, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    building_ids = ["1"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected building reference, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeExclusionBuildingIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    exclusions {
      building_ids = ["1"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_building.headquarters.id") {
		t.Errorf("expected building reference in exclusions, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    department_ids = ["3"]
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected department reference, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationScopeExclusionDepartmentIds(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  scope {
    exclusions {
      department_ids = ["3"]
    }
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_department.engineering.id") {
		t.Errorf("expected department reference in exclusions, got:\n%s", result)
	}
}

func TestMobileDeviceApplicationVppReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_mobile_device_application" "test" {
  name = "test"
  vpp {
    vpp_admin_account_id = "600"
  }
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_mobile_device_application", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_volume_purchasing_locations.main_vpp.id") {
		t.Errorf("expected VPP location reference, got:\n%s", result)
	}
}

// --- Advanced User Search reference tests ---

func TestAdvancedUserSearchSiteReference(t *testing.T) {
	reg := setupRegistry()
	rules := pro.DefaultRules()

	f := parseHCLRef(t, `
resource "jamfpro_advanced_user_search" "test" {
  name    = "test"
  site_id = "1"
}
`)
	body := refBlockBody(t, f)
	applyRules(body, "jamfpro_advanced_user_search", rules, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "jamfpro_site.main_site.id") {
		t.Errorf("expected site reference, got:\n%s", result)
	}
}

// TestAllDefaultRulesHaveValidTargetTypes verifies all rules reference known resource types.
func TestAllDefaultRulesHaveValidTargetTypes(t *testing.T) {
	knownTypes := pro.TypeToFileMap()
	for _, rule := range pro.DefaultRules() {
		if _, ok := knownTypes[rule.ResourceType]; !ok {
			t.Errorf("rule references unknown source type: %q", rule.ResourceType)
		}
		for _, tt := range rule.TargetTypes {
			if _, ok := knownTypes[tt]; !ok {
				t.Errorf("rule for %s.%s references unknown target type: %q", rule.ResourceType, rule.AttrName, tt)
			}
		}
	}
}

// --- Jamf Protect reference tests ---

func setupProtectRegistry() *registry.Registry {
	reg := registry.New()
	reg.Register("jamfprotect_role", "1", "jamfprotect_role.read_only")
	reg.Register("jamfprotect_role", "2", "jamfprotect_role.full_admin")
	reg.Register("jamfprotect_group", "1", "jamfprotect_group.all_users")
	reg.Register("jamfprotect_analytic", "3c8a88ef-277a-4238-a695-ebaa6eee0921", "jamfprotect_analytic.macos_threat_prevention")
	reg.Register("jamfprotect_analytic", "02e15df8-1656-41ad-8f54-9def66d88ce7", "jamfprotect_analytic.macos_malware_detection")
	reg.Register("jamfprotect_analytic_managed", "c94af094-5ea1-11ec-be1c-0660d8e6ab1f", "jamfprotect_analytic_managed.suspicious_java_activity")
	reg.Register("jamfprotect_analytic_set", "79e0a2a0-3af2-4e0b-8148-1f9c129bfd85", "jamfprotect_analytic_set.default")
	reg.Register("jamfprotect_action_configuration", "abc-123", "jamfprotect_action_configuration.notify")
	reg.Register("jamfprotect_exception_set", "def-456", "jamfprotect_exception_set.trusted_apps")
	reg.Register("jamfprotect_telemetry", "tel-789", "jamfprotect_telemetry.default_telemetry")
	reg.Register("jamfprotect_removable_storage_control_set", "rsc-012", "jamfprotect_removable_storage_control_set.block_usb")
	reg.Register("jamfprotect_unified_logging_filter", "ulf-111", "jamfprotect_unified_logging_filter.time_machine")
	reg.Register("jamfprotect_unified_logging_filter", "ulf-222", "jamfprotect_unified_logging_filter.screen_sharing")
	reg.Register("jamfprotect_unified_logging_filter_set", "ulfs-333", "jamfprotect_unified_logging_filter_set.endpoint_diagnostics")
	return reg
}

func TestProtectGroupRoleReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_group" "test" {
  name     = "Test Group"
  role_ids = [1, 2]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_group", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_role.read_only.id") {
		t.Error("Expected role_ids to contain jamfprotect_role.read_only.id")
	}
	if !strings.Contains(result, "jamfprotect_role.full_admin.id") {
		t.Error("Expected role_ids to contain jamfprotect_role.full_admin.id")
	}
}

func TestProtectUserRoleReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_user" "test" {
  email    = "test@example.com"
  role_ids = [2]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_user", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_role.full_admin.id") {
		t.Errorf("Expected role_ids to reference full_admin, got:\n%s", result)
	}
}

func TestProtectUserGroupReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_user" "test" {
  email     = "test@example.com"
  group_ids = [1]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_user", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_group.all_users.id") {
		t.Errorf("Expected group_ids to reference all_users, got:\n%s", result)
	}
}

func TestProtectAPIClientRoleReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_api_client" "test" {
  name     = "Test Client"
  role_ids = [1]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_api_client", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_role.read_only.id") {
		t.Errorf("Expected role_ids to reference read_only, got:\n%s", result)
	}
}

func TestProtectAnalyticSetAnalyticsReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_analytic_set" "test" {
  name      = "Test Set"
  analytics = ["3c8a88ef-277a-4238-a695-ebaa6eee0921", "02e15df8-1656-41ad-8f54-9def66d88ce7"]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_analytic_set", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_analytic.macos_threat_prevention.id") {
		t.Errorf("Expected analytics to reference macos_threat_prevention, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_analytic.macos_malware_detection.id") {
		t.Errorf("Expected analytics to reference macos_malware_detection, got:\n%s", result)
	}
}

func TestProtectAnalyticSetManagedAnalyticReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_analytic_set" "test" {
  name      = "Test Set"
  analytics = ["3c8a88ef-277a-4238-a695-ebaa6eee0921", "c94af094-5ea1-11ec-be1c-0660d8e6ab1f"]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_analytic_set", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_analytic.macos_threat_prevention.id") {
		t.Errorf("Expected analytics to reference macos_threat_prevention, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_analytic_managed.suspicious_java_activity.id") {
		t.Errorf("Expected analytics to reference suspicious_java_activity (managed), got:\n%s", result)
	}
}

func TestProtectUnifiedLoggingFilterSetFiltersReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_unified_logging_filter_set" "test" {
  name    = "Endpoint Diagnostics"
  filters = ["ulf-111", "ulf-222"]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_unified_logging_filter_set", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_unified_logging_filter.time_machine.id") {
		t.Errorf("Expected filters to reference time_machine, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_unified_logging_filter.screen_sharing.id") {
		t.Errorf("Expected filters to reference screen_sharing, got:\n%s", result)
	}
}

// TestProtectUnifiedLoggingFilterSetEmptyFilters covers a filter set with no
// members — valid per the provider schema, and must survive rewriting untouched.
func TestProtectUnifiedLoggingFilterSetEmptyFilters(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_unified_logging_filter_set" "test" {
  name    = "Placeholder Set"
  filters = []
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_unified_logging_filter_set", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "filters = []") {
		t.Errorf("Expected empty filters list preserved, got:\n%s", result)
	}
	if strings.Contains(result, "TODO: unresolved reference") {
		t.Errorf("Empty filters list should not produce an unresolved-reference TODO, got:\n%s", result)
	}
}

// TestUnresolvedListLiteralsStayValidHCL covers an unresolvable ID inside a list
// attribute. A string element must be re-quoted (a bare UUID parses as an
// identifier and makes the file invalid HCL), while a numeric element must stay
// bare so a number-typed list keeps its element type.
func TestUnresolvedListLiteralsStayValidHCL(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()

	// String IDs: the member filters were never discovered, so neither resolves.
	src := `resource "jamfprotect_unified_logging_filter_set" "test" {
  name    = "Endpoint Diagnostics"
  filters = ["4c8552c0-8347-43fb-b74b-eda602d02e15", "ulf-111"]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_unified_logging_filter_set", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, `"4c8552c0-8347-43fb-b74b-eda602d02e15", # TODO: unresolved reference`) {
		t.Errorf("expected unresolved string element re-quoted, got:\n%s", result)
	}
	// The resolvable one still becomes a reference.
	if !strings.Contains(result, "jamfprotect_unified_logging_filter.time_machine.id") {
		t.Errorf("expected resolvable element rewritten, got:\n%s", result)
	}
	// The whole file must still parse.
	if _, diags := hclwrite.ParseConfig([]byte(result), "test.tf", hcl.Pos{Line: 1, Column: 1}); diags.HasErrors() {
		t.Errorf("rewritten HCL does not parse: %s\n%s", diags.Error(), result)
	}
	if _, diags := hclsyntax.ParseConfig([]byte(result), "test.tf", hcl.Pos{Line: 1, Column: 1}); diags.HasErrors() {
		t.Errorf("rewritten HCL is not valid syntax: %s\n%s", diags.Error(), result)
	}

	// Numeric IDs must not gain quotes.
	numSrc := `resource "jamfprotect_user" "test" {
  email    = "test@example.com"
  role_ids = [999]
}`
	nf := parseHCLRef(t, numSrc)
	nbody := refBlockBody(t, nf)
	applyRules(nbody, "jamfprotect_user", rules, reg)
	numResult := string(nf.Bytes())

	if !strings.Contains(numResult, "999, # TODO: unresolved reference") {
		t.Errorf("expected unresolved numeric element left bare, got:\n%s", numResult)
	}
	if strings.Contains(numResult, `"999"`) {
		t.Errorf("numeric element must not be quoted, got:\n%s", numResult)
	}
}

func TestProtectPlanReferences(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_plan" "test" {
  name                         = "Test Plan"
  action_configuration         = "abc-123"
  exception_sets               = ["def-456"]
  telemetry                    = "tel-789"
  removable_storage_control_set = "rsc-012"
  analytic_sets                = ["79e0a2a0-3af2-4e0b-8148-1f9c129bfd85"]
  unified_logging_filter_sets  = ["ulfs-333"]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_plan", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "jamfprotect_action_configuration.notify.id") {
		t.Errorf("Expected action_configuration to reference notify, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_exception_set.trusted_apps.id") {
		t.Errorf("Expected exception_sets to reference trusted_apps, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_telemetry.default_telemetry.id") {
		t.Errorf("Expected telemetry to reference default_telemetry, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_removable_storage_control_set.block_usb.id") {
		t.Errorf("Expected removable_storage_control_set to reference block_usb, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_analytic_set.default.id") {
		t.Errorf("Expected analytic_sets to reference default, got:\n%s", result)
	}
	if !strings.Contains(result, "jamfprotect_unified_logging_filter_set.endpoint_diagnostics.id") {
		t.Errorf("Expected unified_logging_filter_sets to reference endpoint_diagnostics, got:\n%s", result)
	}
}

func TestProtectUnresolvedReference(t *testing.T) {
	reg := setupProtectRegistry()
	rules := protect.DefaultRules()
	src := `resource "jamfprotect_user" "test" {
  email    = "test@example.com"
  role_ids = [999]
}`
	f := parseHCLRef(t, src)
	body := refBlockBody(t, f)
	applyRules(body, "jamfprotect_user", rules, reg)
	result := string(f.Bytes())

	if !strings.Contains(result, "TODO: unresolved reference") {
		t.Errorf("Expected unresolved reference TODO comment, got:\n%s", result)
	}
}

// TestEmbeddedDeviceGroupIDsInString tests that device-group UUIDs embedded in a
// blueprint activation_conditions expression are rewritten to ${...id}
// interpolations, with surrounding text and unresolvable ids left intact and the
// result remaining valid HCL.
func TestEmbeddedDeviceGroupIDsInString(t *testing.T) {
	reg := registry.New()
	reg.Register("jamfplatform_device_group", "fce3d9a5-8660-42ff-a95e-625e7b53b48a", "jamfplatform_device_group.shared_ipads")

	f := parseHCLRef(t, `
resource "jamfplatform_blueprints_blueprint" "test" {
  name                  = "test"
  activation_conditions = "ANY @property(jamf.device.groups) IN {'fce3d9a5-8660-42ff-a95e-625e7b53b48a', '11111111-2222-3333-4444-555555555555'} AND @status(device.model.family) == 'iPad'"
}
`)
	body := refBlockBody(t, f)
	rule := postprocess.ReferenceRule{
		ResourceType: "jamfplatform_blueprints_blueprint",
		AttrName:     "activation_conditions",
		TargetTypes:  []string{"jamfplatform_device_group"},
		TargetAttr:   "id",
		EmbeddedIDs:  true,
	}
	postprocess.RewriteBlockForTest(body, rule.BlockPath, rule, reg)

	result := string(f.Bytes())
	if !strings.Contains(result, "${jamfplatform_device_group.shared_ipads.id}") {
		t.Errorf("expected embedded device-group interpolation, got:\n%s", result)
	}
	// Unresolvable UUID stays literal.
	if !strings.Contains(result, "11111111-2222-3333-4444-555555555555") {
		t.Errorf("expected unresolvable UUID left intact, got:\n%s", result)
	}
	// Surrounding expression preserved.
	if !strings.Contains(result, "@status(device.model.family) == 'iPad'") {
		t.Errorf("expected surrounding expression preserved, got:\n%s", result)
	}
	// Result must still be valid HCL.
	if _, diags := hclwrite.ParseConfig(f.Bytes(), "out.tf", hcl.Pos{Line: 1, Column: 1}); diags.HasErrors() {
		t.Errorf("rewritten HCL no longer parses: %s\n%s", diags.Error(), result)
	}
}
