// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// RenameLabels rewrites auto-generated labels (like "all_0") in the generated
// HCL file to friendly names derived from resource attributes. It also updates
// the corresponding import blocks' "to" attributes to match.
//
// nameAttrFn returns the attribute name used to derive a friendly label for the
// given resource type (e.g. "name", "email", "title").
func RenameLabels(generatedFile string, nameAttrFn func(string) string) error {
	return RenameLabelsWithFallback(generatedFile, nameAttrFn, nil)
}

// RenameLabelsWithFallback is like RenameLabels but, when the resource block's
// name attribute is missing or empty, falls back to looking up the name by
// import-block ID via idToName[resourceType][id]. This is needed for resource
// types whose name field is computed/read-only and so doesn't appear in the
// HCL produced by terraform query / -generate-config-out (e.g. Jamf Protect's
// jamfprotect_analytic_managed).
func RenameLabelsWithFallback(generatedFile string, nameAttrFn func(string) string, idToName map[string]map[string]string) error {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return fmt.Errorf("reading generated file: %w", err)
	}

	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	// Build map of address -> import-block ID so we can resolve names by ID
	// when the resource block lacks a usable name attribute.
	addrToImportID := make(map[string]string)
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		addr := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		id := extractImportID(block)
		if id != "" {
			addrToImportID[addr] = id
		}
	}

	// Phase 1: Build mapping from old address to new label
	tracker := naming.NewTracker()
	renames := make(map[string]string)

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		resourceType := labels[0]
		oldLabel := labels[1]
		oldAddress := fmt.Sprintf("%s.%s", resourceType, oldLabel)

		var name string
		nameAttr := nameAttrFn(resourceType)
		if attr := block.Body().GetAttribute(nameAttr); attr != nil {
			name = ExtractStringValue(attr)
		}
		if name == "" && idToName != nil {
			if byID, ok := idToName[resourceType]; ok {
				if id, ok := addrToImportID[oldAddress]; ok {
					name = byID[id]
				}
			}
		}
		if name == "" {
			continue
		}

		newLabel := tracker.Label(resourceType, name)
		if newLabel != oldLabel {
			renames[oldAddress] = newLabel
		}
	}

	if len(renames) == 0 {
		return nil
	}

	// Phase 2: Apply renames to resource blocks
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		oldAddress := fmt.Sprintf("%s.%s", labels[0], labels[1])
		if newLabel, ok := renames[oldAddress]; ok {
			block.SetLabels([]string{labels[0], newLabel})
		}
	}

	// Phase 3: Update import blocks' "to" attributes
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		toBytes := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		if newLabel, ok := renames[toBytes]; ok {
			parts := strings.SplitN(toBytes, ".", 2)
			if len(parts) == 2 {
				newTo := fmt.Sprintf("%s.%s", parts[0], newLabel)
				block.Body().SetAttributeRaw("to", hclwrite.Tokens{
					{Type: 9, Bytes: []byte(newTo)}, // hclsyntax.TokenIdent = 9
				})
			}
		}
	}

	return os.WriteFile(generatedFile, f.Bytes(), 0644)
}

// extractImportID returns the resource ID from an import block. It handles
// both terraform-query format (identity = { id = "..." }) and the flat
// singleton format (id = "...").
func extractImportID(block *hclwrite.Block) string {
	if identityAttr := block.Body().GetAttribute("identity"); identityAttr != nil {
		for _, tok := range identityAttr.Expr().BuildTokens(nil) {
			if tok.Type == hclsyntax.TokenQuotedLit {
				return string(tok.Bytes)
			}
		}
	}
	if idAttr := block.Body().GetAttribute("id"); idAttr != nil {
		return ExtractStringValue(idAttr)
	}
	return ""
}
