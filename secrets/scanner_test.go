// Copyright 2026, Jamf Software LLC

package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanFindsHCLPassword(t *testing.T) {
	dir := t.TempDir()

	content := `resource "jamfpro_webhook" "alerts" {
  name                     = "alerts"
  authentication_type      = "BASIC"
  username                 = "admin"
  authentication_password  = "SuperS3cretP@ssw0rd!"
}
`
	if err := os.WriteFile(filepath.Join(dir, "webhooks.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}

	f := findings[0]
	if f.ResourceAddress != "jamfpro_webhook.alerts" {
		t.Errorf("ResourceAddress = %q, want %q", f.ResourceAddress, "jamfpro_webhook.alerts")
	}
	if f.AttrName != "authentication_password" {
		t.Errorf("AttrName = %q, want %q", f.AttrName, "authentication_password")
	}
	if f.Secret != "SuperS3cretP@ssw0rd!" {
		t.Errorf("Secret = %q, want %q", f.Secret, "SuperS3cretP@ssw0rd!")
	}
}

func TestScanFindsPlistPassword(t *testing.T) {
	dir := t.TempDir()

	// Simulate an extracted appconfig XML with a password
	sfDir := filepath.Join(dir, "support_files", "app_configurations")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `<dict>
<key>Password</key>
<string>thisisapasswordthatshouldnotbehere134!4^</string>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "TestApp.xml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least one finding for plist password")
	}

	f := findings[0]
	if !f.InSupportFiles {
		t.Error("expected InSupportFiles=true")
	}
	if f.SupportFileRef == "" {
		t.Error("expected SupportFileRef to be set")
	}
}

func TestScanFindsPlistPasswordMultiline(t *testing.T) {
	dir := t.TempDir()

	// Simulate a mobileconfig where <key> and <string> are on separate lines
	sfDir := filepath.Join(dir, "support_files", "macos_configuration_profiles")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `<?xml version="1.0" encoding="UTF-8"?>
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>Password</key>
			<string>gameplan</string>
			<key>UserName</key>
			<string>admin</string>
		</dict>
	</array>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "Test.mobileconfig"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least one finding for multiline plist password")
	}

	found := false
	for _, f := range findings {
		if f.Secret == "gameplan" {
			found = true
			if !f.InSupportFiles {
				t.Error("expected InSupportFiles=true")
			}
			break
		}
	}
	if !found {
		t.Error("expected to find 'gameplan' secret")
		for _, f := range findings {
			t.Logf("  found: rule=%s secret=%q", f.RuleID, f.Secret)
		}
	}
}

func TestScanFindsAppConfigVendorSpecificKey(t *testing.T) {
	dir := t.TempDir()

	sfDir := filepath.Join(dir, "support_files", "app_configurations")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Vendor-specific key with a high-entropy license key value
	content := `<dict>
<key>LicenseKey</key>
<string>aB3kQ9mZ2pX7nL5vW8cD1eY4rT6sU0jH</string>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "VendorApp.xml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected finding for high-entropy vendor-specific app config key")
	}

	f := findings[0]
	if !f.InSupportFiles {
		t.Error("expected InSupportFiles=true")
	}
	if f.RuleID != "jamf-appconfig-high-entropy" {
		t.Errorf("RuleID = %q, want %q", f.RuleID, "jamf-appconfig-high-entropy")
	}
}

func TestScanSkipsAppConfigLowEntropyValues(t *testing.T) {
	dir := t.TempDir()

	sfDir := filepath.Join(dir, "support_files", "app_configurations")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Bundle ID and server URL — low-entropy, should not be flagged
	content := `<dict>
<key>BundleIdentifier</key>
<string>com.company.myapp</string>
<key>ServerURL</key>
<string>https://mdm.example.com</string>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "Config.xml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	for _, f := range findings {
		if f.RuleID == "jamf-appconfig-high-entropy" {
			t.Errorf("unexpected high-entropy finding for low-entropy config value: secret=%q", f.Secret)
		}
	}
}

