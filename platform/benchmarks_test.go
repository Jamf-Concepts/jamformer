// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestStripComplianceBenchmarkArtifacts(t *testing.T) {
	dir := t.TempDir()
	// "55" is the benchmark category's Jamf ID; the benchmark script references it
	// (no name prefix). The user category "Security" (id 99) and its policy must
	// survive.
	generated := `import {
  to       = jamfplatform_pro_category.cis_v8
  identity = { id = "55" }
}
import {
  to       = jamfplatform_pro_category.security
  identity = { id = "99" }
}
import {
  to       = jamfplatform_pro_script.bench_audit
  identity = { id = "200" }
}
import {
  to       = jamfplatform_pro_policy.bench_audit_policy
  identity = { id = "300" }
}
import {
  to       = jamfplatform_pro_policy.user_policy
  identity = { id = "301" }
}

resource "jamfplatform_cbengine_benchmark" "cis_v8" {
  title = "CIS v8"
}

resource "jamfplatform_pro_category" "cis_v8" {
  name = "CIS v8"
}

resource "jamfplatform_pro_category" "security" {
  name = "Security"
}

resource "jamfplatform_pro_computer_extension_attribute" "cis_v8_failed" {
  name = "CIS v8 - Failed Result List"
}

resource "jamfplatform_pro_computer_extension_attribute" "uptime" {
  name = "Uptime"
}

resource "jamfplatform_device_group" "cis_v8_compliant" {
  name = "CIS v8 - Compliant"
}

resource "jamfplatform_pro_script" "bench_audit" {
  category_id = "55"
  name        = "compliance_benchmark_abc_sequoia.sh"
}

resource "jamfplatform_pro_policy" "bench_audit_policy" {
  general = {
    name        = "CIS v8 - Sequoia Audit policy"
    category_id = "55"
  }
}

resource "jamfplatform_pro_policy" "user_policy" {
  general = {
    name        = "Install Chrome"
    category_id = "99"
  }
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	reg.Register("jamfplatform_pro_category", "55", "jamfplatform_pro_category.cis_v8")
	reg.Register("jamfplatform_pro_script", "200", "jamfplatform_pro_script.bench_audit")

	n, err := StripComplianceBenchmarkArtifacts(genFile, reg)
	if err != nil {
		t.Fatalf("strip: %v", err)
	}
	// Stripped: cis_v8 category, cis_v8_failed EA, cis_v8_compliant group,
	// bench_audit script (category match), bench_audit_policy (name + category).
	if n != 5 {
		t.Errorf("expected 5 stripped, got %d", n)
	}

	out, _ := os.ReadFile(genFile)
	s := string(out)

	mustKeep := []string{
		`resource "jamfplatform_cbengine_benchmark" "cis_v8"`, // the benchmark itself
		`resource "jamfplatform_pro_category" "security"`,
		`resource "jamfplatform_pro_computer_extension_attribute" "uptime"`,
		`resource "jamfplatform_pro_policy" "user_policy"`,
		`to       = jamfplatform_pro_policy.user_policy`,
	}
	for _, m := range mustKeep {
		if !strings.Contains(s, m) {
			t.Errorf("expected to keep %q", m)
		}
	}

	mustStrip := []string{
		`resource "jamfplatform_pro_category" "cis_v8"`,
		`"jamfplatform_pro_computer_extension_attribute" "cis_v8_failed"`,
		`"jamfplatform_device_group" "cis_v8_compliant"`,
		`resource "jamfplatform_pro_script" "bench_audit"`,
		`resource "jamfplatform_pro_policy" "bench_audit_policy"`,
		`to       = jamfplatform_pro_category.cis_v8`,
		`to       = jamfplatform_pro_script.bench_audit`,
	}
	for _, m := range mustStrip {
		if strings.Contains(s, m) {
			t.Errorf("expected to strip %q", m)
		}
	}

	// Stripped resources must be deregistered so references fall back to raw IDs.
	if _, ok := reg.Resolve("jamfplatform_pro_category", "55"); ok {
		t.Error("benchmark category should be deregistered")
	}
	if _, ok := reg.Resolve("jamfplatform_pro_script", "200"); ok {
		t.Error("benchmark script should be deregistered")
	}
}

func TestStripComplianceBenchmarkArtifactsNoBenchmarks(t *testing.T) {
	dir := t.TempDir()
	generated := `resource "jamfplatform_pro_category" "cis_v8" {
  name = "CIS v8"
}
`
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	// No cbengine_benchmark present → nothing is stripped, even though a category
	// happens to be named "CIS v8".
	n, err := StripComplianceBenchmarkArtifacts(genFile, registry.New())
	if err != nil {
		t.Fatalf("strip: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 stripped without a benchmark, got %d", n)
	}
}
