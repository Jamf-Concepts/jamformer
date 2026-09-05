// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
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
		{"basic_auth_credentials.0.password", "basic_auth_credentials"},
		{"a.0.b.0.c", "a.b"},
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

	vars, fixed := fixRequiredNulls(dir, diags, schema)
	if fixed != 1 {
		t.Errorf("expected 1 fix, got %d", fixed)
	}
	if len(vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(vars))
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

	vars, fixed := fixRequiredNulls(dir, diags, schema)
	if fixed != 1 {
		t.Errorf("expected 1 fix, got %d", fixed)
	}
	if len(vars) != 0 {
		t.Errorf("expected 0 vars for non-sensitive, got %d", len(vars))
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
