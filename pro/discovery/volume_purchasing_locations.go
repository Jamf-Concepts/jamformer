// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceVolumePurchasingLocation = "jamfpro_volume_purchasing_locations"

func discoverVolumePurchasingLocations(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetVolumePurchaseLocations(params)
	if err != nil {
		return nil, fmt.Errorf("listing volume purchasing locations: %w", err)
	}

	var resources []Resource
	for _, vpl := range resp.Results {
		label := tracker.Label(tfResourceVolumePurchasingLocation, vpl.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceVolumePurchasingLocation, label)

		reg.Register(tfResourceVolumePurchasingLocation, vpl.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: vpl.ID,
			Name:   vpl.Name,
			Label:  label,
		})
	}

	return resources, nil
}
