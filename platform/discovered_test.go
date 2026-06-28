// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectResourceRefs(t *testing.T) {
	dir := t.TempDir()

	// generated.tf carries list-resource import blocks (terraform query identity
	// format) inline with their resource blocks, plus a singleton resource whose
	// import block lives in a separate file.
	generated := `import {
  to       = jamfplatform_pro_policy.install_chrome
  identity = {
    id = "42"
  }
}

resource "jamfplatform_pro_policy" "install_chrome" {
  general = {
    name = "Install Chrome"
  }
}

resource "jamfplatform_pro_smtp_server_settings" "smtp" {
  enabled = true
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	singletonImports := `import {
  to = jamfplatform_pro_smtp_server_settings.smtp
  id = "singleton"
}
`
	singFile := filepath.Join(dir, "singletons_import.tf")
	if err := os.WriteFile(singFile, []byte(singletonImports), 0644); err != nil {
		t.Fatal(err)
	}

	refs, err := CollectResourceRefs(genFile, singFile, filepath.Join(dir, "jamf_connect_import.tf"))
	if err != nil {
		t.Fatalf("CollectResourceRefs: %v", err)
	}

	byAddr := map[string]DiscoveredResource{}
	for _, r := range refs {
		byAddr[r.TFType+"."+r.Label] = r
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 resources, got %d: %+v", len(refs), refs)
	}

	policy, ok := byAddr["jamfplatform_pro_policy.install_chrome"]
	if !ok {
		t.Fatal("policy not collected")
	}
	if policy.JamfID != "42" {
		t.Errorf("policy JamfID = %q, want 42", policy.JamfID)
	}

	smtp, ok := byAddr["jamfplatform_pro_smtp_server_settings.smtp"]
	if !ok {
		t.Fatal("smtp singleton not collected")
	}
	if smtp.JamfID != "singleton" {
		t.Errorf("smtp JamfID = %q, want singleton", smtp.JamfID)
	}
}

func TestCollectResourceRefsMissingImport(t *testing.T) {
	dir := t.TempDir()
	generated := `resource "jamfplatform_pro_policy" "orphan" {
  general = {
    name = "Orphan"
  }
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	refs, err := CollectResourceRefs(genFile)
	if err != nil {
		t.Fatalf("CollectResourceRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(refs))
	}
	if refs[0].JamfID != "" {
		t.Errorf("expected empty JamfID for resource without import block, got %q", refs[0].JamfID)
	}
}
