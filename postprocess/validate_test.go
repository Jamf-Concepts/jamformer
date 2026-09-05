// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

func TestExtractAttrName(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		detail  string
		want    string
	}{
		{
			name:    "attribute in summary",
			summary: "Attribute secondary_auth_required Is Not Valid For CDN Type",
			detail:  "Attribute secondary_auth_required only when cdn_type is AKAMAI. Remove it or change cdn_type.",
			want:    "secondary_auth_required",
		},
		{
			name:    "attribute only in detail",
			summary: "Invalid configuration",
			detail:  "Attribute expiration_seconds only when cdn_type is AMAZON_S3. Remove it or change cdn_type.",
			want:    "expiration_seconds",
		},
		{
			name:    "no attribute name",
			summary: "Something went wrong",
			detail:  "An error occurred",
			want:    "",
		},
		{
			name:    "case insensitive",
			summary: "attribute require_signed_urls Is Not Valid",
			detail:  "Remove it.",
			want:    "require_signed_urls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAttrName(tt.summary, tt.detail)
			if got != tt.want {
				t.Errorf("extractAttrName(%q, %q) = %q, want %q", tt.summary, tt.detail, got, tt.want)
			}
		})
	}
}

func TestClassifyFix(t *testing.T) {
	tests := []struct {
		name      string
		summary   string
		detail    string
		wantNil   bool
		wantAttr  string
		wantValue string // empty = remove
	}{
		{
			name:      "remove it",
			summary:   "Attribute secondary_auth_required Is Not Valid For CDN Type",
			detail:    "Attribute secondary_auth_required only when cdn_type is AKAMAI. Remove it or change cdn_type.",
			wantAttr:  "secondary_auth_required",
			wantValue: "",
		},
		{
			name:      "must be value",
			summary:   "in 'jamfpro_computer_prestage_enrollment.shared': 'recovery_lock_password_type' must be 'MANUAL' when 'enable_recovery_lock' is false (this is the default value)",
			detail:    "'recovery_lock_password_type' must be 'MANUAL' when 'enable_recovery_lock' is false (this is the default value)",
			wantAttr:  "recovery_lock_password_type",
			wantValue: "MANUAL",
		},
		{
			name:      "must be one of",
			summary:   `"authentication_type" must be one of [BASIC HEADER], got: NONE`,
			detail:    "",
			wantAttr:  "authentication_type",
			wantValue: "",
		},
		{
			name:      "conflicts with",
			summary:   "Conflicting configuration arguments",
			detail:    `"expiration_interval_days": conflicts with expiration_interval_seconds`,
			wantAttr:  "expiration_interval_days",
			wantValue: "",
		},
		{
			name:    "unrecognised error",
			summary: "Something went wrong",
			detail:  "An error occurred",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fix := classifyFix(tt.summary, tt.detail, "/tmp/test.tf", 1, nil)
			if tt.wantNil {
				if fix != nil {
					t.Errorf("expected nil fix, got %+v", fix)
				}
				return
			}
			if fix == nil {
				t.Fatal("expected non-nil fix, got nil")
			}
			if fix.attrName != tt.wantAttr {
				t.Errorf("attrName = %q, want %q", fix.attrName, tt.wantAttr)
			}
			if fix.newValue != tt.wantValue {
				t.Errorf("newValue = %q, want %q", fix.newValue, tt.wantValue)
			}
		})
	}
}

