// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceDeviceEnrollment = "jamfpro_device_enrollments"

func discoverDeviceEnrollments(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetDeviceEnrollments(params)
	if err != nil {
		return nil, fmt.Errorf("listing device enrollments: %w", err)
	}

	var resources []Resource
	for _, de := range resp.Results {
		label := tracker.Label(tfResourceDeviceEnrollment, de.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceDeviceEnrollment, label)

		reg.Register(tfResourceDeviceEnrollment, de.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: de.ID,
			Name:   de.Name,
			Label:  label,
		})
	}

	return resources, nil
}
