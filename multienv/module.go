// Copyright 2026, Jamf Software LLC

package multienv

import (
	"crypto/sha256"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/Jamf-Concepts/jamformer/terraform"
)

// classifySupportFiles compares support files extracted per-env by SHA-256 hash.
// Files identical across all environments are classified as shared (module);
// files that differ get per-env copies (divergent).
func classifySupportFiles(outputDir, sourceEnv string, envNames []string) ([]ClassifiedFile, error) {
	sourceDir := filepath.Join(outputDir, "support_files", sourceEnv)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, nil // no support files at all
	}

	var classified []ClassifiedFile

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		// Hash the source env's copy
		sourceHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", path, err)
		}

		// Compare against all other envs
		shared := true
		for _, envName := range envNames {
			if envName == sourceEnv {
				continue
			}
			otherPath := filepath.Join(outputDir, "support_files", envName, relPath)
			otherHash, err := hashFile(otherPath)
			if err != nil {
				// File doesn't exist in this env - still shared (only exists in source)
				continue
			}
			if otherHash != sourceHash {
				shared = false
				break
			}
		}

		class := SupportFileShared
		if !shared {
			class = SupportFileDivergent
		}
		classified = append(classified, ClassifiedFile{
			RelPath: relPath,
			Class:   class,
		})
		return nil
	})

	return classified, err
}

// hashFile returns the hex-encoded SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// assembleModule moves resource .tf files and shared support files into
// modules/jamf/, rewriting file() references to strip the env prefix.
func assembleModule(outputDir, sourceEnv string, classified []ClassifiedFile) error {
	moduleDir := filepath.Join(outputDir, "modules", "jamf")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		return fmt.Errorf("creating module directory: %w", err)
	}

	// Move resource .tf files from output root into module
	files, err := filepath.Glob(filepath.Join(outputDir, "*.tf"))
	if err != nil {
		return err
	}
	for _, f := range files {
		base := filepath.Base(f)
		if shouldSkipForModule(base) {
			continue
		}
		dst := filepath.Join(moduleDir, base)
		if err := os.Rename(f, dst); err != nil {
			return fmt.Errorf("moving %s to module: %w", base, err)
		}
	}

	// Move shared support files into module
	for _, cf := range classified {
		if cf.Class != SupportFileShared {
			continue
		}
		src := filepath.Join(outputDir, "support_files", sourceEnv, cf.RelPath)
		dst := filepath.Join(moduleDir, "support_files", cf.RelPath)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			// File might not exist if only in source env
			if !os.IsNotExist(err) {
				return fmt.Errorf("moving shared file %s: %w", cf.RelPath, err)
			}
		}
	}

	// Rewrite file() references in module .tf files to strip the source env prefix
	// e.g. support_files/prod/scripts/foo.sh → support_files/scripts/foo.sh
	oldPrefix := "support_files/" + sourceEnv + "/"
	newPrefix := "support_files/"
	moduleFiles, _ := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	for _, f := range moduleFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		if !strings.Contains(content, oldPrefix) {
			continue
		}
		updated := strings.ReplaceAll(content, oldPrefix, newPrefix)
		if err := os.WriteFile(f, []byte(updated), 0644); err != nil {
			return fmt.Errorf("rewriting %s: %w", filepath.Base(f), err)
		}
	}

	// Clean up now-empty support_files directories (shared files have been moved)
	cleanupEmptyDirs(filepath.Join(outputDir, "support_files"))

	return nil
}

// shouldSkipForModule returns true for files that should NOT be moved into the module.
func shouldSkipForModule(basename string) bool {
	if strings.HasSuffix(basename, "_import.tf") {
		return true
	}
	switch basename {
	case "provider.tf", "variables.tf", "terraform.tfvars", "locals_env.tf", ".gitignore":
		return true
	}
	return strings.HasSuffix(basename, ".tfvars")
}

// cleanupEmptyDirs removes empty directories under the given root, bottom-up.
func cleanupEmptyDirs(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		return err // just walk
	})
	// Walk bottom-up by collecting dirs then removing in reverse
	var dirs []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // only removes if empty
	}
}