func TestResourceAtLine(t *testing.T) {
	src := []byte(`resource "jamfpro_cloud_distribution_point" "settings" {
  cdn_type                   = "JAMFCLOUD"
  secondary_auth_required    = false
  secondary_auth_status_code = 0
  require_signed_urls        = false
}

resource "jamfpro_policy" "test" {
  name = "test"
}
`)

	tests := []struct {
		name     string
		line     int
		wantType string
		wantName string
	}{
		{
			name:     "first resource at declaration line",
			line:     1,
			wantType: "jamfpro_cloud_distribution_point",
			wantName: "settings",
		},
		{
			name:     "first resource at attribute line",
			line:     3,
			wantType: "jamfpro_cloud_distribution_point",
			wantName: "settings",
		},
		{
			name:     "second resource",
			line:     8,
			wantType: "jamfpro_policy",
			wantName: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotName := resourceAtLine(src, tt.line)
			if gotType != tt.wantType || gotName != tt.wantName {
				t.Errorf("resourceAtLine(src, %d) = (%q, %q), want (%q, %q)",
					tt.line, gotType, gotName, tt.wantType, tt.wantName)
			}
		})
	}
}

func TestRemoveAttributeFromFile(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.tf"

	content := `resource "jamfpro_cloud_distribution_point" "settings" {
  cdn_type                   = "JAMFCLOUD"
  secondary_auth_required    = false
  secondary_auth_status_code = 0
  require_signed_urls        = false
  expiration_seconds         = 600
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Remove secondary_auth_required (line 1 is resource declaration)
	if !removeAttributeFromFile(filePath, 1, "secondary_auth_required") {
		t.Fatal("removeAttributeFromFile returned false, expected true")
	}

	result, _ := os.ReadFile(filePath)
	if strings.Contains(string(result), "secondary_auth_required") {
		t.Error("secondary_auth_required should have been removed")
	}
	if !strings.Contains(string(result), "cdn_type") {
		t.Error("cdn_type should still be present")
	}
	if !strings.Contains(string(result), "secondary_auth_status_code") {
		t.Error("secondary_auth_status_code should still be present")
	}

	// Remove a second attribute
	if !removeAttributeFromFile(filePath, 1, "expiration_seconds") {
		t.Fatal("removeAttributeFromFile returned false for expiration_seconds")
	}

	result, _ = os.ReadFile(filePath)
	if strings.Contains(string(result), "expiration_seconds") {
		t.Error("expiration_seconds should have been removed")
	}

	// Try to remove non-existent attribute
	if removeAttributeFromFile(filePath, 1, "nonexistent") {
		t.Error("removeAttributeFromFile should return false for non-existent attribute")
	}
}

func TestRemoveAttributeFromFileMultipleResources(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.tf"

	content := `resource "jamfpro_cloud_distribution_point" "settings" {
  cdn_type                = "JAMFCLOUD"
  secondary_auth_required = false
}

resource "jamfpro_cloud_distribution_point" "other" {
  cdn_type                = "AKAMAI"
  secondary_auth_required = true
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Remove from the second resource (line 6 is where the second resource starts)
	if !removeAttributeFromFile(filePath, 6, "secondary_auth_required") {
		t.Fatal("removeAttributeFromFile returned false")
	}

	result, _ := os.ReadFile(filePath)
	// The first resource should still have secondary_auth_required
	count := strings.Count(string(result), "secondary_auth_required")
	if count != 1 {
		t.Errorf("expected 1 occurrence of secondary_auth_required after removing from second resource, got %d", count)
	}
}

func TestSetAttributeInFile(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.tf"

	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  enable_recovery_lock       = false
  recovery_lock_password_type = "RANDOM"
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if !setAttributeInFile(filePath, 1, "recovery_lock_password_type", "MANUAL") {
		t.Fatal("setAttributeInFile returned false, expected true")
	}

	result, _ := os.ReadFile(filePath)
	body := string(result)
	if !strings.Contains(body, `"MANUAL"`) {
		t.Errorf("expected recovery_lock_password_type set to MANUAL, got:\n%s", body)
	}
	if strings.Contains(body, `"RANDOM"`) {
		t.Error("RANDOM should have been replaced")
	}
	if !strings.Contains(body, "enable_recovery_lock") {
		t.Error("enable_recovery_lock should still be present")
	}
}

func TestSetAttributeInFileMultipleResources(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.tf"

	content := `resource "jamfpro_computer_prestage_enrollment" "first" {
  enable_recovery_lock       = false
  recovery_lock_password_type = "RANDOM"
}

resource "jamfpro_computer_prestage_enrollment" "second" {
  enable_recovery_lock       = true
  recovery_lock_password_type = "RANDOM"
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Set only in the first resource (line 1)
	if !setAttributeInFile(filePath, 1, "recovery_lock_password_type", "MANUAL") {
		t.Fatal("setAttributeInFile returned false")
	}

	result, _ := os.ReadFile(filePath)
	body := string(result)
	if !strings.Contains(body, `"MANUAL"`) {
		t.Error("expected MANUAL in first resource")
	}
	// The second resource should still have RANDOM
	count := strings.Count(body, `"RANDOM"`)
	if count != 1 {
		t.Errorf("expected 1 occurrence of RANDOM (in second resource), got %d", count)
	}
}

func TestSetAttributeInFileNonExistent(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.tf"

	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  enable_recovery_lock = false
}
`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if setAttributeInFile(filePath, 1, "recovery_lock_password_type", "MANUAL") {
		t.Error("setAttributeInFile should return false for non-existent attribute")
	}
}

func TestReplaceNullWithVar_TopLevel(t *testing.T) {
	src := `resource "jamfpro_activation_code" "settings" {
  code              = null
  organization_name = "JAMF Software"
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	if !replaceNullWithVar(f, "jamfpro_activation_code", "settings", "code", "activation_code_code") {
		t.Fatal("replaceNullWithVar returned false")
	}

	result := string(f.Bytes())
	if !strings.Contains(result, "var.activation_code_code") {
		t.Errorf("expected var.activation_code_code, got:\n%s", result)
	}
	if strings.Contains(result, "null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(result, "organization_name") {
		t.Error("other attributes should be preserved")
	}
}

func TestReplaceNullWithVar_Nested(t *testing.T) {
	src := `resource "jamfpro_smtp_server" "settings" {
  authentication_type = "BASIC"

  basic_auth_credentials {
    password = null
    username = "admin@example.com"
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	if !replaceNullWithVar(f, "jamfpro_smtp_server", "settings", "basic_auth_credentials.0.password", "smtp_server_password") {
		t.Fatal("replaceNullWithVar returned false")
	}

	result := string(f.Bytes())
	if !strings.Contains(result, "var.smtp_server_password") {
		t.Errorf("expected var.smtp_server_password, got:\n%s", result)
	}
	if strings.Contains(result, "null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(result, "username") {
		t.Error("other attributes in block should be preserved")
	}
}

func TestReplaceNullWithVar_NotNull(t *testing.T) {
	src := `resource "jamfpro_activation_code" "settings" {
  code = "REAL_VALUE"
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	if replaceNullWithVar(f, "jamfpro_activation_code", "settings", "code", "activation_code_code") {
		t.Error("should return false when value is not null")
	}
}

func TestLeafAttrName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"code", "code"},
		{"basic_auth_credentials.0.password", "password"},
		{"a.0.b.0.c", "c"},
	}
	for _, tt := range tests {
		if got := leafAttrName(tt.path); got != tt.want {
			t.Errorf("leafAttrName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestAttrBlockPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"code", ""},
		// SDKv2 shape: every nested block carries a list index, so the block
		// path is every other element.
		{"basic_auth_credentials.0.password", "basic_auth_credentials"},
		{"a.0.b.0.c", "a.b"},
		// Plugin-framework nested attributes are plain dotted paths, and
		// ProviderSchema.attrs is keyed on the path minus its leaf. Stepping
		// in pairs here returned "a" and missed the "a.b" key.
		{"sender_settings.email_address", "sender_settings"},
		{"a.b.c", "a.b"},
		{"a.b.c.d", "a.b.c"},
	}
	for _, tt := range tests {
		if got := attrBlockPath(tt.path); got != tt.want {
			t.Errorf("attrBlockPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestStripProviderPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"jamfpro_smtp_server", "smtp_server"},
		{"jamfpro_activation_code", "activation_code"},
		{"jamfprotect_role", "role"},
		{"noprefixhere", "noprefixhere"},
	}
	for _, tt := range tests {
		if got := stripProviderPrefix(tt.input); got != tt.want {
			t.Errorf("stripProviderPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFixRequiredNulls_SensitiveGetsVar(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "smtp_server.tf")
	content := `resource "jamfpro_smtp_server" "settings" {
  authentication_type = "BASIC"

  basic_auth_credentials {
    password = null
    username = "admin@example.com"
  }
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_smtp_server": {
				"basic_auth_credentials": {
					"password": {Required: true, Sensitive: true, Type: cty.String},
				},
			},
		},
	}

	diags := []requiredNullDiag{{
		attrPath: "basic_auth_credentials.0.password",
		filePath: filePath,
		filename: "smtp_server.tf",
		line:     1,
	}}

	vars, edits := fixRequiredNulls(dir, diags, schema)
	if len(edits) != 1 {
		t.Errorf("expected 1 edit, got %d", len(edits))
	}
	if len(vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(vars))
	}
	if edits[0].Resource != "jamfpro_smtp_server.settings" || edits[0].Attr != "basic_auth_credentials.0.password" {
		t.Errorf("edit does not name what was changed: %+v", edits[0])
	}
	if vars[0].VarName != "smtp_server_settings_password" {
		t.Errorf("expected var name smtp_server_settings_password, got %q", vars[0].VarName)
	}

	result, _ := os.ReadFile(filePath)
	if !strings.Contains(string(result), "var.smtp_server_settings_password") {
		t.Errorf("expected variable reference, got:\n%s", result)
	}
}

