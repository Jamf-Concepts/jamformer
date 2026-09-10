// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// StripBlueprintDrafts removes non-creatable blueprint drafts — and their
// import blocks — from a generated.tf before anything else reads it. Process
// makes the same call mid-pass for the single-env pipeline; this exists for the
// multi-env path, where the validation auto-fix runs against the raw
// generated.tf first and would otherwise reach a draft ahead of the skip.
//
// It matters because the two treatments contradict each other. The draft's
// empty device_groups trips the provider's SizeAtLeast(1) validator, which the
// auto-fix reads as a Required empty collection and answers by turning the
// attribute into a variable for the operator to supply. That rewrite hides the
// emptiness the skip keys on, so the draft then survives into the module as a
// resource nobody can plan without inventing a device group for a blueprint
// that was never creatable in the first place.
//
// Returns the number of drafts removed. A parse failure leaves the file alone.
func StripBlueprintDrafts(genPath string) (int, error) {
	data, err := os.ReadFile(genPath)
	if err != nil {
		return 0, err
	}
	f, diags := hclwrite.ParseConfig(data, genPath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing %s: %s", filepath.Base(genPath), diags.Error())
	}

	removed := 0
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "jamfplatform_blueprints_blueprint" {
			continue
		}
		if !hasEmptyDeviceGroups(block.Body()) {
			continue
		}

		name := labels[1]
		if nm := block.Body().GetAttribute("name"); nm != nil {
			if v := ExtractStringValue(nm); v != "" {
				name = v
			}
		}
		if !Quiet {
			fmt.Printf("  Skipping blueprint %q: empty device_groups (non-creatable draft)\n", name)
		}
		removeImportBlocksFor(f, labels[0]+"."+labels[1])
		f.Body().RemoveBlock(block)
		removed++
	}

	if removed == 0 {
		return 0, nil
	}
	if err := os.WriteFile(genPath, f.Bytes(), 0644); err != nil {
		return 0, err
	}
	return removed, nil
}

// removeImportBlocksFor drops any import block in f whose `to` names addr. The
// multi-env path folds every import block into generated.tf, so the blocks are
// here rather than in a per-type file.
func removeImportBlocksFor(f *hclwrite.File, addr string) {
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		if strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes())) == addr {
			f.Body().RemoveBlock(block)
		}
	}
}
