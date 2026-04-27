// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// RewriteBlockForTest is an exported wrapper around rewriteBlock for use by
// external test packages. Not intended for production use.
func RewriteBlockForTest(body *hclwrite.Body, blockPath []string, rule ReferenceRule, reg *registry.Registry) {
	rewriteBlock(body, blockPath, rule, reg)
}

// rewriteBlock navigates into nested blocks following the block path,
// then rewrites the target attribute.
func rewriteBlock(body *hclwrite.Body, blockPath []string, rule ReferenceRule, reg *registry.Registry) {
	if len(blockPath) == 0 {
		// We're at the target level — rewrite the attribute
		if rule.IsList {
			rewriteListAttribute(body, rule, reg)
		} else {
			rewriteSingleAttribute(body, rule, reg)
		}
		return
	}

	// Navigate into the next nested block
	nextBlock := blockPath[0]
	remaining := blockPath[1:]

	for _, block := range body.Blocks() {
		if block.Type() == nextBlock {
			rewriteBlock(block.Body(), remaining, rule, reg)
		}
	}
}

// rewriteSingleAttribute replaces a single string ID with a resource reference.
func rewriteSingleAttribute(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return
	}

	// Extract the literal string value from the attribute tokens
	val := ExtractStringValue(attr)
	if val == "" {
		return
	}

	// Try to resolve via registry
	for _, targetType := range rule.TargetTypes {
		if ref, ok := reg.AttrReference(targetType, val, rule.TargetAttr); ok {
			body.SetAttributeRaw(rule.AttrName, referenceTokens(ref))
			return
		}
	}

	// Couldn't resolve — leave as-is and add a TODO comment
	body.SetAttributeRaw(rule.AttrName, todoTokens(val))
}

// rewriteListAttribute replaces a list of string/int IDs with resource references.
func rewriteListAttribute(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return
	}

	values := extractListValues(attr)
	if len(values) == 0 {
		return
	}

	var tokens hclwrite.Tokens
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	for _, val := range values {
		resolved := false
		for _, targetType := range rule.TargetTypes {
			if ref, ok := reg.AttrReference(targetType, val, rule.TargetAttr); ok {
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("    " + ref)})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
				tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
				resolved = true
				break
			}
		}
		if !resolved {
			// Keep as literal with TODO comment
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, "    %s, # TODO: unresolved reference", val)})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}
	}

	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("  ")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})

	body.SetAttributeRaw(rule.AttrName, tokens)
}