func TestFixRequiredNulls_NonSensitiveGetsZeroValue(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "prestage.tf")
	content := `resource "jamfpro_computer_prestage_enrollment" "test" {
  display_name         = "Test"
  authentication_prompt = null
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{
		attrs: map[string]map[string]map[string]attrInfo{
			"jamfpro_computer_prestage_enrollment": {
				"": {
					"authentication_prompt": {Required: true, Type: cty.String},
				},
			},
		},
	}

	diags := []requiredNullDiag{{
		attrPath: "authentication_prompt",
		filePath: filePath,
		filename: "prestage.tf",
		line:     1,
	}}

	vars, edits := fixRequiredNulls(dir, diags, schema)
	if len(edits) != 1 {
		t.Errorf("expected 1 edit, got %d", len(edits))
	}
	if len(vars) != 0 {
		t.Errorf("expected 0 vars for non-sensitive, got %d", len(vars))
	}
	if len(edits) == 1 && edits[0].Action != `set to ""` {
		t.Errorf("expected the edit to name the value written, got %q", edits[0].Action)
	}

	result, _ := os.ReadFile(filePath)
	if strings.Contains(string(result), "null") {
		t.Error("null should have been replaced")
	}
	if !strings.Contains(string(result), `authentication_prompt = ""`) {
		t.Errorf("expected empty string, got:\n%s", result)
	}
}

func TestAppendVariables(t *testing.T) {
	dir := t.TempDir()

	// Write initial variables.tf
	initial := `variable "existing" {
  type = string
}
`
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	vars := []RequiredVar{
		{VarName: "activation_code_code", AttrPath: "code", Resource: "jamfpro_activation_code.settings"},
		{VarName: "smtp_server_password", AttrPath: "basic_auth_credentials.0.password", Resource: "jamfpro_smtp_server.settings"},
	}

	appendVariables(dir, vars)

	result, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `variable "activation_code_code"`) {
		t.Error("expected activation_code_code variable")
	}
	if !strings.Contains(body, `variable "smtp_server_password"`) {
		t.Error("expected smtp_server_password variable")
	}
	if !strings.Contains(body, "sensitive   = true") {
		t.Error("expected sensitive = true")
	}
	if !strings.Contains(body, `variable "existing"`) {
		t.Error("existing variable should be preserved")
	}

	// Append again — should not duplicate
	appendVariables(dir, vars)
	result2, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if strings.Count(string(result2), `variable "activation_code_code"`) != 1 {
		t.Error("variable should not be duplicated on second append")
	}
}

func TestAppendVariables_CreatesFileIfMissing(t *testing.T) {
	dir := t.TempDir()
	// No variables.tf exists yet

	vars := []RequiredVar{
		{VarName: "new_var", AttrPath: "password", Resource: "jamfpro_smtp_server.settings"},
	}

	appendVariables(dir, vars)

	result, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatal(err)
	}

	body := string(result)
	if !strings.Contains(body, `variable "new_var"`) {
		t.Error("expected new_var variable in newly created file")
	}
	if !strings.Contains(body, "sensitive   = true") {
		t.Error("expected sensitive = true")
	}
}

func TestAppendVariables_DeduplicatesWithinBatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	vars := []RequiredVar{
		{VarName: "duplicate_var", AttrPath: "code", Resource: "jamfpro_activation_code.settings"},
		{VarName: "duplicate_var", AttrPath: "code", Resource: "jamfpro_activation_code.other"},
	}

	appendVariables(dir, vars)

	result, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	count := strings.Count(string(result), `variable "duplicate_var"`)
	if count != 1 {
		t.Errorf("expected 1 occurrence of duplicate_var, got %d", count)
	}
}

func TestAppendVariables_EmptyVarList(t *testing.T) {
	dir := t.TempDir()
	initial := `variable "existing" {
  type = string
}
`
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	appendVariables(dir, nil)

	result, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if string(result) != initial {
		t.Errorf("expected file unchanged, got:\n%s", result)
	}
}

func TestNavigateToBody_SimplePath(t *testing.T) {
	src := `resource "jamfpro_script" "test" {
  name = "test"
  code = null
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	body := f.Body().Blocks()[0].Body()

	// Simple path "code" should return the resource body directly
	result := navigateToBody(body, "code")
	if result == nil {
		t.Fatal("expected non-nil body for simple path")
	}

	if result.GetAttribute("code") == nil {
		t.Error("expected to find 'code' attribute in returned body")
	}
}

