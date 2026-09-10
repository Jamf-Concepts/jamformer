// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

// A settings singleton gets an import block written up front, but
// -generate-config-out only emits a resource block for a setting the tenant
// has configured. The import block for the one it skipped names an address no
// resource carries, and terraform rejects the entire plan with "Configuration
// for import target does not exist" — so the export must not ship it.
const orphanImportFixture = `
resource "jamfplatform_pro_smtp_server" "singleton" {
  enabled = true
}

import {
  to       = jamfplatform_pro_smtp_server.singleton
  identity = { id = "singleton" }
}

import {
  to       = jamfplatform_pro_self_service_branding_ios.singleton
  identity = { id = "singleton" }
}
`

func TestDropImportWithoutResourceBlock(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(orphanImportFixture), 0644); err != nil {
		t.Fatal(err)
	}

	opts := &ProcessOptions{
		TypeToFileMap: map[string]string{
			"jamfplatform_pro_smtp_server":               "pro_smtp_server.tf",
			"jamfplatform_pro_self_service_branding_ios": "pro_self_service_branding_ios.tf",
		},
	}
	if err := Process(dir, genPath, registry.New(), opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	// The configured singleton keeps its import block.
	kept := readFile(t, filepath.Join(dir, "pro_smtp_server_import.tf"))
	if !strings.Contains(kept, "jamfplatform_pro_smtp_server.singleton") {
		t.Errorf("import block for a generated resource should be kept:\n%s", kept)
	}

	// The one with no resource block ships no import file at all.
	orphan := filepath.Join(dir, "pro_self_service_branding_ios_import.tf")
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected no import file for a resource that was never generated, got err=%v; content:\n%s",
			err, readFile(t, orphan))
	}
}
