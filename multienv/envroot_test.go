// Copyright 2026, Jamf Software LLC

package multienv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEnvMainTF_OAuth2(t *testing.T) {
	envDir := t.TempDir()
	env := EnvConfig{
		Name:       "prod",
		URL:        "https://prod.jamfcloud.com",
		AuthMethod: "oauth2",
	}
	diffs := []AttrDiff{
		{VarName: "policy_chrome_priority", ResourceType: "jamfpro_policy", Label: "chrome", AttrName: "priority", VarType: "string"},
	}

	if err := generateEnvMainTF(envDir, env, diffs, nil, "", "1.0.0", 300); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `source = "deploymenttheory/jamfpro"`) {
		t.Error("missing provider source")
	}
	if !strings.Contains(content, `>= 1.0.0`) {
		t.Error("missing version constraint")
	}
	if !strings.Contains(content, "client_id") {
		t.Error("missing oauth2 client_id")
	}
	if !strings.Contains(content, "client_secret") {
		t.Error("missing oauth2 client_secret")
	}
	if !strings.Contains(content, "token_refresh_buffer_period_seconds = 300") {
		t.Error("missing token refresh period")
	}
	if !strings.Contains(content, `source = "../../modules/jamf"`) {
		t.Error("missing module source")
	}
	if !strings.Contains(content, "policy_chrome_priority") {
		t.Error("missing diff variable in module call")
	}
}

func TestGenerateEnvMainTF_SortedAndGrouped(t *testing.T) {
	envDir := t.TempDir()
	env := EnvConfig{
		Name:       "prod",
		URL:        "https://prod.jamfcloud.com",
		AuthMethod: "basic",
	}
	diffs := []AttrDiff{
		{VarName: "script_deploy_category_id", ResourceType: "jamfpro_script", VarType: "string"},
		{VarName: "category_browsers_priority", ResourceType: "jamfpro_category", VarType: "string"},
		{VarName: "policy_chrome_name", ResourceType: "jamfpro_policy", VarType: "string"},
		{VarName: "category_apps_priority", ResourceType: "jamfpro_category", VarType: "string"},
	}

	if err := generateEnvMainTF(envDir, env, diffs, nil, "", "", 0); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should be sorted alphabetically
	catAppsIdx := strings.Index(content, "category_apps_priority")
	catBrowIdx := strings.Index(content, "category_browsers_priority")
	polIdx := strings.Index(content, "policy_chrome_name")
	scrIdx := strings.Index(content, "script_deploy_category_id")

	if catAppsIdx < 0 || catBrowIdx < 0 || polIdx < 0 || scrIdx < 0 {
		t.Fatalf("missing variables in module call:\n%s", content)
	}
	if catAppsIdx > catBrowIdx || catBrowIdx > polIdx || polIdx > scrIdx {
		t.Errorf("variables not sorted in module call:\n%s", content)
	}

	// Should have resource type comments
	if !strings.Contains(content, "# jamfpro_category") {
		t.Error("missing category group comment in module call")
	}
	if !strings.Contains(content, "# jamfpro_policy") {
		t.Error("missing policy group comment in module call")
	}
}

func TestGenerateEnvMainTF_BasicAuth(t *testing.T) {
	envDir := t.TempDir()
	env := EnvConfig{
		Name:       "dev",
		URL:        "https://dev.jamfcloud.com",
		AuthMethod: "basic",
	}

	if err := generateEnvMainTF(envDir, env, nil, nil, "", "", 0); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "basic_auth_username") {
		t.Error("missing basic auth username")
	}
	if !strings.Contains(content, "basic_auth_password") {
		t.Error("missing basic auth password")
	}
	if strings.Contains(content, "client_id") {
		t.Error("should not contain oauth2 fields for basic auth")
	}
	if strings.Contains(content, "token_refresh_buffer_period") {
		t.Error("should not contain token refresh for basic auth")
	}
}

func TestGenerateEnvBackendTF(t *testing.T) {
	envDir := t.TempDir()

	if err := generateEnvBackendTF(envDir, "staging"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "backend.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "jamf/staging/terraform.tfstate") {
		t.Error("missing env-specific state key in example")
	}
	if !strings.Contains(content, "#") {
		t.Error("backend should be commented out")
	}
}

