// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMobileDeviceConfigProfile = "jamfpro_mobile_device_configuration_profile_plist"

func discoverMobileDeviceConfigurationProfiles(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetMobileDeviceConfigurationProfiles()
	if err != nil {
		return nil, fmt.Errorf("listing mobile device configuration profiles: %w", err)
	}

	var resources []Resource
	for _, p := range resp.ConfigurationProfiles {
		jamfID := strconv.Itoa(p.ID)
		label := tracker.Label(tfResourceMobileDeviceConfigProfile, p.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMobileDeviceConfigProfile, label)

		reg.Register(tfResourceMobileDeviceConfigProfile, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   p.Name,
			Label:  label,
		})
	}

	return resources, nil
}
