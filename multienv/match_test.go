// Copyright 2026, Jamf Software LLC

package multienv

import (
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestMatchResources(t *testing.T) {
	t.Run("all resources match across envs", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev": {
				EnvName:  "dev",
				Registry: registry.New(),
				Resources: []ResourceRef{
					{TFType: "jamfpro_category", JamfID: "1", Name: "Productivity", Label: "productivity"},
					{TFType: "jamfpro_category", JamfID: "2", Name: "Security", Label: "security"},
				},
			},
			"prod": {
				EnvName:  "prod",
				Registry: registry.New(),
				Resources: []ResourceRef{
					{TFType: "jamfpro_category", JamfID: "10", Name: "Productivity", Label: "productivity"},
					{TFType: "jamfpro_category", JamfID: "20", Name: "Security", Label: "security"},
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
				Resources: []ResourceRef{
					{TFType: "jamfpro_script", JamfID: "1", Name: "Install Rosetta", Label: "install_rosetta"},
					{TFType: "jamfpro_script", JamfID: "2", Name: "Dev Only Script", Label: "dev_only_script"},
				},
			},
			"prod": {
				EnvName:  "prod",
				Registry: registry.New(),
				Resources: []ResourceRef{
					{TFType: "jamfpro_script", JamfID: "10", Name: "Install Rosetta", Label: "install_rosetta"},
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
			"dev":  {EnvName: "dev", Registry: registry.New(), Resources: nil},
			"prod": {EnvName: "prod", Registry: registry.New(), Resources: nil},
		}

		matches := MatchResources(envResults, []string{"dev", "prod"})
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("three environments", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev":     {EnvName: "dev", Registry: registry.New(), Resources: []ResourceRef{{TFType: "jamfpro_category", JamfID: "1", Name: "Test", Label: "test"}}},
			"staging": {EnvName: "staging", Registry: registry.New(), Resources: []ResourceRef{{TFType: "jamfpro_category", JamfID: "2", Name: "Test", Label: "test"}}},
			"prod":    {EnvName: "prod", Registry: registry.New(), Resources: []ResourceRef{{TFType: "jamfpro_category", JamfID: "3", Name: "Test", Label: "test"}}},
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

	t.Run("empty JamfID still matches but records empty id", func(t *testing.T) {
		envResults := map[string]*PerEnvResult{
			"dev":  {EnvName: "dev", Registry: registry.New(), Resources: []ResourceRef{{TFType: "jamfplatform_pro_smtp_server_settings", JamfID: "", Name: "smtp", Label: "smtp"}}},
			"prod": {EnvName: "prod", Registry: registry.New(), Resources: []ResourceRef{{TFType: "jamfplatform_pro_smtp_server_settings", JamfID: "", Name: "smtp", Label: "smtp"}}},
		}

		matches := MatchResources(envResults, []string{"dev", "prod"})
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		if !matches[0].AllEnvs {
			t.Error("singleton present in both envs should be AllEnvs")
		}
		if matches[0].IDs["dev"] != "" {
			t.Errorf("expected empty id, got %q", matches[0].IDs["dev"])
		}
	})
}
