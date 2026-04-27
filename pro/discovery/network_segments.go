// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceNetworkSegment = "jamfpro_network_segment"

func discoverNetworkSegments(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetNetworkSegments()
	if err != nil {
		return nil, fmt.Errorf("listing network segments: %w", err)
	}

	var resources []Resource
	for _, ns := range resp.Results {
		jamfID := strconv.Itoa(ns.ID)
		label := tracker.Label(tfResourceNetworkSegment, ns.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceNetworkSegment, label)

		reg.Register(tfResourceNetworkSegment, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   ns.Name,
			Label:  label,
		})
	}

	return resources, nil
}
