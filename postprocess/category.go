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

// categorySplitTypes returns resourceType → the single category_id reference
// rule for types that should be split into per-category output files. Matches
// both flat top-level category_id rules (Jamf Pro) and nested object-attribute
// rules (Jamf Platform, e.g. AttrPath ["general"]); the rule carries the
// AttrPath and category target type extractCategoryLabel needs.
func categorySplitTypes(rules []ReferenceRule) map[string]ReferenceRule {
	types := make(map[string]ReferenceRule)
	for _, rule := range rules {
		if rule.AttrName != "category_id" || rule.ElementAttr != "" || rule.IsList || len(rule.BlockPath) != 0 {
			continue
		}
		if _, exists := types[rule.ResourceType]; !exists {
			types[rule.ResourceType] = rule
		}
	}
	return types
}

// categoryTargetType returns the category resource type a rule resolves against,
// defaulting to "jamfpro_category".
func categoryTargetType(rule ReferenceRule) string {
	if len(rule.TargetTypes) > 0 {
		return rule.TargetTypes[0]
	}
	return "jamfpro_category"
}

// categoryIDTokens returns the raw tokens of the category_id attribute located at
// the rule's AttrPath (empty = top-level flat; ["general"] = inside the general
// object expression), read-only.
func categoryIDTokens(body *hclwrite.Body, attrPath []string) (hclwrite.Tokens, bool) {
	if len(attrPath) == 0 {
		if a := body.GetAttribute("category_id"); a != nil {
			return a.Expr().BuildTokens(nil), true
		}
		return nil, false
	}
	var out hclwrite.Tokens
	found := false
	withLeafBody(body, nil, attrPath, func(leaf *hclwrite.Body) bool {
		if a := leaf.GetAttribute("category_id"); a != nil {
			out = a.Expr().BuildTokens(nil)
			found = true
		}
		return false // read-only: never re-serialize
	})
	return out, found
}

// extractCategoryLabel extracts the category label from a resource block's
// category_id reference (at the rule's AttrPath) after reference rewriting. It
// checks for a rewritten Terraform reference (<catType>.<label>.id) first, then
// falls back to a registry lookup for raw ID values. Returns empty string if no
// category can be determined.
func extractCategoryLabel(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) string {
	tokens, ok := categoryIDTokens(body, rule.AttrPath)
	if !ok {
		return ""
	}
	catType := categoryTargetType(rule)
	prefix := catType + "."

	// Single-token form (from referenceTokens): "<catType>.<label>.id".
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenIdent {
			ref := string(tok.Bytes)
			if strings.HasPrefix(ref, prefix) {
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
	if len(identTokens) >= 2 && identTokens[0] == catType {
		return identTokens[1]
	}

	// Fall back to registry lookup for raw string/numeric IDs.
	if reg != nil {
		val := tokensStringValue(tokens)
		if val != "" {
			if addr, ok := reg.Resolve(catType, val); ok {
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
