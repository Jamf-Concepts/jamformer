// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSingletonImportsUsesIdentityForm(t *testing.T) {
	dir := t.TempDir()
	wrote, err := WriteSingletonImports(dir, nil)
	if err != nil {
		t.Fatalf("WriteSingletonImports: %v", err)
	}
	if !wrote {
		t.Fatal("expected singleton import blocks to be written")
	}

	b, err := os.ReadFile(filepath.Join(dir, "singletons_import.tf"))
	if err != nil {
		t.Fatalf("reading singletons_import.tf: %v", err)
	}
	got := string(b)

	// The identity form is what `plan -generate-config-out` emits, and it is
	// the only form on which a singleton that is not configured on the tenant
	// reports plainly instead of raising a framework-level "Missing Resource
	// Identity After Read". A revert to the flat form has to fail here.
	if !strings.Contains(got, `identity = { id = "singleton" }`) {
		t.Errorf("expected the identity import form, got:\n%s", got)
	}
	// The flat form must not appear. Matched with the surrounding whitespace so
	// the assertion cannot be satisfied by the identity block's own `id =`.
	for _, flat := range []string{"\n  id = \"singleton\"", "\n  id  = \"singleton\""} {
		if strings.Contains(got, flat) {
			t.Errorf("flat id form present, which raises a framework identity error for an unconfigured singleton:\n%s", got)
		}
	}

	// One block per singleton, each targeting the "singleton" label.
	if n := strings.Count(got, "import {"); n != len(SingletonResources()) {
		t.Errorf("got %d import blocks, want %d", n, len(SingletonResources()))
	}
	// hclwrite aligns the `=` across a block's attributes, so the assertion
	// cannot assume single spacing.
	for _, r := range SingletonResources() {
		if !strings.Contains(got, r.TFType+".singleton") {
			t.Errorf("missing import target for %s", r.TFType)
		}
	}
}

func TestWriteSingletonImportsHonoursSelection(t *testing.T) {
	dir := t.TempDir()
	wrote, err := WriteSingletonImports(dir, map[string]bool{"smtp_server": true})
	if err != nil {
		t.Fatalf("WriteSingletonImports: %v", err)
	}
	if !wrote {
		t.Fatal("expected an import block for the selected singleton")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "singletons_import.tf"))
	got := string(b)
	if n := strings.Count(got, "import {"); n != 1 {
		t.Errorf("got %d import blocks, want 1", n)
	}
	if !strings.Contains(got, "jamfplatform_pro_smtp_server.singleton") {
		t.Errorf("selected singleton missing:\n%s", got)
	}

	// A selection matching no singleton writes nothing at all, rather than an
	// empty file the plan step would still be asked to read.
	dir2 := t.TempDir()
	wrote2, err := WriteSingletonImports(dir2, map[string]bool{"policy": true})
	if err != nil {
		t.Fatalf("WriteSingletonImports: %v", err)
	}
	if wrote2 {
		t.Error("expected no import file when the selection contains no singleton")
	}
	if _, err := os.Stat(filepath.Join(dir2, "singletons_import.tf")); !os.IsNotExist(err) {
		t.Error("singletons_import.tf should not exist when nothing was selected")
	}
}
