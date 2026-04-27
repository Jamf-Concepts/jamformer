// Copyright 2026, Jamf Software LLC

package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RemediateResult summarises what the remediation changed.
type RemediateResult struct {
	VariablesAdded int
	TFFilesChanged int
	SupportFiles   int // support_files converted to templatefile()
	Skipped        int // secrets embedded in larger strings that couldn't be auto-replaced
}

// Remediate moves detected secrets to sensitive Terraform variables.
//
// For .tf file findings: replaces the literal value with var.<name> and appends
// the variable declaration + tfvars entry.
//
// For support_files findings: replaces the secret with a template variable,
// renames the file to .tpl, updates the .tf reference from file() to
// templatefile(), and creates the variable + tfvars entry.
func Remediate(dir string, findings []Finding) (*RemediateResult, error) {
	result := &RemediateResult{}

	// Collect variable declarations and tfvars entries
	var varBlocks []string
	var tfvarsEntries []string
	usedNames := make(map[string]bool)

	// Track which .tf files we've already modified (re-read after each change)
	changedTFFiles := make(map[string]bool)
	changedSupportFiles := make(map[string]bool)

	for i := range findings {
		f := &findings[i]

		varName := buildVarName(f, usedNames)
		if varName == "" {
			continue
		}

		if f.InSupportFiles {
			if err := remediateSupportFile(dir, f, varName); err != nil {
				return nil, fmt.Errorf("remediating support file %s: %w", f.SupportFileRef, err)
			}
			changedSupportFiles[f.File] = true
		} else if strings.HasSuffix(f.File, ".tf") {
			replaced, err := remediateTFFile(f, varName)
			if err != nil {
				return nil, fmt.Errorf("remediating %s: %w", filepath.Base(f.File), err)
			}
			if !replaced {
				result.Skipped++
				continue
			}
			changedTFFiles[f.File] = true
		} else {
			continue
		}

		varBlocks = append(varBlocks, buildVariableBlock(varName, f))
		tfvarsEntries = append(tfvarsEntries, fmt.Sprintf("%s = %q", varName, f.Secret))
		result.VariablesAdded++
	}

	result.TFFilesChanged = len(changedTFFiles)
	result.SupportFiles = len(changedSupportFiles)

	// Append to variables.tf
	if len(varBlocks) > 0 {
		if err := appendToFile(filepath.Join(dir, "variables.tf"),
			"\n"+strings.Join(varBlocks, "\n")); err != nil {
			return nil, fmt.Errorf("writing variables.tf: %w", err)
		}
	}

	// Append to terraform.tfvars
	if len(tfvarsEntries) > 0 {
		if err := appendToFile(filepath.Join(dir, "terraform.tfvars"),
			"\n# Secrets extracted by jamformer secret scan\n"+
				strings.Join(tfvarsEntries, "\n")+"\n"); err != nil {
			return nil, fmt.Errorf("writing terraform.tfvars: %w", err)
		}
	}

	return result, nil
}

// remediateTFFile replaces a secret literal in a .tf file with a variable reference.
// Returns true if the replacement was made, false if the secret wasn't found as a
// standalone quoted value (e.g. embedded in a larger string like "prefix=SECRET").
func remediateTFFile(f *Finding, varName string) (bool, error) {
	content, err := os.ReadFile(f.File)
	if err != nil {
		return false, err
	}

	text := string(content)

	// Try replacing the exact quoted secret with a variable reference
	// e.g. "SuperSecret!" → var.webhook_password
	old := fmt.Sprintf("%q", f.Secret)
	newVal := "var." + varName
	updated := strings.Replace(text, old, newVal, 1)

	if updated == text {
		// Secret is embedded in a larger string — use interpolation instead
		// e.g. "enrollauthtoken=SECRET" → "enrollauthtoken=${var.name}"
		updated = strings.Replace(text, f.Secret, "${var."+varName+"}", 1)
	}

	if updated == text {
		return false, nil
	}

	return true, os.WriteFile(f.File, []byte(updated), 0644)
}

// remediateSupportFile replaces the secret inside a support file with a template
// variable, renames the file to .tpl, and updates the .tf reference from file()
// to templatefile(). When multiple secrets exist in the same file, the first call
// converts it to .tpl; subsequent calls update the .tpl in place.
func remediateSupportFile(dir string, f *Finding, varName string) error {
	tplPath := f.File + ".tpl"

	// If the original file was already renamed to .tpl by a previous finding
	// in the same file, update the .tpl in place (skip escape + rename).
	if _, err := os.Stat(f.File); os.IsNotExist(err) {
		if _, statErr := os.Stat(tplPath); statErr != nil {
			return fmt.Errorf("file not found: %s", f.File)
		}
		content, readErr := os.ReadFile(tplPath)
		if readErr != nil {
			return readErr
		}
		templateVar := "${" + varName + "}"
		updated := strings.Replace(string(content), f.Secret, templateVar, 1)
		if err := os.WriteFile(tplPath, []byte(updated), 0644); err != nil {
			return err
		}
		// Add the new variable to the existing templatefile() call in the .tf
		return updateFileRefToTemplatefile(dir, f.SupportFileRef, varName)
	}

	// First finding in this file: escape, replace, rename to .tpl, update .tf ref.
	content, err := os.ReadFile(f.File)
	if err != nil {
		return err
	}

	text := string(content)

	// When converting to a Terraform templatefile, any existing ${...} and %{...}
	// patterns in the file must be escaped as $${...} and %%{...} respectively,
	// otherwise Terraform will try to interpolate them.
	text = escapeTemplateExpressions(text)

	// Now replace the secret with our template variable reference.
	templateVar := "${" + varName + "}"
	updated := strings.Replace(text, f.Secret, templateVar, 1)

	if err := os.WriteFile(tplPath, []byte(updated), 0644); err != nil {
		return err
	}
	if err := os.Remove(f.File); err != nil {
		return err
	}

	return updateFileRefToTemplatefile(dir, f.SupportFileRef, varName)
}

