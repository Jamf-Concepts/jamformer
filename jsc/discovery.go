// Copyright 2026, Jamf Software LLC

package jsc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jamf-Concepts/jamformer/naming"
)

// DiscoveredResource represents a resource discovered via data source.
type DiscoveredResource struct {
	ID    string
	Name  string
	Label string
}

// DiscoveryResults holds all discovered resources by type.
type DiscoveryResults struct {
	ActivationProfiles []DiscoveredResource
	EntraIdps          []DiscoveredResource
	HostnameMappings   []DiscoveredResource
	AccessPolicies     []DiscoveredResource
}

// generateDiscoveryConfig creates a Terraform config with data sources for discovery.
func generateDiscoveryConfig(outputDir string, selectedResources map[string]bool) error {
	var config string
	config = `# Auto-generated data sources for resource discovery
`

	for _, r := range DiscoverableResources() {
		if selectedResources != nil && !selectedResources[r.FilterKey] {
			continue
		}
		config += fmt.Sprintf(`
data "%s" "all" {}
`, r.DataSource)
	}

	return os.WriteFile(filepath.Join(outputDir, "discovery.tf"), []byte(config), 0644)
}

// parseDiscoveryState reads the terraform state and extracts discovered resources.
func parseDiscoveryState(outputDir string, selectedResources map[string]bool) (*DiscoveryResults, error) {
	stateFile := filepath.Join(outputDir, "terraform.tfstate")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state struct {
		Resources []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Mode      string `json:"mode"`
			Instances []struct {
				Attributes map[string]any `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	results := &DiscoveryResults{}
	tracker := naming.NewTracker()

	for _, res := range state.Resources {
		if res.Mode != "data" || len(res.Instances) == 0 {
			continue
		}

		attrs := res.Instances[0].Attributes

		switch res.Type {
		case "jsc_activation_profiles":
			if selectedResources != nil && !selectedResources["activation_profiles"] {
				continue
			}
			profiles, ok := attrs["profiles"].([]any)
			if !ok {
				continue
			}
			for _, p := range profiles {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				id, _ := pm["id"].(string)
				name, _ := pm["name"].(string)
				if id != "" {
					label := tracker.Label("jsc_ap", naming.Sanitize(name))
					results.ActivationProfiles = append(results.ActivationProfiles, DiscoveredResource{
						ID:    id,
						Name:  name,
						Label: label,
					})
				}
			}

		case "jsc_entra_idps":
			if selectedResources != nil && !selectedResources["entra_idps"] {
				continue
			}
			connections, ok := attrs["connections"].([]any)
			if !ok {
				continue
			}
			for _, c := range connections {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				id, _ := cm["id"].(string)
				name, _ := cm["name"].(string)
				if id != "" {
					label := tracker.Label("jsc_entra_idp", naming.Sanitize(name))
					results.EntraIdps = append(results.EntraIdps, DiscoveredResource{
						ID:    id,
						Name:  name,
						Label: label,
					})
				}
			}

		case "jsc_hostnamemappings":
			if selectedResources != nil && !selectedResources["hostname_mappings"] {
				continue
			}
			mappings, ok := attrs["mappings"].([]any)
			if !ok {
				continue
			}
			for _, m := range mappings {
				mm, ok := m.(map[string]any)
				if !ok {
					continue
				}
				hostname, _ := mm["hostname"].(string)
				if hostname != "" {
					label := tracker.Label("jsc_hostnamemapping", naming.Sanitize(hostname))
					results.HostnameMappings = append(results.HostnameMappings, DiscoveredResource{
						ID:    hostname, // hostname is the import ID
						Name:  hostname,
						Label: label,
					})
				}
			}

		case "jsc_access_policies":
			if selectedResources != nil && !selectedResources["access_policies"] {
				continue
			}
			policies, ok := attrs["policies"].([]any)
			if !ok {
				continue
			}
			for _, p := range policies {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				id, _ := pm["id"].(string)
				name, _ := pm["name"].(string)
				if id != "" {
					label := tracker.Label("jsc_access_policy", naming.Sanitize(name))
					results.AccessPolicies = append(results.AccessPolicies, DiscoveredResource{
						ID:    id,
						Name:  name,
						Label: label,
					})
				}
			}
		}
	}

	return results, nil
}

// cleanupDiscoveryFiles removes temporary discovery files.
func cleanupDiscoveryFiles(outputDir string) {
	_ = os.Remove(filepath.Join(outputDir, "discovery.tf"))
	_ = os.Remove(filepath.Join(outputDir, "terraform.tfstate"))
	_ = os.Remove(filepath.Join(outputDir, "terraform.tfstate.backup"))
}
