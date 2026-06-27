// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
)

// TestPlatformProcessEndToEnd drives postprocess.Process with the real platform
// rules / extract specs over a synthetic generated.tf exercising the nested
// object-attribute surface: category/site references, scope group bridging via
// the device-group subtype, list-of-objects script references, and .mobileconfig
// payload extraction.
func TestPlatformProcessEndToEnd(t *testing.T) {
	dir := t.TempDir()

	generated := `
resource "jamfplatform_pro_policy" "deploy" {
  general = {
    name        = "Deploy Thing"
    category_id = "5"
    site_id     = "-1"
  }
  scope = {
    targets = {
      computer_group_ids = ["12"]
      building_ids       = ["3"]
    }
  }
  scripts = {
    scripts = [
      {
        id       = "44"
        priority = "After"
      },
    ]
  }
}

resource "jamfplatform_pro_macos_configuration_profile" "wifi" {
  general = {
    name        = "Corp WiFi"
    category_id = "5"
    payloads    = "<?xml version=\"1.0\"?><plist><dict><key>k</key><string>v</string></dict></plist>"
  }
}

resource "jamfplatform_pro_printer" "lab" {
  name         = "Lab LaserJet"
  use_generic  = false
  ppd_path     = "/Library/Printers/PPDs/Contents/Resources/Lab.ppd"
  ppd_contents = "*PPD-Adobe: \"4.3\"\n*ModelName: \"Lab LaserJet\"\n*PCFileName: \"LAB.PPD\"\n"
}

resource "jamfplatform_pro_mobile_device_provisioning_profile" "enterprise" {
  name         = "Enterprise App"
  profile_data = "MIIabcdefBASE64mobileprovisionDATA=="
}

resource "jamfplatform_pro_jamf_connect" "corp" {
  profile_id           = 77
  auto_deployment_type = "PATCH_UPDATES"
}

resource "jamfplatform_pro_self_service_branding_macos" "singleton" {
  icon_id         = 81
  banner_image_id = 0
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	reg.Register("jamfplatform_pro_category", "5", "jamfplatform_pro_category.productivity")
	reg.Register("jamfplatform_pro_building", "3", "jamfplatform_pro_building.hq")
	reg.Register("jamfplatform_pro_script", "44", "jamfplatform_pro_script.rotate")
	reg.Register(DeviceGroupComputerType, "12", "jamfplatform_device_group.eng_computer")
	reg.Register("jamfplatform_pro_macos_configuration_profile", "77", "jamfplatform_pro_macos_configuration_profile.wifi")
	reg.Register("jamfplatform_pro_self_service_branding_image", "81", "jamfplatform_pro_self_service_branding_image.branding_image_81")

	if err := postprocess.Process(dir, genFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap: TypeToFileMap(),
		Rules:         DefaultRules(),
		ExtractSpecs:  ExtractSpecs(),
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	policy, err := os.ReadFile(filepath.Join(dir, "pro_policy.tf"))
	if err != nil {
		t.Fatalf("reading pro_policy.tf: %v", err)
	}
	ps := string(policy)
	for _, want := range []string{
		"jamfplatform_pro_category.productivity.id",
		"jamfplatform_device_group.eng_computer.jamf_pro_id",
		"jamfplatform_pro_building.hq.id",
		"jamfplatform_pro_script.rotate.id",
	} {
		if !strings.Contains(ps, want) {
			t.Errorf("pro_policy.tf missing %q\n%s", want, ps)
		}
	}
	// The "-1" no-site sentinel must be left as a literal, not flagged unresolved.
	if strings.Contains(ps, "TODO: unresolved reference") {
		t.Errorf("pro_policy.tf must not flag the -1 site sentinel as unresolved:\n%s", ps)
	}

	profile, err := os.ReadFile(filepath.Join(dir, "pro_macos_configuration_profile.tf"))
	if err != nil {
		t.Fatalf("reading pro_macos_configuration_profile.tf: %v", err)
	}
	prof := string(profile)
	if !strings.Contains(prof, "jamfplatform_pro_category.productivity.id") {
		t.Errorf("profile missing category reference:\n%s", prof)
	}
	if !strings.Contains(prof, `file("${path.module}/support_files/macos_configuration_profiles/Corp WiFi.mobileconfig")`) {
		t.Errorf("profile payload not extracted to a file() reference:\n%s", prof)
	}

	// The extracted payload file must exist and hold the XML.
	payload, err := os.ReadFile(filepath.Join(dir, "support_files", "macos_configuration_profiles", "Corp WiFi.mobileconfig"))
	if err != nil {
		t.Fatalf("reading extracted payload: %v", err)
	}
	if !strings.Contains(string(payload), "<plist>") {
		t.Errorf("extracted payload missing plist content: %s", payload)
	}

	// Printer ppd_contents extracted to a .ppd file() reference (FileKindRaw).
	printer, err := os.ReadFile(filepath.Join(dir, "pro_printer.tf"))
	if err != nil {
		t.Fatalf("reading pro_printer.tf: %v", err)
	}
	if !strings.Contains(string(printer), `file("${path.module}/support_files/printers/Lab LaserJet.ppd")`) {
		t.Errorf("printer ppd_contents not extracted to a file() reference:\n%s", printer)
	}
	ppd, err := os.ReadFile(filepath.Join(dir, "support_files", "printers", "Lab LaserJet.ppd"))
	if err != nil {
		t.Fatalf("reading extracted ppd: %v", err)
	}
	if !strings.Contains(string(ppd), "*PPD-Adobe") {
		t.Errorf("extracted ppd missing PPD header: %s", ppd)
	}

	// Provisioning profile profile_data extracted to a .mobileprovision file() reference.
	prov, err := os.ReadFile(filepath.Join(dir, "pro_mobile_device_provisioning_profile.tf"))
	if err != nil {
		t.Fatalf("reading pro_mobile_device_provisioning_profile.tf: %v", err)
	}
	if !strings.Contains(string(prov), `file("${path.module}/support_files/mobile_device_provisioning_profiles/Enterprise App.mobileprovision")`) {
		t.Errorf("provisioning profile_data not extracted to a file() reference:\n%s", prov)
	}
	if _, err := os.Stat(filepath.Join(dir, "support_files", "mobile_device_provisioning_profiles", "Enterprise App.mobileprovision")); err != nil {
		t.Errorf("extracted .mobileprovision file missing: %v", err)
	}

	// jamf_connect.profile_id (Int64) → macOS config profile id (String),
	// wrapped in tonumber() so the assignment type-checks (Numeric rule).
	jc, err := os.ReadFile(filepath.Join(dir, "pro_jamf_connect.tf"))
	if err != nil {
		t.Fatalf("reading pro_jamf_connect.tf: %v", err)
	}
	if !strings.Contains(string(jc), "tonumber(jamfplatform_pro_macos_configuration_profile.wifi.id)") {
		t.Errorf("jamf_connect profile_id not rewritten to a tonumber() reference:\n%s", jc)
	}

	// Branding singleton icon_id (Int64) → branding image id (String), wrapped
	// in tonumber(); banner_image_id = 0 (no image) stays literal.
	br, err := os.ReadFile(filepath.Join(dir, "pro_self_service_branding_macos.tf"))
	if err != nil {
		t.Fatalf("reading pro_self_service_branding_macos.tf: %v", err)
	}
	if !strings.Contains(string(br), "tonumber(jamfplatform_pro_self_service_branding_image.branding_image_81.id)") {
		t.Errorf("branding icon_id not rewritten to a tonumber() reference:\n%s", br)
	}
	if !strings.Contains(string(br), "banner_image_id = 0") {
		t.Errorf("banner_image_id = 0 (no image) should stay literal:\n%s", br)
	}
}

// TestPlatformProcessSkipsVendorProfile verifies that a vendor-managed profile
// is dropped and its import block is not written.
func TestPlatformProcessSkipsVendorProfile(t *testing.T) {
	dir := t.TempDir()

	generated := `
resource "jamfplatform_pro_macos_configuration_profile" "vendor" {
  general = {
    name     = "Protect Managed"
    payloads = "<?xml version=\"1.0\"?><plist><dict><key>PayloadIdentifier</key><string>com.jamf.protect.example</string></dict></plist>"
  }
}

import {
  to = jamfplatform_pro_macos_configuration_profile.vendor
  identity = {
    id = "99"
  }
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	if err := postprocess.Process(dir, genFile, registry.New(), &postprocess.ProcessOptions{
		TypeToFileMap: TypeToFileMap(),
		Rules:         DefaultRules(),
		ExtractSpecs:  ExtractSpecs(),
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Neither the resource file nor the import file should be created.
	if _, err := os.Stat(filepath.Join(dir, "pro_macos_configuration_profile.tf")); !os.IsNotExist(err) {
		t.Error("vendor profile resource file should not be written")
	}
	if _, err := os.Stat(filepath.Join(dir, "pro_macos_configuration_profile_import.tf")); !os.IsNotExist(err) {
		t.Error("vendor profile import file should not be written")
	}
}

// TestPlatformCriteriaReferences exercises the smart-group criteria reference
// resolution: device-group "member of" (by name), user-group "member of" (by
// id), and extension-attribute-name criterion fields (by name, device_type
// scoped), with built-in criteria left untouched.
func TestPlatformCriteriaReferences(t *testing.T) {
	dir := t.TempDir()

	generated := `
resource "jamfplatform_device_group" "all_managed" {
  name        = "All Managed"
  device_type = "computer"
  group_type  = "smart"
}

resource "jamfplatform_pro_computer_extension_attribute" "admin_rights" {
  name = "Administrator Rights"
}

resource "jamfplatform_device_group" "smart_grp" {
  name        = "Smart Group"
  device_type = "computer"
  group_type  = "smart"
  criteria = [
    {
      and_or   = "and"
      criteria = "Computer Group"
      operator = "member of"
      order    = 0
      value    = "All Managed"
    },
    {
      and_or   = "and"
      criteria = "Administrator Rights"
      operator = "is"
      order    = 1
      value    = "Yes"
    },
    {
      and_or   = "and"
      criteria = "Application Bundle ID"
      operator = "is"
      order    = 2
      value    = "com.example.app"
    },
  ]
}

resource "jamfplatform_pro_user_group" "members" {
  name       = "Members"
  group_type = "smart"
  criteria = [
    {
      and_or      = "and"
      name        = "User Group"
      search_type = "member of"
      priority    = 0
      value       = "7"
    },
    {
      and_or      = "and"
      name        = "Managed Apple ID"
      search_type = "like"
      priority    = 1
      value       = "@example.com"
    },
  ]
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	reg.Register("jamfplatform_pro_user_group", "7", "jamfplatform_pro_user_group.staff")

	if err := PopulateCriteriaNameIndexes(genFile, reg); err != nil {
		t.Fatalf("PopulateCriteriaNameIndexes: %v", err)
	}
	if _, err := ResolveCriteriaExtensionAttributes(genFile, reg); err != nil {
		t.Fatalf("ResolveCriteriaExtensionAttributes: %v", err)
	}
	if err := postprocess.Process(dir, genFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap: TypeToFileMap(),
		Rules:         DefaultRules(),
	}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	dg, err := os.ReadFile(filepath.Join(dir, "device_groups.tf"))
	if err != nil {
		t.Fatalf("reading device_groups.tf: %v", err)
	}
	dgs := string(dg)
	// (A) device-group member-of value -> .name
	if !strings.Contains(dgs, "jamfplatform_device_group.all_managed.name") {
		t.Errorf("device-group member-of not resolved to .name:\n%s", dgs)
	}
	// (C) EA-name criterion field -> EA .name
	if !strings.Contains(dgs, "jamfplatform_pro_computer_extension_attribute.admin_rights.name") {
		t.Errorf("EA criterion field not resolved:\n%s", dgs)
	}
	// built-in criterion left untouched
	if !strings.Contains(dgs, `"Application Bundle ID"`) {
		t.Errorf("built-in criterion should be untouched:\n%s", dgs)
	}

	ug, err := os.ReadFile(filepath.Join(dir, "pro_user_group.tf"))
	if err != nil {
		t.Fatalf("reading pro_user_group.tf: %v", err)
	}
	ugs := string(ug)
	// (B) user-group member-of value -> .name (looked up by the wire id "7";
	// name is the provider's author-by-name contract).
	if !strings.Contains(ugs, "jamfplatform_pro_user_group.staff.name") {
		t.Errorf("user-group member-of not resolved to .name:\n%s", ugs)
	}
	// non-group criterion left untouched
	if !strings.Contains(ugs, `"@example.com"`) {
		t.Errorf("non-group user criterion should be untouched:\n%s", ugs)
	}
}

// TestPopulateCriteriaNameIndexesNoTopLevelName guards against the panic where a
// resource carrying its name nested (general.name) has no top-level "name" attr.
func TestPopulateCriteriaNameIndexesNoTopLevelName(t *testing.T) {
	dir := t.TempDir()
	gen := `resource "jamfplatform_pro_policy" "p" {
  general = {
    name = "Deploy"
  }
}

resource "jamfplatform_device_group" "grp" {
  name        = "All Managed"
  device_type = "computer"
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(gen), 0644); err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	if err := PopulateCriteriaNameIndexes(genFile, reg); err != nil {
		t.Fatalf("PopulateCriteriaNameIndexes: %v", err)
	}
	// The device group was indexed by name; the policy (no top-level name) was skipped without panicking.
	if _, ok := reg.Resolve(DeviceGroupComputerNameType, "All Managed"); !ok {
		t.Error("expected device group indexed by name")
	}
}
