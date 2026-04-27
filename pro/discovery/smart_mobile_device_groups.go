// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceSmartMobileDeviceGroup = "jamfpro_smart_mobile_device_group_v1"

func discoverSmartMobileDeviceGroups(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetSmartMobileDeviceGroupsV1(params)
	if err != nil {
		return nil, fmt.Errorf("listing smart mobile device groups: %w", err)
	}

	var resources []Resource
	for _, g := range resp.Results {
		label := tracker.Label(tfResourceSmartMobileDeviceGroup, g.GroupName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceSmartMobileDeviceGroup, label)

		reg.Register(tfResourceSmartMobileDeviceGroup, g.GroupID, tfAddr)

		resources = append(resources, Resource{
			JamfID: g.GroupID,
			Name:   g.GroupName,
			Label:  label,
		})
	}

	return resources, nil
}
