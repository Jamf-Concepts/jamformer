// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceDockItem = "jamfpro_dock_item"

func discoverDockItems(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetDockItems()
	if err != nil {
		return nil, fmt.Errorf("listing dock items: %w", err)
	}

	var resources []Resource
	for _, d := range resp.DockItems {
		jamfID := strconv.Itoa(d.ID)
		label := tracker.Label(tfResourceDockItem, d.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceDockItem, label)

		reg.Register(tfResourceDockItem, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   d.Name,
			Label:  label,
		})
	}

	return resources, nil
}
