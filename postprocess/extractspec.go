// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// FileKind selects the filename extension and derivation strategy for an
// extracted support file.
type FileKind int

const (
	// FileKindScript guesses the extension from the script shebang (.sh/.py/...).
	FileKindScript FileKind = iota
	// FileKindMobileconfig writes a .mobileconfig file.
	FileKindMobileconfig
	// FileKindXML writes a .xml file.
	FileKindXML
)

// ExtractSpec declaratively describes a string attribute whose value should be
// extracted to a support file and replaced with a file() reference. It is
// provider-agnostic: BlockPath navigates HCL nested blocks (jamfpro), AttrPath
// navigates plugin-framework object-expression attributes (jamfplatform), and
// they compose (BlockPath first, then AttrPath).
type ExtractSpec struct {
	// ResourceType is the TF resource type this spec applies to.
	ResourceType string
	// BlockPath navigates nested HCL blocks to the container holding AttrName.
	BlockPath []string
	// AttrPath navigates nested object-expression attributes to the container.
	AttrPath []string
	// AttrName is the string attribute to extract (e.g. "script_contents", "payloads").
	AttrName string
	// OutputSubdir is the directory under support_files/ to write the file into.
	OutputSubdir string
	// FileKind selects the file extension.
	FileKind FileKind
	// NameAttr names the attribute used to derive the filename (default "name").
	NameAttr string
	// NameAttrPath is the object-attribute path to the container holding NameAttr
	// (empty = resource root; e.g. ["general"] for jamfplatform profiles).
	NameAttrPath []string
	// SkipFn, when set, is consulted with the extracted content; a true result
	// means the whole resource should be dropped from output (handled by the caller).
	SkipFn func(content string) (bool, string)
}

// ReadObjectAttrString reads attrName from the nested object-expression
// attribute path attrPath within body, returning "" if absent. Exported for
// provider label composers that derive names from a nested attribute (e.g. the
// federated jamfplatform_pro_* types whose display name lives at general.name).
func ReadObjectAttrString(body *hclwrite.Body, attrPath []string, attrName string) string {
	return readLeafString(body, nil, attrPath, attrName)
}

// RemoveNestedAttrs removes the named attributes from the object expression at
// attrPath within body, re-serializing the affected object expressions, and
// returns whether anything changed. Exported for provider post-processing that
// drops server-computed echo fields from a nested object (e.g. dropping a
// self_service_icon's uri/filename so only the rewritten id reference remains).
func RemoveNestedAttrs(body *hclwrite.Body, attrPath []string, names ...string) bool {
	return withLeafBody(body, nil, attrPath, func(leaf *hclwrite.Body) bool {
		changed := false
		for _, n := range names {
			if leaf.GetAttribute(n) != nil {
				leaf.RemoveAttribute(n)
				changed = true
			}
		}
		return changed
	})
}

// readLeafString navigates blockPath then attrPath from body and returns the
// string value of attrName at the leaf container, or "" if absent. Read-only.
func readLeafString(body *hclwrite.Body, blockPath, attrPath []string, attrName string) string {
	var out string
	withLeafBody(body, blockPath, attrPath, func(leaf *hclwrite.Body) bool {
		if a := leaf.GetAttribute(attrName); a != nil {
			out = extractFullStringValue(a)
		}
		return false // read-only: never re-serialize
	})
	return out
}

// specContent returns the raw content string the spec targets, for skip checks.
func specContent(body *hclwrite.Body, spec ExtractSpec) string {
	return readLeafString(body, spec.BlockPath, spec.AttrPath, spec.AttrName)
}

// fileExtFor returns the file extension for a FileKind.
func fileExtFor(kind FileKind, content string) string {
	switch kind {
	case FileKindMobileconfig:
		return ".mobileconfig"
	case FileKindXML:
		return ".xml"
	default:
		return guessScriptExtension(content)
	}
}

// buildExtractFileName derives a collision-safe filename from a resource name.
// The guessed/known extension is appended only when the sanitized name does not
// already end in it (case-insensitively), so a profile named
// "Foo.mobileconfig" yields "Foo.mobileconfig", not "Foo.mobileconfig.mobileconfig".
func buildExtractFileName(name string, kind FileKind, content string, fileNames map[string]int) string {
	base := sanitizeFilename(name)
	ext := fileExtFor(kind, content)
	if !strings.HasSuffix(strings.ToLower(base), ext) {
		base += ext
	}

	fileNames[base]++
	if fileNames[base] > 1 {
		nameWithoutExt := strings.TrimSuffix(base, ext)
		base = fmt.Sprintf("%s_%d%s", nameWithoutExt, fileNames[base], ext)
	}
	return base
}

// extractStringAttr writes the spec's target string attribute to a support file
// in absDir and replaces it with a file("${path.module}/<relDir>/<file>")
// reference. Returns (wrote, err). It is a no-op (false, nil) when the name or
// content is absent. This is the single implementation behind the legacy
// extractScriptContents/extractProfilePayloads/extractAppConfiguration wrappers
// and the platform pipeline's spec loop.
func extractStringAttr(body *hclwrite.Body, spec ExtractSpec, absDir, relDir string, fileNames map[string]int) (bool, error) {
	nameAttr := spec.NameAttr
	if nameAttr == "" {
		nameAttr = "name"
	}
	name := readLeafString(body, nil, spec.NameAttrPath, nameAttr)
	if name == "" {
		return false, nil
	}

	var extractErr error
	wrote := withLeafBody(body, spec.BlockPath, spec.AttrPath, func(leaf *hclwrite.Body) bool {
		contentAttr := leaf.GetAttribute(spec.AttrName)
		if contentAttr == nil {
			return false
		}
		content := extractFullStringValue(contentAttr)
		if content == "" {
			return false
		}

		fileName := buildExtractFileName(name, spec.FileKind, content, fileNames)
		if err := os.WriteFile(filepath.Join(absDir, fileName), []byte(content), 0644); err != nil {
			extractErr = fmt.Errorf("writing %s: %w", fileName, err)
			return false
		}

		relPath := filepath.ToSlash(filepath.Join(relDir, fileName))
		fileRef := fmt.Sprintf(`file("${path.module}/%s")`, relPath)
		leaf.SetAttributeRaw(spec.AttrName, hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
		})
		return true
	})

	return wrote, extractErr
}