// updateFileRefToTemplatefile finds the file() reference in .tf files and
// converts it to templatefile() with the variable map. If the reference was
// already converted (by a previous finding in the same file), the new variable
// is added to the existing templatefile() variable map.
func updateFileRefToTemplatefile(dir, supportFileRef, varName string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Case 1: first finding — convert file() → templatefile()
	oldFileRef := fmt.Sprintf(`file("${path.module}/%s")`, supportFileRef)
	newRef := fmt.Sprintf(`templatefile("${path.module}/%s.tpl", { %s = var.%s })`,
		supportFileRef, varName, varName)

	// Case 2: subsequent finding — add variable to existing templatefile() map
	// Match the existing templatefile() call for this file and append the new var
	// before the closing })
	tplPathRef := fmt.Sprintf(`templatefile("${path.module}/%s.tpl"`, supportFileRef)
	newVarEntry := fmt.Sprintf(", %s = var.%s", varName, varName)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}

		tfPath := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(tfPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		text := string(content)

		if strings.Contains(text, oldFileRef) {
			// First finding: replace file() with templatefile()
			updated := strings.Replace(text, oldFileRef, newRef, 1)
			if err := os.WriteFile(tfPath, []byte(updated), 0644); err != nil {
				return fmt.Errorf("updating %s: %w", entry.Name(), err)
			}
			return nil
		}

		if strings.Contains(text, tplPathRef) {
			// Subsequent finding: add variable to existing templatefile() map
			// Find the closing }) of the templatefile() call and insert before it
			idx := strings.Index(text, tplPathRef)
			if idx == -1 {
				continue
			}
			// Find the "})" that closes this templatefile() call, allowing
			// optional whitespace before it (e.g. " })", "})", "\n})")
			rest := text[idx:]
			closeIdx := strings.Index(rest, "})")
			if closeIdx == -1 {
				continue
			}
			insertPos := idx + closeIdx
			updated := text[:insertPos] + newVarEntry + " " + text[insertPos:]
			if err := os.WriteFile(tfPath, []byte(updated), 0644); err != nil {
				return fmt.Errorf("updating %s: %w", entry.Name(), err)
			}
			return nil
		}
	}

	return nil
}

// buildVarName generates a unique, valid Terraform variable name from a finding.
func buildVarName(f *Finding, used map[string]bool) string {
	var parts []string

	if f.ResourceAddress != "" {
		parts = append(parts, f.ResourceAddress)
	} else if f.SupportFileRef != "" {
		// Use the support file name without extension
		base := filepath.Base(f.SupportFileRef)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		parts = append(parts, base)
	} else {
		return ""
	}

	if f.AttrName != "" {
		parts = append(parts, f.AttrName)
	} else {
		parts = append(parts, "secret")
	}

	name := strings.Join(parts, "_")
	name = sanitizeVarName(name)

	// Deduplicate
	base := name
	counter := 2
	for used[name] {
		name = fmt.Sprintf("%s_%d", base, counter)
		counter++
	}
	used[name] = true

	return name
}

// sanitizeVarName converts a string to a valid Terraform variable name.
var varNameRe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeVarName(s string) string {
	s = strings.ToLower(s)
	s = varNameRe.ReplaceAllString(s, "_")
	// Collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	// Must start with a letter or underscore
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	return s
}

// escapeTemplateExpressions escapes existing ${...} and %{...} patterns in file
// content so they are treated as literals by Terraform's templatefile() function.
// In Terraform templates, $${...} produces a literal ${...} and %%{...} produces
// a literal %{...}.
func escapeTemplateExpressions(s string) string {
	// Escape ${ → $${ and %{ → %%{
	s = strings.ReplaceAll(s, "${", "$${")
	s = strings.ReplaceAll(s, "%{", "%%{")
	return s
}

// appendToFile appends content to a file.
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

// buildVariableBlock generates a Terraform variable declaration for a secret.
func buildVariableBlock(varName string, f *Finding) string {
	desc := "Sensitive value"
	if f.ResourceAddress != "" {
		desc = fmt.Sprintf("Sensitive value from %s", f.ResourceAddress)
	} else if f.SupportFileRef != "" {
		desc = fmt.Sprintf("Sensitive value from %s", f.SupportFileRef)
	}

	return fmt.Sprintf(`variable %q {
  description = %q
  type        = string
  sensitive   = true
}
`, varName, desc)
}
