// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceScript = "jamfpro_script"

func discoverScripts(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetScripts(params)
	if err != nil {
		return nil, fmt.Errorf("listing scripts: %w", err)
	}

	var resources []Resource
	for _, s := range resp.Results {
		label := tracker.Label(tfResourceScript, naming.StripScriptExtension(s.Name))
		tfAddr := fmt.Sprintf("%s.%s", tfResourceScript, label)

		reg.Register(tfResourceScript, s.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: s.ID,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
