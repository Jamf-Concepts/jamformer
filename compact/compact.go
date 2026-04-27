// Copyright 2026, Jamf Software LLC

// Package compact consolidates simple, uniform resource types into for_each +
// locals patterns. This is a provider-agnostic post-processing step that
// operates on HCL files in the output directory.
//
// Eligibility is determined dynamically at runtime: a resource type qualifies
// if the file contains ≥2 resource blocks, all blocks share the same attribute
// names, and none have nested blocks (other than lifecycle). Callers can
// further restrict which types are considered via include/exclude lists.
// Copyright 2026, Jamf Software LLC

package compact

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jamf-Concepts/jamformer/terraform"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Quiet suppresses progress messages when true.
var Quiet bool

// resourceLabel is the label used on the consolidated for_each resource block.
// e.g. resource "jamfpro_category" "all" { for_each = ... }
const resourceLabel = "all"

// Options controls which resource types are compacted.
type Options struct {
	// Include, if non-empty, restricts compaction to only these types.
	// Matched against the output filename stem (e.g. "categories", "buildings").
	Include map[string]bool

	// Exclude excludes these types from compaction, even if they appear eligible.
	// Matched against the output filename stem.
	Exclude map[string]bool
}

// Run scans all .tf files in the output directory and consolidates eligible
// resource types into for_each + locals patterns.
func Run(outputDir string, opts *Options) error {
	files, err := filepath.Glob(filepath.Join(outputDir, "*.tf"))
	if err != nil {
		return err
	}

	// Track all address rewrites across all files
	allRewrites := make(map[string]string)

	for _, filePath := range files {
		base := filepath.Base(filePath)

		// Skip non-resource files
		if shouldSkipFile(base) {
			continue
		}

		stem := strings.TrimSuffix(base, ".tf")

		// Apply include/exclude filters
		if opts != nil {
			if len(opts.Include) > 0 && !opts.Include[stem] {
				continue
			}
			if opts.Exclude[stem] {
				continue
			}
		}

		result, err := consolidateFile(filePath, stem)
		if err != nil {
			if !Quiet {
				fmt.Printf("  Skipping %s compaction: %v\n", stem, err)
			}
			continue
		}
		if result == nil {
			continue // not eligible
		}

		if err := os.WriteFile(filePath, result.content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", base, err)
		}

		maps.Copy(allRewrites, result.rewrites)

		if !Quiet {
			fmt.Printf("  Compacted %d %s into for_each\n", len(result.labels), result.resourceType)
		}
	}

	if len(allRewrites) == 0 {
		return nil
	}

	// Rewrite references in all .tf files
	if err := rewriteReferences(outputDir, allRewrites); err != nil {
		return fmt.Errorf("rewriting references: %w", err)
	}

	// Rewrite import files
	if err := rewriteImportFiles(outputDir, allRewrites); err != nil {
		return fmt.Errorf("rewriting import files: %w", err)
	}

	terraform.FormatDir(outputDir)

	return nil
}

// shouldSkipFile returns true for files that should never be considered for
// compaction (infrastructure files, import files, moved blocks, etc.).
func shouldSkipFile(filename string) bool {
	switch {
	case filename == "provider.tf",
		filename == "variables.tf",
		filename == "terraform.tfvars",
		filename == "moved.tf",
		filename == "locals.tf":
		return true
	case strings.HasSuffix(filename, "_import.tf"):
		return true
	}
	return false
}

// consolidateResult holds the output of consolidating a single file.
type consolidateResult struct {
	content      []byte            // new file content
	rewrites     map[string]string // old address → new address
	labels       []string          // original labels, sorted
	resourceType string            // TF resource type that was consolidated
}

