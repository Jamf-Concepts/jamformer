// Copyright 2026, Jamf Software LLC

package multienv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderFor(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{"", "jamfpro", false},
		{"jamfpro", "jamfpro", false},
		{"jamfplatform", "jamfplatform", false},
		{"jamfprotect", "", true},
		{"jsc", "", true},
	}
	for _, c := range cases {
		prov, err := providerFor(c.name)
		if c.wantErr {
			if err == nil {
				t.Errorf("providerFor(%q): expected error", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("providerFor(%q): unexpected error %v", c.name, err)
			continue
		}
		if prov.Name() != c.want {
			t.Errorf("providerFor(%q).Name() = %q, want %q", c.name, prov.Name(), c.want)
		}
	}
}

func TestPlatformProviderBlocks(t *testing.T) {
	prov := platformProvider{}
	env := EnvConfig{
		Name:     "prod",
		URL:      "https://eu.apigw.jamf.com",
		TenantID: "tenant-123",
	}

	header := prov.EnvProviderHeader(env, ` version = "0.19.0"`, 0)
	for _, want := range []string{
		`source = "Jamf-Concepts/jamfplatform"`,
		`provider "jamfplatform"`,
		"base_url      = var.jamfplatform_base_url",
		"client_id     = var.jamfplatform_client_id",
		"client_secret = var.jamfplatform_client_secret",
		"tenant_id     = var.jamfplatform_tenant_id",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("EnvProviderHeader missing %q\n%s", want, header)
		}
	}
	// jamfplatform has no token_refresh attribute — it must never appear.
	if strings.Contains(header, "token_refresh") {
		t.Errorf("EnvProviderHeader should not contain token_refresh:\n%s", header)
	}

	vars := prov.EnvAuthVariables(env)
	for _, want := range []string{
		`variable "jamfplatform_base_url"`,
		`default     = "https://eu.apigw.jamf.com"`,
		`variable "jamfplatform_client_id"`,
		`variable "jamfplatform_client_secret"`,
		`variable "jamfplatform_tenant_id"`,
		`default     = "tenant-123"`,
		"sensitive   = true",
	} {
		if !strings.Contains(vars, want) {
			t.Errorf("EnvAuthVariables missing %q\n%s", want, vars)
		}
	}

	mod := prov.ModuleProvidersBlock("")
	if !strings.Contains(mod, `source = "Jamf-Concepts/jamfplatform"`) {
		t.Errorf("ModuleProvidersBlock missing source:\n%s", mod)
	}
}

func TestPlatformResourceRefsMapping(t *testing.T) {
	refs := platformResourceRefs(nil)
	if len(refs) != 0 {
		t.Errorf("expected empty refs for nil input, got %d", len(refs))
	}
}

func TestModuleResourceAddrs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.tf"), []byte(`resource "jamfplatform_pro_policy" "p1" {
  general = { name = "P1" }
}
resource "jamfplatform_pro_category" "c1" {
  name = "C1"
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	// variables.tf must not contribute resource addresses.
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "x" { type = string }
`), 0644); err != nil {
		t.Fatal(err)
	}

	addrs := moduleResourceAddrs(dir)
	if !addrs["jamfplatform_pro_policy.p1"] || !addrs["jamfplatform_pro_category.c1"] {
		t.Errorf("expected both resources, got %v", addrs)
	}
	if addrs["jamfplatform_pro_policy.skipped"] {
		t.Error("did not expect a resource that is not present")
	}
	if len(addrs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addrs))
	}
}
