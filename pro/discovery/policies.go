// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourcePolicy = "jamfpro_policy"

func discoverPolicies(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetPolicies()
	if err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}

	var resources []Resource
	for _, p := range resp.Policy {
		jamfID := strconv.Itoa(p.ID)
		label := tracker.Label(tfResourcePolicy, p.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourcePolicy, label)

		reg.Register(tfResourcePolicy, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   p.Name,
			Label:  label,
		})
	}

	return resources, nil
}