// consolidateFile parses a per-type .tf file and attempts to consolidate all
// resource blocks into a locals + for_each pattern. Returns nil if the file is
// not eligible (fewer than 2 instances of a single type, non-uniform attributes,
// nested blocks, or multiple resource types in the file).
func consolidateFile(filePath, localsKey string) (*consolidateResult, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	parsed, diags := hclwrite.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse error: %s", diags.Error())
	}

	// Collect all resource blocks, grouped by type
	type resourceInfo struct {
		label string
		attrs map[string][]byte // attr name → raw expression bytes
	}

	resourcesByType := make(map[string][]resourceInfo)
	var nonResourceBlocks []*hclwrite.Block

	for _, block := range parsed.Body().Blocks() {
		if block.Type() != "resource" || len(block.Labels()) < 2 {
			nonResourceBlocks = append(nonResourceBlocks, block)
			continue
		}

		resourceType := block.Labels()[0]
		label := block.Labels()[1]
		body := block.Body()

		// Nested blocks (other than lifecycle) make the type ineligible
		hasComplexBlocks := false
		for _, nested := range body.Blocks() {
			if nested.Type() != "lifecycle" {
				hasComplexBlocks = true
				break
			}
		}
		if hasComplexBlocks {
			return nil, nil
		}

		attrs := make(map[string][]byte)
		for name, attr := range body.Attributes() {
			tokens := attr.Expr().BuildTokens(nil)
			// Nested attributes (structural types) use object literals { ... }
			// in the expression. These are too complex for locals map compaction.
			if containsObjectLiteral(tokens) {
				return nil, nil
			}
			attrs[name] = tokens.Bytes()
		}

		resourcesByType[resourceType] = append(resourcesByType[resourceType], resourceInfo{label: label, attrs: attrs})
	}

	// Must be exactly one resource type in the file, with ≥2 instances
	if len(resourcesByType) != 1 {
		return nil, nil
	}

	var resourceType string
	var resources []resourceInfo
	for rt, rs := range resourcesByType {
		resourceType = rt
		resources = rs
	}

	if len(resources) < 2 {
		return nil, nil
	}

	// Check uniformity: all resources must have the same attribute names
	refAttrs := attrNames(resources[0].attrs)
	for _, r := range resources[1:] {
		if !equalStringSlices(refAttrs, attrNames(r.attrs)) {
			return nil, nil // non-uniform
		}
	}

	// Sort resources by label for deterministic output
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].label < resources[j].label
	})

	// Sort attribute names for consistent ordering
	sort.Strings(refAttrs)

	// Partition attributes into shared (identical across all instances) and
	// varying (at least one instance differs). Shared attributes go directly
	// on the resource block as literals; varying attributes go in the locals map.
	var varyingAttrs []string
	sharedValues := make(map[string][]byte) // attr name → shared value bytes
	for _, attrName := range refAttrs {
		refVal := string(resources[0].attrs[attrName])
		identical := true
		for _, r := range resources[1:] {
			if string(r.attrs[attrName]) != refVal {
				identical = false
				break
			}
		}
		if identical {
			sharedValues[attrName] = resources[0].attrs[attrName]
		} else {
			varyingAttrs = append(varyingAttrs, attrName)
		}
	}

	// Must have at least one varying attribute (otherwise all resources are
	// identical and compaction adds no value — the user should just have one)
	if len(varyingAttrs) == 0 {
		return nil, nil
	}

	// Build the output file
	out := hclwrite.NewEmptyFile()
	outBody := out.Body()

	// Write the locals block with only varying attributes
	localsBlock := outBody.AppendNewBlock("locals", nil)
	localsBody := localsBlock.Body()

	var mapBuf strings.Builder
	mapBuf.WriteString("{\n")
	for _, r := range resources {
		fmt.Fprintf(&mapBuf, "    %s = {", r.label)
		if len(varyingAttrs) == 1 {
			fmt.Fprintf(&mapBuf, " %s = %s ", varyingAttrs[0], string(r.attrs[varyingAttrs[0]]))
			mapBuf.WriteString("}\n")
		} else {
			mapBuf.WriteString("\n")
			for _, attrName := range varyingAttrs {
				fmt.Fprintf(&mapBuf, "      %s = %s\n", attrName, string(r.attrs[attrName]))
			}
			mapBuf.WriteString("    }\n")
		}
	}
	mapBuf.WriteString("  }")

	localsBody.SetAttributeRaw(localsKey, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(mapBuf.String())},
	})

	outBody.AppendNewline()

	// Write the single resource block with for_each
	resourceBlock := outBody.AppendNewBlock("resource", []string{resourceType, resourceLabel})
	resourceBody := resourceBlock.Body()

	resourceBody.SetAttributeRaw("for_each", hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, "local.%s", localsKey)},
	})
	resourceBody.AppendNewline()

	// Write all attributes in sorted order: shared attrs get literal values,
	// varying attrs get each.value references
	for _, attrName := range refAttrs {
		if _, isShared := sharedValues[attrName]; isShared {
			resourceBody.SetAttributeRaw(attrName, hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: sharedValues[attrName]},
			})
		} else {
			resourceBody.SetAttributeRaw(attrName, hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, "each.value.%s", attrName)},
			})
		}
	}

	// Copy lifecycle blocks from first resource if present
	for _, block := range parsed.Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) >= 2 && block.Labels()[0] == resourceType {
			for _, nested := range block.Body().Blocks() {
				if nested.Type() == "lifecycle" {
					resourceBody.AppendNewline()
					newLifecycle := resourceBody.AppendNewBlock("lifecycle", nil)
					copyBody(newLifecycle.Body(), nested.Body())
				}
			}
			break
		}
	}

	// Append any non-resource blocks that were in the file
	for _, block := range nonResourceBlocks {
		outBody.AppendNewline()
		newBlock := outBody.AppendNewBlock(block.Type(), block.Labels())
		copyBody(newBlock.Body(), block.Body())
	}

	// Build rewrites map
	rewrites := make(map[string]string)
	var labels []string
	for _, r := range resources {
		old := fmt.Sprintf("%s.%s", resourceType, r.label)
		new := fmt.Sprintf("%s.%s[\"%s\"]", resourceType, resourceLabel, r.label)
		rewrites[old] = new
		labels = append(labels, r.label)
	}

	return &consolidateResult{
		content:      out.Bytes(),
		rewrites:     rewrites,
		labels:       labels,
		resourceType: resourceType,
	}, nil
}

