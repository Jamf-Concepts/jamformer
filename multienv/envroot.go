// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/Jamf-Concepts/jamformer/terraform"
)

// generateEnvRoot creates the full environment root directory with main.tf,
// backend.tf, variables.tf, terraform.tfvars, imports.tf, and divergent
// support files.
func generateEnvRoot(outputDir string, env EnvConfig, matches []MatchedResource, diffs []AttrDiff, divergent []ClassifiedFile, fileVars []ModuleVar, pinnedVersion, resolvedVersion string, tokenRefreshPeriod int) error {
	envDir := filepath.Join(outputDir, "environments", env.Name)
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return fmt.Errorf("creating env directory %s: %w", env.Name, err)
	}

	if err := generateEnvMainTF(envDir, env, diffs, fileVars, pinnedVersion, resolvedVersion, tokenRefreshPeriod); err != nil {
		return fmt.Errorf("generating main.tf for %s: %w", env.Name, err)
	}
	if err := generateEnvBackendTF(envDir, env.Name); err != nil {
		return fmt.Errorf("generating backend.tf for %s: %w", env.Name, err)
	}
	if err := generateEnvVariablesTF(envDir, env, diffs, fileVars); err != nil {
		return fmt.Errorf("generating variables.tf for %s: %w", env.Name, err)
	}
	if err := generateEnvTfvars(envDir, env.Name, diffs); err != nil {
		return fmt.Errorf("generating terraform.tfvars for %s: %w", env.Name, err)
	}
	if err := generateEnvImports(envDir, matches, env.Name); err != nil {
		return fmt.Errorf("generating imports.tf for %s: %w", env.Name, err)
	}
	if err := placeDivergentFiles(envDir, env.Name, outputDir, divergent); err != nil {
		return fmt.Errorf("placing divergent files for %s: %w", env.Name, err)
	}

	return nil
}

// generateEnvMainTF writes the main.tf with provider config and module call.
func generateEnvMainTF(envDir string, env EnvConfig, diffs []AttrDiff, fileVars []ModuleVar, pinnedVersion, resolvedVersion string, tokenRefreshPeriod int) error {
	versionLine := terraform.FormatVersionConstraint(pinnedVersion, resolvedVersion)

	var providerAttrs string
	if env.AuthMethod == "oauth2" {
		providerAttrs = `  client_id             = var.jamfpro_client_id
  client_secret         = var.jamfpro_client_secret`
		if tokenRefreshPeriod > 0 {
			providerAttrs += fmt.Sprintf("\n  token_refresh_buffer_period_seconds = %d", tokenRefreshPeriod)
		}
	} else {
		providerAttrs = `  basic_auth_username   = var.jamfpro_basic_auth_username
  basic_auth_password   = var.jamfpro_basic_auth_password`
	}

	// Build module call arguments: collect all entries, sort, group by resource type
	type moduleArg struct {
		resourceType string
		varName      string
		value        string
	}
	var args []moduleArg
	for _, d := range diffs {
		args = append(args, moduleArg{
			resourceType: d.ResourceType,
			varName:      d.VarName,
			value:        fmt.Sprintf("var.%s", d.VarName),
		})
	}
	for _, v := range fileVars {
		var value string
		if v.FilePath != "" {
			// Divergent support file - use file() with path relative to env root
			relPath := filepath.ToSlash(v.FilePath)
			value = fmt.Sprintf("file(\"${path.root}/support_files/%s\")", relPath)
		} else {
			// Token path var (file(var.X) in module) - pass through as var
			value = fmt.Sprintf("var.%s", v.Name)
		}
		args = append(args, moduleArg{
			resourceType: v.ResourceType,
			varName:      v.Name,
			value:        value,
		})
	}
	sort.Slice(args, func(i, j int) bool { return args[i].varName < args[j].varName })

	// Find max varName length for alignment
	maxLen := 0
	for _, a := range args {
		if len(a.varName) > maxLen {
			maxLen = len(a.varName)
		}
	}

	var moduleArgStr strings.Builder
	prevType := ""
	for _, a := range args {
		if a.resourceType != prevType {
			fmt.Fprintf(&moduleArgStr, "\n\n  # %s", a.resourceType)
		}
		fmt.Fprintf(&moduleArgStr, "\n  %-*s = %s", maxLen, a.varName, a.value)
		prevType = a.resourceType
	}

	content := fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}

