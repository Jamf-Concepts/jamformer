// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// DiscoverJamfConnect lists the Jamf Connect config-profile links via the
// federated pro SDK and writes jamf_connect_import.tf with one import block per
// link (friendly label from the profile name, id = the config-profile id).
//
// jamfplatform_pro_jamf_connect has no list resource — it adopts an existing
// macOS configuration profile that already carries a Jamf Connect payload — so
// it is not query-discoverable. The SDK enumerates the linked profiles; the
// subsequent `terraform plan -generate-config-out` materialises each resource's
// config from the import blocks written here (the same mechanism that handles
// the settings singletons). The import id is a passthrough of the config-profile
// id, which post-processing rewrites to a jamfplatform_pro_macos_configuration_profile
// reference (via the Numeric profile_id rule). Returns the number of import
// blocks written.
func DiscoverJamfConnect(ctx context.Context, pc *sdkpro.Client, outputDir string) (int, error) {
	profiles, err := pc.ListJamfConnectConfigProfilesV1(ctx, nil, "")
	if err != nil {
		return 0, fmt.Errorf("listing Jamf Connect config profiles: %w", err)
	}

	f := hclwrite.NewEmptyFile()
	body := f.Body()
	tracker := naming.NewTracker()
	count := 0
	for _, p := range profiles {
		if p.ProfileID == nil {
			continue
		}
		id := strconv.Itoa(*p.ProfileID)
		seed := "jamf_connect_" + id
		if p.ProfileName != nil && *p.ProfileName != "" {
			seed = *p.ProfileName
		}
		label := tracker.Label(tJamfConnect, seed)

		if count > 0 {
			body.AppendNewline()
		}
		blk := body.AppendNewBlock("import", nil)
		blk.Body().SetAttributeRaw("to", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(tJamfConnect + "." + label)},
		})
		blk.Body().SetAttributeValue("id", cty.StringVal(id))
		count++
	}

	if count == 0 {
		return 0, nil
	}
	if err := os.WriteFile(filepath.Join(outputDir, "jamf_connect_import.tf"), f.Bytes(), 0644); err != nil {
		return 0, err
	}
	return count, nil
}
