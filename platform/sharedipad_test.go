// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSharedIpadActivationLock(t *testing.T) {
	dir := t.TempDir()
	content := `resource "jamfplatform_pro_mobile_device_prestage_enrollment" "shared_legacy" {
  multi_user              = true
  prevent_activation_lock = false
}

resource "jamfplatform_pro_mobile_device_prestage_enrollment" "shared_ok" {
  multi_user              = true
  prevent_activation_lock = true
}

resource "jamfplatform_pro_mobile_device_prestage_enrollment" "not_shared" {
  multi_user              = false
  prevent_activation_lock = false
}
`
	path := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	n, err := NormalizeSharedIpadActivationLock(path)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	// Only the legacy Shared iPad prestage with false is changed.
	if n != 1 {
		t.Errorf("expected 1 adjusted, got %d", n)
	}

	out, _ := os.ReadFile(path)
	s := string(out)

	legacy := s[strings.Index(s, `"shared_legacy"`):]
	if !strings.Contains(legacy[:strings.Index(legacy, "}")], "prevent_activation_lock = true") {
		t.Error("shared_legacy prevent_activation_lock should be coerced to true")
	}
	// A non-Shared prestage keeps its false (Jamf doesn't force it there).
	notShared := s[strings.Index(s, `"not_shared"`):]
	if !strings.Contains(notShared[:strings.Index(notShared, "}")], "prevent_activation_lock = false") {
		t.Error("not_shared prestage must keep prevent_activation_lock = false")
	}
}
