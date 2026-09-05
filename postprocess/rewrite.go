// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

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
	case rule.EmbeddedIDs:
		return rewriteEmbeddedIDs(body, rule, reg)
	case rule.PrefixedIDs && rule.ElementAttr == "":
		return rewritePrefixedID(body, rule.AttrName, rule, reg)
	case rule.ElementAttr != "":
		return rewriteListOfObjectsElement(body, rule, reg)
	case rule.IsList:
		return rewriteListAttribute(body, rule, reg)
	default:
		return rewriteSingleAttribute(body, rule, reg)
	}
}

// uuidPattern matches a canonical UUID — the form device-group Platform ids take
// when embedded in a blueprint activation_conditions expression.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// rewriteEmbeddedIDs rewrites target-type ids embedded inside a free-form string
// attribute into ${addr.TargetAttr} interpolations, leaving the surrounding text
// and any unresolvable ids intact. Used for the blueprint activation_conditions
// expression, where device-group UUIDs appear inside quoted-literal sets such as
// IN {'<uuid>'}. Returns true if the attribute was modified.
func rewriteEmbeddedIDs(body *hclwrite.Body, rule ReferenceRule, reg *registry.Registry) bool {
	attr := body.GetAttribute(rule.AttrName)
	if attr == nil {
		return false
	}
	raw := extractFullStringValue(attr)
	if raw == "" {
		return false
	}

	locs := uuidPattern.FindAllStringIndex(raw, -1)
	if len(locs) == 0 {
		return false
	}

	var b strings.Builder
	b.WriteByte('"')
	last := 0
	changed := false
	for _, loc := range locs {
		id := raw[loc[0]:loc[1]]
		var ref string
		for _, targetType := range rule.TargetTypes {
			if r, ok := reg.AttrReference(targetType, id, rule.TargetAttr); ok {
				ref = r
				break
			}
		}
		if ref == "" {
			continue // unresolvable id: leave it inside the literal text
		}
		b.WriteString(hclEscapeLiteral(raw[last:loc[0]]))
		b.WriteString("${" + ref + "}")
		last = loc[1]
		changed = true
	}
	if !changed {
		return false
	}
	b.WriteString(hclEscapeLiteral(raw[last:]))
	b.WriteByte('"')

	body.SetAttributeRaw(rule.AttrName, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(b.String())},
	})
	return true
}

// hclEscapeLiteral escapes a literal segment for embedding inside a double-quoted
// HCL template string, neutralising any pre-existing ${ / %{ so only the
// interpolations this package inserts are evaluated.
func hclEscapeLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "${", "$${")
	s = strings.ReplaceAll(s, "%{", "%%{")
	return s
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
	// Jamf IDs are positive integers, so any non-positive value names no real
	// object: -1 (no category/site/group), 0 (no enrollment customization), and
	// -2 (the Jamf Cloud distribution point sentinel on distribution-point fields).
	return val == "-1" || val == "0" || val == "-2"
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
			if rule.Numeric {
				body.SetAttributeRaw(rule.AttrName, numericReferenceTokens(ref))
			} else {
				body.SetAttributeRaw(rule.AttrName, referenceTokens(ref))
			}
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

	values := extractListLiterals(attr)
	if len(values) == 0 {
		return false
	}

	var tokens hclwrite.Tokens
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	for _, literal := range values {
		val := literal.Val
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
			// Keep as literal with TODO comment, re-quoting a string element so
			// the list stays valid HCL (a bare UUID parses as an identifier and
			// trips "Missing item separator").
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, "    %s, # TODO: unresolved reference", unresolvedListLiteral(literal))})
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
				if rule.PrefixedIDs {
					return rewritePrefixedID(leaf, rule.ElementAttr, rule, reg)
				}
				er := elemRule
				if rule.DiscriminatorAttr != "" {
					target, ok := discriminatorTarget(leaf, rule)
					if !ok {
						return false
					}
					er.TargetTypes = []string{target}
				}
				return rewriteSingleAttribute(leaf, er, reg)
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

