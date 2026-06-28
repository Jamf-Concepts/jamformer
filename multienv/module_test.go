// Copyright 2026, Jamf Software LLC

package multienv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	hash1, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile() error: %v", err)
	}
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// Same content produces same hash
	path2 := filepath.Join(dir, "test2.txt")
	if err := os.WriteFile(path2, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	hash2, _ := hashFile(path2)
	if hash1 != hash2 {
		t.Errorf("same content should produce same hash: %q != %q", hash1, hash2)
	}

	// Different content produces different hash
	path3 := filepath.Join(dir, "test3.txt")
	if err := os.WriteFile(path3, []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	hash3, _ := hashFile(path3)
	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}

	// Missing file errors
	_, err = hashFile(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestGenerateModuleProviders(t *testing.T) {
	t.Run("with pinned version", func(t *testing.T) {
		dir := t.TempDir()
		if err := generateModuleProviders(dir, proProvider{}, "1.2.3", ""); err != nil {
			t.Fatal(err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "providers.tf"))
		if !strings.Contains(string(content), `source = "deploymenttheory/jamfpro"`) {
			t.Error("expected provider source")
		}
		if !strings.Contains(string(content), `version = "1.2.3"`) {
			t.Errorf("expected pinned version, got:\n%s", content)
		}
	})

	t.Run("without version", func(t *testing.T) {
		dir := t.TempDir()
		if err := generateModuleProviders(dir, proProvider{}, "", ""); err != nil {
			t.Fatal(err)
		}
		content, _ := os.ReadFile(filepath.Join(dir, "providers.tf"))
		if strings.Contains(string(content), "version") {
			t.Errorf("expected no version line, got:\n%s", content)
		}
	})
}

func TestClassifySupportFiles(t *testing.T) {
	t.Run("identical files are shared", func(t *testing.T) {
		dir := t.TempDir()
		// Create identical files in two envs
		for _, env := range []string{"dev", "prod"} {
			p := filepath.Join(dir, "support_files", env, "scripts")
			if err := os.MkdirAll(p, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "install.sh"), []byte("#!/bin/bash\necho hello"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		classified, err := classifySupportFiles(dir, "prod", []string{"dev", "prod"})
		if err != nil {
			t.Fatal(err)
		}
		if len(classified) != 1 {
			t.Fatalf("expected 1 classified file, got %d", len(classified))
		}
		if classified[0].Class != SupportFileShared {
			t.Errorf("expected shared, got divergent")
		}
		if classified[0].RelPath != "scripts/install.sh" {
			t.Errorf("unexpected relpath: %s", classified[0].RelPath)
		}
	})

	t.Run("different files are divergent", func(t *testing.T) {
		dir := t.TempDir()
		for _, env := range []string{"dev", "prod"} {
			p := filepath.Join(dir, "support_files", env, "scripts")
			if err := os.MkdirAll(p, 0755); err != nil {
				t.Fatal(err)
			}
			content := "#!/bin/bash\necho " + env
			if err := os.WriteFile(filepath.Join(p, "install.sh"), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}

		classified, err := classifySupportFiles(dir, "prod", []string{"dev", "prod"})
		if err != nil {
			t.Fatal(err)
		}
		if len(classified) != 1 {
			t.Fatalf("expected 1 classified file, got %d", len(classified))
		}
		if classified[0].Class != SupportFileDivergent {
			t.Errorf("expected divergent, got shared")
		}
	})

	t.Run("no support files returns nil", func(t *testing.T) {
		dir := t.TempDir()
		classified, err := classifySupportFiles(dir, "prod", []string{"dev", "prod"})
		if err != nil {
			t.Fatal(err)
		}
		if classified != nil {
			t.Errorf("expected nil, got %v", classified)
		}
	})

	t.Run("file only in source env is shared", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "support_files", "prod", "scripts")
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "only_prod.sh"), []byte("#!/bin/bash"), 0644); err != nil {
			t.Fatal(err)
		}

		classified, err := classifySupportFiles(dir, "prod", []string{"dev", "prod"})
		if err != nil {
			t.Fatal(err)
		}
		if len(classified) != 1 {
			t.Fatalf("expected 1 classified file, got %d", len(classified))
		}
		if classified[0].Class != SupportFileShared {
			t.Errorf("file only in source env should be shared, got divergent")
		}
	})
}

func TestAssembleModule(t *testing.T) {
	dir := t.TempDir()

	// Create resource files in output root
	if err := os.WriteFile(filepath.Join(dir, "policies.tf"), []byte(`resource "jamfpro_policy" "test" {}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts.tf"), []byte(`resource "jamfpro_script" "test" {
  script_contents = file("${path.module}/support_files/prod/scripts/test.sh")
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Files that should NOT be moved
	if err := os.WriteFile(filepath.Join(dir, "provider.tf"), []byte("provider {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts_import.tf"), []byte("import {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables.tf"), []byte("variable {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create shared support file
	srcScript := filepath.Join(dir, "support_files", "prod", "scripts")
	if err := os.MkdirAll(srcScript, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcScript, "test.sh"), []byte("#!/bin/bash"), 0644); err != nil {
		t.Fatal(err)
	}

	classified := []ClassifiedFile{
		{RelPath: "scripts/test.sh", Class: SupportFileShared},
	}

	if err := assembleModule(dir, "prod", classified); err != nil {
		t.Fatal(err)
	}

	moduleDir := filepath.Join(dir, "modules", "jamf")

	// Resource files moved to module
	if _, err := os.Stat(filepath.Join(moduleDir, "policies.tf")); err != nil {
		t.Error("policies.tf not moved to module")
	}
	if _, err := os.Stat(filepath.Join(moduleDir, "scripts.tf")); err != nil {
		t.Error("scripts.tf not moved to module")
	}

	// Skipped files remain in root
	if _, err := os.Stat(filepath.Join(dir, "provider.tf")); err != nil {
		t.Error("provider.tf should remain in root")
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts_import.tf")); err != nil {
		t.Error("scripts_import.tf should remain in root")
	}
	if _, err := os.Stat(filepath.Join(dir, "variables.tf")); err != nil {
		t.Error("variables.tf should remain in root")
	}

	// Shared support file moved to module
	if _, err := os.Stat(filepath.Join(moduleDir, "support_files", "scripts", "test.sh")); err != nil {
		t.Error("shared support file not moved to module")
	}

	// file() references rewritten to strip env prefix
	data, err := os.ReadFile(filepath.Join(moduleDir, "scripts.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "support_files/prod/") {
		t.Error("env prefix not stripped from file() reference")
	}
	if !strings.Contains(content, "support_files/scripts/test.sh") {
		t.Error("expected rewritten file() reference")
	}
}

func TestSplitPartialEnvResources(t *testing.T) {
	moduleDir := t.TempDir()

	// Create a policies.tf with shared and partial resources
	hcl := `resource "jamfpro_policy" "shared_policy" {
  name = "Shared"
}

resource "jamfpro_policy" "staging_only" {
  name = "Staging Only"
}

resource "jamfpro_policy" "prod_only" {
  name = "Prod Only"
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "policies.tf"), []byte(hcl), 0644); err != nil {
		t.Fatal(err)
	}

	matches := []MatchedResource{
		{ResourceType: "jamfpro_policy", Label: "shared_policy", AllEnvs: true, Present: []string{"staging", "prod"}},
		{ResourceType: "jamfpro_policy", Label: "staging_only", AllEnvs: false, Present: []string{"staging"}},
		{ResourceType: "jamfpro_policy", Label: "prod_only", AllEnvs: false, Present: []string{"prod"}},
	}
	typeToFileMap := map[string]string{"jamfpro_policy": "policies.tf"}

	if err := splitPartialEnvResources(moduleDir, matches, typeToFileMap); err != nil {
		t.Fatal(err)
	}

	// Shared file should only contain shared_policy
	data, err := os.ReadFile(filepath.Join(moduleDir, "policies.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "shared_policy") {
		t.Error("shared_policy should remain in policies.tf")
	}
	if strings.Contains(content, "staging_only") {
		t.Error("staging_only should be removed from policies.tf")
	}
	if strings.Contains(content, "prod_only") {
		t.Error("prod_only should be removed from policies.tf")
	}

	// staging_only.tf should exist
	stagingFile := filepath.Join(moduleDir, "policies_staging_only.tf")
	if _, err := os.Stat(stagingFile); err != nil {
		t.Fatal("policies_staging_only.tf not created")
	}
	data, err = os.ReadFile(stagingFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "staging_only") {
		t.Error("staging_only resource not in policies_staging_only.tf")
	}

	// prod_only.tf should exist
	prodFile := filepath.Join(moduleDir, "policies_prod_only.tf")
	if _, err := os.Stat(prodFile); err != nil {
		t.Fatal("policies_prod_only.tf not created")
	}
	data, err = os.ReadFile(prodFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "prod_only") {
		t.Error("prod_only resource not in policies_prod_only.tf")
	}
}

func TestSplitPartialEnvResources_NoPartials(t *testing.T) {
	moduleDir := t.TempDir()
	hcl := `resource "jamfpro_policy" "shared" {
  name = "Shared"
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "policies.tf"), []byte(hcl), 0644); err != nil {
		t.Fatal(err)
	}

	matches := []MatchedResource{
		{ResourceType: "jamfpro_policy", Label: "shared", AllEnvs: true},
	}

	if err := splitPartialEnvResources(moduleDir, matches, map[string]string{}); err != nil {
		t.Fatal(err)
	}

	// No _only files should be created
	onlyFiles, _ := filepath.Glob(filepath.Join(moduleDir, "*_only.tf"))
	if len(onlyFiles) > 0 {
		t.Errorf("no _only.tf files expected, got %v", onlyFiles)
	}
}

func TestGenerateModuleVariables(t *testing.T) {
	moduleDir := t.TempDir()

	diffs := []AttrDiff{
		{
			ResourceType: "jamfpro_policy",
			Label:        "install_chrome",
			AttrName:     "priority",
			VarName:      "policy_install_chrome_priority",
			VarType:      "string",
		},
	}
	fileVars := []ModuleVar{
		{
			Name:         "script_install_agent_script_contents",
			Description:  "Content of scripts/install_agent.sh for jamfpro_script.install_agent",
			ResourceType: "jamfpro_script",
			Label:        "install_agent",
			AttrName:     "script_contents",
		},
	}

	if err := generateModuleVariables(moduleDir, diffs, fileVars); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(moduleDir, "variables.tf"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `"policy_install_chrome_priority"`) {
		t.Error("missing diff variable")
	}
	if !strings.Contains(content, `"script_install_agent_script_contents"`) {
		t.Error("missing file variable")
	}
	if !strings.Contains(content, "type        = string") {
		t.Error("missing type declaration")
	}
	// Policy comes before script alphabetically
	polIdx := strings.Index(content, "policy_install_chrome")
	scrIdx := strings.Index(content, "script_install_agent")
	if polIdx > scrIdx {
		t.Error("variables should be sorted alphabetically")
	}
	// Should have resource type comments
	if !strings.Contains(content, "# jamfpro_policy") {
		t.Error("missing policy group comment")
	}
	if !strings.Contains(content, "# jamfpro_script") {
		t.Error("missing script group comment")
	}
}

func TestScanFileVarRefs(t *testing.T) {
	dir := t.TempDir()

	// Create a .tf file with file(var.X) pattern
	tf := `resource "jamfpro_device_enrollments" "test_enrollment" {
  name          = "Test"
  encoded_token = file(var.dep_token_path_test_enrollment)
}
`
	if err := os.WriteFile(filepath.Join(dir, "device_enrollments.tf"), []byte(tf), 0644); err != nil {
		t.Fatal(err)
	}

	vars := scanFileVarRefs(dir)
	if len(vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(vars))
	}
	if vars[0].Name != "dep_token_path_test_enrollment" {
		t.Errorf("name = %q, want %q", vars[0].Name, "dep_token_path_test_enrollment")
	}
	if vars[0].FilePath != "" {
		t.Error("FilePath should be empty for token path vars")
	}
	if vars[0].ResourceType != "jamfpro_device_enrollments" {
		t.Errorf("ResourceType = %q", vars[0].ResourceType)
	}
}

func TestScanFileVarRefs_SkipsRegularFiles(t *testing.T) {
	dir := t.TempDir()

	// Regular file() with string literal - should NOT be picked up
	tf := `resource "jamfpro_script" "test" {
  script_contents = file("${path.module}/support_files/scripts/test.sh")
}
`
	if err := os.WriteFile(filepath.Join(dir, "scripts.tf"), []byte(tf), 0644); err != nil {
		t.Fatal(err)
	}

	vars := scanFileVarRefs(dir)
	if len(vars) != 0 {
		t.Errorf("expected 0 vars for regular file() refs, got %d", len(vars))
	}
}

func TestCollapseBlankLines(t *testing.T) {
	dir := t.TempDir()

	// File with leading blank line and consecutive blank lines
	content := "\n\nresource \"test\" \"a\" {\n}\n\n\nresource \"test\" \"b\" {\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "test.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	collapseBlankLines(dir)

	data, err := os.ReadFile(filepath.Join(dir, "test.tf"))
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)

	// Should not start with a blank line
	if strings.HasPrefix(result, "\n") {
		t.Error("leading blank line not removed")
	}
	// Should not have consecutive blank lines
	if strings.Contains(result, "\n\n\n") {
		t.Error("consecutive blank lines not collapsed")
	}
	// Should still have one blank line between blocks
	if !strings.Contains(result, "}\n\n") {
		t.Error("single blank line between blocks should be preserved")
	}
}

func TestGenerateModuleVariables_Empty(t *testing.T) {
	moduleDir := t.TempDir()

	if err := generateModuleVariables(moduleDir, nil, nil); err != nil {
		t.Fatal(err)
	}

	// No variables.tf should be created
	if _, err := os.Stat(filepath.Join(moduleDir, "variables.tf")); !os.IsNotExist(err) {
		t.Error("variables.tf should not be created when no variables needed")
	}
}

func TestGenerateFileVarName(t *testing.T) {
	tests := []struct {
		resourceType, label, attr string
		want                      string
	}{
		{"jamfpro_script", "install_agent", "script_contents", "script_install_agent_script_contents"},
		{"jamfpro_policy", "deploy_chrome", "priority", "policy_deploy_chrome_priority"},
	}
	for _, tt := range tests {
		got := generateFileVarName(tt.resourceType, tt.label, tt.attr)
		if got != tt.want {
			t.Errorf("generateFileVarName(%q, %q, %q) = %q, want %q", tt.resourceType, tt.label, tt.attr, got, tt.want)
		}
	}
}

func TestShouldSkipForModule(t *testing.T) {
	tests := []struct {
		name string
		skip bool
	}{
		{"provider.tf", true},
		{"variables.tf", true},
		{"terraform.tfvars", true},
		{"locals_env.tf", true},
		{"dev.tfvars", true},
		{"scripts_import.tf", true},
		{".gitignore", true},
		{"policies.tf", false},
		{"scripts.tf", false},
		{"categories.tf", false},
	}
	for _, tt := range tests {
		got := shouldSkipForModule(tt.name)
		if got != tt.skip {
			t.Errorf("shouldSkipForModule(%q) = %v, want %v", tt.name, got, tt.skip)
		}
	}
}

func TestScanWriteOnlyVarRefs_NestedObjectAttr(t *testing.T) {
	dir := t.TempDir()
	// A top-level secret and a secret nested inside an object-expression
	// attribute (the pro_* idiom). Both must be detected; a diff var passed in
	// `known` must be skipped.
	src := `resource "jamfplatform_pro_disk_encryption_configuration" "test" {
  some_diff_attr = var.known_diff
  institutional_recovery_key = {
    data = var.dek_secret
  }
}

resource "jamfplatform_pro_account" "a" {
  password = var.account_secret
}
`
	if err := os.WriteFile(filepath.Join(dir, "r.tf"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	known := map[string]bool{"known_diff": true}
	vars := scanWriteOnlyVarRefs(dir, known)

	got := map[string]bool{}
	for _, v := range vars {
		got[v.Name] = true
		if !v.Sensitive {
			t.Errorf("var %q should be marked sensitive", v.Name)
		}
	}
	if !got["dek_secret"] {
		t.Error("expected nested-object var dek_secret to be detected")
	}
	if !got["account_secret"] {
		t.Error("expected top-level var account_secret to be detected")
	}
	if got["known_diff"] {
		t.Error("known diff var should not be re-collected")
	}
}
