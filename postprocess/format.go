// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"sort"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Meta-argument attributes that should appear first in a block body.
var metaArgAttrs = map[string]bool{
	"count":    true,
	"for_each": true,
	"provider": true,
}

// Attributes that should appear last in a block body.
var trailingAttrs = map[string]bool{
	"depends_on": true,
}

// Block types that should appear after all other blocks.
var trailingBlockTypes = map[string]bool{
	"lifecycle":   true,
	"provisioner": true,
	"connection":  true,
}

// formatBody copies attributes and blocks from src to dst with Terraform style
// guide ordering:
//  1. Meta-arguments (count, for_each, provider) — separated by blank line
//  2. Regular attributes — sorted alphabetically
//  3. [blank line]
//  4. Regular nested blocks — sorted alphabetically by type (stable)
//  5. [blank line]
//  6. Trailing blocks (lifecycle, provisioner, connection)
//  7. Trailing attributes (depends_on)
//
// Nested block bodies are recursively formatted.
func formatBody(dst, src *hclwrite.Body) {
	attrs := src.Attributes()
	blocks := src.Blocks()

	// Classify attributes into meta, regular, and trailing
	var metaNames, regularNames, trailingNames []string
	for name := range attrs {
		switch {
		case metaArgAttrs[name]:
			metaNames = append(metaNames, name)
		case trailingAttrs[name]:
			trailingNames = append(trailingNames, name)
		default:
			regularNames = append(regularNames, name)
		}
	}
	sort.Strings(metaNames)
	sort.Strings(regularNames)
	sort.Strings(trailingNames)

	// Classify blocks into regular and trailing (preserve relative order within each group)
	var regularBlocks, trailingBlockList []*hclwrite.Block
	for _, b := range blocks {
		if trailingBlockTypes[b.Type()] {
			trailingBlockList = append(trailingBlockList, b)
		} else {
			regularBlocks = append(regularBlocks, b)
		}
	}
	// Stable sort so blocks of the same type keep their original order
	sort.SliceStable(regularBlocks, func(i, j int) bool {
		return regularBlocks[i].Type() < regularBlocks[j].Type()
	})

	hasMetaAttrs := len(metaNames) > 0
	hasRegularAttrs := len(regularNames) > 0
	hasRegularBlocks := len(regularBlocks) > 0
	hasTrailingBlocks := len(trailingBlockList) > 0
	hasTrailingAttrs := len(trailingNames) > 0

	// 1. Meta-arguments first
	for _, name := range metaNames {
		dst.SetAttributeRaw(name, attrs[name].Expr().BuildTokens(nil))
	}
	if hasMetaAttrs && (hasRegularAttrs || hasRegularBlocks || hasTrailingBlocks || hasTrailingAttrs) {
		dst.AppendNewline()
	}

	// 2. Regular attributes alphabetically
	for _, name := range regularNames {
		dst.SetAttributeRaw(name, attrs[name].Expr().BuildTokens(nil))
	}

	// 3. Blank line before regular blocks
	if (hasMetaAttrs || hasRegularAttrs) && hasRegularBlocks {
		dst.AppendNewline()
	}

	// 4. Regular blocks sorted by type
	for _, b := range regularBlocks {
		newBlock := dst.AppendNewBlock(b.Type(), b.Labels())
		formatBody(newBlock.Body(), b.Body())
	}

	// 5. Trailing blocks (lifecycle, etc.)
	if hasTrailingBlocks {
		if hasMetaAttrs || hasRegularAttrs || hasRegularBlocks {
			dst.AppendNewline()
		}
		for _, b := range trailingBlockList {
			newBlock := dst.AppendNewBlock(b.Type(), b.Labels())
			formatBody(newBlock.Body(), b.Body())
		}
	}

	// 6. Trailing attributes (depends_on)
	if hasTrailingAttrs {
		if hasMetaAttrs || hasRegularAttrs || hasRegularBlocks || hasTrailingBlocks {
			dst.AppendNewline()
		}
		for _, name := range trailingNames {
			dst.SetAttributeRaw(name, attrs[name].Expr().BuildTokens(nil))
		}
	}
}
