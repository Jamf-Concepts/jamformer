// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"fmt"
	"os"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// apiRolePrivLister is the slice of the federated pro client used to fetch the
// instance's assignable API-role privilege catalog (kept as an interface so the
// filter is unit-testable).
type apiRolePrivLister interface {
	ListApiRolePrivilegesV1(ctx context.Context) (*sdkpro.ApiRolePrivileges, error)
}

// FilterApiRolePrivileges drops privileges from jamfplatform_pro_api_role
// resources that are not in the instance's current assignable-privilege
// catalog. The provider validates each privilege against this same
// instance-sourced list at plan time and rejects unknown values (Jamf Pro would
// also reject them on apply); a role can legitimately still hold privileges that
// have since been removed/renamed in the instance (e.g. for features no longer
// enabled). Keeping them makes the config unappliable, so we drop them and warn
// per role. Returns the total number of privileges dropped.
func FilterApiRolePrivileges(ctx context.Context, pc apiRolePrivLister, generatedFile string) (int, error) {
	resp, err := pc.ListApiRolePrivilegesV1(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing api role privileges: %w", err)
	}
	if resp == nil || len(resp.Privileges) == 0 {
		return 0, nil // no catalog to validate against — leave the config untouched
	}
	valid := make(map[string]bool, len(resp.Privileges))
	for _, p := range resp.Privileges {
		valid[p] = true
	}

	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	dropped := 0
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" || len(block.Labels()) < 2 || block.Labels()[0] != "jamfplatform_pro_api_role" {
			continue
		}
		attr := block.Body().GetAttribute("privileges")
		if attr == nil {
			continue
		}

		var kept []cty.Value
		var removed []string
		for _, p := range stringListElements(attr) {
			if valid[p] {
				kept = append(kept, cty.StringVal(p))
			} else {
				removed = append(removed, p)
			}
		}
		if len(removed) == 0 {
			continue
		}

		if len(kept) == 0 {
			block.Body().SetAttributeValue("privileges", cty.ListValEmpty(cty.String))
		} else {
			block.Body().SetAttributeValue("privileges", cty.ListVal(kept))
		}
		dropped += len(removed)
		if !Quiet {
			fmt.Printf("  ⚠ jamfplatform_pro_api_role.%s: dropped %d privilege(s) not valid on this instance: %v\n", block.Labels()[1], len(removed), removed)
		}
	}

	if dropped == 0 {
		return 0, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return dropped, nil
}

// stringListElements returns the string literals of a list-of-strings attribute
// expression (e.g. `["A", "B"]`), in order.
func stringListElements(attr *hclwrite.Attribute) []string {
	var out []string
	for _, tok := range attr.Expr().BuildTokens(nil) {
		if tok.Type == hclsyntax.TokenQuotedLit {
			out = append(out, string(tok.Bytes))
		}
	}
	return out
}
