// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceRestrictedSoftware = "jamfpro_restricted_software"

func discoverRestrictedSoftware(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetRestrictedSoftwares()
	if err != nil {
		return nil, fmt.Errorf("listing restricted software: %w", err)
	}

	var resources []Resource
	for _, rs := range resp.RestrictedSoftware {
		jamfID := strconv.Itoa(rs.ID)
		label := tracker.Label(tfResourceRestrictedSoftware, rs.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceRestrictedSoftware, label)

		reg.Register(tfResourceRestrictedSoftware, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   rs.Name,
			Label:  label,
		})
	}

	return resources, nil
}