// splitPartialEnvResources separates resources that exist in only some
// environments into clearly-labeled files like policies_staging_only.tf.
func splitPartialEnvResources(moduleDir string, matches []MatchedResource, typeToFileMap map[string]string) error {
	// Build map of partial-env resources: "type.label" → sorted env list
	partial := make(map[string][]string)
	for _, m := range matches {
		if m.AllEnvs {
			continue
		}
		addr := m.ResourceType + "." + m.Label
		envs := make([]string, len(m.Present))
		copy(envs, m.Present)
		sort.Strings(envs)
		partial[addr] = envs
	}
	if len(partial) == 0 {
		return nil
	}

	// Build reverse map: output filename → resource type
	typeToFile := make(map[string]string, len(typeToFileMap))
	maps.Copy(typeToFile, typeToFileMap)

	// Process each .tf file in the module
	moduleFiles, _ := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	for _, file := range moduleFiles {
		base := filepath.Base(file)
		if base == "variables.tf" || strings.HasSuffix(base, "_only.tf") {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		f, diags := hclwrite.ParseConfig(data, file, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}

		// Collect blocks to move, grouped by env-set suffix
		type moveTarget struct {
			suffix string // e.g. "staging_only" or "dev_staging_only"
			blocks []*hclwrite.Block
		}
		targets := make(map[string]*moveTarget)
		var blocksToRemove []*hclwrite.Block

		for _, block := range f.Body().Blocks() {
			if block.Type() != "resource" {
				continue
			}
			labels := block.Labels()
			if len(labels) < 2 {
				continue
			}
			addr := labels[0] + "." + labels[1]
			envs, ok := partial[addr]
			if !ok {
				continue
			}

			suffix := strings.Join(envs, "_") + "_only"
			if targets[suffix] == nil {
				targets[suffix] = &moveTarget{suffix: suffix}
			}
			targets[suffix].blocks = append(targets[suffix].blocks, block)
			blocksToRemove = append(blocksToRemove, block)
		}

		if len(blocksToRemove) == 0 {
			continue
		}

		// Remove partial blocks from the shared file
		for _, block := range blocksToRemove {
			f.Body().RemoveBlock(block)
		}
		if err := os.WriteFile(file, f.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", base, err)
		}

		// Write partial blocks to _<envs>_only.tf files
		stem := strings.TrimSuffix(base, ".tf")
		for _, target := range targets {
			outFile := filepath.Join(moduleDir, fmt.Sprintf("%s_%s.tf", stem, target.suffix))
			newF := hclwrite.NewEmptyFile()
			first := true
			for _, block := range target.blocks {
				// Serialize and reparse to copy the block cleanly
				blockBytes := block.BuildTokens(nil).Bytes()
				parsed, pDiags := hclwrite.ParseConfig(blockBytes, "", hcl.Pos{Line: 1, Column: 1})
				if pDiags.HasErrors() {
					continue
				}
				for _, pb := range parsed.Body().Blocks() {
					if !first {
						newF.Body().AppendNewline()
					}
					appendBlockToBody(newF.Body(), pb)
					first = false
				}
			}
			if err := os.WriteFile(outFile, newF.Bytes(), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", filepath.Base(outFile), err)
			}
		}
	}

	return nil
}

// appendBlockToBody copies a block from one file to another body via serialization.
func appendBlockToBody(dst *hclwrite.Body, src *hclwrite.Block) {
	newBlock := dst.AppendNewBlock(src.Type(), src.Labels())
	for name, attr := range src.Body().Attributes() {
		newBlock.Body().SetAttributeRaw(name, attr.Expr().BuildTokens(nil))
	}
	for _, block := range src.Body().Blocks() {
		appendBlockToBody(newBlock.Body(), block)
	}
}

// rewriteDivergentFileRefs converts file() references for divergent support
// files into module variable references. Returns the list of module variables
// that need to be declared.
func rewriteDivergentFileRefs(moduleDir string, divergent []ClassifiedFile) ([]ModuleVar, error) {
	if len(divergent) == 0 {
		return nil, nil
	}

	// Build a map of divergent file relative paths for quick lookup
	divergentPaths := make(map[string]bool)
	for _, cf := range divergent {
		divergentPaths[cf.RelPath] = true
	}

	var moduleVars []ModuleVar

	moduleFiles, _ := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	for _, file := range moduleFiles {
		base := filepath.Base(file)
		if base == "variables.tf" {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		f, diags := hclwrite.ParseConfig(data, file, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}

		modified := false
		for _, block := range f.Body().Blocks() {
			if block.Type() != "resource" {
				continue
			}
			labels := block.Labels()
			if len(labels) < 2 {
				continue
			}
			resourceType := labels[0]
			label := labels[1]

			for attrName, attr := range block.Body().Attributes() {
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if !strings.Contains(val, "file(") || !strings.Contains(val, "support_files/") {
					continue
				}

				// Check if this references a divergent file
				for _, cf := range divergent {
					if !strings.Contains(val, cf.RelPath) {
						continue
					}

					// Generate variable name
					varName := generateFileVarName(resourceType, label, attrName)

					// Replace file() with var reference
					block.Body().SetAttributeRaw(attrName, hclwrite.Tokens{
						{Type: hclsyntax.TokenIdent, Bytes: []byte("var." + varName)},
					})

					moduleVars = append(moduleVars, ModuleVar{
						Name:         varName,
						Description:  fmt.Sprintf("Content of %s for %s.%s", cf.RelPath, resourceType, label),
						ResourceType: resourceType,
						Label:        label,
						AttrName:     attrName,
						FilePath:     cf.RelPath,
					})
					modified = true
					break
				}
			}
		}

		if modified {
			if err := os.WriteFile(file, f.Bytes(), 0644); err != nil {
				return nil, fmt.Errorf("rewriting %s: %w", base, err)
			}
		}
	}

	return moduleVars, nil
}

// collapseBlankLines removes consecutive blank lines from all .tf files in a
// directory, leaving at most one blank line between blocks.
func collapseBlankLines(dir string) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.tf"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var out []string
		prevBlank := false
		for _, line := range lines {
			blank := strings.TrimSpace(line) == ""
			if blank && prevBlank {
				continue // skip consecutive blank lines
			}
			out = append(out, line)
			prevBlank = blank
		}
		// Trim leading blank line
		for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
			out = out[1:]
		}
		_ = os.WriteFile(file, []byte(strings.Join(out, "\n")), 0644)
	}
}

