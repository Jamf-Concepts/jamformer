// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/Jamf-Concepts/jamformer/terraform"
)

// Diagnostic patterns used by classifyFix to determine auto-fix strategies.
var (
	// attrNameFromDiag matches "Attribute foo_bar ..."
	attrNameFromDiag = regexp.MustCompile(`(?i)\battribute\s+(\w+)\b`)

	// mustBeValueRe matches "'attr_name' must be 'VALUE' when ..."
	mustBeValueRe = regexp.MustCompile(`'(\w+)'\s+must\s+be\s+'([^']+)'`)

	// mustBeOneOfRe matches `"attr" must be one of [A B C], got: X`
	mustBeOneOfRe = regexp.MustCompile(`"(\w+)"\s+must\s+be\s+one\s+of\s+\[`)

	// conflictsWithRe matches `"attr": conflicts with other_attr`
	conflictsWithRe = regexp.MustCompile(`"(\w+)":\s+conflicts\s+with\s+(\w+)`)

	// requiredNullRe matches `The argument "attr.path" is required, but no definition was found.`
	requiredNullRe = regexp.MustCompile(`The argument "([^"]+)" is required`)

	// mustSetConfigRe matches the explicit-null form of a required attribute:
	// `Must set a configuration value for the <attr> attribute` (top-level) or
	// `Must set a configuration value for the <block>.<attr>` (nested, no
	// trailing "attribute"). This fires when a Required attribute is present but
	// set to null (a value the provider can't read back — a secret or unreadable
	// field). The capture may be a dotted path (e.g. institutional_recovery_key.data).
	mustSetConfigRe = regexp.MustCompile(`Must set a configuration value for the ([\w.]+)`)

	// attrLineRe splits an `  attr = value  # comment` line into its assignment
	// prefix, value, and optional trailing comment.
	attrLineRe = regexp.MustCompile(`^(\s*[\w.]+\s*=\s*)(.*?)(\s*#.*)?$`)

	// resourceDeclRe matches a resource block declaration.
	resourceDeclRe = regexp.MustCompile(`resource\s+"([^"]+)"\s+"([^"]+)"`)
)

// setAttributeValueAtLine replaces the right-hand side of the attribute on the
// given 1-based line with rawExpr (written verbatim — pass `true`, `"-1"`,
// `var.x`, etc.), preserving indentation and any trailing comment. Operating by
// line lets us fix nested attributes and non-string values that the hclwrite
// top-level setter cannot. Returns true if the line was rewritten.
func setAttributeValueAtLine(filePath string, line int, rawExpr string) bool {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return false
	}
	m := attrLineRe.FindStringSubmatch(lines[line-1])
	if m == nil {
		return false
	}
	lines[line-1] = m[1] + rawExpr + m[3]
	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644) == nil
}

// validationFix represents a fix to apply to a resource attribute.
type validationFix struct {
	filePath string
	line     int
	attrName string
	newValue string // empty = remove, non-empty = set to this value
}

// RequiredVar describes a write-only variable that was created to replace a
// required null attribute. The caller should flag these to the user.
type RequiredVar struct {
	VarName  string // Terraform variable name (e.g. "smtp_server_password")
	AttrPath string // original attribute path (e.g. "basic_auth_credentials.0.password")
	Resource string // resource address (e.g. "jamfpro_smtp_server.settings")
	Filename string // source file (e.g. "smtp_server.tf")
}

// FixResult holds the outcome of FixValidationErrors.
type FixResult struct {
	Fixed        int           // total attributes fixed (removed, set, or replaced)
	RequiredVars []RequiredVar // write-only variables that need user-supplied values
}

