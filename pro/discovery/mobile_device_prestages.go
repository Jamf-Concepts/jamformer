// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMobileDevicePrestage = "jamfpro_mobile_device_prestage_enrollment"

func discoverMobileDevicePrestages(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetMobileDevicePrestages(params)
	if err != nil {
		return nil, fmt.Errorf("listing mobile device prestages: %w", err)
	}

	var resources []Resource
	for _, mp := range resp.Results {
		label := tracker.Label(tfResourceMobileDevicePrestage, mp.DisplayName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMobileDevicePrestage, label)

		reg.Register(tfResourceMobileDevicePrestage, mp.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: mp.ID,
			Name:   mp.DisplayName,
			Label:  label,
		})
	}

	return resources, nil
}
