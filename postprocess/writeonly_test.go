// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

// writeOnlySchema mirrors the real jamfplatform flags for the Required WriteOnly
// secrets and their _wo_version companions (wire-confirmed against the provider).
func writeOnlySchema() *tfjson.ProviderSchemas {
	return &tfjson.ProviderSchemas{
		FormatVersion: "1.0",
		Schemas: map[string]*tfjson.ProviderSchema{
			"registry.terraform.io/Jamf-Concepts/jamfplatform": {
				ResourceSchemas: map[string]*tfjson.Schema{
					"jamfplatform_pro_automated_device_enrollment": {
						Block: &tfjson.SchemaBlock{
							Attributes: map[string]*tfjson.SchemaAttribute{
								"name":                    {Required: true, AttributeType: cty.String},
								"server_token":            {Required: true, Sensitive: true, WriteOnly: true, AttributeType: cty.String},
								"server_token_wo_version": {Required: true, AttributeType: cty.Number},
								"site_id":                 {Optional: true, Computed: true, AttributeType: cty.String},
							},
						},
					},
					"jamfplatform_pro_supervision_identity": {
						Block: &tfjson.SchemaBlock{
							Attributes: map[string]*tfjson.SchemaAttribute{
								"display_name": {Required: true, AttributeType: cty.String},
								"password":     {Required: true, Sensitive: true, WriteOnly: true, AttributeType: cty.String},
							},
						},
					},
				},
			},
		},
	}
}

const writeOnlyFixture = `
resource "jamfplatform_pro_automated_device_enrollment" "abm" {
  name                    = "Apple Business Manager"
  server_token            = null
  server_token_wo_version = null
  site_id                 = "-1"
}

resource "jamfplatform_pro_supervision_identity" "ident" {
  display_name = "Test"
  password     = null
}
`

func TestInjectRequiredWriteOnly(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(writeOnlyFixture), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	opts := &ProcessOptions{
		TypeToFileMap: map[string]string{
			"jamfplatform_pro_automated_device_enrollment": "pro_automated_device_enrollment.tf",
			"jamfplatform_pro_supervision_identity":        "pro_supervision_identity.tf",
		},
		ProviderSchemas:         writeOnlySchema(),
		InjectRequiredWriteOnly: true,
	}
	if err := Process(dir, genPath, reg, opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	ade := readFile(t, filepath.Join(dir, "pro_automated_device_enrollment.tf"))
	if strings.Contains(ade, "server_token            = null") || strings.Contains(ade, "server_token = null") {
		t.Errorf("server_token should no longer be null:\n%s", ade)
	}
	if !strings.Contains(ade, "server_token") || !strings.Contains(ade, "var.jamfplatform_pro_automated_device_enrollment_abm_server_token") {
		t.Errorf("server_token should be wired to its sensitive var:\n%s", ade)
	}
	if !strings.Contains(ade, "server_token_wo_version = 1") {
		t.Errorf("server_token_wo_version should be seeded to 1:\n%s", ade)
	}
	if !strings.Contains(ade, `site_id                 = "-1"`) {
		t.Errorf("site_id should be left untouched:\n%s", ade)
	}

	ident := readFile(t, filepath.Join(dir, "pro_supervision_identity.tf"))
	if !strings.Contains(ident, "var.jamfplatform_pro_supervision_identity_ident_password") {
		t.Errorf("password should be wired to its sensitive var:\n%s", ident)
	}

	vars := readFile(t, filepath.Join(dir, "variables.tf"))
	for _, want := range []string{
		`variable "jamfplatform_pro_automated_device_enrollment_abm_server_token"`,
		`variable "jamfplatform_pro_supervision_identity_ident_password"`,
		"sensitive   = true",
	} {
		if !strings.Contains(vars, want) {
			t.Errorf("variables.tf missing %q:\n%s", want, vars)
		}
	}

	tfvars := readFile(t, filepath.Join(dir, "terraform.tfvars"))
	if !strings.Contains(tfvars, "# jamfplatform_pro_automated_device_enrollment_abm_server_token = \"REPLACE_ME\"") {
		t.Errorf("terraform.tfvars missing commented placeholder:\n%s", tfvars)
	}
}

// TestInjectRequiredWriteOnly_DisabledByDefault confirms the injection is opt-in:
// with the flag off, a null Required WriteOnly attribute is left as-is.
func TestInjectRequiredWriteOnly_DisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	genPath := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genPath, []byte(writeOnlyFixture), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	opts := &ProcessOptions{
		TypeToFileMap: map[string]string{
			"jamfplatform_pro_automated_device_enrollment": "pro_automated_device_enrollment.tf",
			"jamfplatform_pro_supervision_identity":        "pro_supervision_identity.tf",
		},
		ProviderSchemas: writeOnlySchema(),
	}
	if err := Process(dir, genPath, reg, opts); err != nil {
		t.Fatalf("Process: %v", err)
	}

	ade := readFile(t, filepath.Join(dir, "pro_automated_device_enrollment.tf"))
	if !strings.Contains(ade, "server_token            = null") {
		t.Errorf("with flag off, server_token should remain null:\n%s", ade)
	}
	if _, err := os.Stat(filepath.Join(dir, "variables.tf")); !os.IsNotExist(err) {
		t.Errorf("no variables.tf should be written when injection is off")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}
