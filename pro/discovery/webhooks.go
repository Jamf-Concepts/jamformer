// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceWebhook = "jamfpro_webhook"

func discoverWebhooks(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetWebhooks()
	if err != nil {
		return nil, fmt.Errorf("listing webhooks: %w", err)
	}

	var resources []Resource
	for _, w := range resp.Webhooks {
		jamfID := strconv.Itoa(w.ID)
		label := tracker.Label(tfResourceWebhook, w.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceWebhook, label)

		reg.Register(tfResourceWebhook, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   w.Name,
			Label:  label,
		})
	}

	return resources, nil
}
