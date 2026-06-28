// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixConditionalAttributes(t *testing.T) {
	dir := t.TempDir()
	content := `resource "jamfplatform_pro_mobile_device_prestage_enrollment" "shared" {
  multi_user              = true
  prevent_activation_lock = false
}

resource "jamfplatform_pro_mobile_device_prestage_enrollment" "single" {
  multi_user              = false
  prevent_activation_lock = false
}

resource "jamfplatform_pro_self_service_macos_settings" "singleton" {
  default_home_category_id = "1"
  default_landing_page     = "HOME"
}

resource "jamfplatform_pro_self_service_macos_settings" "browse" {
  default_home_category_id = "5"
  default_landing_page     = "BROWSE"
}
`
	path := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	n, err := FixConditionalAttributes(path)
	if err != nil {
		t.Fatalf("FixConditionalAttributes: %v", err)
	}
	// Shared iPad prestage + non-Browse home category get corrected; the
	// multi_user=false prestage and the BROWSE settings are left alone.
	if n != 2 {
		t.Errorf("expected 2 corrections, got %d", n)
	}

	out, _ := os.ReadFile(path)
	s := string(out)

	// Shared iPad: prevent_activation_lock flipped to true.
	sharedIdx := strings.Index(s, `"shared"`)
	if sharedIdx < 0 || !strings.Contains(s[sharedIdx:], "prevent_activation_lock = true") {
		t.Error("expected shared prestage prevent_activation_lock = true")
	}
	// Non-Browse settings: home category reset to -1.
	if !strings.Contains(s, `default_home_category_id = "-1"`) {
		t.Error("expected non-Browse default_home_category_id reset to -1")
	}
	// BROWSE settings keep their category.
	browseIdx := strings.Index(s, `"browse"`)
	if browseIdx < 0 || !strings.Contains(s[browseIdx:], `default_home_category_id = "5"`) {
		t.Error("expected BROWSE settings to keep category 5")
	}
}
