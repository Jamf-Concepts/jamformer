// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"bytes"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

// ProviderSchema holds the parsed schema information for resource types.
type ProviderSchema struct {
	// attrs maps resourceType -> blockPath -> attrName -> attrInfo
	attrs map[string]map[string]map[string]attrInfo
}

type attrInfo struct {
	Required    bool
	Optional    bool
	Computed    bool
	Sensitive   bool
	Type        cty.Type // attribute type (cty.String, cty.Bool, cty.Number, etc.)
	NestingMode string   // "single", "list", "set" for nested_type attributes; empty for plain
}

// LoadProviderSchema builds a ProviderSchema from the typed tfjson.ProviderSchemas
// returned by tfexec's ProvidersSchema().
func LoadProviderSchema(schemas *tfjson.ProviderSchemas) *ProviderSchema {
	ps := &ProviderSchema{
		attrs: make(map[string]map[string]map[string]attrInfo),
	}

	for _, provider := range schemas.Schemas {
		for resourceType, resource := range provider.ResourceSchemas {
			if resource.Block == nil {
				continue
			}
			ps.attrs[resourceType] = make(map[string]map[string]attrInfo)
			collectAttrs(ps.attrs[resourceType], "", resource.Block)
		}
	}

	return ps
}

func collectAttrs(out map[string]map[string]attrInfo, path string, block *tfjson.SchemaBlock) {
	if out[path] == nil {
		out[path] = make(map[string]attrInfo)
	}
	for name, attr := range block.Attributes {
		info := attrInfo{
			Required:  attr.Required,
			Optional:  attr.Optional,
			Computed:  attr.Computed,
			Sensitive: attr.Sensitive,
			Type:      attr.AttributeType,
		}
		if attr.AttributeNestedType != nil {
			info.NestingMode = string(attr.AttributeNestedType.NestingMode)
			childPath := name
			if path != "" {
				childPath = path + "." + name
			}
			collectNestedType(out, childPath, attr.AttributeNestedType)
		}
		out[path][name] = info
	}
	for blockName, bt := range block.NestedBlocks {
		childPath := blockName
		if path != "" {
			childPath = path + "." + blockName
		}
		if bt.Block != nil {
			collectAttrs(out, childPath, bt.Block)
		}
	}
}

// collectNestedType recursively collects attribute info from nested_type definitions.
func collectNestedType(out map[string]map[string]attrInfo, path string, nt *tfjson.SchemaNestedAttributeType) {
	if out[path] == nil {
		out[path] = make(map[string]attrInfo)
	}
	for name, attr := range nt.Attributes {
		info := attrInfo{
			Required:  attr.Required,
			Optional:  attr.Optional,
			Computed:  attr.Computed,
			Sensitive: attr.Sensitive,
			Type:      attr.AttributeType,
		}
		if attr.AttributeNestedType != nil {
			info.NestingMode = string(attr.AttributeNestedType.NestingMode)
			childPath := path + "." + name
			collectNestedType(out, childPath, attr.AttributeNestedType)
		}
		out[path][name] = info
	}
}

// canStripNull returns true if a null attribute can be safely removed.
// We strip any optional attribute (whether computed or not) — required
// attributes are never removed. Any diffs caused by hidden provider SDK
// defaults (e.g. package_distribution_point) are provider-level issues
// that exist regardless of whether the null is explicit or omitted.
func (ps *ProviderSchema) canStripNull(resourceType, blockPath, attrName string) bool {
	if ps == nil {
		return false
	}
	paths, ok := ps.attrs[resourceType]
	if !ok {
		return false
	}
	attrs, ok := paths[blockPath]
	if !ok {
		return false
	}
	info, ok := attrs[attrName]
	if !ok {
		return false
	}

	return !info.Required
}

// isSensitive returns true if the attribute is marked sensitive in the provider schema.
func (ps *ProviderSchema) isSensitive(resourceType, blockPath, attrName string) bool {
	if ps == nil {
		return false
	}
	paths, ok := ps.attrs[resourceType]
	if !ok {
		return false
	}
	attrs, ok := paths[blockPath]
	if !ok {
		return false
	}
	info, ok := attrs[attrName]
	if !ok {
		return false
	}
	return info.Sensitive
}

// zeroValue returns the cty zero value for the attribute's type.
// Falls back to cty.StringVal("") if the type is unknown.
func (ps *ProviderSchema) zeroValue(resourceType, blockPath, attrName string) cty.Value {
	if ps != nil {
		if paths, ok := ps.attrs[resourceType]; ok {
			if attrs, ok := paths[blockPath]; ok {
				if info, ok := attrs[attrName]; ok {
					return ctyZeroValue(info.Type)
				}
			}
		}
	}
	return cty.StringVal("")
}

// ctyZeroValue returns the zero value for a cty type.
func ctyZeroValue(t cty.Type) cty.Value {
	switch t {
	case cty.Bool:
		return cty.False
	case cty.Number:
		return cty.NumberIntVal(0)
	case cty.String:
		return cty.StringVal("")
	default:
		// Unknown or complex type — default to empty string
		return cty.StringVal("")
	}
}

// nestingMode returns the nesting mode for a nested_type attribute, or "" if plain.
func (ps *ProviderSchema) nestingMode(resourceType, blockPath, attrName string) string {
	if ps == nil {
		return ""
	}
	paths, ok := ps.attrs[resourceType]
	if !ok {
		return ""
	}
	attrs, ok := paths[blockPath]
	if !ok {
		return ""
	}
	info, ok := attrs[attrName]
	if !ok {
		return ""
	}
	return info.NestingMode
}

