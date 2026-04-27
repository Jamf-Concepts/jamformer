// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceLDAPServer = "jamfpro_ldap_server"

func discoverLDAPServers(client *jamfpro.Client, reg *registry.Registry) ([]Resource, error) {
	tracker := naming.NewTracker()

	resp, err := client.GetLDAPServers()
	if err != nil {
		return nil, fmt.Errorf("listing LDAP servers: %w", err)
	}

	var resources []Resource
	for _, s := range resp.LDAPServers {
		jamfID := strconv.Itoa(s.ID)
		label := tracker.Label(tfResourceLDAPServer, s.Name)
		tfAddr := fmt.Sprintf("%s.%s", tfResourceLDAPServer, label)

		reg.Register(tfResourceLDAPServer, jamfID, tfAddr)

		resources = append(resources, Resource{
			JamfID: jamfID,
			Name:   s.Name,
			Label:  label,
		})
	}

	return resources, nil
}
