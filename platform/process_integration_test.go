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
