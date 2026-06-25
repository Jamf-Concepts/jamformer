// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

// A computer group and a mobile group share the same display name AND the same
// Jamf Pro numeric ID (the two ID spaces are independent). jamf_pro_id is shown
// in-block here to simulate a provider/events-recovered value.
const dgGenerated = `
resource "jamfplatform_device_group" "all_0" {
  name        = "All Staff"
  device_type = "computer"
  group_type  = "smart"
  jamf_pro_id = "12"
}

resource "jamfplatform_device_group" "all_1" {
  name        = "All Staff"
  device_type = "mobile"
  group_type  = "smart"
  jamf_pro_id = "12"
}

import {
  to = jamfplatform_device_group.all_0
  identity = {
    id = "uuid-computer"
  }
}

import {
  to = jamfplatform_device_group.all_1
  identity = {
    id = "uuid-mobile"
  }
}
`

func writeGenerated(t *testing.T, content string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func TestDeviceGroupLabelDisambiguation(t *testing.T) {
	_, path := writeGenerated(t, dgGenerated)

	if err := RenameLabels(path); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)

	if !strings.Contains(body, "jamfplatform_device_group.all_staff_computer") {
		t.Errorf("expected computer group label all_staff_computer, got:\n%s", body)
	}
	if !strings.Contains(body, "jamfplatform_device_group.all_staff_mobile") {
		t.Errorf("expected mobile group label all_staff_mobile, got:\n%s", body)
	}
}

func TestDeviceGroupSubtypeRegistry(t *testing.T) {
	_, path := writeGenerated(t, dgGenerated)
	if err := RenameLabels(path); err != nil {
		t.Fatalf("RenameLabels: %v", err)
	}

	info, err := CollectDeviceGroupInfo(path)
	if err != nil {
		t.Fatalf("CollectDeviceGroupInfo: %v", err)
	}
	if len(info) != 2 {
		t.Fatalf("expected 2 device groups, got %d", len(info))
	}

	reg := registry.New()
	if unbridged := PopulateDeviceGroupSubtypes(reg, info); unbridged != 0 {
		t.Errorf("expected 0 unbridged groups, got %d", unbridged)
	}

	// Same numeric ID "12" must resolve to different addresses per device type.
	comp, ok := reg.AttrReference("jamfplatform_device_group#computer", "12", "jamf_pro_id")
	if !ok {
		t.Fatal("computer subtype id 12 did not resolve")
	}
	mob, ok := reg.AttrReference("jamfplatform_device_group#mobile", "12", "jamf_pro_id")
	if !ok {
		t.Fatal("mobile subtype id 12 did not resolve")
	}
	if comp != "jamfplatform_device_group.all_staff_computer.jamf_pro_id" {
		t.Errorf("unexpected computer reference: %s", comp)
	}
	if mob != "jamfplatform_device_group.all_staff_mobile.jamf_pro_id" {
		t.Errorf("unexpected mobile reference: %s", mob)
	}
}

func TestPopulateDeviceGroupSubtypesCountsUnbridged(t *testing.T) {
	reg := registry.New()
	info := map[string]DeviceGroupInfo{
		"a": {Address: "jamfplatform_device_group.a", DeviceType: "computer", JamfProID: "1"},
		"b": {Address: "jamfplatform_device_group.b", DeviceType: "mobile"}, // no jamf_pro_id
	}
	if unbridged := PopulateDeviceGroupSubtypes(reg, info); unbridged != 1 {
		t.Errorf("expected 1 unbridged group, got %d", unbridged)
	}
	if _, ok := reg.Resolve("jamfplatform_device_group#computer", "1"); !ok {
		t.Error("expected computer group 1 to be registered")
	}
}
