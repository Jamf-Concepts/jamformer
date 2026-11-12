// Copyright 2026, Jamf Software LLC

package protect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
)

// generatedPLS mirrors terraform query output for custom prevent lists, the
// prevent list set that groups them, and the plan that assigns the set.
const generatedPLS = `
resource "jamfprotect_custom_prevent_list" "all_0" {
  name = "Known Good Team IDs"
  prevent_type = "Team ID"
  list_data    = ["EQHXZ8M8AV"]
}

import {
  identity = {
    id = "cpl-aaa"
  }
  to = jamfprotect_custom_prevent_list.all_0
}

resource "jamfprotect_prevent_list_set" "all_0" {
  name          = "Endpoint Baseline"
  description   = "Baseline prevent lists"
  prevent_lists = ["cpl-aaa"]
}

import {
  identity = {
    id = "pls-bbb"
  }
  to = jamfprotect_prevent_list_set.all_0
}

resource "jamfprotect_plan" "all_0" {
  name              = "Baseline Plan"
  prevent_list_sets = ["pls-bbb"]
}

import {
  identity = {
    id = "plan-ccc"
  }
  to = jamfprotect_plan.all_0
}
`

// TestPreventListSetEndToEnd runs the post-query pipeline over prevent list set
// output: set membership and plan assignment must become references, and the
// set must land in its own file.
func TestPreventListSetEndToEnd(t *testing.T) {
	prevQuiet := postprocess.Quiet
	postprocess.Quiet = true
	t.Cleanup(func() { postprocess.Quiet = prevQuiet })

	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generatedPLS), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RenameLabelsWithEvents(genFile, nil); err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	if err := PopulateRegistryFromGenerated(genFile, reg); err != nil {
		t.Fatal(err)
	}
	if addr, ok := reg.Resolve("jamfprotect_prevent_list_set", "pls-bbb"); !ok || addr != "jamfprotect_prevent_list_set.endpoint_baseline" {
		t.Fatalf("prevent list set not registered correctly: %q %v", addr, ok)
	}
	if err := postprocess.Process(dir, genFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap: TypeToFileMap(),
		Rules:         DefaultRules(),
	}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		return string(b)
	}
	if set := read("prevent_list_sets.tf"); !strings.Contains(set, "jamfprotect_custom_prevent_list.known_good_team_ids.id") || strings.Contains(set, `"cpl-aaa"`) {
		t.Errorf("prevent_list_sets.tf membership not rewritten:\n%s", set)
	}
	if plan := read("plans.tf"); !strings.Contains(plan, "jamfprotect_prevent_list_set.endpoint_baseline.id") || strings.Contains(plan, `"pls-bbb"`) {
		t.Errorf("plans.tf assignment not rewritten:\n%s", plan)
	}
	if lists := read("custom_prevent_lists.tf"); strings.Contains(lists, `resource "jamfprotect_prevent_list_set"`) {
		t.Errorf("prevent list set leaked into custom_prevent_lists.tf:\n%s", lists)
	}
}
