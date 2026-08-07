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

// generatedULFS mirrors what terraform query emits for unified logging filters,
// filter sets, and the plan that assigns them: auto-generated "all_N" labels,
// nested identity { id } import blocks, and raw UUIDs in the cross-resource
// attributes.
const generatedULFS = `
resource "jamfprotect_unified_logging_filter" "all_0" {
  name        = "Time Machine Activity"
  description = "Captures Time Machine backup activity"
  filter      = "subsystem == \"com.apple.TimeMachine\""
  enabled     = true
  tags        = ["backup"]
}

import {
  identity = {
    id = "ulf-aaa"
  }
  to = jamfprotect_unified_logging_filter.all_0
}

resource "jamfprotect_unified_logging_filter" "all_1" {
  name        = "Screen Sharing Sessions"
  description = "Captures screen sharing connections"
  filter      = "subsystem == \"com.apple.screensharing\""
  enabled     = true
  tags        = ["remote-access"]
}

import {
  identity = {
    id = "ulf-bbb"
  }
  to = jamfprotect_unified_logging_filter.all_1
}

resource "jamfprotect_unified_logging_filter_set" "all_0" {
  name        = "Endpoint Diagnostics"
  description = "Diagnostic log capture"
  filters     = ["ulf-aaa", "ulf-bbb"]
}

import {
  identity = {
    id = "ulfs-ccc"
  }
  to = jamfprotect_unified_logging_filter_set.all_0
}

resource "jamfprotect_plan" "all_0" {
  name                        = "Diagnostics Plan"
  unified_logging_filter_sets = ["ulfs-ccc"]
}

import {
  identity = {
    id = "plan-ddd"
  }
  to = jamfprotect_plan.all_0
}
`

// TestUnifiedLoggingFilterSetEndToEnd runs the post-query half of the Protect
// pipeline (label renaming → registry population → reference rewriting → file
// splitting) over realistic terraform query output, verifying that filter set
// membership and plan assignment resolve to cross-resource references rather
// than staying as literal UUIDs.
func TestUnifiedLoggingFilterSetEndToEnd(t *testing.T) {
	prevQuiet := postprocess.Quiet
	postprocess.Quiet = true
	t.Cleanup(func() { postprocess.Quiet = prevQuiet })

	dir := t.TempDir()
	genFile := filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(genFile, []byte(generatedULFS), 0644); err != nil {
		t.Fatalf("writing generated.tf: %v", err)
	}

	if err := RenameLabelsWithEvents(genFile, nil); err != nil {
		t.Fatalf("RenameLabelsWithEvents() error: %v", err)
	}

	reg := registry.New()
	if err := PopulateRegistryFromGenerated(genFile, reg); err != nil {
		t.Fatalf("PopulateRegistryFromGenerated() error: %v", err)
	}

	// Labels must be renamed before registry population, otherwise the filter
	// and the filter set both register under "all_0" and collide.
	if addr, ok := reg.Resolve("jamfprotect_unified_logging_filter", "ulf-aaa"); !ok {
		t.Fatal("filter ulf-aaa not registered")
	} else if addr != "jamfprotect_unified_logging_filter.time_machine_activity" {
		t.Errorf("unexpected filter address: %s", addr)
	}
	if addr, ok := reg.Resolve("jamfprotect_unified_logging_filter_set", "ulfs-ccc"); !ok {
		t.Fatal("filter set ulfs-ccc not registered")
	} else if addr != "jamfprotect_unified_logging_filter_set.endpoint_diagnostics" {
		t.Errorf("unexpected filter set address: %s", addr)
	}

	if err := postprocess.Process(dir, genFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap: TypeToFileMap(),
		Rules:         DefaultRules(),
	}); err != nil {
		t.Fatalf("postprocess.Process() error: %v", err)
	}

	readFile := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		return string(data)
	}

	// Filter set membership resolves to both member filters.
	setFile := readFile("unified_logging_filter_sets.tf")
	for _, want := range []string{
		"jamfprotect_unified_logging_filter.time_machine_activity.id",
		"jamfprotect_unified_logging_filter.screen_sharing_sessions.id",
	} {
		if !strings.Contains(setFile, want) {
			t.Errorf("unified_logging_filter_sets.tf missing %s, got:\n%s", want, setFile)
		}
	}
	if strings.Contains(setFile, `"ulf-aaa"`) || strings.Contains(setFile, `"ulf-bbb"`) {
		t.Errorf("filter set still holds literal filter UUIDs:\n%s", setFile)
	}

	// Plan assignment resolves to the filter set.
	planFile := readFile("plans.tf")
	if !strings.Contains(planFile, "jamfprotect_unified_logging_filter_set.endpoint_diagnostics.id") {
		t.Errorf("plans.tf missing filter set reference, got:\n%s", planFile)
	}
	if strings.Contains(planFile, `"ulfs-ccc"`) {
		t.Errorf("plan still holds literal filter set UUID:\n%s", planFile)
	}

	// Filters land in their own file, keeping the two types separate despite
	// the shared resource-type prefix.
	filterFile := readFile("unified_logging_filters.tf")
	if !strings.Contains(filterFile, `resource "jamfprotect_unified_logging_filter" "time_machine_activity"`) {
		t.Errorf("unified_logging_filters.tf missing renamed filter, got:\n%s", filterFile)
	}
	if strings.Contains(filterFile, `resource "jamfprotect_unified_logging_filter_set"`) {
		t.Errorf("filter set leaked into unified_logging_filters.tf:\n%s", filterFile)
	}
}