func TestNavigateToBody_NestedPath(t *testing.T) {
	src := `resource "jamfpro_smtp_server" "settings" {
  authentication_type = "BASIC"

  basic_auth_credentials {
    password = null
    username = "admin@example.com"
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	body := f.Body().Blocks()[0].Body()

	result := navigateToBody(body, "basic_auth_credentials.0.password")
	if result == nil {
		t.Fatal("expected non-nil body for nested path")
	}

	if result.GetAttribute("password") == nil {
		t.Error("expected to find 'password' attribute in nested body")
	}
	if result.GetAttribute("username") == nil {
		t.Error("expected to find 'username' attribute in nested body")
	}
}

func TestNavigateToBody_DeeplyNestedPath(t *testing.T) {
	src := `resource "jamfpro_policy" "test" {
  name = "test"

  account_maintenance {
    directory_bindings {
      id = "42"
    }
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	body := f.Body().Blocks()[0].Body()

	result := navigateToBody(body, "account_maintenance.0.directory_bindings.0.id")
	if result == nil {
		t.Fatal("expected non-nil body for deeply nested path")
	}

	if result.GetAttribute("id") == nil {
		t.Error("expected to find 'id' attribute in deeply nested body")
	}
}

func TestNavigateToBody_NonexistentBlock(t *testing.T) {
	src := `resource "jamfpro_script" "test" {
  name = "test"
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	body := f.Body().Blocks()[0].Body()

	result := navigateToBody(body, "nonexistent_block.0.attr")
	if result != nil {
		t.Error("expected nil for nonexistent block path")
	}
}

func TestNavigateToBody_PartiallyValidPath(t *testing.T) {
	src := `resource "jamfpro_policy" "test" {
  name = "test"

  account_maintenance {
    local_accounts {
      username = "admin"
    }
  }
}
`
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	body := f.Body().Blocks()[0].Body()

	// account_maintenance exists but missing_block does not
	result := navigateToBody(body, "account_maintenance.0.missing_block.0.attr")
	if result != nil {
		t.Error("expected nil when intermediate block doesn't exist")
	}
}

func TestSetAttributeValueAtLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.tf")
	content := `resource "jamfplatform_pro_patch_external_source" "s" {
  host_name = null
}

resource "jamfplatform_pro_mobile_device_prestage_enrollment" "p" {
  prevent_activation_lock                      = false
}

resource "jamfplatform_pro_disk_encryption_configuration" "d" {
  institutional_recovery_key = {
    data             = null # sensitive
  }
}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Wire a top-level required-null to a variable.
	if !setAttributeValueAtLine(path, 2, "var.host") {
		t.Fatal("expected host_name line rewritten")
	}
	// Set a bool conditional attribute.
	if !setAttributeValueAtLine(path, 6, "true") {
		t.Fatal("expected prevent_activation_lock line rewritten")
	}
	// Rewrite a nested attribute, preserving its trailing comment.
	if !setAttributeValueAtLine(path, 11, "var.dek") {
		t.Fatal("expected nested data line rewritten")
	}

	out, _ := os.ReadFile(path)
	s := string(out)
	for _, want := range []string{
		"host_name = var.host",
		"prevent_activation_lock                      = true",
		"data             = var.dek # sensitive",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in:\n%s", want, s)
		}
	}
	// Out-of-range and non-attribute lines are no-ops.
	if setAttributeValueAtLine(path, 9999, "x") {
		t.Error("expected false for out-of-range line")
	}
	if setAttributeValueAtLine(path, 1, "x") {
		t.Error("expected false for a resource declaration line")
	}
}

// The empty-collection removal is only ever taken against a schema that
// positively says the attribute is Optional. Everything that leaves that
// unproven has to decline, because removing a Required attribute trades the
// size error for a missing-argument error that fixRequiredNulls answers by
// putting the attribute back — the two then take turns until the iteration cap
// and the run reports success over a project that does not validate.
//
// This test is also the tripwire on the guard: delete the schema check and
// every case below starts returning a fix.
func TestEmptyCollectionDeclinesWithoutProof(t *testing.T) {
	src := `resource "jamfplatform_pro_app_request_settings" "settings" {
  approver_emails = []
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "app_request.tf")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	const detail = "Attribute approver_emails set must contain at least 1 elements, got: 0"
	withInfo := func(info attrInfo) *ProviderSchema {
		return &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
			"jamfplatform_pro_app_request_settings": {"": {"approver_emails": info}},
		}}
	}

	declines := []struct {
		name   string
		path   string
		line   int
		schema *ProviderSchema
	}{
		{
			// terraform providers schema failed, which every pipeline reduces
			// to a warning. Nothing can be proved, so nothing is removed.
			name: "no schema at all", path: path, line: 2, schema: nil,
		},
		{
			name: "unreadable file", path: filepath.Join(dir, "gone.tf"), line: 2, schema: withInfo(attrInfo{Optional: true}),
		},
		{
			// A line no resource declaration precedes: the attribute cannot be
			// attributed to a resource type, so the schema cannot be consulted.
			name: "no resource at the line", path: path, line: 0, schema: withInfo(attrInfo{Optional: true}),
		},
		{
			name: "required attribute", path: path, line: 2, schema: withInfo(attrInfo{Required: true}),
		},
		{
			// A schema that does not carry the attribute answers "not
			// Required" for a missing key exactly as it does for an Optional
			// one, and those are not the same answer here.
			name: "attribute absent from schema", path: path, line: 2,
			schema: &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
				"jamfplatform_pro_app_request_settings": {"": {}},
			}},
		},
	}
	for _, tt := range declines {
		t.Run(tt.name, func(t *testing.T) {
			if fix := classifyFix("Invalid Attribute Value", detail, tt.path, tt.line, tt.schema); fix != nil {
				t.Errorf("expected no fix, got %q — a Required attribute may now be removed and put back on every pass", fix.attrName)
			}
		})
	}

	t.Run("optional attribute is removed", func(t *testing.T) {
		fix := classifyFix("Invalid Attribute Value", detail, path, 2, withInfo(attrInfo{Optional: true}))
		if fix == nil {
			t.Fatal("expected a fix for an Optional empty collection")
		}
		if fix.attrName != "approver_emails" {
			t.Errorf("attrName = %q, want approver_emails", fix.attrName)
		}
	})
}

// An Optional string the server returned empty is removed, not turned into a
// variable the operator has to invent a value for.
func TestOptionalEmptyStringIsRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smtp.tf")
	src := `resource "jamfplatform_pro_smtp_server" "settings" {
  sender_display_name = ""
  authentication_type = "BASIC"
}
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
		"jamfplatform_pro_smtp_server": {"": {"sender_display_name": {Optional: true, Type: cty.String}}},
	}}

	vars, edits := applyFixPass(dir, []tfjson.Diagnostic{{
		Severity: "error",
		Summary:  "Invalid Attribute Value Length",
		Detail:   "Attribute sender_display_name string length must be at least 1, got: 0",
		Range:    &tfjson.Range{Filename: "smtp.tf", Start: tfjson.Pos{Line: 2}},
	}}, schema)

	if len(vars) != 0 {
		t.Fatalf("an Optional empty string must not become a mandatory variable, got %+v", vars)
	}
	if len(edits) != 1 || edits[0].Action != "removed" {
		t.Fatalf("expected one removal edit, got %+v", edits)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "sender_display_name") {
		t.Errorf("sender_display_name survived:\n%s", out)
	}
	if !strings.Contains(string(out), "authentication_type") {
		t.Errorf("sibling attribute was damaged:\n%s", out)
	}
}