// rewriteReferences replaces old-style addresses with for_each addresses
// across all .tf files in the output directory.
func rewriteReferences(outputDir string, rewrites map[string]string) error {
	files, err := filepath.Glob(filepath.Join(outputDir, "*.tf"))
	if err != nil {
		return err
	}

	// Sort old addresses longest first so we don't accidentally match a prefix
	type rewriteEntry struct {
		old string
		new string
	}
	var entries []rewriteEntry
	for old, new := range rewrites {
		entries = append(entries, rewriteEntry{old: old, new: new})
	}
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].old) > len(entries[j].old)
	})

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		content := string(data)
		modified := false
		for _, e := range entries {
			if strings.Contains(content, e.old) {
				content = replaceAddress(content, e.old, e.new)
				modified = true
			}
		}

		if modified {
			if err := os.WriteFile(file, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

// replaceAddress replaces all occurrences of old with new, but only where old
// is not followed by an identifier character (letter, digit, underscore).
// This prevents partial matches like "type.foo" matching inside "type.foo_2".
func replaceAddress(content, old, new string) string {
	var b strings.Builder
	b.Grow(len(content))
	for {
		idx := strings.Index(content, old)
		if idx == -1 {
			b.WriteString(content)
			break
		}
		afterIdx := idx + len(old)
		if afterIdx < len(content) {
			ch := content[afterIdx]
			if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
				// Not a full match — skip past this occurrence
				b.WriteString(content[:afterIdx])
				content = content[afterIdx:]
				continue
			}
		}
		b.WriteString(content[:idx])
		b.WriteString(new)
		content = content[afterIdx:]
	}
	return b.String()
}

// rewriteImportFiles updates import block `to` attributes for consolidated types.
func rewriteImportFiles(outputDir string, rewrites map[string]string) error {
	files, err := filepath.Glob(filepath.Join(outputDir, "*_import.tf"))
	if err != nil {
		return err
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}

		content := string(data)
		modified := false
		for old, new := range rewrites {
			if strings.Contains(content, old) {
				content = strings.ReplaceAll(content, old, new)
				modified = true
			}
		}

		if modified {
			if err := os.WriteFile(file, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
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

// containsObjectLiteral checks if an attribute's expression tokens contain
// an object literal ({ ... }), indicating a nested/structural attribute that
// shouldn't be compacted into a locals map.
func containsObjectLiteral(tokens hclwrite.Tokens) bool {
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenOBrace {
			return true
		}
	}
	return false
}

// attrNames returns sorted attribute names from a map.
func attrNames(attrs map[string][]byte) []string {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// equalStringSlices checks if two sorted string slices are equal.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