provider "jamfpro" {
  jamfpro_instance_fqdn = var.jamfpro_instance_url
  auth_method           = var.jamfpro_auth_method
%s
}

module "jamf" {
  source = "../../modules/jamf"%s
}
`, versionLine, providerAttrs, moduleArgStr.String())

	return os.WriteFile(filepath.Join(envDir, "main.tf"), []byte(content), 0644)
}

// generateEnvBackendTF writes a commented-out backend placeholder.
func generateEnvBackendTF(envDir, envName string) error {
	content := fmt.Sprintf(`# Configure your Terraform backend below.
# Example using S3:
#
# terraform {
#   backend "s3" {
#     bucket = "myorg-jamf-state"
#     key    = "jamf/%s/terraform.tfstate"
#     region = "us-east-1"
#   }
# }
`, envName)

	return os.WriteFile(filepath.Join(envDir, "backend.tf"), []byte(content), 0644)
}

// generateEnvVariablesTF writes the variables.tf for an environment with auth
// variables and pass-through variables for module inputs.
func generateEnvVariablesTF(envDir string, env EnvConfig, diffs []AttrDiff, fileVars []ModuleVar) error {
	var content strings.Builder

	fmt.Fprintf(&content, "variable \"jamfpro_instance_url\" {\n")
	fmt.Fprintf(&content, "  description = \"Jamf Pro instance URL\"\n")
	fmt.Fprintf(&content, "  type        = string\n")
	fmt.Fprintf(&content, "  default     = %q\n", env.URL)
	fmt.Fprintf(&content, "}\n\n")

	fmt.Fprintf(&content, "variable \"jamfpro_auth_method\" {\n")
	fmt.Fprintf(&content, "  description = \"Authentication method for the Jamf Pro provider\"\n")
	fmt.Fprintf(&content, "  type        = string\n")
	fmt.Fprintf(&content, "  default     = %q\n", env.AuthMethod)
	fmt.Fprintf(&content, "}\n\n")

	if env.AuthMethod == "oauth2" {
		content.WriteString(`variable "jamfpro_client_id" {
  description = "Jamf Pro API client ID for OAuth2 authentication"
  type        = string
  sensitive   = true
}

variable "jamfpro_client_secret" {
  description = "Jamf Pro API client secret for OAuth2 authentication"
  type        = string
  sensitive   = true
}

`)
	} else {
		content.WriteString(`variable "jamfpro_basic_auth_username" {
  description = "Jamf Pro username for basic authentication"
  type        = string
}

variable "jamfpro_basic_auth_password" {
  description = "Jamf Pro password for basic authentication"
  type        = string
  sensitive   = true
}