// A Required string the server returned empty can be neither removed nor
// invented, so it becomes a variable — described by what actually went wrong,
// not as a write-only secret the API is hiding.
func TestRequiredEmptyStringBecomesDescribedVariable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smtp.tf")
	src := `resource "jamfplatform_pro_smtp_server" "settings" {
  sender_settings = {
    email_address = ""
  }
}
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
		"jamfplatform_pro_smtp_server": {
			"sender_settings": {"email_address": {Required: true, Type: cty.String}},
		},
	}}

	vars, edits := applyFixPass(dir, []tfjson.Diagnostic{{
		Severity: "error",
		Summary:  "Invalid Attribute Value Length",
		Detail:   "Attribute sender_settings.email_address string length must be at least 1, got: 0",
		Range:    &tfjson.Range{Filename: "smtp.tf", Start: tfjson.Pos{Line: 3}},
	}}, schema)

	if len(vars) != 1 {
		t.Fatalf("expected one variable, got %+v", vars)
	}
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %+v", edits)
	}
	if !vars[0].NotSensitive {
		t.Error("an empty server value is not a secret; it should not be marked sensitive")
	}
	out, _ := os.ReadFile(path)
	if !strings.Contains(string(out), "= var."+vars[0].VarName) {
		t.Errorf("expected the attribute wired to var.%s, got:\n%s", vars[0].VarName, out)
	}
	decl, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if !strings.Contains(string(decl), "the server returned an empty value") {
		t.Errorf("description should name the empty server value, got:\n%s", decl)
	}
	if strings.Contains(string(decl), "write-only") {
		t.Errorf("description misstates the cause as a write-only secret:\n%s", decl)
	}
}

// A Required empty string is never removed — that is the oscillation guard,
// and it applies to the string case as much as the collection case.
func TestRequiredEmptyStringIsNotRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "smtp.tf")
	if err := os.WriteFile(path, []byte("resource \"x\" \"y\" {\n  a = \"\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	schema := &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
		"x": {"": {"a": {Required: true, Type: cty.String}}},
	}}
	if fix := classifyFix("Invalid Attribute Value Length",
		"Attribute a string length must be at least 1, got: 0", path, 2, schema); fix != nil {
		t.Errorf("expected no removal for a Required empty string, got %q", fix.attrName)
	}
}

// The write-only probe has to look the attribute up at its own block path. A
// nested WriteOnly secret is not keyed at the top level, so passing "" made
// the guard answer false and wired a variable over the one
// injectRequiredWriteOnly had already paired with its _wo_version companion.
func TestWriteOnlyProbeUsesTheAttributeBlockPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.tf")
	src := `resource "jamfplatform_pro_disk_encryption_configuration" "d" {
  institutional_recovery_key = {
    data = null
  }
}
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
		"jamfplatform_pro_disk_encryption_configuration": {
			"institutional_recovery_key": {"data": {Required: true, WriteOnly: true, Type: cty.String}},
		},
	}}

	vars, edits := applyFixPass(dir, []tfjson.Diagnostic{{
		Severity: "error",
		Summary:  "Missing Configuration for Required Attribute",
		Detail:   "Must set a configuration value for the institutional_recovery_key.data attribute",
		Range:    &tfjson.Range{Filename: "disk.tf", Start: tfjson.Pos{Line: 3}},
	}}, schema)

	if len(vars) != 0 || len(edits) != 0 {
		t.Fatalf("a WriteOnly attribute belongs to injectRequiredWriteOnly, got vars=%+v edits=%+v", vars, edits)
	}
	out, _ := os.ReadFile(path)
	if strings.Contains(string(out), "var.") {
		t.Errorf("the WriteOnly attribute was wired a second time:\n%s", out)
	}
}

