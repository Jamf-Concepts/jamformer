// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceComputerExtensionAttribute = "jamfpro_computer_extension_attribute"

func discoverComputerExtensionAttributes(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetComputerExtensionAttributes(params)
	if err != nil {
		return nil, fmt.Errorf("listing computer extension attributes: %w", err)
	}

	var resources []Resource
	for _, ea := range resp.Results {
		label := tracker.Label(tfResourceComputerExtensionAttribute, ea.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceComputerExtensionAttribute, label)

		reg.Register(tfResourceComputerExtensionAttribute, ea.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: ea.ID,
			Name:   ea.Name,
			Label:  label,
		})
	}

	return resources, nil
}
