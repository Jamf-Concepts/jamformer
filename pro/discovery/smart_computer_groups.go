// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceSmartGroup = "jamfpro_smart_computer_group_v2"

func discoverSmartComputerGroups(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetSmartComputerGroupsV2(params)
	if err != nil {
		return nil, fmt.Errorf("listing smart computer groups: %w", err)
	}

	var resources []Resource
	for _, g := range resp.Results {
		label := tracker.Label(tfResourceSmartGroup, g.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceSmartGroup, label)

		reg.Register(tfResourceSmartGroup, g.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: g.ID,
			Name:   g.Name,
			Label:  label,
		})
	}

	return resources, nil
}
