// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// DiffResources compares generated HCL across environments for matched resources
// and identifies attributes that differ. Differing attributes are replaced with
// variable references in the module .tf files and the per-env values are returned.
// moduleDir is the directory containing postprocessed resource .tf files
// (e.g. output/modules/jamf/).
func DiffResources(moduleDir string, envResults map[string]*PerEnvResult, matches []MatchedResource, sourceEnv string) ([]AttrDiff, error) {
	// Parse generated.tf from each non-source env for comparison
	envBlocks := make(map[string]map[string]*hclwrite.Body) // envName → "type.label" → body
	for envName, result := range envResults {
		genFile := filepath.Join(result.OutputDir, "generated.tf")
		data, err := os.ReadFile(genFile)
		if err != nil {
			continue // env may not have generated.tf if discovery found nothing
		}
		f, diags := hclwrite.ParseConfig(data, genFile, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			continue
		}
		blocks := make(map[string]*hclwrite.Body)
		for _, block := range f.Body().Blocks() {
			if block.Type() == "resource" {
				labels := block.Labels()
				if len(labels) >= 2 {
					key := labels[0] + "." + labels[1]
					blocks[key] = block.Body()
				}
			}
		}
		envBlocks[envName] = blocks
	}

	if len(envBlocks) < 2 {
		return nil, nil // nothing to diff
	}
	if envBlocks[sourceEnv] == nil {
		return nil, nil // source env has no parseable resources
	}

	// Build a set of attributes that postprocessing already resolved to file()
	// refs or Terraform resource references. These should not be diffed —
	// file() refs are handled by per-env support files, and resource references
	// (like jamfpro_category.foo.id) are correct cross-resource links.
	resolvedAttrs := make(map[string]map[string]bool) // "type.label" → set of attr names
	outputFiles, _ := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	for _, file := range outputFiles {
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
			key := labels[0] + "." + labels[1]
			for name, attr := range block.Body().Attributes() {
				val := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if strings.Contains(val, "file(") || isResourceReference(val) {
					if resolvedAttrs[key] == nil {
						resolvedAttrs[key] = make(map[string]bool)
					}
					resolvedAttrs[key][name] = true
				}
			}
		}
	}

	var diffs []AttrDiff

	for _, match := range matches {
		if !match.AllEnvs {
			continue // only diff resources present in all envs
		}

		key := match.ResourceType + "." + match.Label

		// Get the source env's body as baseline
		sourceBody, ok := envBlocks[sourceEnv][key]
		if !ok {
			continue
		}

		// Compare each attribute across envs
		for attrName, sourceAttr := range sourceBody.Attributes() {
			sourceVal := strings.TrimSpace(string(sourceAttr.Expr().BuildTokens(nil).Bytes()))

			// Skip attributes that are already variable references or file() calls
			if strings.HasPrefix(sourceVal, "var.") || strings.Contains(sourceVal, "file(") ||
				strings.Contains(sourceVal, "local.") || strings.Contains(sourceVal, "terraform.workspace") {
				continue
			}

			// Skip attributes that postprocessing already resolved (file() refs,
			// resource references like jamfpro_category.foo.id, etc.)
			if resolvedAttrs[key] != nil && resolvedAttrs[key][attrName] {
				continue
			}

			// Compare against all other envs
			allMatch := true
			values := map[string]string{sourceEnv: sourceVal}
			for envName, blocks := range envBlocks {
				if envName == sourceEnv {
					continue
				}
				body, ok := blocks[key]
				if !ok {
					allMatch = false
					break
				}
				attr := body.GetAttribute(attrName)
				if attr == nil {
					allMatch = false
					break
				}
				envVal := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				values[envName] = envVal
				if envVal != sourceVal {
					allMatch = false
				}
			}

			if allMatch {
				continue // identical across all envs, no diff needed
			}

			// Check if we just couldn't find the attr in some env (skip those)
			if len(values) < len(envBlocks) {
				continue
			}

			// If every env has null, skip — the attribute is absent everywhere
			// and postprocessing strips it. Any other combination (including
			// "-1" vs null) is a real diff worth preserving.
			allNull := true
			for _, v := range values {
				if v != "null" {
					allNull = false
					break
				}
			}
			if allNull {
				continue
			}

			// Generate variable name: strip provider prefix, combine with label and attr
			typeName := match.ResourceType
			if idx := strings.Index(typeName, "_"); idx >= 0 {
				typeName = typeName[idx+1:]
			}
			varName := fmt.Sprintf("%s_%s_%s", typeName, match.Label, attrName)

			// Determine variable type; make it optional if any env has null
			varType := inferVarType(sourceVal)
			for _, v := range values {
				if v == "null" {
					varType = "optional(" + varType + ")"
					break
				}
			}

			diffs = append(diffs, AttrDiff{
				ResourceType: match.ResourceType,
				Label:        match.Label,
				AttrName:     attrName,
				Values:       values,
				VarName:      varName,
				VarType:      varType,
			})
		}
	}

	// Apply diffs: replace attribute values with var references in the module files
	if len(diffs) > 0 {
		if err := applyDiffs(moduleDir, diffs); err != nil {
			return diffs, fmt.Errorf("applying diffs: %w", err)
		}
	}

	return diffs, nil
}

