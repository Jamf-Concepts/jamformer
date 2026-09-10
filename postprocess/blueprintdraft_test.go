// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const draftStripFixture = `
resource "jamfplatform_blueprints_blueprint" "draft" {
  name          = "Empty Draft"
  device_groups = []
}

resource "jamfplatform_blueprints_blueprint" "real" {
  name          = "Real Blueprint"
  device_groups = ["uuid-1"]
}

import {
  to       = jamfplatform_blueprints_blueprint.draft
  identity = { id = "draft-uuid" }
}

import {
  to       = jamfplatform_blueprints_blueprint.real
  identity = { id = "real-uuid" }
}
`

func TestStripBlueprintDrafts(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(draftStripFixture), 0644); err != nil {
		t.Fatal(err)
	}

	n, err := StripBlueprintDrafts(genPath)
	if err != nil {
		t.Fatalf("StripBlueprintDrafts: %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}

	out := readFile(t, genPath)
	if strings.Contains(out, `"draft"`) {
		t.Errorf("draft resource and its import block should be gone:\n%s", out)
	}
	if !strings.Contains(out, `"real"`) || strings.Count(out, "import {") != 1 {
		t.Errorf("populated blueprint and its import block must survive:\n%s", out)
	}
}

// Nothing to strip must leave the file untouched, so the single-env pipeline's
// own skip stays the only place that reports one.
func TestStripBlueprintDraftsNoop(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	src := `
resource "jamfplatform_blueprints_blueprint" "real" {
  device_groups = ["uuid-1"]
}
`
	if err := os.WriteFile(genPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := StripBlueprintDrafts(genPath)
	if err != nil || n != 0 {
		t.Fatalf("removed = %d, err = %v; want 0, nil", n, err)
	}
	if readFile(t, genPath) != src {
		t.Error("file should be byte-identical when nothing is stripped")
	}
}
