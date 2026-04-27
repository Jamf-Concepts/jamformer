// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourcePackage = "jamfpro_package"

// PackageInfo holds extra metadata about a package needed for post-processing.
type PackageInfo struct {
	PackageName string
	FileName    string
}

func discoverPackages(client *jamfpro.Client, reg *registry.Registry, packageInfo map[string]string) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetPackages("", "")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	var resources []Resource
	for _, p := range resp.Results {
		label := tracker.Label(tfResourcePackage, p.PackageName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourcePackage, label)

		reg.Register(tfResourcePackage, p.ID, tfAddr)

		// Store package_name -> filename mapping for post-processing
		if p.FileName != "" {
			packageInfo[p.PackageName] = p.FileName
		}

		resources = append(resources, Resource{
			JamfID: p.ID,
			Name:   p.PackageName,
			Label:  label,
		})
	}

	return resources, nil
}