// FixValidationErrors runs terraform validate in a loop, parsing error
// diagnostics to auto-fix attributes the provider says are invalid.
//
// Supported fix strategies:
//   - "remove it" errors: remove the offending attribute
//   - "must be 'VALUE'" errors: set the attribute to the required value
//   - "must be one of [...]" errors: remove the attribute with the invalid value
//   - "conflicts with" errors: remove the attribute pointed to by the diagnostic
//   - "required null" errors: sensitive attrs get variable references, non-sensitive get ""
//
// The schema parameter enables sensitive-aware handling of required nulls.
// If nil, all required nulls are replaced with variable references.
//
// Returns a FixResult with the count and any required variables, or an error.
func FixValidationErrors(outputDir string, schema *ProviderSchema) (*FixResult, error) {
	const maxIterations = 10
	result := &FixResult{}

	for range maxIterations {
		valResult, err := terraform.Validate(outputDir)
		if err != nil {
			return result, fmt.Errorf("terraform validate: %w", err)
		}
		if valResult.Valid {
			return result, nil
		}

		fixed := 0
		var requiredNulls []requiredNullDiag

		for _, diag := range valResult.Diagnostics {
			if diag.Severity != "error" {
				continue
			}
			if diag.Range == nil || diag.Range.Filename == "" {
				continue
			}

			filePath := filepath.Join(outputDir, diag.Range.Filename)

			// Check for required null first (separate handling)
			if m := requiredNullRe.FindStringSubmatch(diag.Detail); len(m) >= 2 {
				requiredNulls = append(requiredNulls, requiredNullDiag{
					attrPath: m[1],
					filePath: filePath,
					filename: diag.Range.Filename,
					line:     diag.Range.Start.Line,
				})
				continue
			}

			// Explicit-null Required attribute ("Must set a configuration value for
			// the <attr> attribute"): the provider can't read it back (a secret or
			// otherwise unreadable required field). Wire it to a sensitive variable
			// at its exact line (handles nested attributes), so the user supplies
			// the value rather than the config failing. WriteOnly attributes and
			// their _wo_version companions are left to injectRequiredWriteOnly,
			// which pairs them correctly (the secret → var, _wo_version → 1).
			if m := mustSetConfigRe.FindStringSubmatch(diag.Detail); len(m) >= 2 {
				attr := m[1]
				if isWoVersionAttr(attr) {
					continue
				}
				src, _ := os.ReadFile(filePath)
				resType, resLabel := resourceAtLine(src, diag.Range.Start.Line)
				if resType != "" && !schema.isWriteOnly(resType, "", attr) {
					varName := sanitizeVarName(stripProviderPrefix(resType) + "_" + resLabel + "_" + attr)
					if setAttributeValueAtLine(filePath, diag.Range.Start.Line, "var."+varName) {
						rv := RequiredVar{VarName: varName, AttrPath: attr, Resource: resType + "." + resLabel, Filename: diag.Range.Filename}
						appendVariables(outputDir, []RequiredVar{rv})
						result.RequiredVars = append(result.RequiredVars, rv)
						fixed++
					}
				}
				continue
			}

			fix := classifyFix(diag.Summary, diag.Detail, filePath, diag.Range.Start.Line)
			if fix == nil {
				continue
			}

			if fix.newValue == "" {
				if removeAttributeFromFile(fix.filePath, fix.line, fix.attrName) {
					fixed++
				}
			} else {
				if setAttributeInFile(fix.filePath, fix.line, fix.attrName, fix.newValue) {
					fixed++
				}
			}
		}

		// Handle required nulls — sensitive get variable references, non-sensitive get ""
		if len(requiredNulls) > 0 {
			vars, n := fixRequiredNulls(outputDir, requiredNulls, schema)
			fixed += n
			result.RequiredVars = append(result.RequiredVars, vars...)
		}

		if fixed == 0 {
			break
		}

		result.Fixed += fixed
		if !Quiet {
			fmt.Printf("  Auto-fixed %d validation errors, re-validating...\n", fixed)
		}
		terraform.FormatDir(outputDir)
	}

	return result, nil
}

// requiredNullDiag holds info parsed from a "required, but no definition was found" diagnostic.
type requiredNullDiag struct {
	attrPath string // e.g. "code" or "basic_auth_credentials.0.password"
	filePath string // absolute path
	filename string // relative filename
	line     int
}

// fixRequiredNulls handles "required, but no definition was found" errors.
// Sensitive attributes are replaced with variable references (var.X) and
// appended to variables.tf. Non-sensitive attributes are replaced with empty
// strings (""). If schema is nil, all attributes are treated as sensitive.
func fixRequiredNulls(outputDir string, diags []requiredNullDiag, schema *ProviderSchema) ([]RequiredVar, int) {
	var vars []RequiredVar
	fixed := 0

	for _, d := range diags {
		src, err := os.ReadFile(d.filePath)
		if err != nil {
			continue
		}

		resType, resLabel := resourceAtLine(src, d.line)
		if resType == "" {
			continue
		}

		// Determine block path and leaf attribute for schema lookup.
		// Attr paths like "basic_auth_credentials.0.password" → blockPath="basic_auth_credentials", leaf="password"
		// Simple paths like "code" → blockPath="", leaf="code"
		leafAttr := leafAttrName(d.attrPath)
		blockPath := attrBlockPath(d.attrPath)

		// Non-sensitive: replace null with type-appropriate zero value
		if schema != nil && !schema.isSensitive(resType, blockPath, leafAttr) {
			if setNullToZeroValue(d.filePath, src, resType, resLabel, d.attrPath, schema) {
				fixed++
			}
			continue
		}

		// Sensitive (or no schema): replace null with variable reference
		// Include resource label to ensure uniqueness per resource instance
		// e.g. "computer_prestage_enrollment_shared_admin_password"
		shortType := stripProviderPrefix(resType)
		varName := shortType + "_" + resLabel + "_" + leafAttr

		f, hclDiags := hclwrite.ParseConfig(src, d.filePath, hcl.Pos{Line: 1, Column: 1})
		if hclDiags.HasErrors() {
			continue
		}

		if replaceNullWithVar(f, resType, resLabel, d.attrPath, varName) {
			if err := os.WriteFile(d.filePath, f.Bytes(), 0644); err != nil {
				continue
			}
			vars = append(vars, RequiredVar{
				VarName:  varName,
				AttrPath: d.attrPath,
				Resource: resType + "." + resLabel,
				Filename: d.filename,
			})
			fixed++
		}
	}

	// Append variable definitions to variables.tf
	if len(vars) > 0 {
		appendVariables(outputDir, vars)
	}

	return vars, fixed
}

