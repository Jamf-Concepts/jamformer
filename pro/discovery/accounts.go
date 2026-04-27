// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAccount = "jamfpro_account"

func discoverAccounts(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAccounts()
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}

	var resources []Resource
	for _, u := range resp.Users {
		jamfID := strconv.Itoa(u.ID)
		label := tracker.Label(tfResourceAccount, u.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAccount, label)

		reg.Register(tfResourceAccount, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   u.Name,
			Label:  label,
		})
	}

	return resources, nil
}
