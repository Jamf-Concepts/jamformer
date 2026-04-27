// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceDepartment = "jamfpro_department"

func discoverDepartments(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetDepartments(params)
	if err != nil {
		return nil, fmt.Errorf("listing departments: %w", err)
	}

	var resources []Resource
	for _, d := range resp.Results {
		label := tracker.Label(tfResourceDepartment, d.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceDepartment, label)

		reg.Register(tfResourceDepartment, d.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: d.ID,
			Name:   d.Name,
			Label:  label,
		})
	}

	return resources, nil
}
