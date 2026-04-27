// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceSite = "jamfpro_site"

func discoverSites(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetSites()
	if err != nil {
		return nil, fmt.Errorf("listing sites: %w", err)
	}

	var resources []Resource
	for _, s := range resp.Site {
		jamfID := strconv.Itoa(s.ID)
		label := tracker.Label(tfResourceSite, s.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceSite, label)

		reg.Register(tfResourceSite, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
