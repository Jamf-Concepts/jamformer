// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceDiskEncryptionConfiguration = "jamfpro_disk_encryption_configuration"

func discoverDiskEncryptionConfigurations(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetDiskEncryptionConfigurations()
	if err != nil {
		return nil, fmt.Errorf("listing disk encryption configurations: %w", err)
	}

	var resources []Resource
	for _, dec := range resp.DiskEncryptionConfiguration {
		jamfID := strconv.Itoa(dec.ID)
		label := tracker.Label(tfResourceDiskEncryptionConfiguration, dec.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceDiskEncryptionConfiguration, label)

		reg.Register(tfResourceDiskEncryptionConfiguration, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   dec.Name,
			Label:  label,
		})
	}

	return resources, nil
}
