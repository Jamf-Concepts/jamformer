// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceAllowedFileExtension = "jamfpro_allowed_file_extension"

func discoverAllowedFileExtensions(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetAllowedFileExtensions()
	if err != nil {
		return nil, fmt.Errorf("listing allowed file extensions: %w", err)
	}

	var resources []Resource
	for _, ext := range resp.AllowedFileExtensions {
		jamfID := strconv.Itoa(ext.ID)
		label := tracker.Label(tfResourceAllowedFileExtension, ext.Extension)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceAllowedFileExtension, label)

		reg.Register(tfResourceAllowedFileExtension, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   ext.Extension,
			Label:  label,
		})
	}

	return resources, nil
}
