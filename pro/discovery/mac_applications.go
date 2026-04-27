// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceMacApplication = "jamfpro_mac_application"

func discoverMacApplications(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetMacApplications()
	if err != nil {
		return nil, fmt.Errorf("listing mac applications: %w", err)
	}

	var resources []Resource
	for _, app := range resp.MacApplications {
		idStr := strconv.Itoa(app.ID)
		label := tracker.Label(tfResourceMacApplication, app.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceMacApplication, label)

		reg.Register(tfResourceMacApplication, idStr, tfAddr)

		resources = append(resources, Resource{
			JamfID: idStr,
			Name:   app.Name,
			Label:  label,
		})
	}

	return resources, nil
}
