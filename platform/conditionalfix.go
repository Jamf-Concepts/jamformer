// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// FixConditionalAttributes corrects attribute combinations the provider rejects
// in its plan-time ValidateConfig/ModifyPlan (so `terraform validate` does not
// surface them and the generic validation auto-fix can't catch them). These are
// deterministic, server-consistent corrections applied to the generated config
// for both single-env and multi-env exports:
//
//   - mobile_device_prestage_enrollment: a Shared iPad prestage (multi_user =
//     true) requires prevent_activation_lock = true; the provider rejects false.
//   - self_service_macos_settings: a default home category other than -1
//     requires the Browse landing page — with any other landing page Jamf
//     silently resets the category to -1, so we set it to -1 to match.
//
// Returns the number of attributes corrected.
func FixConditionalAttributes(generatedFile string) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	fixed := 0
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" || len(block.Labels()) < 1 {
			continue
		}
		body := block.Body()
		switch block.Labels()[0] {
		case "jamfplatform_pro_mobile_device_prestage_enrollment":
			if rawBool(body, "multi_user") && body.GetAttribute("prevent_activation_lock") != nil && !rawBool(body, "prevent_activation_lock") {
				body.SetAttributeValue("prevent_activation_lock", cty.True)
				fixed++
			}
		case "jamfplatform_pro_self_service_macos_settings":
			landing := postprocess.ExtractStringValue(body.GetAttribute("default_landing_page"))
			homeAttr := body.GetAttribute("default_home_category_id")
			home := postprocess.ExtractStringValue(homeAttr)
			if homeAttr != nil && landing != "BROWSE" && home != "" && home != "-1" {
				body.SetAttributeValue("default_home_category_id", cty.StringVal("-1"))
				fixed++
			}
		}
	}

	if fixed == 0 {
		return 0, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return fixed, nil
}

// rawBool reports whether a top-level attribute's expression is the literal
// `true`.
func rawBool(body *hclwrite.Body, name string) bool {
	attr := body.GetAttribute(name)
	if attr == nil {
		return false
	}
	return strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes())) == "true"
}