// FixResult.Fixed is the length of FixResult.Edits, so the count in the
// auto-fix heading and the lines printed beneath it can never disagree. This
// exercises both an edit-appending path and the required-null path, which used
// to increment the count while recording nothing.
func TestApplyFixPassRecordsAnEditForEveryFix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cdp.tf")
	src := `resource "jamfpro_cloud_distribution_point" "settings" {
  cdn_type                = "JAMFCLOUD"
  secondary_auth_required = false
}

resource "jamfpro_activation_code" "settings" {
  organization_name = "Jamf"
  code              = null
}
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema := &ProviderSchema{attrs: map[string]map[string]map[string]attrInfo{
		"jamfpro_activation_code": {"": {"code": {Required: true, Sensitive: true, Type: cty.String}}},
	}}

	vars, edits := applyFixPass(dir, []tfjson.Diagnostic{
		{
			Severity: "error",
			Summary:  "Attribute secondary_auth_required Is Not Valid For CDN Type",
			Detail:   "Attribute secondary_auth_required only when cdn_type is AKAMAI. Remove it or change cdn_type.",
			Range:    &tfjson.Range{Filename: "cdp.tf", Start: tfjson.Pos{Line: 3}},
		},
		{
			Severity: "error",
			Summary:  "Missing required argument",
			Detail:   `The argument "code" is required, but no definition was found.`,
			Range:    &tfjson.Range{Filename: "cdp.tf", Start: tfjson.Pos{Line: 8}},
		},
	}, schema)

	if len(edits) != 2 {
		t.Fatalf("expected one edit per fix, got %d: %+v", len(edits), edits)
	}
	if len(vars) != 1 {
		t.Fatalf("expected the sensitive required null to declare a variable, got %+v", vars)
	}
	for _, e := range edits {
		if e.Resource == "" || e.Attr == "" || e.Action == "" || e.Reason == "" {
			t.Errorf("edit is not reportable: %+v", e)
		}
	}
	// A FixResult assembled the way FixValidationErrors assembles it keeps the
	// heading and the detail lines in agreement.
	result := &FixResult{}
	result.Edits = append(result.Edits, edits...)
	result.Fixed += len(edits)
	if len(result.Edits) != result.Fixed {
		t.Errorf("len(Edits) = %d, Fixed = %d", len(result.Edits), result.Fixed)
	}
}
