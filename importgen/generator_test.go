// Copyright 2026, Jamf Software LLC

package importgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/pro/discovery"
)

func TestWriteProviderFileBasicAuth(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{AuthMethod: "basic"}

	if err := writeProviderFile(dir, creds); err != nil {
		t.Fatalf("writeProviderFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `source = "deploymenttheory/jamfpro"`) {
		t.Error("expected provider source")
	}
	if !strings.Contains(s, "basic_auth_username") {
		t.Error("expected basic_auth_username in provider block")
	}
	if !strings.Contains(s, "basic_auth_password") {
		t.Error("expected basic_auth_password in provider block")
	}
	if strings.Contains(s, "client_id") {
		t.Error("did not expect client_id in basic auth provider block")
	}
}

func TestWriteProviderFileVersionPin(t *testing.T) {
	t.Run("pinned version uses exact constraint", func(t *testing.T) {
		dir := t.TempDir()
		creds := &Credentials{AuthMethod: "oauth2", ProviderVersion: "1.2.3"}
		if err := writeProviderFile(dir, creds); err != nil {
			t.Fatalf("writeProviderFile: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		s := string(content)
		if !strings.Contains(s, `version = "1.2.3"`) {
			t.Error("expected exact version constraint")
		}
	})
	t.Run("resolved version uses >= constraint", func(t *testing.T) {
		dir := t.TempDir()
		creds := &Credentials{AuthMethod: "oauth2", ResolvedVersion: "0.35.1"}
		if err := writeProviderFile(dir, creds); err != nil {
			t.Fatalf("writeProviderFile: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		s := string(content)
		if !strings.Contains(s, `version = ">= 0.35.1"`) {
			t.Error("expected >= version constraint")
		}
	})
	t.Run("no version omits constraint", func(t *testing.T) {
		dir := t.TempDir()
		creds := &Credentials{AuthMethod: "oauth2"}
		if err := writeProviderFile(dir, creds); err != nil {
			t.Fatalf("writeProviderFile: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		s := string(content)
		if strings.Contains(s, "version") {
			t.Error("did not expect version constraint when neither pinned nor resolved")
		}
	})
	t.Run("pinned takes precedence over resolved", func(t *testing.T) {
		dir := t.TempDir()
		creds := &Credentials{AuthMethod: "oauth2", ProviderVersion: "2.0.0", ResolvedVersion: "1.5.0"}
		if err := writeProviderFile(dir, creds); err != nil {
			t.Fatalf("writeProviderFile: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		s := string(content)
		if !strings.Contains(s, `version = "2.0.0"`) {
			t.Errorf("expected pinned version to take precedence, got:\n%s", s)
		}
	})
}

func TestGenerateProtect_VersionPin(t *testing.T) {
	t.Run("pinned version", func(t *testing.T) {
		dir := t.TempDir()
		creds := &ProtectCredentials{URL: "https://test.protect.jamfcloud.com", ProviderVersion: "0.2.0"}
		if err := GenerateProtect(dir, creds); err != nil {
			t.Fatalf("GenerateProtect: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if !strings.Contains(string(content), `version = "0.2.0"`) {
			t.Error("expected exact version constraint")
		}
	})
	t.Run("no version omits constraint in phase 1", func(t *testing.T) {
		dir := t.TempDir()
		creds := &ProtectCredentials{URL: "https://test.protect.jamfcloud.com"}
		if err := GenerateProtect(dir, creds); err != nil {
			t.Fatalf("GenerateProtect: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if strings.Contains(string(content), "version") {
			t.Error("did not expect version constraint in phase 1 without pin")
		}
	})
	t.Run("resolved version in finalize", func(t *testing.T) {
		dir := t.TempDir()
		creds := &ProtectCredentials{URL: "https://test.protect.jamfcloud.com", ResolvedVersion: "0.1.1"}
		if err := FinalizeProtect(dir, creds); err != nil {
			t.Fatalf("FinalizeProtect: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if !strings.Contains(string(content), `version = ">= 0.1.1"`) {
			t.Error("expected >= version constraint from resolved version")
		}
	})
}

func TestGeneratePlatform_VersionPin(t *testing.T) {
	t.Run("pinned version", func(t *testing.T) {
		dir := t.TempDir()
		creds := &PlatformCredentials{BaseURL: "https://us.apigw.jamf.com", ProviderVersion: "0.15.0"}
		if err := GeneratePlatform(dir, creds); err != nil {
			t.Fatalf("GeneratePlatform: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if !strings.Contains(string(content), `version = "0.15.0"`) {
			t.Error("expected exact version constraint")
		}
	})
	t.Run("no version omits constraint in phase 1", func(t *testing.T) {
		dir := t.TempDir()
		creds := &PlatformCredentials{BaseURL: "https://us.apigw.jamf.com"}
		if err := GeneratePlatform(dir, creds); err != nil {
			t.Fatalf("GeneratePlatform: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if strings.Contains(string(content), "version") {
			t.Error("did not expect version constraint in phase 1 without pin")
		}
	})
	t.Run("resolved version in finalize", func(t *testing.T) {
		dir := t.TempDir()
		creds := &PlatformCredentials{BaseURL: "https://us.apigw.jamf.com", ResolvedVersion: "0.14.1"}
		if err := FinalizePlatform(dir, creds); err != nil {
			t.Fatalf("FinalizePlatform: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "provider.tf"))
		if !strings.Contains(string(content), `version = ">= 0.14.1"`) {
			t.Error("expected >= version constraint from resolved version")
		}
	})
}

func TestWriteProviderFileOAuth2(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{AuthMethod: "oauth2"}

	if err := writeProviderFile(dir, creds); err != nil {
		t.Fatalf("writeProviderFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "client_id") {
		t.Error("expected client_id in oauth2 provider block")
	}
	if !strings.Contains(s, "client_secret") {
		t.Error("expected client_secret in oauth2 provider block")
	}
	if strings.Contains(s, "basic_auth_username") {
		t.Error("did not expect basic_auth_username in oauth2 provider block")
	}
}

func TestWriteVariablesFileBasicAuth(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{AuthMethod: "basic"}

	if err := writeVariablesFile(dir, creds); err != nil {
		t.Fatalf("writeVariablesFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "jamfpro_basic_auth_username") {
		t.Error("expected basic auth username variable")
	}
	if !strings.Contains(s, "jamfpro_basic_auth_password") {
		t.Error("expected basic auth password variable")
	}
	if !strings.Contains(s, `default     = "basic"`) {
		t.Error("expected auth_method default to be basic")
	}
	if !strings.Contains(s, "sensitive") {
		t.Error("expected sensitive attribute on password variable")
	}
}

func TestWriteVariablesFileOAuth2(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{AuthMethod: "oauth2"}

	if err := writeVariablesFile(dir, creds); err != nil {
		t.Fatalf("writeVariablesFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "jamfpro_client_id") {
		t.Error("expected client_id variable")
	}
	if !strings.Contains(s, "jamfpro_client_secret") {
		t.Error("expected client_secret variable")
	}
	if !strings.Contains(s, `default     = "oauth2"`) {
		t.Error("expected auth_method default to be oauth2")
	}
	if !strings.Contains(s, "sensitive") {
		t.Error("expected sensitive attribute on credential variables")
	}
}

func TestWriteTfvarsFileBasicAuth(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{
		URL:        "https://test.jamfcloud.com",
		AuthMethod: "basic",
		Username:   "admin",
		Password:   "secret123",
	}

	if err := writeTfvarsFile(dir, creds); err != nil {
		t.Fatalf("writeTfvarsFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "https://test.jamfcloud.com") {
		t.Error("expected instance URL")
	}
	if !strings.Contains(s, `"basic"`) {
		t.Error("expected auth method basic")
	}
	if strings.Contains(s, "admin") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
	if strings.Contains(s, "secret123") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
}

func TestWriteTfvarsFileOAuth2(t *testing.T) {
	dir := t.TempDir()
	creds := &Credentials{
		URL:          "https://test.jamfcloud.com",
		AuthMethod:   "oauth2",
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
	}

	if err := writeTfvarsFile(dir, creds); err != nil {
		t.Fatalf("writeTfvarsFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `"oauth2"`) {
		t.Error("expected auth method oauth2")
	}
	if strings.Contains(s, "my-client-id") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
	if strings.Contains(s, "my-client-secret") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
}

func TestWriteImportFile(t *testing.T) {
	dir := t.TempDir()

	resources := []discovery.Resource{
		{JamfID: "1", Name: "Test Script", Label: "test_script"},
		{JamfID: "2", Name: "Another Script", Label: "another_script"},
	}

	if err := writeImportFile(dir, "scripts_import.tf", "jamfpro_script", resources); err != nil {
		t.Fatalf("writeImportFile: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "scripts_import.tf"))
	if err != nil {
		t.Fatalf("reading import file: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "jamfpro_script.test_script") {
		t.Error("expected first resource in import file")
	}
	if !strings.Contains(s, "jamfpro_script.another_script") {
		t.Error("expected second resource in import file")
	}
	if !strings.Contains(s, `id = "1"`) {
		t.Error("expected first ID")
	}
	if !strings.Contains(s, `id = "2"`) {
		t.Error("expected second ID")
	}
}

func TestWriteImportFileEmpty(t *testing.T) {
	dir := t.TempDir()

	if err := writeImportFile(dir, "empty_import.tf", "jamfpro_script", nil); err != nil {
		t.Fatalf("writeImportFile: %v", err)
	}

	// Empty resources should not create a file
	if _, err := os.Stat(filepath.Join(dir, "empty_import.tf")); !os.IsNotExist(err) {
		t.Error("expected no file for empty resources")
	}
}

func TestWriteImportFileAllResourceTypes(t *testing.T) {
	dir := t.TempDir()

	// Test that import files are created for every supported resource type
	importFiles := []struct {
		filename     string
		resourceType string
	}{
		{"sites_import.tf", "jamfpro_site"},
		{"buildings_import.tf", "jamfpro_building"},
		{"categories_import.tf", "jamfpro_category"},
		{"departments_import.tf", "jamfpro_department"},
		{"scripts_import.tf", "jamfpro_script"},
		{"computer_extension_attributes_import.tf", "jamfpro_computer_extension_attribute"},
		{"packages_import.tf", "jamfpro_package"},
		{"dock_items_import.tf", "jamfpro_dock_item"},
		{"printers_import.tf", "jamfpro_printer"},
		{"network_segments_import.tf", "jamfpro_network_segment"},
		{"smart_computer_groups_import.tf", "jamfpro_smart_computer_group_v2"},
		{"static_computer_groups_import.tf", "jamfpro_static_computer_group"},
		{"macos_configuration_profiles_import.tf", "jamfpro_macos_configuration_profile_plist"},
		{"policies_import.tf", "jamfpro_policy"},
		{"icons_import.tf", "jamfpro_icon"},
		{"enrollment_customizations_import.tf", "jamfpro_enrollment_customization"},
		{"computer_prestages_import.tf", "jamfpro_computer_prestage_enrollment"},
		{"advanced_computer_searches_import.tf", "jamfpro_advanced_computer_search"},
		{"app_installers_import.tf", "jamfpro_app_installer"},
		{"mac_applications_import.tf", "jamfpro_mac_application"},
		{"device_enrollments_import.tf", "jamfpro_device_enrollments"},
		{"volume_purchasing_locations_import.tf", "jamfpro_volume_purchasing_locations"},
		{"restricted_software_import.tf", "jamfpro_restricted_software"},
		{"smart_mobile_device_groups_import.tf", "jamfpro_smart_mobile_device_group_v1"},
		{"static_mobile_device_groups_import.tf", "jamfpro_static_mobile_device_group"},
		{"mobile_device_configuration_profiles_import.tf", "jamfpro_mobile_device_configuration_profile_plist"},
		{"mobile_device_prestages_import.tf", "jamfpro_mobile_device_prestage_enrollment"},
		{"mobile_device_extension_attributes_import.tf", "jamfpro_mobile_device_extension_attribute"},
		{"advanced_mobile_device_searches_import.tf", "jamfpro_advanced_mobile_device_search"},
		{"api_integrations_import.tf", "jamfpro_api_integration"},
		{"api_roles_import.tf", "jamfpro_api_role"},
		{"accounts_import.tf", "jamfpro_account"},
		{"webhooks_import.tf", "jamfpro_webhook"},
		{"account_groups_import.tf", "jamfpro_account_group"},
		{"disk_encryption_configurations_import.tf", "jamfpro_disk_encryption_configuration"},
		{"allowed_file_extensions_import.tf", "jamfpro_allowed_file_extension"},
		{"ldap_servers_import.tf", "jamfpro_ldap_server"},
		{"mobile_device_applications_import.tf", "jamfpro_mobile_device_application"},
		{"user_groups_import.tf", "jamfpro_user_group"},
		{"self_service_branding_macos_import.tf", "jamfpro_self_service_branding_macos"},
		{"self_service_branding_ios_import.tf", "jamfpro_self_service_branding_ios"},
		{"advanced_user_searches_import.tf", "jamfpro_advanced_user_search"},
	}

	for _, f := range importFiles {
		resources := []discovery.Resource{
			{JamfID: "1", Name: "Test", Label: "test"},
		}

		if err := writeImportFile(dir, f.filename, f.resourceType, resources); err != nil {
			t.Errorf("writeImportFile(%s): %v", f.filename, err)
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, f.filename))
		if err != nil {
			t.Errorf("reading %s: %v", f.filename, err)
			continue
		}

		s := string(content)
		if !strings.Contains(s, f.resourceType+".test") {
			t.Errorf("%s: expected resource address %s.test", f.filename, f.resourceType)
		}
	}
}

func TestGenerateCreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	creds := &Credentials{
		URL:        "https://test.jamfcloud.com",
		AuthMethod: "basic",
		Username:   "admin",
		Password:   "secret",
	}

	results := &discovery.Results{
		Sites:      []discovery.Resource{{JamfID: "1", Name: "Main", Label: "main"}},
		Buildings:  []discovery.Resource{{JamfID: "1", Name: "HQ", Label: "hq"}},
		Categories: []discovery.Resource{{JamfID: "1", Name: "Test", Label: "test"}},
		Scripts:    []discovery.Resource{{JamfID: "1", Name: "Script", Label: "script"}},
	}

	if err := Generate(dir, creds, results); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := Finalize(dir, creds); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	expectedFiles := []string{
		"provider.tf",
		"variables.tf",
		"terraform.tfvars",
		"sites_import.tf",
		"buildings_import.tf",
		"categories_import.tf",
		"scripts_import.tf",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", f)
		}
	}
}

func TestGenerateProtect_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	creds := &ProtectCredentials{
		URL:          "https://test.protect.jamfcloud.com",
		ClientID:     "protect-client-id",
		ClientSecret: "protect-client-secret",
	}

	if err := GenerateProtect(dir, creds); err != nil {
		t.Fatalf("GenerateProtect: %v", err)
	}
	if err := FinalizeProtect(dir, creds); err != nil {
		t.Fatalf("FinalizeProtect: %v", err)
	}

	expectedFiles := []string{
		"provider.tf",
		"variables.tf",
		"terraform.tfvars",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", f)
		}
	}
}

func TestGenerateProtect_ProviderFile(t *testing.T) {
	dir := t.TempDir()

	creds := &ProtectCredentials{
		URL:          "https://test.protect.jamfcloud.com",
		ClientID:     "protect-client-id",
		ClientSecret: "protect-client-secret",
	}

	if err := GenerateProtect(dir, creds); err != nil {
		t.Fatalf("GenerateProtect: %v", err)
	}
	if err := FinalizeProtect(dir, creds); err != nil {
		t.Fatalf("FinalizeProtect: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `source = "Jamf-Concepts/jamfprotect"`) {
		t.Error("expected provider source Jamf-Concepts/jamfprotect")
	}
	if !strings.Contains(s, `provider "jamfprotect"`) {
		t.Error("expected jamfprotect provider block")
	}
	if !strings.Contains(s, "var.jamfprotect_url") {
		t.Error("expected var.jamfprotect_url in provider block")
	}
	if !strings.Contains(s, "var.jamfprotect_client_id") {
		t.Error("expected var.jamfprotect_client_id in provider block")
	}
	if !strings.Contains(s, "var.jamfprotect_client_secret") {
		t.Error("expected var.jamfprotect_client_secret in provider block")
	}
}

func TestGenerateProtect_VariablesFile(t *testing.T) {
	dir := t.TempDir()

	creds := &ProtectCredentials{
		URL:          "https://test.protect.jamfcloud.com",
		ClientID:     "protect-client-id",
		ClientSecret: "protect-client-secret",
	}

	if err := GenerateProtect(dir, creds); err != nil {
		t.Fatalf("GenerateProtect: %v", err)
	}
	if err := FinalizeProtect(dir, creds); err != nil {
		t.Fatalf("FinalizeProtect: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `variable "jamfprotect_url"`) {
		t.Error("expected jamfprotect_url variable")
	}
	if !strings.Contains(s, `variable "jamfprotect_client_id"`) {
		t.Error("expected jamfprotect_client_id variable")
	}
	if !strings.Contains(s, `variable "jamfprotect_client_secret"`) {
		t.Error("expected jamfprotect_client_secret variable")
	}
	if !strings.Contains(s, `type        = string`) {
		t.Error("expected string type for variables")
	}
	if !strings.Contains(s, "sensitive") {
		t.Error("expected sensitive attribute on credential variables")
	}
}

func TestGeneratePlatform_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	creds := &PlatformCredentials{
		BaseURL:      "https://us.apigw.jamf.com",
		ClientID:     "platform-client-id",
		ClientSecret: "platform-client-secret",
	}

	if err := GeneratePlatform(dir, creds); err != nil {
		t.Fatalf("GeneratePlatform: %v", err)
	}
	if err := FinalizePlatform(dir, creds); err != nil {
		t.Fatalf("FinalizePlatform: %v", err)
	}

	expectedFiles := []string{
		"provider.tf",
		"variables.tf",
		"terraform.tfvars",
	}

	for _, f := range expectedFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", f)
		}
	}
}

func TestGeneratePlatform_ProviderFile(t *testing.T) {
	dir := t.TempDir()

	creds := &PlatformCredentials{
		BaseURL:      "https://us.apigw.jamf.com",
		ClientID:     "platform-client-id",
		ClientSecret: "platform-client-secret",
	}

	if err := GeneratePlatform(dir, creds); err != nil {
		t.Fatalf("GeneratePlatform: %v", err)
	}
	if err := FinalizePlatform(dir, creds); err != nil {
		t.Fatalf("FinalizePlatform: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `source = "Jamf-Concepts/jamfplatform"`) {
		t.Error("expected provider source Jamf-Concepts/jamfplatform")
	}
	if !strings.Contains(s, `provider "jamfplatform"`) {
		t.Error("expected jamfplatform provider block")
	}
	if !strings.Contains(s, "var.jamfplatform_base_url") {
		t.Error("expected var.jamfplatform_base_url in provider block")
	}
	if !strings.Contains(s, "var.jamfplatform_client_id") {
		t.Error("expected var.jamfplatform_client_id in provider block")
	}
	if !strings.Contains(s, "var.jamfplatform_client_secret") {
		t.Error("expected var.jamfplatform_client_secret in provider block")
	}
}

func TestGeneratePlatform_TfvarsFile(t *testing.T) {
	dir := t.TempDir()

	creds := &PlatformCredentials{
		BaseURL:      "https://us.apigw.jamf.com",
		ClientID:     "platform-client-id",
		ClientSecret: "platform-client-secret",
	}

	if err := GeneratePlatform(dir, creds); err != nil {
		t.Fatalf("GeneratePlatform: %v", err)
	}
	if err := FinalizePlatform(dir, creds); err != nil {
		t.Fatalf("FinalizePlatform: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `"https://us.apigw.jamf.com"`) {
		t.Error("expected platform base URL")
	}
	if !strings.Contains(s, "jamfplatform_base_url") {
		t.Error("expected jamfplatform_base_url key")
	}
	if strings.Contains(s, "platform-client-id") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
	if strings.Contains(s, "platform-client-secret") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
}

func TestGenerateProtect_TfvarsFile(t *testing.T) {
	dir := t.TempDir()

	creds := &ProtectCredentials{
		URL:          "https://test.protect.jamfcloud.com",
		ClientID:     "protect-client-id",
		ClientSecret: "protect-client-secret",
	}

	if err := GenerateProtect(dir, creds); err != nil {
		t.Fatalf("GenerateProtect: %v", err)
	}
	if err := FinalizeProtect(dir, creds); err != nil {
		t.Fatalf("FinalizeProtect: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `"https://test.protect.jamfcloud.com"`) {
		t.Error("expected protect instance URL")
	}
	if !strings.Contains(s, "jamfprotect_url") {
		t.Error("expected jamfprotect_url key")
	}
	if strings.Contains(s, "protect-client-id") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
	if strings.Contains(s, "protect-client-secret") {
		t.Error("credentials should not be written to terraform.tfvars")
	}
}

func TestGeneratePlatform_VariablesFile(t *testing.T) {
	dir := t.TempDir()

	creds := &PlatformCredentials{
		BaseURL:      "https://us.apigw.jamf.com",
		ClientID:     "platform-client-id",
		ClientSecret: "platform-client-secret",
	}

	if err := GeneratePlatform(dir, creds); err != nil {
		t.Fatalf("GeneratePlatform: %v", err)
	}
	if err := FinalizePlatform(dir, creds); err != nil {
		t.Fatalf("FinalizePlatform: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `variable "jamfplatform_base_url"`) {
		t.Error("expected jamfplatform_base_url variable")
	}
	if !strings.Contains(s, `variable "jamfplatform_client_id"`) {
		t.Error("expected jamfplatform_client_id variable")
	}
	if !strings.Contains(s, `variable "jamfplatform_client_secret"`) {
		t.Error("expected jamfplatform_client_secret variable")
	}
	if !strings.Contains(s, `type        = string`) {
		t.Error("expected string type for variables")
	}
	if !strings.Contains(s, "sensitive") {
		t.Error("expected sensitive attribute on credential variables")
	}
}

func TestGenerateJSC_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	creds := &JSCCredentials{
		Username: "user@example.com",
		Password: "test-password",
	}

	if err := GenerateJSC(dir, creds); err != nil {
		t.Fatalf("GenerateJSC: %v", err)
	}

	for _, f := range []string{"provider.tf", "variables.tf", "terraform.tfvars"} {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			t.Errorf("expected file %s to be created", f)
		}
	}
}

func TestGenerateJSC_ProviderFile(t *testing.T) {
	dir := t.TempDir()

	creds := &JSCCredentials{
		Username: "user@example.com",
		Password: "test-password",
	}

	if err := GenerateJSC(dir, creds); err != nil {
		t.Fatalf("GenerateJSC: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "provider.tf"))
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `source = "Jamf-Concepts/jsctfprovider"`) {
		t.Error("expected provider source Jamf-Concepts/jsctfprovider")
	}
	if !strings.Contains(s, `provider "jsc"`) {
		t.Error("expected jsc provider block")
	}
	if !strings.Contains(s, "var.jsc_username") {
		t.Error("expected var.jsc_username in provider block")
	}
	if !strings.Contains(s, "var.jsc_password") {
		t.Error("expected var.jsc_password in provider block")
	}
}

func TestGenerateJSC_VariablesFile(t *testing.T) {
	dir := t.TempDir()

	creds := &JSCCredentials{
		Username: "user@example.com",
		Password: "test-password",
	}

	if err := GenerateJSC(dir, creds); err != nil {
		t.Fatalf("GenerateJSC: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `variable "jsc_username"`) {
		t.Error("expected jsc_username variable")
	}
	if !strings.Contains(s, `variable "jsc_password"`) {
		t.Error("expected jsc_password variable")
	}
	if !strings.Contains(s, `type        = string`) {
		t.Error("expected string type for variables")
	}
	if !strings.Contains(s, "sensitive") {
		t.Error("expected sensitive attribute on password variable")
	}
}

func TestGenerateJSC_TfvarsFile(t *testing.T) {
	dir := t.TempDir()

	creds := &JSCCredentials{
		Username: "user@example.com",
		Password: "test-password",
	}

	if err := GenerateJSC(dir, creds); err != nil {
		t.Fatalf("GenerateJSC: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, `"user@example.com"`) {
		t.Error("expected username in terraform.tfvars")
	}
	if !strings.Contains(s, `"test-password"`) {
		t.Error("expected password in terraform.tfvars")
	}
	if !strings.Contains(s, "jsc_username") {
		t.Error("expected jsc_username key")
	}
	if !strings.Contains(s, "jsc_password") {
		t.Error("expected jsc_password key")
	}
}

func TestGenerateJSC_SpecialCharacters(t *testing.T) {
	dir := t.TempDir()

	creds := &JSCCredentials{
		Username: "user@example.com",
		Password: `p@ss"word\with'special`,
	}

	if err := GenerateJSC(dir, creds); err != nil {
		t.Fatalf("GenerateJSC: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "terraform.tfvars"))
	if err != nil {
		t.Fatalf("reading terraform.tfvars: %v", err)
	}

	s := string(content)
	// %q format should properly escape special characters
	if !strings.Contains(s, `p@ss\"word\\with'special`) {
		t.Errorf("expected properly escaped password in terraform.tfvars, got:\n%s", s)
	}
}

func TestWriteGitignore(t *testing.T) {
	t.Run("creates .gitignore with expected entries", func(t *testing.T) {
		dir := t.TempDir()
		if err := WriteGitignore(dir); err != nil {
			t.Fatalf("WriteGitignore: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(content)
		for _, expected := range []string{
			"*.tfstate",
			"*.tfvars",
			".terraform/",
			".terraform.lock.hcl",
		} {
			if !strings.Contains(s, expected) {
				t.Errorf(".gitignore missing %q", expected)
			}
		}
	})

	t.Run("does not overwrite existing file", func(t *testing.T) {
		dir := t.TempDir()
		custom := "# custom gitignore\n"
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(custom), 0644); err != nil {
			t.Fatal(err)
		}
		if err := WriteGitignore(dir); err != nil {
			t.Fatalf("WriteGitignore: %v", err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if string(content) != custom {
			t.Error("expected existing .gitignore to be preserved")
		}
	})

	t.Run("created by Generate", func(t *testing.T) {
		dir := t.TempDir()
		creds := &Credentials{AuthMethod: "basic"}
		results := &discovery.Results{}
		if err := Generate(dir, creds, results); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); os.IsNotExist(err) {
			t.Error("expected .gitignore to be created by Generate")
		}
	})

	t.Run("created by GenerateProtect", func(t *testing.T) {
		dir := t.TempDir()
		creds := &ProtectCredentials{URL: "https://test.protect.jamfcloud.com"}
		if err := GenerateProtect(dir, creds); err != nil {
			t.Fatalf("GenerateProtect: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); os.IsNotExist(err) {
			t.Error("expected .gitignore to be created by GenerateProtect")
		}
	})

	t.Run("created by GeneratePlatform", func(t *testing.T) {
		dir := t.TempDir()
		creds := &PlatformCredentials{BaseURL: "https://us.apigw.jamf.com"}
		if err := GeneratePlatform(dir, creds); err != nil {
			t.Fatalf("GeneratePlatform: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); os.IsNotExist(err) {
			t.Error("expected .gitignore to be created by GeneratePlatform")
		}
	})

	t.Run("created by GenerateJSC", func(t *testing.T) {
		dir := t.TempDir()
		creds := &JSCCredentials{Username: "u", Password: "p"}
		if err := GenerateJSC(dir, creds); err != nil {
			t.Fatalf("GenerateJSC: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); os.IsNotExist(err) {
			t.Error("expected .gitignore to be created by GenerateJSC")
		}
	})
}
