// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

const blueprintFixture = `
resource "jamfplatform_blueprints_blueprint" "draft" {
  name          = "Empty Draft"
  device_groups = []
}

resource "jamfplatform_blueprints_blueprint" "real" {
  name          = "Real Blueprint"
  device_groups = ["uuid-1", "uuid-2"]
}

import {
  to = jamfplatform_blueprints_blueprint.draft
  id = "draft-uuid"
}

import {
  to = jamfplatform_blueprints_blueprint.real
  id = "real-uuid"
}
`

// TestSkipEmptyBlueprintDraft confirms a blueprint with empty device_groups is
// dropped (resource and its import block) while a populated one is kept.
func TestSkipEmptyBlueprintDraft(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(blueprintFixture), 0644); err != nil {
		t.Fatal(err)
	}

	opts := &ProcessOptions{
		TypeToFileMap: map[string]string{"jamfplatform_blueprints_blueprint": "blueprints.tf"},
	}
	if err := Process(dir, genPath, registry.New(), opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	out := readFile(t, filepath.Join(dir, "blueprints.tf"))
	if strings.Contains(out, `"draft"`) {
		t.Errorf("empty-device_groups draft should be skipped:\n%s", out)
	}
	if !strings.Contains(out, `"real"`) {
		t.Errorf("populated blueprint should be kept:\n%s", out)
	}

	imports := readFile(t, filepath.Join(dir, "blueprints_import.tf"))
	if strings.Contains(imports, "jamfplatform_blueprints_blueprint.draft") {
		t.Errorf("draft import block should be dropped:\n%s", imports)
	}
	if !strings.Contains(imports, "jamfplatform_blueprints_blueprint.real") {
		t.Errorf("real import block should be kept:\n%s", imports)
	}
}

func TestHasEmptyDeviceGroups(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"absent", `resource "x" "y" {}`, true},
		{"empty", `resource "x" "y" { device_groups = [] }`, true},
		{"empty multiline", "resource \"x\" \"y\" {\n  device_groups = [\n  ]\n}", true},
		{"null", `resource "x" "y" { device_groups = null }`, true},
		{"populated", `resource "x" "y" { device_groups = ["a"] }`, false},
		{"populated refs", `resource "x" "y" { device_groups = [jamfplatform_device_group.g.id] }`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, diags := hclwrite.ParseConfig([]byte(tc.body), "t.tf", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatalf("parse: %s", diags.Error())
			}
			body := f.Body().Blocks()[0].Body()
			if got := hasEmptyDeviceGroups(body); got != tc.want {
				t.Errorf("hasEmptyDeviceGroups(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
