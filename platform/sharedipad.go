// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// NormalizeSharedIpadActivationLock sets prevent_activation_lock = true on any
// mobile_device_prestage_enrollment with multi_user = true (Shared iPad).
//
// This is not a value the admin can choose: when Shared iPad is enabled the Jamf
// Pro UI auto-checks "Prevent user from enabling Activation Lock" and disables
// the control, and the API rejects a write of the pair with
// `400 PREREQUISITE_NOT_MET: "Prevent activation lock needs to be enabled in
// order to enable Shared iPad"` (wire-confirmed by creating a dummy prestage).
// So true is the only value Jamf permits for a Shared iPad prestage — the
// provider validates this and rejects false.
//
// A prestage can still be *read back* with prevent_activation_lock = false if it
// was created before Jamf added the prerequisite (legacy data). Exporting that
// stale false verbatim produces a config that neither validates nor applies, so
// we normalize it to the canonical, Jamf-enforced value (matching what the UI
// shows and what the server requires). Returns the number of prestages adjusted.
func NormalizeSharedIpadActivationLock(generatedFile string) (int, error) {
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
		if block.Type() != "resource" || len(block.Labels()) < 1 ||
			block.Labels()[0] != "jamfplatform_pro_mobile_device_prestage_enrollment" {
			continue
		}
		body := block.Body()
		if !attrIsTrue(body, "multi_user") {
			continue
		}
		pal := body.GetAttribute("prevent_activation_lock")
		if pal == nil || attrIsTrue(body, "prevent_activation_lock") {
			continue // absent (left to provider default) or already true
		}
		body.SetAttributeValue("prevent_activation_lock", cty.True)
		fixed++
	}

	if fixed == 0 {
		return 0, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return fixed, nil
}

// attrIsTrue reports whether a top-level boolean attribute's expression is the
// literal `true`.
func attrIsTrue(body *hclwrite.Body, name string) bool {
	attr := body.GetAttribute(name)
	if attr == nil {
		return false
	}
	return strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes())) == "true"
}