func TestScanFindsPrivateKey(t *testing.T) {
	dir := t.TempDir()

	content := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGdEQ3aRHMPAbR/lt9FMh8f5
-----END RSA PRIVATE KEY-----
`
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least one finding for private key")
	}
}

func TestScanSkipsTfvars(t *testing.T) {
	dir := t.TempDir()

	content := `client_secret = "super-secret-value-AKIAIOSFODNN7EXAMPLE"
`
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	for _, f := range findings {
		if filepath.Base(f.File) == "terraform.tfvars" {
			t.Errorf("expected terraform.tfvars to be excluded, but found: %s", f.RuleID)
		}
	}
}

func TestScanCleanDirectory(t *testing.T) {
	dir := t.TempDir()

	content := `resource "jamfpro_building" "main_office" {
  name            = "Main Office"
  street_address1 = "123 Main St"
}
`
	if err := os.WriteFile(filepath.Join(dir, "buildings.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("expected no findings for clean file, got %d", len(findings))
		for _, f := range findings {
			t.Logf("  %s: %s", f.RuleID, f.Secret)
		}
	}
}

func TestRemediateTFFile(t *testing.T) {
	dir := t.TempDir()

	// Create webhook .tf with a secret
	tfContent := `resource "jamfpro_webhook" "alerts" {
  name                     = "alerts"
  authentication_type      = "BASIC"
  username                 = "admin"
  authentication_password  = "SuperS3cretP@ssw0rd!"
}
`
	if err := os.WriteFile(filepath.Join(dir, "webhooks.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatal(err)
	}
	// Create empty variables.tf and terraform.tfvars
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings to remediate")
	}

	result, err := Remediate(dir, findings)
	if err != nil {
		t.Fatalf("Remediate() error: %v", err)
	}

	if result.VariablesAdded == 0 {
		t.Error("expected at least one variable added")
	}

	// Check the .tf file now has var. reference
	updated, _ := os.ReadFile(filepath.Join(dir, "webhooks.tf"))
	if !strings.Contains(string(updated), "var.") {
		t.Errorf("expected var. reference in webhooks.tf, got:\n%s", updated)
	}
	if strings.Contains(string(updated), "SuperS3cretP@ssw0rd!") {
		t.Error("secret should have been removed from webhooks.tf")
	}

	// Check variables.tf has sensitive variable
	vars, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if !strings.Contains(string(vars), "sensitive") {
		t.Errorf("expected sensitive variable in variables.tf, got:\n%s", vars)
	}

	// Check terraform.tfvars has the secret value
	tfvars, _ := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if !strings.Contains(string(tfvars), "SuperS3cretP@ssw0rd!") {
		t.Errorf("expected secret value in terraform.tfvars, got:\n%s", tfvars)
	}
}

func TestRemediateTFFileEmbeddedSecret(t *testing.T) {
	dir := t.TempDir()

	tfContent := `resource "jamfpro_policy" "install_netskope" {
  package {
    action     = "Install Cached"
    parameter8 = "enrollauthtoken=334bb8e5b4c3a2c2f1bb9d594fd4e575"
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "policies.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}

	result, err := Remediate(dir, findings)
	if err != nil {
		t.Fatalf("Remediate() error: %v", err)
	}

	if result.VariablesAdded == 0 {
		t.Error("expected at least one variable added")
	}
	if result.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", result.Skipped)
	}

	// Check the .tf file uses interpolation for the embedded secret
	updated, _ := os.ReadFile(filepath.Join(dir, "policies.tf"))
	s := string(updated)
	if strings.Contains(s, "334bb8e5b4c3a2c2f1bb9d594fd4e575") {
		t.Errorf("secret should have been removed from policies.tf, got:\n%s", s)
	}
	if !strings.Contains(s, "${var.") {
		t.Errorf("expected interpolation ${var.} in policies.tf, got:\n%s", s)
	}
	if !strings.Contains(s, "enrollauthtoken=") {
		t.Errorf("expected prefix 'enrollauthtoken=' preserved, got:\n%s", s)
	}
}

func TestRemediateSupportFile(t *testing.T) {
	dir := t.TempDir()

	// Create support file with secret
	sfDir := filepath.Join(dir, "support_files", "app_configurations")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}
	xmlContent := `<dict>
<key>Password</key>
<string>thisisapasswordthatshouldnotbehere134!4^</string>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "TestApp.xml"), []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .tf that references the support file
	tfContent := `resource "jamfpro_mobile_device_application" "test_app" {
  name = "TestApp"

  app_configuration {
    preferences = file("${path.module}/support_files/app_configurations/TestApp.xml")
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "mobile_device_applications.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected findings for plist password")
	}

	result, err := Remediate(dir, findings)
	if err != nil {
		t.Fatalf("Remediate() error: %v", err)
	}

	if result.SupportFiles == 0 {
		t.Error("expected at least one support file remediated")
	}

	// Check .xml was renamed to .xml.tpl
	if _, err := os.Stat(filepath.Join(sfDir, "TestApp.xml.tpl")); os.IsNotExist(err) {
		t.Error("expected TestApp.xml.tpl to exist")
	}
	if _, err := os.Stat(filepath.Join(sfDir, "TestApp.xml")); !os.IsNotExist(err) {
		t.Error("expected TestApp.xml to be removed")
	}

	// Check the .tpl has template variable instead of secret
	tpl, _ := os.ReadFile(filepath.Join(sfDir, "TestApp.xml.tpl"))
	if strings.Contains(string(tpl), "thisisapasswordthatshouldnotbehere134!4^") {
		t.Error("secret should have been removed from .tpl file")
	}
	if !strings.Contains(string(tpl), "${") {
		t.Error("expected template variable in .tpl file")
	}

	// Check the .tf now uses templatefile()
	tf, _ := os.ReadFile(filepath.Join(dir, "mobile_device_applications.tf"))
	if !strings.Contains(string(tf), "templatefile(") {
		t.Errorf("expected templatefile() in .tf file, got:\n%s", tf)
	}
}

