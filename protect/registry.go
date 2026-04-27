// Copyright 2026, Jamf Software LLC

package protect

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// PopulateRegistryFromGenerated parses a generated HCL file and populates
// the registry with resource type + ID -> address mappings. IDs are extracted
// from import blocks (which use "identity { id = ... }" for query-generated
// imports, or flat "id = ..." for singleton imports).
func PopulateRegistryFromGenerated(generatedFile string, reg *registry.Registry) error {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return fmt.Errorf("reading generated file: %w", err)
	}

	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}

		// Extract resource address from "to" attribute
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		address := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		parts := strings.SplitN(address, ".", 2)
		if len(parts) < 2 {
			continue
		}
		resourceType := parts[0]

		// Extract ID from "identity" attribute (terraform query format: identity = { id = "..." })
		idVal := ""
		if identityAttr := block.Body().GetAttribute("identity"); identityAttr != nil {
			// The identity attribute is an object literal { id = "..." }
			// Scan its expression tokens for the quoted string value
			tokens := identityAttr.Expr().BuildTokens(nil)
			for _, tok := range tokens {
				if tok.Type == hclsyntax.TokenQuotedLit {
					idVal = string(tok.Bytes)
					break
				}
			}
		}

		// Fall back to flat "id" attribute (singleton import format)
		if idVal == "" {
			idAttr := block.Body().GetAttribute("id")
			if idAttr != nil {
				idVal = postprocess.ExtractStringValue(idAttr)
			}
		}

		if idVal != "" {
			reg.Register(resourceType, idVal, address)
		}
	}

	return nil
}

// PopulateRegistryFromImportFiles reads all *_import.tf files in a directory
// and populates the registry. This handles the case where import blocks have
// already been split into per-type files by the post-processor.
func PopulateRegistryFromImportFiles(outputDir string, reg *registry.Registry) error {
	// Read all .tf files in the directory for import blocks
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("reading output directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_import.tf") {
			continue
		}
		if err := PopulateRegistryFromGenerated(fmt.Sprintf("%s/%s", outputDir, entry.Name()), reg); err != nil {
			return err
		}
	}

	return nil
}

// CountResources counts resource blocks by type in the generated file.
func CountResources(generatedFile string) (map[string]int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return nil, err
	}

	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing HCL: %s", diags.Error())
	}

	counts := make(map[string]int)
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			labels := block.Labels()
			if len(labels) >= 1 {
				counts[labels[0]]++
			}
		}
	}

	return counts, nil
}

// ResourceTypeDisplayName returns a human-readable name for a resource type.
func ResourceTypeDisplayName(resourceType string) string {
	name := strings.TrimPrefix(resourceType, "jamfprotect_")
	return strings.ReplaceAll(name, "_", " ")
}