// generateFileVarName creates a variable name for a divergent support file.
// e.g. ("jamfpro_script", "install_agent", "script_contents") → "script_install_agent_script_contents"
func generateFileVarName(resourceType, label, attrName string) string {
	typeName := resourceType
	if idx := strings.Index(typeName, "_"); idx >= 0 {
		typeName = typeName[idx+1:]
	}
	return fmt.Sprintf("%s_%s_%s", typeName, label, attrName)
}

// scanFileVarRefs scans module .tf files for file(var.X) patterns that
// postprocessing created (e.g. for device enrollment tokens, VPP tokens).
// These variables need to be declared in the module and passed through.
func scanFileVarRefs(moduleDir string) []ModuleVar {
	var result []ModuleVar
	seen := make(map[string]bool)

	files, _ := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	for _, file := range files {
		if filepath.Base(file) == "variables.tf" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		f, diags := hclwrite.ParseConfig(data, file, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}
		for _, block := range f.Body().Blocks() {
			if block.Type() != "resource" {
				continue
			}
			labels := block.Labels()
			if len(labels) < 2 {
				continue
			}
			for attrName, attr := range block.Body().Attributes() {
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				// Match file(var.X) pattern
				if !strings.HasPrefix(val, "file(var.") {
					continue
				}
				varName := strings.TrimPrefix(val, "file(var.")
				varName = strings.TrimSuffix(varName, ")")
				if seen[varName] {
					continue
				}
				seen[varName] = true
				result = append(result, ModuleVar{
					Name:         varName,
					Description:  fmt.Sprintf("Path to file for %s.%s %s", labels[0], labels[1], attrName),
					ResourceType: labels[0],
					Label:        labels[1],
					AttrName:     attrName,
				})
			}
		}
	}
	return result
}

// generateModuleProviders writes a required_providers block in modules/jamf/providers.tf
// so Terraform knows which provider supplies the jamfpro_* resources.
func generateModuleProviders(moduleDir, pinnedVersion, resolvedVersion string) error {
	versionLine := terraform.FormatVersionConstraint(pinnedVersion, resolvedVersion)
	content := fmt.Sprintf(`terraform {
  required_providers {
    jamfpro = {
      source = "deploymenttheory/jamfpro"%s
    }
  }
}
`, versionLine)
	return os.WriteFile(filepath.Join(moduleDir, "providers.tf"), []byte(content), 0644)
}

// generateModuleVariables writes modules/jamf/variables.tf with variable blocks
// for diff-extracted attributes and divergent support file content.
func generateModuleVariables(moduleDir string, diffs []AttrDiff, fileVars []ModuleVar) error {
	// Combine all variables into a single sortable list
	type varEntry struct {
		name         string
		resourceType string
		block        string
	}
	var entries []varEntry

	for _, d := range diffs {
		var b strings.Builder
		fmt.Fprintf(&b, "variable %q {\n", d.VarName)
		fmt.Fprintf(&b, "  description = \"Environment-specific value for %s.%s %s\"\n", d.ResourceType, d.Label, d.AttrName)
		fmt.Fprintf(&b, "  type        = %s\n", d.VarType)
		fmt.Fprintf(&b, "}\n")
		entries = append(entries, varEntry{name: d.VarName, resourceType: d.ResourceType, block: b.String()})
	}

	for _, v := range fileVars {
		var b strings.Builder
		fmt.Fprintf(&b, "variable %q {\n", v.Name)
		fmt.Fprintf(&b, "  description = %q\n", v.Description)
		fmt.Fprintf(&b, "  type        = string\n")
		fmt.Fprintf(&b, "}\n")
		entries = append(entries, varEntry{name: v.Name, resourceType: v.ResourceType, block: b.String()})
	}

	if len(entries) == 0 {
		return nil // no variables needed
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	var content strings.Builder
	prevType := ""
	for _, e := range entries {
		if e.resourceType != prevType {
			if prevType != "" {
				content.WriteString("\n")
			}
			fmt.Fprintf(&content, "# %s\n\n", e.resourceType)
		}
		content.WriteString(e.block)
		content.WriteString("\n")
		prevType = e.resourceType
	}

	return os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(content.String()), 0644)
}
