// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// RenameLabels rewrites auto-generated labels (like "all_0") in the generated
// HCL file to friendly names derived from resource attributes. It also updates
// the corresponding import blocks' "to" attributes to match.
//
// nameAttrFn returns the attribute name used to derive a friendly label for the
// given resource type (e.g. "name", "email", "title").
func RenameLabels(generatedFile string, nameAttrFn func(string) string) error {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return fmt.Errorf("reading generated file: %w", err)
	}

	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parsing generated HCL: %s", diags.Error())
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

		nameAttr := nameAttrFn(resourceType)
		attr := block.Body().GetAttribute(nameAttr)
		if attr == nil {
			continue
		}
		name := ExtractStringValue(attr)
		if name == "" {
			continue
		}

		newLabel := tracker.Label(resourceType, name)
		if newLabel != oldLabel {
			oldAddress := fmt.Sprintf("%s.%s", resourceType, oldLabel)
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