// applyDiffs replaces differing attribute values with var.xxx references
// in the per-type .tf files in the module directory.
func applyDiffs(moduleDir string, diffs []AttrDiff) error {
	// Build a map of file → content for resource .tf files (cached, not per-diff)
	allFiles, err := filepath.Glob(filepath.Join(moduleDir, "*.tf"))
	if err != nil {
		return err
	}

	type fileContent struct {
		path    string
		content string
	}
	var resourceFiles []fileContent
	for _, file := range allFiles {
		base := filepath.Base(file)
		if strings.HasSuffix(base, "_import.tf") || base == "provider.tf" ||
			base == "variables.tf" || base == "terraform.tfvars" || base == "locals_env.tf" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		resourceFiles = append(resourceFiles, fileContent{path: file, content: string(data)})
	}

	// Group diffs by file
	fileMap := make(map[string][]AttrDiff)
	for _, d := range diffs {
		blockAddr := fmt.Sprintf(`"%s" "%s"`, d.ResourceType, d.Label)
		for _, fc := range resourceFiles {
			if strings.Contains(fc.content, blockAddr) {
				fileMap[fc.path] = append(fileMap[fc.path], d)
				break
			}
		}
	}

	// Apply diffs per file using hclwrite
	for file, fileDiffs := range fileMap {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
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

			for _, d := range fileDiffs {
				if labels[0] == d.ResourceType && labels[1] == d.Label {
					// Replace the attribute value with var.xxx
					varRef := fmt.Sprintf("var.%s", d.VarName)
					block.Body().SetAttributeRaw(d.AttrName, hclwrite.Tokens{
						{Type: 9, Bytes: []byte(varRef)}, // TokenIdent
					})
				}
			}
		}

		if err := os.WriteFile(file, f.Bytes(), 0644); err != nil {
			return err
		}
	}

	return nil
}

// isResourceReference returns true if the expression looks like a Terraform
// resource reference (e.g. "jamfpro_category.browsers.id"). These are
// cross-resource links resolved by postprocessing and should not be diffed.
func isResourceReference(expr string) bool {
	// Resource references have the form: resource_type.label.attribute
	// They contain dots and start with a provider prefix (e.g. jamfpro_)
	// but are NOT quoted strings, var refs, locals, or function calls.
	if strings.HasPrefix(expr, `"`) || strings.HasPrefix(expr, "var.") ||
		strings.HasPrefix(expr, "local.") || strings.Contains(expr, "(") {
		return false
	}
	parts := strings.Split(expr, ".")
	if len(parts) >= 3 && strings.Contains(parts[0], "_") {
		return true
	}
	// Also match references inside list expressions like [jamfpro_foo.bar.id, ...]
	if strings.HasPrefix(expr, "[") {
		inner := strings.Trim(expr, "[] \n")
		for item := range strings.SplitSeq(inner, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			itemParts := strings.Split(item, ".")
			if len(itemParts) >= 3 && strings.Contains(itemParts[0], "_") {
				return true
			}
		}
	}
	return false
}

// inferVarType inspects a raw HCL expression string and returns the most
// appropriate Terraform variable type. Defaults to "string".
func inferVarType(expr string) string {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "[") {
		return "list(string)"
	}
	if strings.HasPrefix(expr, "{") {
		return "map(string)"
	}
	return "string"
}
