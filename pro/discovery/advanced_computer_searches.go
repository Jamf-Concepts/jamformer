// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAdvancedComputerSearch = "jamfpro_advanced_computer_search"

func discoverAdvancedComputerSearches(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAdvancedComputerSearches()
	if err != nil {
		return nil, fmt.Errorf("listing advanced computer searches: %w", err)
	}

	var resources []Resource
	for _, s := range resp.AdvancedComputerSearches {
		idStr := strconv.Itoa(s.ID)
		label := tracker.Label(tfResourceAdvancedComputerSearch, s.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAdvancedComputerSearch, label)

		reg.Register(tfResourceAdvancedComputerSearch, idStr, tfAddr)

		resources = append(resources, Resource{
			JamfID: idStr,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