func TestRemediateMultipleSecretsInSameFile(t *testing.T) {
	dir := t.TempDir()

	// Create support file with TWO secrets (like a mobileconfig with multiple passwords)
	sfDir := filepath.Join(dir, "support_files", "macos_configuration_profiles")
	if err := os.MkdirAll(sfDir, 0755); err != nil {
		t.Fatal(err)
	}
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>Password</key>
			<string>firstsecretpassword!</string>
			<key>UserName</key>
			<string>admin</string>
		</dict>
		<dict>
			<key>Password</key>
			<string>secondsecretpassword!</string>
		</dict>
	</array>
</dict>
`
	if err := os.WriteFile(filepath.Join(sfDir, "Test.mobileconfig"), []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .tf that references the support file
	tfContent := `resource "jamfpro_macos_configuration_profile_plist" "test" {
  name = "Test"
  payloads = file("${path.module}/support_files/macos_configuration_profiles/Test.mobileconfig")
}
`
	if err := os.WriteFile(filepath.Join(dir, "macos_configuration_profiles.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := Scan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) < 2 {
		t.Fatalf("expected at least 2 findings, got %d", len(findings))
	}

	result, err := Remediate(dir, findings)
	if err != nil {
		t.Fatalf("Remediate() error: %v", err)
	}

	if result.VariablesAdded < 2 {
		t.Errorf("expected at least 2 variables added, got %d", result.VariablesAdded)
	}
	if result.SupportFiles != 1 {
		t.Errorf("expected 1 support file remediated (2 findings in same file), got %d", result.SupportFiles)
	}

	// Check .tpl exists and both secrets are replaced
	tpl, err := os.ReadFile(filepath.Join(sfDir, "Test.mobileconfig.tpl"))
	if err != nil {
		t.Fatalf("reading .tpl: %v", err)
	}
	if strings.Contains(string(tpl), "firstsecretpassword!") {
		t.Error("first secret should have been removed from .tpl")
	}
	if strings.Contains(string(tpl), "secondsecretpassword!") {
		t.Error("second secret should have been removed from .tpl")
	}
	if !strings.Contains(string(tpl), "${") {
		t.Error("expected template variables in .tpl")
	}

	// Check variables.tf has BOTH sensitive variables
	vars, _ := os.ReadFile(filepath.Join(dir, "variables.tf"))
	count := strings.Count(string(vars), "sensitive")
	if count < 2 {
		t.Errorf("expected at least 2 sensitive variables in variables.tf, got %d:\n%s", count, vars)
	}

	// Check terraform.tfvars has BOTH secret values
	tfvars, _ := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if !strings.Contains(string(tfvars), "firstsecretpassword!") {
		t.Errorf("expected first secret in terraform.tfvars, got:\n%s", tfvars)
	}
	if !strings.Contains(string(tfvars), "secondsecretpassword!") {
		t.Errorf("expected second secret in terraform.tfvars, got:\n%s", tfvars)
	}

	// Check the .tf now uses templatefile() with both variables in the map
	tf, _ := os.ReadFile(filepath.Join(dir, "macos_configuration_profiles.tf"))
	tfStr := string(tf)
	if !strings.Contains(tfStr, "templatefile(") {
		t.Errorf("expected templatefile() in .tf, got:\n%s", tfStr)
	}
	// Count variable assignments in the templatefile() call
	tfLine := ""
	for line := range strings.SplitSeq(tfStr, "\n") {
		if strings.Contains(line, "templatefile(") {
			tfLine = line
			break
		}
	}
	varCount := strings.Count(tfLine, "= var.")
	if varCount < 2 {
		t.Errorf("expected at least 2 variables in templatefile() map, got %d:\n%s", varCount, tfLine)
	}
}

func TestEscapeTemplateExpressions(t *testing.T) {
	input := `#!/bin/bash
echo "Hello ${USER}"
echo "${HOME}/scripts"
PASSWORD="mysecret"
`
	got := escapeTemplateExpressions(input)

	if !strings.Contains(got, "$${USER}") {
		t.Error("expected $${USER} in escaped output")
	}
	if !strings.Contains(got, "$${HOME}") {
		t.Error("expected $${HOME} in escaped output")
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "*****"},
		{"12345678", "********"},
	}
	for _, tt := range tests {
		got := redact(tt.input)
		if got != tt.want {
			t.Errorf("redact(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
