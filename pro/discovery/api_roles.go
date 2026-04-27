// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAPIRole = "jamfpro_api_role"

func discoverAPIRoles(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetJamfAPIRoles(params)
	if err != nil {
		return nil, fmt.Errorf("listing API roles: %w", err)
	}

	var resources []Resource
	for _, r := range resp.Results {
		label := tracker.Label(tfResourceAPIRole, r.DisplayName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAPIRole, label)

		reg.Register(tfResourceAPIRole, r.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: r.ID,
			Name:   r.DisplayName,
			Label:  label,
		})
	}

	return resources, nil
}
