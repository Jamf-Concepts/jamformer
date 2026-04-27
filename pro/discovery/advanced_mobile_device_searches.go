// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAdvancedMobileDeviceSearch = "jamfpro_advanced_mobile_device_search"

func discoverAdvancedMobileDeviceSearches(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAdvancedMobileDeviceSearches()
	if err != nil {
		return nil, fmt.Errorf("listing advanced mobile device searches: %w", err)
	}

	var resources []Resource
	for _, s := range resp.Results {
		label := tracker.Label(tfResourceAdvancedMobileDeviceSearch, s.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAdvancedMobileDeviceSearch, label)

		reg.Register(tfResourceAdvancedMobileDeviceSearch, s.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: s.ID,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
