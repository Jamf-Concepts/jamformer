// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Compliance benchmarks (jamfplatform_cbengine_benchmark) generate a fleet of
// supporting resources — policies, configuration profiles, computer extension
// attributes, smart device groups, scripts, and a category — all owned and
// reconciled by the benchmark engine. Importing those derived resources as
// standalone Terraform resources is wrong: they would double-manage state the
// benchmark already controls and drift on every benchmark update. We keep the
// benchmark resource itself and strip its artifacts.
//
// Two signals identify an artifact:
//   - a category whose name exactly matches a benchmark title; and any
//     policy / profile / script assigned to such a category (category_id);
//   - a computer EA, device group, policy, or profile whose name begins with
//     "<benchmark title> - " (e.g. "CIS v8 - Failed Result List").

// benchmarkArtifactTypes are the resource types a benchmark generates. The
// strip only considers these, so a user resource of an unrelated type can never
// be removed by a coincidental name match.
var benchmarkArtifactTypes = map[string]bool{
	"jamfplatform_pro_category":                     true,
	"jamfplatform_pro_policy":                       true,
	"jamfplatform_pro_macos_configuration_profile":  true,
	"jamfplatform_pro_computer_extension_attribute": true,
	"jamfplatform_device_group":                     true,
	"jamfplatform_pro_script":                       true,
}

var categoryIDRefRe = regexp.MustCompile(`category_id\s*=\s*"(-?\d+)"`)
var nestedNameRe = regexp.MustCompile(`name\s*=\s*"([^"]*)"`)

// StripComplianceBenchmarkArtifacts removes benchmark-derived resources (and
// their import blocks) from the generated config, deregistering them so that any
// reference from a kept resource falls back to a raw ID instead of a dangling
// address. It returns the number of resource blocks removed.
func StripComplianceBenchmarkArtifacts(generatedFile string, reg *registry.Registry) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	// 1. Collect benchmark titles.
	titles := map[string]bool{}
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) >= 1 && block.Labels()[0] == "jamfplatform_cbengine_benchmark" {
			if t := stringAttr(block.Body(), "title"); t != "" {
				titles[t] = true
			}
		}
	}
	if len(titles) == 0 {
		return 0, nil
	}

	// 2. Map import "to" address → Jamf ID (list imports live inline in
	// generated.tf; reused to find benchmark category IDs and to deregister).
	importID := map[string]string{}
	collectImportIDs(f, importID)

	// Identify benchmark categories by name; collect their Jamf IDs so resources
	// assigned to them (raw category_id, pre-rewrite) can be matched.
	benchCategoryIDs := map[string]bool{}
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" || len(block.Labels()) < 2 {
			continue
		}
		if block.Labels()[0] != "jamfplatform_pro_category" {
			continue
		}
		if titles[stringAttr(block.Body(), "name")] {
			addr := block.Labels()[0] + "." + block.Labels()[1]
			if id := importID[addr]; id != "" {
				benchCategoryIDs[id] = true
			}
		}
	}

	// 3. Decide which resource blocks are benchmark artifacts.
	strippedAddrs := map[string]bool{}
	var toRemove []*hclwrite.Block
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" || len(block.Labels()) < 2 {
			continue
		}
		tfType, label := block.Labels()[0], block.Labels()[1]
		if !benchmarkArtifactTypes[tfType] {
			continue
		}

		var strip bool
		switch tfType {
		case "jamfplatform_pro_category":
			strip = titles[stringAttr(block.Body(), "name")]
		case "jamfplatform_pro_computer_extension_attribute", "jamfplatform_device_group":
			strip = hasBenchmarkPrefix(resourceDisplayName(block.Body()), titles)
		default: // policy, macos_configuration_profile, script
			strip = hasBenchmarkPrefix(resourceDisplayName(block.Body()), titles) ||
				referencesBenchmarkCategory(block, benchCategoryIDs)
		}
		if strip {
			toRemove = append(toRemove, block)
			addr := tfType + "." + label
			strippedAddrs[addr] = true
			if id := importID[addr]; id != "" {
				reg.Unregister(tfType, id)
			}
		}
	}

	if len(toRemove) == 0 {
		return 0, nil
	}

	// 4. Remove the resource blocks and their import blocks.
	for _, block := range toRemove {
		f.Body().RemoveBlock(block)
	}
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		addr := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		if strippedAddrs[addr] {
			f.Body().RemoveBlock(block)
		}
	}

	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return len(toRemove), nil
}

// stringAttr returns the string value of a top-level attribute, or "".
func stringAttr(body *hclwrite.Body, name string) string {
	attr := body.GetAttribute(name)
	if attr == nil {
		return ""
	}
	return postprocess.ExtractStringValue(attr)
}

// resourceDisplayName returns a resource's human-readable name: the top-level
// `name` attribute, or `general.name` for the object-attribute types (policies,
// configuration profiles) that nest it.
func resourceDisplayName(body *hclwrite.Body) string {
	if n := stringAttr(body, "name"); n != "" {
		return n
	}
	if g := body.GetAttribute("general"); g != nil {
		if m := nestedNameRe.FindStringSubmatch(string(g.Expr().BuildTokens(nil).Bytes())); m != nil {
			return m[1]
		}
	}
	return ""
}

// hasBenchmarkPrefix reports whether name begins with "<title> - " for any
// benchmark title.
func hasBenchmarkPrefix(name string, titles map[string]bool) bool {
	if name == "" {
		return false
	}
	for title := range titles {
		if strings.HasPrefix(name, title+" - ") {
			return true
		}
	}
	return false
}

// referencesBenchmarkCategory reports whether the block assigns itself to a
// benchmark category via a raw category_id (top-level or nested), pre-rewrite.
func referencesBenchmarkCategory(block *hclwrite.Block, benchCategoryIDs map[string]bool) bool {
	if len(benchCategoryIDs) == 0 {
		return false
	}
	body := string(block.Body().BuildTokens(nil).Bytes())
	for _, m := range categoryIDRefRe.FindAllStringSubmatch(body, -1) {
		if benchCategoryIDs[m[1]] {
			return true
		}
	}
	return false
}
