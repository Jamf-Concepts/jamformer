// Copyright 2026, Jamf Software LLC

package multienv

import (
	"testing"

	"github.com/Jamf-Concepts/jamformer/pro/discovery"
	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestMatchResources(t *testing.T) {
	t.Run("all resources match across envs", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev": {
				EnvName:  "dev",
				Registry: registry.New(),
				Resources: &discovery.Results{
					Categories: []discovery.Resource{
						{JamfID: "1", Name: "Productivity", Label: "productivity"},
						{JamfID: "2", Name: "Security", Label: "security"},
					},
				},
			},
			"prod": {
				EnvName:  "prod",
				Registry: registry.New(),
				Resources: &discovery.Results{
					Categories: []discovery.Resource{
						{JamfID: "10", Name: "Productivity", Label: "productivity"},
						{JamfID: "20", Name: "Security", Label: "security"},
					},
				},
			},
		}

		matches := MatchResources(envResults, []string{"dev", "prod"})

		if len(matches) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(matches))
		}

		for _, m := range matches {
			if !m.AllEnvs {
				t.Errorf("resource %s.%s should be in all envs", m.ResourceType, m.Label)
			}
			if len(m.IDs) != 2 {
				t.Errorf("resource %s.%s should have 2 IDs, got %d", m.ResourceType, m.Label, len(m.IDs))
			}
			if m.IDs["dev"] == m.IDs["prod"] {
				t.Errorf("dev and prod IDs should differ for %s.%s", m.ResourceType, m.Label)
			}
		}
	})

	t.Run("partial match — resource in one env only", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev": {
				EnvName:  "dev",
				Registry: registry.New(),
				Resources: &discovery.Results{
					Scripts: []discovery.Resource{
						{JamfID: "1", Name: "Install Rosetta", Label: "install_rosetta"},
						{JamfID: "2", Name: "Dev Only Script", Label: "dev_only_script"},
					},
				},
			},
			"prod": {
				EnvName:  "prod",
				Registry: registry.New(),
				Resources: &discovery.Results{
					Scripts: []discovery.Resource{
						{JamfID: "10", Name: "Install Rosetta", Label: "install_rosetta"},
					},
				},
			},
		}

		matches := MatchResources(envResults, []string{"dev", "prod"})

		if len(matches) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(matches))
		}

		allEnvCount := 0
		partialCount := 0
		for _, m := range matches {
			if m.AllEnvs {
				allEnvCount++
			} else {
				partialCount++
				if len(m.Present) != 1 || m.Present[0] != "dev" {
					t.Errorf("partial resource should only be present in dev")
				}
			}
		}
		if allEnvCount != 1 || partialCount != 1 {
			t.Errorf("expected 1 all-env + 1 partial, got %d + %d", allEnvCount, partialCount)
		}
	})

	t.Run("empty envs produce no matches", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), Resources: &discovery.Results{}},
			"prod": {EnvName: "prod", Registry: registry.New(), Resources: &discovery.Results{}},
		}

		matches := MatchResources(envResults, []string{"dev", "prod"})
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("three environments", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev":     {EnvName: "dev", Registry: registry.New(), Resources: &discovery.Results{Categories: []discovery.Resource{{JamfID: "1", Name: "Test", Label: "test"}}}},
			"staging": {EnvName: "staging", Registry: registry.New(), Resources: &discovery.Results{Categories: []discovery.Resource{{JamfID: "2", Name: "Test", Label: "test"}}}},
			"prod":    {EnvName: "prod", Registry: registry.New(), Resources: &discovery.Results{Categories: []discovery.Resource{{JamfID: "3", Name: "Test", Label: "test"}}}},
		}

		matches := MatchResources(envResults, []string{"dev", "staging", "prod"})
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		if !matches[0].AllEnvs {
			t.Error("expected resource to be in all 3 envs")
		}
		if len(matches[0].IDs) != 3 {
			t.Errorf("expected 3 IDs, got %d", len(matches[0].IDs))
		}
	})
}
