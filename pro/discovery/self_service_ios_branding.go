// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceSelfServiceBrandingIOS = "jamfpro_self_service_branding_ios"

func discoverSelfServiceBrandingIOS(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetSelfServiceBrandingIOS(params)
	if err != nil {
		return nil, fmt.Errorf("listing self-service branding (iOS): %w", err)
	}

	var resources []Resource
	for _, b := range resp.Results {
		label := tracker.Label(tfResourceSelfServiceBrandingIOS, b.BrandingName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceSelfServiceBrandingIOS, label)

		reg.Register(tfResourceSelfServiceBrandingIOS, b.ID, tfAddr)

		resources = append(resources, Resource{
			JamfID: b.ID,
			Name:   b.BrandingName,
			Label:  label,
		})
	}

	return resources, nil
}