// attrBlockPath returns the schema block path for an attribute path.
// "code" → "", "basic_auth_credentials.0.password" → "basic_auth_credentials"
func attrBlockPath(attrPath string) string {
	parts := strings.Split(attrPath, ".")
	if len(parts) <= 1 {
		return ""
	}
	// Build block path from pairs: "block.0.nested.0.leaf" → "block.nested"
	var blocks []string
	for i := 0; i < len(parts)-1; i += 2 {
		blocks = append(blocks, parts[i])
	}
	return strings.Join(blocks, ".")
}

// setNullToZeroValue replaces a null attribute with the type-appropriate zero
// value (empty string for strings, false for bools, 0 for numbers) in the
// resource block matching resType/resLabel. Returns true if the attribute was set.
func setNullToZeroValue(filePath string, src []byte, resType, resLabel, attrPath string, schema *ProviderSchema) bool {
	f, hclDiags := hclwrite.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if hclDiags.HasErrors() {
		return false
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != resType || labels[1] != resLabel {
			continue
		}

		body := navigateToBody(block.Body(), attrPath)
		if body == nil {
			continue
		}

		leaf := leafAttrName(attrPath)
		attr := body.GetAttribute(leaf)
		if attr == nil {
			continue
		}

		if !isNullValue(attr) {
			continue
		}

		blockPath := attrBlockPath(attrPath)
		body.SetAttributeValue(leaf, schema.zeroValue(resType, blockPath, leaf))
		if err := os.WriteFile(filePath, f.Bytes(), 0644); err != nil {
			return false
		}
		return true
	}
	return false
}

// replaceNullWithVar navigates to the attribute at attrPath inside the resource
// block matching resType/resLabel and replaces its null value with var.<varName>.
func replaceNullWithVar(f *hclwrite.File, resType, resLabel, attrPath, varName string) bool {
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != resType || labels[1] != resLabel {
			continue
		}

		body := navigateToBody(block.Body(), attrPath)
		if body == nil {
			continue
		}

		leaf := leafAttrName(attrPath)
		attr := body.GetAttribute(leaf)
		if attr == nil {
			continue
		}

		// Only replace if the current value is null
		if !isNullValue(attr) {
			continue
		}

		varRef := hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte("var." + varName)},
		}
		body.SetAttributeRaw(leaf, varRef)
		return true
	}
	return false
}

