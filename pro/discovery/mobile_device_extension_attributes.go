// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMobileDeviceExtAttr = "jamfpro_mobile_device_extension_attribute"

func discoverMobileDeviceExtensionAttributes(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetMobileDeviceExtensionAttributes(params)
	if err != nil {
		return nil, fmt.Errorf("listing mobile device extension attributes: %w", err)
	}

	var resources []Resource
	for _, ea := range resp.Results {
		label := tracker.Label(tfResourceMobileDeviceExtAttr, ea.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMobileDeviceExtAttr, label)

		reg.Register(tfResourceMobileDeviceExtAttr, ea.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: ea.ID,
			Name:   ea.Name,
			Label:  label,
		})
	}

	return resources, nil
}