`)
	}

	// Pass-through variables for diffs (declared here so user can set in tfvars,
	// then passed to the module via main.tf)
	sorted := sortedDiffs(diffs)
	prevType := ""
	for _, d := range sorted {
		if d.ResourceType != prevType {
			if prevType != "" {
				content.WriteString("\n")
			}
			fmt.Fprintf(&content, "# %s\n\n", d.ResourceType)
		}
		fmt.Fprintf(&content, "variable %q {\n", d.VarName)
		fmt.Fprintf(&content, "  description = \"Environment-specific value for %s.%s %s\"\n", d.ResourceType, d.Label, d.AttrName)
		fmt.Fprintf(&content, "  type        = %s\n", d.VarType)
		fmt.Fprintf(&content, "}\n\n")
		prevType = d.ResourceType
	}

	// Pass-through variables for file(var.X) patterns (token paths etc.)
	for _, v := range fileVars {
		if v.FilePath != "" {
			continue // divergent file vars are handled by file() in main.tf, not passed through
		}
		fmt.Fprintf(&content, "variable %q {\n", v.Name)
		fmt.Fprintf(&content, "  description = %q\n", v.Description)
		fmt.Fprintf(&content, "  type        = string\n")
		fmt.Fprintf(&content, "}\n\n")
	}

	return os.WriteFile(filepath.Join(envDir, "variables.tf"), []byte(content.String()), 0644)
}

// generateEnvTfvars writes the terraform.tfvars for an environment.
func generateEnvTfvars(envDir, envName string, diffs []AttrDiff) error {
	var content strings.Builder
	fmt.Fprintf(&content, "# Terraform variables for the %q environment.\n", envName)
	fmt.Fprintf(&content, "# Usage: terraform plan\n\n")

	sorted := sortedDiffs(diffs)

	// Find max varName length for alignment
	maxLen := 0
	for _, d := range sorted {
		if len(d.VarName) > maxLen {
			maxLen = len(d.VarName)
		}
	}

	prevType := ""
	for _, d := range sorted {
		val, ok := d.Values[envName]
		if !ok {
			continue
		}
		if d.ResourceType != prevType {
			if prevType != "" {
				content.WriteString("\n")
			}
			fmt.Fprintf(&content, "# %s\n", d.ResourceType)
		}
		switch {
		case val == "null":
			fmt.Fprintf(&content, "%-*s = null\n", maxLen, d.VarName)
		case strings.HasPrefix(val, `"`), strings.HasPrefix(val, "["), strings.HasPrefix(val, "{"):
			fmt.Fprintf(&content, "%-*s = %s\n", maxLen, d.VarName, val)
		default:
			fmt.Fprintf(&content, "%-*s = %q\n", maxLen, d.VarName, val)
		}
		prevType = d.ResourceType
	}

	return os.WriteFile(filepath.Join(envDir, "terraform.tfvars"), []byte(content.String()), 0644)
}

// generateEnvImports writes import blocks for all resources present in this
// environment, with module.jamf. prefix on the to address.
func generateEnvImports(envDir string, matches []MatchedResource, envName string) error {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for _, m := range matches {
		id, ok := m.IDs[envName]
		if !ok {
			continue // resource not in this env
		}

		importBlock := body.AppendNewBlock("import", nil)
		ib := importBlock.Body()

		// to = module.jamf.<type>.<label>
		toAddr := fmt.Sprintf("module.jamf.%s.%s", m.ResourceType, m.Label)
		ib.SetAttributeRaw("to", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(toAddr)},
		})

		// id = "<jamf_id>"
		ib.SetAttributeRaw("id", ctyStringVal(id))

		body.AppendNewline()
	}

	data := f.Bytes()
	if len(data) == 0 {
		return nil // no resources in this env
	}

	return os.WriteFile(filepath.Join(envDir, "imports.tf"), data, 0644)
}

// ctyStringVal creates an hclwrite-compatible string value token.
func ctyStringVal(s string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(s)},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
	}
}

// sortedDiffs returns a copy of diffs sorted alphabetically by VarName.
func sortedDiffs(diffs []AttrDiff) []AttrDiff {
	sorted := make([]AttrDiff, len(diffs))
	copy(sorted, diffs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].VarName < sorted[j].VarName })
	return sorted
}

// placeDivergentFiles copies divergent support files from the per-env
// extraction directory to the environment's support_files/ directory.
func placeDivergentFiles(envDir, envName, outputDir string, divergent []ClassifiedFile) error {
	if len(divergent) == 0 {
		return nil
	}

	for _, cf := range divergent {
		src := filepath.Join(outputDir, "support_files", envName, cf.RelPath)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue // file doesn't exist in this env
		}

		dst := filepath.Join(envDir, "support_files", cf.RelPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", dst, err)
		}
	}

	return nil
}
