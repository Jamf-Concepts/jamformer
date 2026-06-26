// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"bytes"
	"fmt"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// RewriteBlockForTest is an exported wrapper around rewriteBlock for use by
// external test packages. Not intended for production use.
func RewriteBlockForTest(body *hclwrite.Body, blockPath []string, rule ReferenceRule, reg *registry.Registry) {
	rewriteBlock(body, blockPath, rule, reg)
}

// rewriteBlock navigates into nested blocks following the block path, then
// navigates any object-attribute path, then rewrites the target attribute.
func rewriteBlock(body *hclwrite.Body, blockPath []string, rule ReferenceRule, reg *registry.Registry) {
	if len(blockPath) == 0 {
		// Block path exhausted. Descend any object-attribute path, then rewrite.
		if len(rule.AttrPath) > 0 {
			withLeafBody(body, nil, rule.AttrPath, func(leaf *hclwrite.Body) bool {
				return applyLeafRewrite(leaf, rule, reg)
			})
			return
		}
		applyLeafRewrite(body, rule, reg)
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

// applyLeafRewrite rewrites the rule's target attribute on the supplied body,
// dispatching on whether it is a list-of-objects element, a list of IDs, or a
// single ID. Returns true if the body was modified.
func applyLeafRewrite(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) bool {
	switch {
	case rule.ElementAttr != "":
		return rewriteListOfObjectsElement(body, rule, reg)
	case rule.IsList:
		return rewriteListAttribute(body, rule, reg)
	default:
		return rewriteSingleAttribute(body, rule, reg)
	}
}

// withLeafBody navigates blockPath (live hclwrite blocks) then attrPath
// (object-expression attributes) from body, invokes fn on the final inner
// *hclwrite.Body, and re-serializes object expressions whose fn reported a
// change. Returns true if any change was made. This is the shared traversal
// primitive used by both reference rewriting and file extraction; it reuses the
// object-expression byte-surgery idiom established in schema.go.
func withLeafBody(body *hclwrite.Body, blockPath, attrPath []string, fn func(leaf *hclwrite.Body) bool) bool {
	// Phase 1: descend live nested blocks.
	if len(blockPath) > 0 {
		next := blockPath[0]
		rest := blockPath[1:]
		changed := false
		for _, block := range body.Blocks() {
			if block.Type() == next {
				if withLeafBody(block.Body(), rest, attrPath, fn) {
					changed = true
				}
			}
		}
		return changed
	}

	// Phase 2: descend object-expression attributes.
	if len(attrPath) == 0 {
		return fn(body)
	}

	seg := attrPath[0]
	rest := attrPath[1:]
	attr := body.GetAttribute(seg)
	if attr == nil {
		return false
	}
	exprBytes := bytes.TrimSpace(attr.Expr().BuildTokens(nil).Bytes())
	if len(exprBytes) == 0 || exprBytes[0] != '{' {
		return false
	}
	modified, ok := descendObjectExpr(exprBytes, rest, fn)
	if !ok {
		return false
	}
	body.SetAttributeRaw(seg, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: modified},
	})
	return true
}

// descendObjectExpr parses the inner of an object expression { ... }, recurses
// via withLeafBody with the remaining attribute path, and re-wraps the result.
// Mirrors processObjectExpr in schema.go. Returns (nil, false) if nothing changed.
func descendObjectExpr(exprBytes []byte, attrPath []string, fn func(leaf *hclwrite.Body) bool) ([]byte, bool) {
	closeIdx := findMatchingDelimiter(exprBytes, 0)
	if closeIdx == -1 || closeIdx != len(exprBytes)-1 {
		return nil, false
	}
	inner := exprBytes[1:closeIdx]

	f, diags := hclwrite.ParseConfig(inner, "inner.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, false
	}

	if !withLeafBody(f.Body(), nil, attrPath, fn) {
		return nil, false
	}

	return wrapObjectInner(f.Bytes()), true
}

