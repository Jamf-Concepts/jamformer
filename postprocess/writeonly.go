// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// requiredVar describes a sensitive Terraform variable jamformer synthesises for a
// Required WriteOnly attribute the server never returns. Wiring the attribute to
// var.<Name> lets a generated config satisfy the attribute and validate, while
// leaving the actual secret for the user to supply.
type requiredVar struct {
	Name        string
	Description string
}

// injectRequiredWriteOnly fills top-level Required attributes the provider omits
// on read. generate-config-out emits these as null (or absent), so the provider's
// own schema then rejects the config with "Missing Configuration for Required
// Attribute". A Required WriteOnly attribute (a user-supplied secret/token the
// server never echoes) is wired to a sensitive Terraform variable; its
// `<attr>_wo_version` rotation companion is filled with 1 (the initial-write
// value) so the secret is sent on the first apply. Other Required-but-missing
// attributes are left untouched (they belong to other audit categories).
// Attribute names are processed in sorted order so output is deterministic.
func injectRequiredWriteOnly(body *hclwrite.Body, resourceType, label string, schema *ProviderSchema, used map[string]bool) []requiredVar {
	attrs := schema.requiredTopLevelAttrs(resourceType)
	if attrs == nil {
		return nil
	}

	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)

	var vars []requiredVar
	for _, name := range names {
		info := attrs[name]
		if !info.Required {
			continue
		}
		if existing := body.GetAttribute(name); existing != nil && !isNullValue(existing) {
			continue
		}

		switch {
		case info.WriteOnly:
			varName := requiredVarName(resourceType, label, name, used)
			body.SetAttributeRaw(name, hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: []byte("var." + varName)},
			})
			vars = append(vars, requiredVar{
				Name:        varName,
				Description: fmt.Sprintf("Write-only secret for %s.%s (%s); Jamf never returns it, so supply it before apply.", resourceType, label, name),
			})
		case isWoVersionAttr(name):
			body.SetAttributeValue(name, cty.NumberIntVal(1))
		}
	}
	return vars
}

// isWoVersionAttr reports whether an attribute is a WriteOnly secret's rotation
// companion (its name ends in _wo_version). Bumping it forces a re-send of the
// paired secret; jamformer seeds it to 1 for the initial write.
func isWoVersionAttr(name string) bool {
	return strings.HasSuffix(name, "_wo_version")
}

// requiredVarName builds a unique, valid Terraform variable name from the
// resource address and attribute, mirroring the secret-remediation convention.
func requiredVarName(resourceType, label, attr string, used map[string]bool) string {
	base := sanitizeVarName(resourceType + "_" + label + "_" + attr)
	name := base
	for counter := 2; used[name]; counter++ {
		name = fmt.Sprintf("%s_%d", base, counter)
	}
	used[name] = true
	return name
}

// appendRequiredVars appends sensitive variable declarations to variables.tf and
// commented placeholders to terraform.tfvars for each synthesised WriteOnly var.
// The placeholders stay commented so an unset secret surfaces at apply rather than
// silently applying an empty value; validate passes regardless (the variable is
// declared and needs no value).
func appendRequiredVars(outputDir string, vars []requiredVar) error {
	if len(vars) == 0 {
		return nil
	}

	var varBlocks, tfvarsLines []string
	for _, v := range vars {
		varBlocks = append(varBlocks, fmt.Sprintf("variable %q {\n  description = %q\n  type        = string\n  sensitive   = true\n}\n", v.Name, v.Description))
		tfvarsLines = append(tfvarsLines, fmt.Sprintf("# %s = \"REPLACE_ME\"", v.Name))
	}

	if err := appendToFile(filepath.Join(outputDir, "variables.tf"),
		"\n# Write-only secrets synthesised by jamformer (Jamf never returns these)\n"+
			strings.Join(varBlocks, "\n")); err != nil {
		return fmt.Errorf("writing variables.tf: %w", err)
	}
	if err := appendToFile(filepath.Join(outputDir, "terraform.tfvars"),
		"\n# Write-only secrets — uncomment and supply a value before apply\n"+
			strings.Join(tfvarsLines, "\n")+"\n"); err != nil {
		return fmt.Errorf("writing terraform.tfvars: %w", err)
	}
	return nil
}

// sanitizeVarName converts a string to a valid Terraform variable name.
var varNameRe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeVarName(s string) string {
	s = strings.ToLower(s)
	s = varNameRe.ReplaceAllString(s, "_")
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	return s
}

// appendToFile appends content to a file, creating it if absent.
func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
