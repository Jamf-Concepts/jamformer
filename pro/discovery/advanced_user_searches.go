// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAdvancedUserSearch = "jamfpro_advanced_user_search"

func discoverAdvancedUserSearches(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAdvancedUserSearches()
	if err != nil {
		return nil, fmt.Errorf("listing advanced user searches: %w", err)
	}

	var resources []Resource
	for _, s := range resp.AdvancedUserSearch {
		jamfID := strconv.Itoa(s.ID)
		label := tracker.Label(tfResourceAdvancedUserSearch, s.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAdvancedUserSearch, label)

		reg.Register(tfResourceAdvancedUserSearch, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
