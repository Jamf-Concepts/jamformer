// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceCategory = "jamfpro_category"

func discoverCategories(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetCategories(params)
	if err != nil {
		return nil, fmt.Errorf("listing categories: %w", err)
	}

	var resources []Resource
	for _, c := range resp.Results {
		label := tracker.Label(tfResourceCategory, c.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceCategory, label)

		reg.Register(tfResourceCategory, c.Id, tfAddr)

		resources = append(resources, Resource{
			JamfID: c.Id,
			Name:   c.Name,
			Label:  label,
		})
	}

	return resources, nil
}
