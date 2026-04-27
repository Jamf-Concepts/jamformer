// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAPIIntegration = "jamfpro_api_integration"

func discoverAPIIntegrations(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetApiIntegrations(params)
	if err != nil {
		return nil, fmt.Errorf("listing API integrations: %w", err)
	}

	var resources []Resource
	for _, ai := range resp.Results {
		jamfID := strconv.Itoa(ai.ID)
		label := tracker.Label(tfResourceAPIIntegration, ai.DisplayName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAPIIntegration, label)

		reg.Register(tfResourceAPIIntegration, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   ai.DisplayName,
			Label:  label,
		})
	}

	return resources, nil
}
