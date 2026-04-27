// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMobileDeviceApplication = "jamfpro_mobile_device_application"

func discoverMobileDeviceApplications(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetMobileDeviceApplications()
	if err != nil {
		return nil, fmt.Errorf("listing mobile device applications: %w", err)
	}

	var resources []Resource
	for _, app := range resp.MobileDeviceApplications {
		jamfID := strconv.Itoa(app.ID)
		label := tracker.Label(tfResourceMobileDeviceApplication, app.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMobileDeviceApplication, label)

		reg.Register(tfResourceMobileDeviceApplication, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   app.Name,
			Label:  label,
		})
	}

	return resources, nil
}
