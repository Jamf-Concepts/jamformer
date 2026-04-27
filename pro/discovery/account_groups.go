// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAccountGroup = "jamfpro_account_group"

func discoverAccountGroups(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAccounts()
	if err != nil {
		return nil, fmt.Errorf("listing account groups: %w", err)
	}

	var resources []Resource
	for _, g := range resp.Groups {
		jamfID := strconv.Itoa(g.ID)
		label := tracker.Label(tfResourceAccountGroup, g.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAccountGroup, label)

		reg.Register(tfResourceAccountGroup, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   g.Name,
			Label:  label,
		})
	}

	return resources, nil
}