// stripNullAttributes removes null attributes that are safe to strip (per provider
// schema), recurses into nested blocks and into nested_type object expressions.
func stripNullAttributes(body *hclwrite.Body, resourceType, blockPath string, schema *ProviderSchema) {
	// Pass 1: Remove top-level null attributes that are optional
	for name, attr := range body.Attributes() {
		if !isNullValue(attr) {
			continue
		}
		if schema.canStripNull(resourceType, blockPath, name) {
			body.RemoveAttribute(name)
		}
	}

	// Pass 2: Recurse into nested_type object expression attributes
	var nestedNames []string
	for name := range body.Attributes() {
		if schema.nestingMode(resourceType, blockPath, name) != "" {
			nestedNames = append(nestedNames, name)
		}
	}
	for _, name := range nestedNames {
		attr := body.GetAttribute(name)
		if attr == nil {
			continue
		}
		nm := schema.nestingMode(resourceType, blockPath, name)
		childPath := name
		if blockPath != "" {
			childPath = blockPath + "." + name
		}
		stripNullsInObjectAttr(body, name, attr, resourceType, childPath, nm, schema)
	}

	// Pass 3: Recurse into nested blocks
	for _, block := range body.Blocks() {
		childPath := block.Type()
		if blockPath != "" {
			childPath = blockPath + "." + block.Type()
		}
		stripNullAttributes(block.Body(), resourceType, childPath, schema)
	}
}

// stripNullsInObjectAttr processes an object expression attribute (nested_type)
// to strip null values from within. Handles "single" (object { ... }) and
// "list"/"set" (array of objects [{ ... }]) nesting modes.
func stripNullsInObjectAttr(body *hclwrite.Body, attrName string, attr *hclwrite.Attribute, resourceType, schemaPath, nestingMode string, schema *ProviderSchema) {
	exprBytes := bytes.TrimSpace(attr.Expr().BuildTokens(nil).Bytes())
	if len(exprBytes) == 0 {
		return
	}

	switch nestingMode {
	case "single":
		if exprBytes[0] != '{' {
			return
		}
		if modified, ok := processObjectExpr(exprBytes, resourceType, schemaPath, schema); ok {
			body.SetAttributeRaw(attrName, hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: modified},
			})
		}
	case "list", "set":
		if exprBytes[0] != '[' {
			return
		}
		if modified, ok := processListOfObjects(exprBytes, resourceType, schemaPath, schema); ok {
			body.SetAttributeRaw(attrName, hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: modified},
			})
		}
	}
}

// processObjectExpr parses an object expression { ... }, strips optional null
// attributes via the standard stripNullAttributes path, and returns the
// modified bytes. Returns (nil, false) if nothing changed.
func processObjectExpr(exprBytes []byte, resourceType, schemaPath string, schema *ProviderSchema) ([]byte, bool) {
	closeIdx := findMatchingDelimiter(exprBytes, 0)
	if closeIdx == -1 || closeIdx != len(exprBytes)-1 {
		return nil, false
	}

	inner := exprBytes[1:closeIdx]

	f, diags := hclwrite.ParseConfig(inner, "inner.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, false
	}

	origBytes := append([]byte(nil), f.Bytes()...)
	stripNullAttributes(f.Body(), resourceType, schemaPath, schema)
	newBytes := f.Bytes()

	if bytes.Equal(origBytes, newBytes) {
		return nil, false
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	buf.WriteByte('\n')
	buf.Write(newBytes)
	buf.WriteByte('}')
	return buf.Bytes(), true
}

// processListOfObjects processes a list of object expressions [{ ... }, { ... }],
// stripping null attributes from each object.
func processListOfObjects(exprBytes []byte, resourceType, schemaPath string, schema *ProviderSchema) ([]byte, bool) {
	closeIdx := findMatchingDelimiter(exprBytes, 0)
	if closeIdx == -1 || closeIdx != len(exprBytes)-1 {
		return nil, false
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
			if processed, ok := processObjectExpr(objBytes, resourceType, schemaPath, schema); ok {
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

// findMatchingDelimiter finds the index of the matching closing delimiter
// (} for {, ] for [) starting from openPos, respecting nesting and strings.
func findMatchingDelimiter(src []byte, openPos int) int {
	if openPos >= len(src) {
		return -1
	}
	open := src[openPos]
	var close byte
	switch open {
	case '{':
		close = '}'
	case '[':
		close = ']'
	default:
		return -1
	}

	depth := 0
	inString := false
	for i := openPos; i < len(src); i++ {
		if inString {
			if src[i] == '\\' {
				i++ // skip escaped character
				continue
			}
			if src[i] == '"' {
				inString = false
			}
			continue
		}
		switch src[i] {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// isNullValue checks whether an attribute's expression is the literal "null".
func isNullValue(attr *hclwrite.Attribute) bool {
	tokens := attr.Expr().BuildTokens(nil)
	// Filter to meaningful tokens (skip whitespace/newlines)
	for _, tok := range tokens {
		switch tok.Type {
		case hclsyntax.TokenNewline, hclsyntax.TokenNil:
			continue
		case hclsyntax.TokenIdent:
			return strings.TrimSpace(string(tok.Bytes)) == "null"
		default:
			// First non-whitespace token isn't an ident — not null
			return false
		}
	}
	return false
}