func TestGenerateEnvVariablesTF_OAuth2(t *testing.T) {
	envDir := t.TempDir()
	env := EnvConfig{
		Name:       "prod",
		URL:        "https://prod.jamfcloud.com",
		AuthMethod: "oauth2",
	}
	diffs := []AttrDiff{
		{VarName: "policy_chrome_priority", ResourceType: "jamfpro_policy", Label: "chrome", AttrName: "priority", VarType: "string"},
	}

	if err := generateEnvVariablesTF(envDir, env, diffs, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "variables.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `default     = "https://prod.jamfcloud.com"`) {
		t.Error("missing instance URL default")
	}
	if !strings.Contains(content, `default     = "oauth2"`) {
		t.Error("missing auth method default")
	}
	if !strings.Contains(content, "jamfpro_client_id") {
		t.Error("missing client_id variable")
	}
	if !strings.Contains(content, "sensitive   = true") {
		t.Error("missing sensitive flag")
	}
	if !strings.Contains(content, "policy_chrome_priority") {
		t.Error("missing diff pass-through variable")
	}
}

func TestGenerateEnvVariablesTF_BasicAuth(t *testing.T) {
	envDir := t.TempDir()
	env := EnvConfig{
		Name:       "dev",
		URL:        "https://dev.jamfcloud.com",
		AuthMethod: "basic",
	}

	if err := generateEnvVariablesTF(envDir, env, nil, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "variables.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "jamfpro_basic_auth_username") {
		t.Error("missing username variable")
	}
	if !strings.Contains(content, "jamfpro_basic_auth_password") {
		t.Error("missing password variable")
	}
	if strings.Contains(content, "client_id") {
		t.Error("should not contain oauth2 vars for basic auth")
	}
}

func TestGenerateEnvTfvars(t *testing.T) {
	envDir := t.TempDir()
	diffs := []AttrDiff{
		{
			VarName:      "policy_chrome_priority",
			ResourceType: "jamfpro_policy",
			Values:       map[string]string{"dev": "5", "prod": "10"},
		},
		{
			VarName:      "policy_chrome_name",
			ResourceType: "jamfpro_policy",
			Values:       map[string]string{"dev": `"Chrome Dev"`, "prod": `"Chrome"`},
		},
	}

	if err := generateEnvTfvars(envDir, "prod", diffs); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "terraform.tfvars"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `policy_chrome_name`) || !strings.Contains(content, `"Chrome"`) {
		t.Error("missing name value (should preserve quotes)")
	}
	if !strings.Contains(content, `policy_chrome_priority`) || !strings.Contains(content, `"10"`) {
		t.Error("missing priority value")
	}
	if !strings.Contains(content, "prod") {
		t.Error("missing environment name in comment")
	}
	if !strings.Contains(content, "# jamfpro_policy") {
		t.Error("missing resource type comment")
	}
}

func TestGenerateEnvTfvars_Sorting(t *testing.T) {
	envDir := t.TempDir()
	diffs := []AttrDiff{
		{VarName: "script_deploy_category_id", ResourceType: "jamfpro_script", Values: map[string]string{"prod": `"1"`}},
		{VarName: "category_browsers_priority", ResourceType: "jamfpro_category", Values: map[string]string{"prod": `"5"`}},
		{VarName: "policy_chrome_name", ResourceType: "jamfpro_policy", Values: map[string]string{"prod": `"Chrome"`}},
		{VarName: "category_apps_priority", ResourceType: "jamfpro_category", Values: map[string]string{"prod": `"3"`}},
	}

	if err := generateEnvTfvars(envDir, "prod", diffs); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "terraform.tfvars"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should be sorted: category_apps, category_browsers, policy_chrome, script_deploy
	catAppsIdx := strings.Index(content, "category_apps_priority")
	catBrowIdx := strings.Index(content, "category_browsers_priority")
	polIdx := strings.Index(content, "policy_chrome_name")
	scrIdx := strings.Index(content, "script_deploy_category_id")

	if catAppsIdx < 0 || catBrowIdx < 0 || polIdx < 0 || scrIdx < 0 {
		t.Fatalf("missing variables in output:\n%s", content)
	}
	if catAppsIdx > catBrowIdx || catBrowIdx > polIdx || polIdx > scrIdx {
		t.Errorf("variables not sorted alphabetically:\n%s", content)
	}

	// Should have resource type grouping comments
	if !strings.Contains(content, "# jamfpro_category") {
		t.Error("missing category group comment")
	}
	if !strings.Contains(content, "# jamfpro_policy") {
		t.Error("missing policy group comment")
	}
}

