// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceUserGroup = "jamfpro_user_group"

func discoverUserGroups(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetUserGroups()
	if err != nil {
		return nil, fmt.Errorf("listing user groups: %w", err)
	}

	var resources []Resource
	for _, g := range resp.UserGroup {
		jamfID := strconv.Itoa(g.ID)
		label := tracker.Label(tfResourceUserGroup, g.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceUserGroup, label)

		reg.Register(tfResourceUserGroup, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   g.Name,
			Label:  label,
		})
	}

	return resources, nil
}
