// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// ExtractStringValue extracts a string or numeric literal from an HCL
// attribute's expression tokens. Exported for use by provider packages.
func ExtractStringValue(attr *hclwrite.Attribute) string {
	tokens := attr.Expr().BuildTokens(nil)
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenQuotedLit {
			return string(tok.Bytes)
		}
	}
	// Also try numeric literals (some IDs come as numbers)
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenNumberLit {
			return string(tok.Bytes)
		}
	}
	return ""
}

// extractListValues extracts string or numeric values from a list expression.
func extractListValues(attr *hclwrite.Attribute) []string {
	tokens := attr.Expr().BuildTokens(nil)
	var values []string
	for _, tok := range tokens {
		switch tok.Type {
		case hclsyntax.TokenQuotedLit:
			values = append(values, string(tok.Bytes))
		case hclsyntax.TokenNumberLit:
			values = append(values, string(tok.Bytes))
		}
	}
	return values
}

// referenceTokens creates HCL tokens for a resource attribute reference like
// jamfpro_script.my_script.id
func referenceTokens(ref string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(ref)},
	}
}

// todoTokens creates tokens for an unresolved reference with a TODO comment.
func todoTokens(val string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(val)},
		{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
		{Type: hclsyntax.TokenComment, Bytes: []byte(" # TODO: unresolved reference")},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
	}
}

// extractFullStringValue extracts the complete string value from an attribute
// by reconstructing the expression source and parsing it with the HCL library
// to properly handle all escape sequences.
func extractFullStringValue(attr *hclwrite.Attribute) string {
	// Rebuild the expression source from tokens
	exprTokens := attr.Expr().BuildTokens(nil)
	exprSrc := exprTokens.Bytes()

	// Wrap in a minimal HCL file to parse the expression properly
	hclSrc := append([]byte("v = "), exprSrc...)

	f, diags := hclsyntax.ParseConfig(hclSrc, "expr", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return ""
	}
	if f == nil {
		return ""
	}

	attrs, diags := f.Body.JustAttributes()
	if diags.HasErrors() {
		return ""
	}

	vAttr, ok := attrs["v"]
	if !ok {
		return ""
	}

	val, diags := vAttr.Expr.Value(nil)
	if diags.HasErrors() {
		return ""
	}

	if val.Type() != cty.String {
		return ""
	}

	return val.AsString()
}

// sanitizeFilename converts a script name to a safe filename.
func sanitizeFilename(name string) string {
	// Replace characters that are unsafe in filenames
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	result := replacer.Replace(name)

	// Trim whitespace and dots from edges
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")

	if result == "" {
		result = "unnamed_script"
	}

	return result
}

// guessScriptExtension looks at the shebang line to determine the file extension.
func guessScriptExtension(content string) string {
	firstLine := content
	if before, _, ok := strings.Cut(content, "\n"); ok {
		firstLine = before
	}
	firstLine = strings.TrimSpace(firstLine)

	switch {
	case strings.Contains(firstLine, "python"):
		return ".py"
	case strings.Contains(firstLine, "ruby"):
		return ".rb"
	case strings.Contains(firstLine, "perl"):
		return ".pl"
	case strings.Contains(firstLine, "zsh"):
		return ".zsh"
	case strings.Contains(firstLine, "bash"), strings.Contains(firstLine, "/bin/sh"):
		return ".sh"
	default:
		return ".sh"
	}
}

// appendBlock copies a resource block into a target body.
// hclwrite doesn't have a direct clone method, so we serialize and re-parse.
func appendBlock(body *hclwrite.Body, block *hclwrite.Block) {
	// Create a temporary file with just this block to get its bytes
	tmpFile := hclwrite.NewEmptyFile()
	tmpBody := tmpFile.Body()

	newBlock := tmpBody.AppendNewBlock(block.Type(), block.Labels())
	copyBody(newBlock.Body(), block.Body())

	// Re-parse and append
	src := tmpFile.Bytes()
	parsed, diags := hclwrite.ParseConfig(src, "tmp", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		// Fallback: just write raw bytes
		body.AppendUnstructuredTokens(hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: src},
		})
		return
	}

	for _, b := range parsed.Body().Blocks() {
		newB := body.AppendNewBlock(b.Type(), b.Labels())
		formatBody(newB.Body(), b.Body())
	}
}

// copyBody copies attributes and blocks from src to dst.
func copyBody(dst, src *hclwrite.Body) {
	for name, attr := range src.Attributes() {
		dst.SetAttributeRaw(name, attr.Expr().BuildTokens(nil))
	}
	for _, block := range src.Blocks() {
		newBlock := dst.AppendNewBlock(block.Type(), block.Labels())
		copyBody(newBlock.Body(), block.Body())
	}
}