func TestGenerateEnvImports(t *testing.T) {
	envDir := t.TempDir()
	matches := []MatchedResource{
		{
			ResourceType: "jamfpro_script",
			Label:        "install_rosetta",
			IDs:          map[string]string{"dev": "123", "prod": "456"},
		},
		{
			ResourceType: "jamfpro_policy",
			Label:        "deploy_chrome",
			IDs:          map[string]string{"prod": "789"},
		},
	}

	if err := generateEnvImports(envDir, matches, "prod"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(envDir, "imports.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should have module.jamf. prefix
	if !strings.Contains(content, "module.jamf.jamfpro_script.install_rosetta") {
		t.Error("missing module-prefixed script import")
	}
	if !strings.Contains(content, `"456"`) {
		t.Error("missing prod script ID")
	}
	if !strings.Contains(content, "module.jamf.jamfpro_policy.deploy_chrome") {
		t.Error("missing module-prefixed policy import")
	}
	if !strings.Contains(content, `"789"`) {
		t.Error("missing prod policy ID")
	}
	// Should NOT contain dev-only resources
	if strings.Contains(content, `"123"`) {
		t.Error("should not contain dev ID in prod imports")
	}
}

func TestGenerateEnvImports_NoResources(t *testing.T) {
	envDir := t.TempDir()
	matches := []MatchedResource{
		{
			ResourceType: "jamfpro_script",
			Label:        "test",
			IDs:          map[string]string{"dev": "123"},
		},
	}

	// "prod" has no resources
	if err := generateEnvImports(envDir, matches, "prod"); err != nil {
		t.Fatal(err)
	}

	// imports.tf should not be created
	if _, err := os.Stat(filepath.Join(envDir, "imports.tf")); !os.IsNotExist(err) {
		t.Error("imports.tf should not be created when env has no resources")
	}
}

func TestPlaceDivergentFiles(t *testing.T) {
	outputDir := t.TempDir()
	envDir := t.TempDir()

	// Create divergent file in source extraction dir
	srcDir := filepath.Join(outputDir, "support_files", "prod", "scripts")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "agent.sh"), []byte("#!/bin/bash\nprod version"), 0644); err != nil {
		t.Fatal(err)
	}

	divergent := []ClassifiedFile{
		{RelPath: "scripts/agent.sh", Class: SupportFileDivergent},
	}

	if err := placeDivergentFiles(envDir, "prod", outputDir, divergent); err != nil {
		t.Fatal(err)
	}

	// File should be in env's support_files
	dst := filepath.Join(envDir, "support_files", "scripts", "agent.sh")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal("divergent file not placed in env directory")
	}
	if !strings.Contains(string(data), "prod version") {
		t.Error("file content mismatch")
	}
}

func TestPlaceDivergentFiles_Empty(t *testing.T) {
	envDir := t.TempDir()
	// Should be a no-op with no divergent files
	if err := placeDivergentFiles(envDir, "prod", t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateEnvRoot_Integration(t *testing.T) {
	outputDir := t.TempDir()
	env := EnvConfig{
		Name:       "staging",
		URL:        "https://staging.jamfcloud.com",
		AuthMethod: "oauth2",
	}
	matches := []MatchedResource{
		{
			ResourceType: "jamfpro_category",
			Label:        "browsers",
			IDs:          map[string]string{"staging": "42"},
		},
	}

	if err := generateEnvRoot(outputDir, env, matches, nil, nil, nil, "", "1.0.0", 300); err != nil {
		t.Fatal(err)
	}

	envDir := filepath.Join(outputDir, "environments", "staging")

	// All expected files should exist
	for _, f := range []string{"main.tf", "backend.tf", "variables.tf", "terraform.tfvars", "imports.tf"} {
		if _, err := os.Stat(filepath.Join(envDir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
}