// isSentinelID reports whether an ID value is a Jamf "none"/"unset" sentinel
// rather than a real reference. Jamf IDs are positive integers, so -1 (no
// category/site) and 0 (no enrollment customization) never name a real object
// and must not be rewritten or flagged as unresolved.
func isSentinelID(val string) bool {
	return val == "-1" || val == "0"
}

// rewriteSingleAttribute replaces a single string ID with a resource reference.
// Returns true if the attribute was modified.
func rewriteSingleAttribute(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) bool {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return false
	}

	// Extract the literal string value from the attribute tokens
	val := ExtractStringValue(attr)
	if val == "" || isSentinelID(val) {
		return false
	}

	// Try to resolve via registry
	for _, targetType := range rule.TargetTypes {
		if ref, ok := reg.AttrReference(targetType, val, rule.TargetAttr); ok {
			body.SetAttributeRaw(rule.AttrName, referenceTokens(ref))
			return true
		}
	}

	// Couldn't resolve — leave as-is and add a TODO comment
	body.SetAttributeRaw(rule.AttrName, todoTokens(val))
	return true
}

// rewriteListAttribute replaces a list of string/int IDs with resource references.
// Returns true if the attribute was modified.
func rewriteListAttribute(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) bool {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return false
	}

	values := extractListValues(attr)
	if len(values) == 0 {
		return false
	}

	var tokens hclwrite.Tokens
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	for _, val := range values {
		resolved := false
		// Preserve "none" sentinels verbatim (never a real reference).
		if isSentinelID(val) {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, "    %q,", val)})
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
			continue
		}
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
	return true
}

// rewriteListOfObjectsElement rewrites rule.ElementAttr on every object element
// of the list-of-objects attribute named rule.AttrName (e.g. scripts = [ { id = "44" } ]).
// Returns true if the attribute was modified.
func rewriteListOfObjectsElement(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) bool {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return false
	}
	exprBytes := bytes.TrimSpace(attr.Expr().BuildTokens(nil).Bytes())
	if len(exprBytes) == 0 || exprBytes[0] != '[' {
		return false
	}
	modified, ok := rewriteElementsInList(exprBytes, rule, reg)
	if !ok {
		return false
	}
	body.SetAttributeRaw(rule.AttrName, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: modified},
	})
	return true
}

// rewriteElementsInList iterates a list expression [ {..}, {..} ], rewriting
// rule.ElementAttr on each object element. Mirrors processListOfObjects.
func rewriteElementsInList(exprBytes []byte, rule ReferenceRule, reg *registry.Registry) ([]byte, bool) {
	closeIdx := findMatchingDelimiter(exprBytes, 0)
	if closeIdx == -1 || closeIdx != len(exprBytes)-1 {
		return nil, false
	}

	// Per-element rewrite uses a single-attribute rule keyed on ElementAttr.
	elemRule := ReferenceRule{
		AttrName:    rule.ElementAttr,
		TargetTypes: rule.TargetTypes,
		TargetAttr:  rule.TargetAttr,
	}

	modified := false
	var buf bytes.Buffer
	buf.WriteByte('[')

	i := 1 // skip opening [
	for i < closeIdx {
		if exprBytes[i] == '{' {
			objClose := findMatchingDelimiter(exprBytes, i)
			if objClose == -1 || objClose > closeIdx {
				buf.Write(exprBytes[i:closeIdx])
				break
			}
			objBytes := exprBytes[i : objClose+1]
			if processed, ok := descendObjectExpr(objBytes, nil, func(leaf *hclwrite.Body) bool {
				return rewriteSingleAttribute(leaf, elemRule, reg)
			}); ok {
				buf.Write(processed)
				modified = true
			} else {
				buf.Write(objBytes)
			}
			i = objClose + 1
		} else {
			buf.WriteByte(exprBytes[i])
			i++
		}
	}

	buf.WriteByte(']')
	if !modified {
		return nil, false
	}
	return buf.Bytes(), true
}
