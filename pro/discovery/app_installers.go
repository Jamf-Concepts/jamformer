// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAppInstaller = "jamfpro_app_installer"

func discoverAppInstallers(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	// List all deployments via the deployments endpoint
	var deployments struct {
		Results []jamfpro.ResourceJamfAppCatalogDeployment `json:"results"`
	}

	resp, err := client.HTTP.DoRequest("GET", "/api/v1/app-installers/deployments", nil, &deployments)
	if err != nil {
		return nil, fmt.Errorf("listing app installer deployments: %w", err)
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	var resources []Resource
	for _, d := range deployments.Results {
		label := tracker.Label(tfResourceAppInstaller, d.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAppInstaller, label)

		reg.Register(tfResourceAppInstaller, d.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: d.ID,
			Name:   d.Name,
			Label:  label,
		})
	}

	return resources, nil
}