// RewriteListElementField rewrites elemAttr on each object element of the
// list-of-objects attribute attrName in body. For each element it reads the
// current string value of elemAttr and calls resolve(value); when resolve
// returns (ref, true) the field is replaced with the raw ref token, otherwise
// the element is left untouched (no TODO marker — unmatched values are expected,
// e.g. built-in smart-group criteria). Returns true if any element changed.
//
// Exported for provider-specific criteria rewriting where the resolution target
// depends on context the generic ReferenceRule engine cannot express from a
// single element (e.g. the owning resource's device_type selecting which
// extension-attribute name space to resolve against).
func RewriteListElementField(body *hclwrite.Body, attrName, elemAttr string, resolve func(string) (string, bool)) bool {
	attr := body.GetAttribute(attrName)
	if attr == nil {
		return false
	}
	exprBytes := bytes.TrimSpace(attr.Expr().BuildTokens(nil).Bytes())
	if len(exprBytes) == 0 || exprBytes[0] != '[' {
		return false
	}
	closeIdx := findMatchingDelimiter(exprBytes, 0)
	if closeIdx == -1 || closeIdx != len(exprBytes)-1 {
		return false
	}

	modified := false
	var buf bytes.Buffer
	buf.WriteByte('[')
	i := 1
	for i < closeIdx {
		if exprBytes[i] != '{' {
			buf.WriteByte(exprBytes[i])
			i++
			continue
		}
		objClose := findMatchingDelimiter(exprBytes, i)
		if objClose == -1 || objClose > closeIdx {
			buf.Write(exprBytes[i:closeIdx])
			break
		}
		objBytes := exprBytes[i : objClose+1]
		if processed, ok := descendObjectExpr(objBytes, nil, func(leaf *hclwrite.Body) bool {
			a := leaf.GetAttribute(elemAttr)
			if a == nil {
				return false
			}
			val := ExtractStringValue(a)
			if val == "" {
				return false
			}
			ref, ok := resolve(val)
			if !ok {
				return false
			}
			leaf.SetAttributeRaw(elemAttr, referenceTokens(ref))
			return true
		}); ok {
			buf.Write(processed)
			modified = true
		} else {
			buf.Write(objBytes)
		}
		i = objClose + 1
	}
	buf.WriteByte(']')

	if !modified {
		return false
	}
	body.SetAttributeRaw(attrName, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: buf.Bytes()},
	})
	return true
}

// discriminatorTarget reads the rule's DiscriminatorAttr from a list element and
// maps its value to a target resource type via DiscriminatorMap. Returns false
// when the discriminator is absent or its value is unmapped.
func discriminatorTarget(leaf *hclwrite.Body, rule ReferenceRule) (string, bool) {
	attr := leaf.GetAttribute(rule.DiscriminatorAttr)
	if attr == nil {
		return "", false
	}
	target, ok := rule.DiscriminatorMap[ExtractStringValue(attr)]
	return target, ok
}

// rewritePrefixedID rewrites an attribute whose value is an ID carried behind a
// literal prefix that also selects the target type — UEM Connect's
// uem_group_id ("computer_12", "mobile_7") is the case it exists for.
//
// The longest prefix in rule.DiscriminatorMap that the value starts with wins.
// Its tail is resolved against the mapped resource type and the value is
// rebuilt as a quoted template keeping the prefix literal, so "computer_12"
// becomes "computer_${jamfplatform_device_group.x.jamf_pro_id}".
//
// Anything that does not match — an unknown prefix, an empty tail, a tail no
// registry entry claims — is left exactly as it was. A prefixed value is
// meaningful on its own, so an unresolved one is not a broken reference and
// gets no TODO marker.
func rewritePrefixedID(body *hclwrite.Body, attrName string, rule ReferenceRule, reg *registry.Registry) bool {
	attr := body.GetAttribute(attrName)
	if attr == nil {
		return false
	}
	val := ExtractStringValue(attr)
	if val == "" {
		return false
	}

	// Longest matching prefix wins, so that "computer_" is preferred over a
	// hypothetical "c_" and the map's iteration order cannot change the result.
	bestPrefix, bestTarget := "", ""
	for prefix, target := range rule.DiscriminatorMap {
		if strings.HasPrefix(val, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix, bestTarget = prefix, target
		}
	}
	if bestPrefix == "" {
		return false
	}

	tail := val[len(bestPrefix):]
	if tail == "" {
		return false
	}
	ref, ok := reg.AttrReference(bestTarget, tail, rule.TargetAttr)
	if !ok {
		return false
	}

	// A quoted template: the prefix stays a literal, the reference becomes an
	// interpolation. hclwrite has no builder for a mixed template, so the
	// tokens are assembled directly.
	body.SetAttributeRaw(attrName, hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(bestPrefix)},
		{Type: hclsyntax.TokenTemplateInterp, Bytes: []byte("${")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte(ref)},
		{Type: hclsyntax.TokenTemplateSeqEnd, Bytes: []byte("}")},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
	})
	return true
}
