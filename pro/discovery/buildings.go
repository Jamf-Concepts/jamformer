// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceBuilding = "jamfpro_building"

func discoverBuildings(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetBuildings(params)
	if err != nil {
		return nil, fmt.Errorf("listing buildings: %w", err)
	}

	var resources []Resource
	for _, b := range resp.Results {
		label := tracker.Label(tfResourceBuilding, b.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceBuilding, label)

		reg.Register(tfResourceBuilding, b.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: b.ID,
			Name:   b.Name,
			Label:  label,
		})
	}

	return resources, nil
}
