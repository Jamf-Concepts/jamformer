// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

const platformPackageFixture = `
resource "jamfplatform_pro_package" "p1" {
  display_name = "Pkg One"
  file_name    = "pkg-one.pkg"
  sha3_512     = "aaa"
  sha256       = "bbb"
  md5          = "ccc"
  hash_type    = "SHA3_512"
  hash_value   = "aaa"
  priority     = 10
}

resource "jamfplatform_pro_package" "p2" {
  display_name = "Pkg Two"
  file_name    = "pkg-two.pkg"
  sha3_512     = "zzz"
  hash_type    = "SHA3_512"
}
`

func platformPackageTypeMap() map[string]string {
	return map[string]string{"jamfplatform_pro_package": "pro_package.tf"}
}

// TestPlatformPackageUploadMode confirms that a package whose file_name matches a
// downloaded file switches to upload mode (package_file_source = file(...)) with
// the conflicting hash attributes removed, while a package with no matching file
// is left as metadata + hashes.
func TestPlatformPackageUploadMode(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(platformPackageFixture), 0644); err != nil {
		t.Fatal(err)
	}

	opts := &ProcessOptions{
		TypeToFileMap: platformPackageTypeMap(),
		PlatformPackageFiles: map[string]string{
			"pkg-one.pkg": "support_files/packages/pkg-one.pkg",
		},
	}
	if err := Process(dir, genPath, registry.New(), opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	out := readFile(t, filepath.Join(dir, "pro_package.tf"))

	// p1: downloaded -> upload mode, hashes stripped. The hash values "aaa"/"bbb"/
	// "ccc" are p1-specific, so their absence proves the strip without touching p2.
	if !strings.Contains(out, `package_file_source = "${path.module}/support_files/packages/pkg-one.pkg"`) {
		t.Errorf("p1 should reference the downloaded file path:\n%s", out)
	}
	for _, gone := range []string{`"aaa"`, `"bbb"`, `"ccc"`} {
		if strings.Contains(out, gone) {
			t.Errorf("p1 hash value %s should be stripped in upload mode:\n%s", gone, out)
		}
	}

	// p2: no matching file -> left as metadata + hashes, no package_file_source.
	p2Start := strings.Index(out, `"p2"`)
	if p2Start < 0 {
		t.Fatalf("p2 block missing:\n%s", out)
	}
	p2 := out[p2Start:]
	if strings.Contains(p2, "package_file_source") {
		t.Errorf("p2 should not have package_file_source (no downloaded file):\n%s", p2)
	}
	if !strings.Contains(p2, `sha3_512     = "zzz"`) {
		t.Errorf("p2 should retain its supplied hash:\n%s", p2)
	}
}

// TestPlatformPackageNoDownloads confirms that with no downloaded files the
// package blocks are passed through unchanged (supplied-hashes mode).
func TestPlatformPackageNoDownloads(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(platformPackageFixture), 0644); err != nil {
		t.Fatal(err)
	}

	opts := &ProcessOptions{TypeToFileMap: platformPackageTypeMap()}
	if err := Process(dir, genPath, registry.New(), opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	out := readFile(t, filepath.Join(dir, "pro_package.tf"))
	if strings.Contains(out, "package_file_source") {
		t.Errorf("no package_file_source should be injected without downloads:\n%s", out)
	}
	if !strings.Contains(out, `sha3_512     = "aaa"`) || !strings.Contains(out, `sha3_512     = "zzz"`) {
		t.Errorf("both packages should retain their hashes:\n%s", out)
	}
}