// navigateToBody follows the attribute path to return the hclwrite.Body that
// contains the leaf attribute. For simple paths like "code", returns the
// resource body directly. For nested paths like "basic_auth_credentials.0.password",
// navigates into the named block.
func navigateToBody(body *hclwrite.Body, attrPath string) *hclwrite.Body {
	parts := strings.Split(attrPath, ".")
	if len(parts) <= 1 {
		return body
	}

	// Navigate through nested blocks: "block_name.index.attr" or deeper
	for i := 0; i < len(parts)-1; i += 2 {
		blockType := parts[i]
		// Skip the index part (e.g. "0")
		found := false
		for _, b := range body.Blocks() {
			if b.Type() == blockType {
				body = b.Body()
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return body
}

// leafAttrName returns the last component of a dot-separated attribute path.
func leafAttrName(attrPath string) string {
	parts := strings.Split(attrPath, ".")
	return parts[len(parts)-1]
}

// stripProviderPrefix removes the provider prefix from a resource type.
// e.g. "jamfpro_smtp_server" -> "smtp_server"
func stripProviderPrefix(resourceType string) string {
	if _, after, ok := strings.Cut(resourceType, "_"); ok {
		return after
	}
	return resourceType
}

// appendVariables adds sensitive variable definitions to variables.tf.
func appendVariables(outputDir string, vars []RequiredVar) {
	varFile := filepath.Join(outputDir, "variables.tf")

	// Read existing content to check for duplicates
	existing, _ := os.ReadFile(varFile) // ignore error: file may not exist yet
	content := string(existing)

	seen := make(map[string]bool)
	var newVars []byte
	for _, v := range vars {
		// Skip if variable already exists on disk or in this batch
		varDecl := fmt.Sprintf("variable %q", v.VarName)
		if strings.Contains(content, varDecl) || seen[v.VarName] {
			continue
		}
		seen[v.VarName] = true

		block := fmt.Sprintf("\nvariable %q {\n  description = %q\n  type        = string\n  sensitive   = true\n}\n",
			v.VarName,
			fmt.Sprintf("Required value for %s %s (write-only, not returned by API)", v.Resource, v.AttrPath),
		)
		newVars = append(newVars, []byte(block)...)
	}

	if len(newVars) > 0 {
		f, err := os.OpenFile(varFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			if !Quiet {
				fmt.Printf("  Warning: could not write variables to %s: %v\n", varFile, err)
			}
			return
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(newVars); err != nil && !Quiet {
			fmt.Printf("  Warning: could not write variables to %s: %v\n", varFile, err)
		}
	}
}

// classifyFix determines what fix to apply based on the diagnostic message.
// Returns nil if the error is not auto-fixable.
func classifyFix(summary, detail, filePath string, line int) *validationFix {
	detailLower := strings.ToLower(detail)
	combined := summary + " " + detail

	// Strategy 1: "remove it" — remove the attribute entirely
	if strings.Contains(detailLower, "remove it") {
		attrName := extractAttrName(summary, detail)
		if attrName == "" {
			return nil
		}
		return &validationFix{filePath: filePath, line: line, attrName: attrName}
	}

	// Strategy 2: "'attr' must be 'VALUE' when ..." — set attribute to required value
	if m := mustBeValueRe.FindStringSubmatch(combined); len(m) >= 3 {
		return &validationFix{filePath: filePath, line: line, attrName: m[1], newValue: m[2]}
	}

	// Strategy 3: "must be one of [A B], got: X" — remove the invalid attribute
	if m := mustBeOneOfRe.FindStringSubmatch(combined); len(m) >= 2 {
		return &validationFix{filePath: filePath, line: line, attrName: m[1]}
	}

	// Strategy 4: "conflicts with other_attr" — remove the attribute the diagnostic points to
	if m := conflictsWithRe.FindStringSubmatch(detail); len(m) >= 3 {
		return &validationFix{filePath: filePath, line: line, attrName: m[1]}
	}

	return nil
}

// extractAttrName pulls the attribute name from a diagnostic summary or detail.
func extractAttrName(summary, detail string) string {
	if m := attrNameFromDiag.FindStringSubmatch(summary); len(m) >= 2 {
		return m[1]
	}
	if m := attrNameFromDiag.FindStringSubmatch(detail); len(m) >= 2 {
		return m[1]
	}
	return ""
}

// removeAttributeFromFile removes an attribute from the resource block at or
// near the given line in a .tf file. Returns true if the attribute was removed.
func removeAttributeFromFile(filePath string, line int, attrName string) bool {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	resType, resLabel := resourceAtLine(src, line)

	f, diags := hclwrite.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return false
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}

		if resType != "" && resLabel != "" {
			if labels[0] != resType || labels[1] != resLabel {
				continue
			}
		}

		if block.Body().GetAttribute(attrName) != nil {
			block.Body().RemoveAttribute(attrName)
			if err := os.WriteFile(filePath, f.Bytes(), 0644); err != nil {
				return false
			}
			return true
		}
	}

	return false
}

// setAttributeInFile sets an attribute to a string value in the resource block
// at or near the given line in a .tf file. Returns true if the attribute was set.
func setAttributeInFile(filePath string, line int, attrName, newValue string) bool {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	resType, resLabel := resourceAtLine(src, line)

	f, diags := hclwrite.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return false
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}

		if resType != "" && resLabel != "" {
			if labels[0] != resType || labels[1] != resLabel {
				continue
			}
		}

		if block.Body().GetAttribute(attrName) != nil {
			block.Body().SetAttributeValue(attrName, cty.StringVal(newValue))
			if err := os.WriteFile(filePath, f.Bytes(), 0644); err != nil {
				return false
			}
			return true
		}
	}

	return false
}

// resourceAtLine finds the resource type and label of the resource block
// declaration at or just before the given line number. Scans backwards
// through the file with no limit to handle resources of any size.
func resourceAtLine(src []byte, line int) (string, string) {
	lines := strings.Split(string(src), "\n")
	for i := min(line-1, len(lines)-1); i >= 0; i-- {
		if m := resourceDeclRe.FindStringSubmatch(lines[i]); len(m) >= 3 {
			return m[1], m[2]
		}
	}
	return "", ""
}
