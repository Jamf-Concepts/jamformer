// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// DiscoveredResource is a flat (type, label, name, id) record for a single
// resource, used by the multi-env merge pipeline to match resources across
// environments by (type, label) and to write per-env import blocks.
type DiscoveredResource struct {
	TFType string // e.g. "jamfplatform_pro_policy"
	Label  string // e.g. "install_chrome"
	Name   string // human-readable Jamf name (falls back to the label)
	JamfID string // import ID; empty if no import block was found
}

// CollectResourceRefs enumerates the discovered resources from the generated
// HCL by joining each resource block with its import block. List-resource
// import blocks live inside generatedFile (terraform query format,
// identity = { id = "..." }); singleton and jamf_connect import blocks live in
// the supplied importFiles (flat id = "..."). Resource labels and import "to"
// addresses are consistent after RenameLabels, so the join is by address.
//
// A resource with no matching import block is still returned, with an empty
// JamfID (it gets no per-env import block but still participates in matching).
func CollectResourceRefs(generatedFile string, importFiles ...string) ([]DiscoveredResource, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return nil, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	// address ("type.label") → import ID
	ids := make(map[string]string)
	collectImportIDs(f, ids)

	// Pick up import blocks that live in their own files (singletons, jamf_connect).
	for _, path := range importFiles {
		data, rErr := os.ReadFile(path)
		if rErr != nil {
			continue // file may not exist (no singletons / no jamf_connect links)
		}
		pf, pDiags := hclwrite.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
		if pDiags.HasErrors() {
			continue
		}
		collectImportIDs(pf, ids)
	}

	var out []DiscoveredResource
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		tfType, label := labels[0], labels[1]
		address := tfType + "." + label
		out = append(out, DiscoveredResource{
			TFType: tfType,
			Label:  label,
			Name:   label,
			JamfID: ids[address],
		})
	}
	return out, nil
}

// collectImportIDs records the import ID for each "import" block in the file,
// keyed by its "to" address. Handles both the terraform query identity format
// (identity = { id = "..." }) and the flat singleton format (id = "...").
func collectImportIDs(f *hclwrite.File, ids map[string]string) {
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		address := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		parts := strings.SplitN(address, ".", 2)
		if len(parts) < 2 {
			continue
		}

		idVal := ""
		if identityAttr := block.Body().GetAttribute("identity"); identityAttr != nil {
			for _, tok := range identityAttr.Expr().BuildTokens(nil) {
				if tok.Type == hclsyntax.TokenQuotedLit {
					idVal = string(tok.Bytes)
					break
				}
			}
		}
		if idVal == "" {
			if idAttr := block.Body().GetAttribute("id"); idAttr != nil {
				idVal = postprocess.ExtractStringValue(idAttr)
			}
		}
		if idVal != "" {
			ids[address] = idVal
		}
	}
}
