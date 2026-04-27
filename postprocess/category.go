// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// categorySplitTypes returns the set of resource types that have a top-level
// category_id reference rule and should be split into per-category output files.
func categorySplitTypes(rules []ReferenceRule) map[string]bool {
	types := make(map[string]bool)
	for _, rule := range rules {
		if rule.AttrName == "category_id" && len(rule.BlockPath) == 0 {
			types[rule.ResourceType] = true
		}
	}
	return types
}

// extractCategoryLabel extracts the category label from a resource block's
// category_id attribute after reference rewriting. It checks for a rewritten
// Terraform reference (jamfpro_category.<label>.id) first, then falls back to
// a registry lookup for raw ID values. Returns empty string if no category
// can be determined.
func extractCategoryLabel(body *hclwrite.Body, reg *registry.Registry) string {
	attr := body.GetAttribute("category_id")
	if attr == nil {
		return ""
	}

	// After reference rewriting via referenceTokens(), category_id is a single
	// TokenIdent with the full reference (e.g. "jamfpro_category.productivity.id").
	// When parsed from source, it's separate tokens (ident, dot, ident, dot, ident).
	// Handle both forms.
	tokens := attr.Expr().BuildTokens(nil)

	// Single-token form (from referenceTokens)
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenIdent {
			ref := string(tok.Bytes)
			if strings.HasPrefix(ref, "jamfpro_category.") {
				parts := strings.SplitN(ref, ".", 3)
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}

	// Multi-token form (parsed from HCL source): reconstruct the reference
	// from consecutive ident tokens joined by dots.
	var identTokens []string
	for _, tok := range tokens {
		switch tok.Type {
		case hclsyntax.TokenIdent:
			identTokens = append(identTokens, string(tok.Bytes))
		case hclsyntax.TokenDot:
			// continue collecting
		default:
			// reset on unexpected token
			identTokens = nil
		}
	}
	if len(identTokens) >= 2 && identTokens[0] == "jamfpro_category" {
		return identTokens[1]
	}

	// Fall back to registry lookup for raw string/numeric IDs
	if reg != nil {
		val := ExtractStringValue(attr)
		if val != "" {
			if addr, ok := reg.Resolve("jamfpro_category", val); ok {
				parts := strings.SplitN(addr, ".", 2)
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}

	return ""
}

// splitImportFileByCategory reads an existing per-type import file, splits its
// import blocks into per-category files using the provided label-to-category
// mapping, and removes the original file.
func splitImportFileByCategory(outputDir, baseFilename string, labelCategories map[string]string) error {
	importFilename := strings.TrimSuffix(baseFilename, ".tf") + "_import.tf"
	importPath := filepath.Join(outputDir, importFilename)

	src, err := os.ReadFile(importPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", importFilename, err)
	}

	f, diags := hclwrite.ParseConfig(src, importFilename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parsing %s: %s", importFilename, diags.Error())
	}

	categoryFiles := make(map[string]*hclwrite.File)
	baseName := strings.TrimSuffix(baseFilename, ".tf")

	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}

		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}

		// Extract resource label from "to = jamfpro_policy.my_policy"
		toBytes := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		parts := strings.SplitN(toBytes, ".", 2)
		if len(parts) < 2 {
			continue
		}
		resourceLabel := parts[1]

		categoryLabel := "uncategorised"
		if cat, ok := labelCategories[resourceLabel]; ok {
			categoryLabel = cat
		}

		outFile, ok := categoryFiles[categoryLabel]
		if !ok {
			outFile = hclwrite.NewEmptyFile()
			categoryFiles[categoryLabel] = outFile
		}

		outFile.Body().AppendNewline()
		appendBlock(outFile.Body(), block)
	}

	for categoryLabel, outFile := range categoryFiles {
		content := outFile.Bytes()
		if len(strings.TrimSpace(string(content))) == 0 {
			continue
		}

		catFilename := baseName + "_" + categoryLabel + "_import.tf"
		if err := os.WriteFile(filepath.Join(outputDir, catFilename), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", catFilename, err)
		}
	}

	if err := os.Remove(importPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", importFilename, err)
	}

	return nil
}
