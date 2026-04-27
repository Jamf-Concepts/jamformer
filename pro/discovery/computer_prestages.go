// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceComputerPrestage = "jamfpro_computer_prestage_enrollment"

func discoverComputerPrestages(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetComputerPrestages(params)
	if err != nil {
		return nil, fmt.Errorf("listing computer prestages: %w", err)
	}

	var resources []Resource
	for _, cp := range resp.Results {
		label := tracker.Label(tfResourceComputerPrestage, cp.DisplayName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceComputerPrestage, label)

		reg.Register(tfResourceComputerPrestage, cp.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: cp.ID,
			Name:   cp.DisplayName,
			Label:  label,
		})
	}

	return resources, nil
}
