// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMacOSConfigProfile = "jamfpro_macos_configuration_profile_plist"

func discoverMacOSConfigurationProfiles(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetMacOSConfigurationProfiles()
	if err != nil {
		return nil, fmt.Errorf("listing macOS configuration profiles: %w", err)
	}

	var resources []Resource
	for _, p := range resp.Results {
		jamfID := strconv.Itoa(p.ID)
		label := tracker.Label(tfResourceMacOSConfigProfile, p.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMacOSConfigProfile, label)

		reg.Register(tfResourceMacOSConfigProfile, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   p.Name,
			Label:  label,
		})
	}

	return resources, nil
}
