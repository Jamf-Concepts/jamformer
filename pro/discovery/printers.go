// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourcePrinter = "jamfpro_printer"

func discoverPrinters(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetPrinters()
	if err != nil {
		return nil, fmt.Errorf("listing printers: %w", err)
	}

	var resources []Resource
	for _, p := range resp.Printer {
		jamfID := strconv.Itoa(p.ID)
		label := tracker.Label(tfResourcePrinter, p.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourcePrinter, label)

		reg.Register(tfResourcePrinter, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   p.Name,
			Label:  label,
		})
	}

	return resources, nil
}
