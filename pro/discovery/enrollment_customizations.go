// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"net/url"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceEnrollmentCustomization = "jamfpro_enrollment_customization"

// EnrollmentCustomizationInfo holds metadata for downloading the branding image.
type EnrollmentCustomizationInfo struct {
	ID      string
	Name    string
	IconURL string
}

func discoverEnrollmentCustomizations(client *jamfpro.Client, reg *registry.Registry) ([]Resource, map[string]EnrollmentCustomizationInfo, error) {
	tracker := naming.NewTracker()

	params := url.Values{}
	params.Set("page-size", "200")

	resp, err := client.GetEnrollmentCustomizations(params)
	if err != nil {
		return nil, nil, fmt.Errorf("listing enrollment customizations: %w", err)
	}

	infoMap := make(map[string]EnrollmentCustomizationInfo)
	var resources []Resource
	for _, ec := range resp.Results {
		label := tracker.Label(tfResourceEnrollmentCustomization, ec.DisplayName)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceEnrollmentCustomization, label)

		reg.Register(tfResourceEnrollmentCustomization, ec.ID, tfAddr)

		if ec.BrandingSettings.IconUrl != "" {
			infoMap[ec.ID] = EnrollmentCustomizationInfo{
				ID:      ec.ID,
				Name:    ec.DisplayName,
				IconURL: ec.BrandingSettings.IconUrl,
			}
		}

		resources = append(resources, Resource{
			JamfID: ec.ID,
			Name:   ec.DisplayName,
			Label:  label,
		})
	}

	return resources, infoMap, nil
}
