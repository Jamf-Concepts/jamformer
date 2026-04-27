// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// formatResource parses HCL, formats the first resource block, and returns the result.
func formatResource(t *testing.T, src string) string {
	t.Helper()
	f, diags := hclwrite.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	outFile := hclwrite.NewEmptyFile()
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			newBlock := outFile.Body().AppendNewBlock(block.Type(), block.Labels())
			formatBody(newBlock.Body(), block.Body())
		}
	}
	return string(outFile.Bytes())
}

func TestFormatAlphabeticalAttributes(t *testing.T) {
	src := `
resource "jamfpro_script" "test" {
  script_contents = "#!/bin/bash"
  name            = "My Script"
  category_id     = "5"
}
`
	result := formatResource(t, src)

	// Attributes should be in alphabetical order: category_id, name, script_contents
	catIdx := strings.Index(result, "category_id")
	nameIdx := strings.Index(result, "name")
	scriptIdx := strings.Index(result, "script_contents")

	if catIdx == -1 || nameIdx == -1 || scriptIdx == -1 {
		t.Fatalf("missing attributes in output:\n%s", result)
	}

	if catIdx >= nameIdx || nameIdx >= scriptIdx {
		t.Errorf("expected alphabetical order (category_id < name < script_contents), got:\n%s", result)
	}
}

func TestFormatMetaArgumentsFirst(t *testing.T) {
	src := `
resource "jamfpro_script" "test" {
  name     = "My Script"
  for_each = var.scripts
  category_id = "5"
}
`
	result := formatResource(t, src)

	forEachIdx := strings.Index(result, "for_each")
	catIdx := strings.Index(result, "category_id")
	nameIdx := strings.Index(result, "name")

	if forEachIdx == -1 || catIdx == -1 || nameIdx == -1 {
		t.Fatalf("missing attributes in output:\n%s", result)
	}

	if forEachIdx >= catIdx || forEachIdx >= nameIdx {
		t.Errorf("expected for_each before regular attributes, got:\n%s", result)
	}
}

func TestFormatMetaArgBlankLine(t *testing.T) {
	src := `
resource "jamfpro_script" "test" {
  name     = "My Script"
  for_each = var.scripts
}
`
	result := formatResource(t, src)

	// There should be a blank line between for_each and name
	lines := strings.Split(result, "\n")
	forEachLine := -1
	nameLine := -1
	for i, line := range lines {
		if strings.Contains(line, "for_each") {
			forEachLine = i
		}
		if strings.Contains(line, "name") {
			nameLine = i
		}
	}

	if forEachLine == -1 || nameLine == -1 {
		t.Fatalf("missing attributes in output:\n%s", result)
	}

	// Expect at least one blank line between them
	if nameLine-forEachLine < 2 {
		t.Errorf("expected blank line between for_each and regular attributes, got:\n%s", result)
	}
}

func TestFormatBlankLineBetweenAttrsAndBlocks(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    all_computers = true
  }
}
`
	result := formatResource(t, src)

	lines := strings.Split(result, "\n")
	nameLine := -1
	scopeLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name") {
			nameLine = i
		}
		if trimmed == "scope {" {
			scopeLine = i
		}
	}

	if nameLine == -1 || scopeLine == -1 {
		t.Fatalf("missing name or scope in output:\n%s", result)
	}

	if scopeLine-nameLine < 2 {
		t.Errorf("expected blank line between attributes and nested blocks, got:\n%s", result)
	}
}

func TestFormatNestedBlocksSorted(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    all_computers = true
  }
  payloads {
    scripts {
      id = "1"
    }
  }
}
`
	result := formatResource(t, src)

	// payloads should come before scope alphabetically
	payloadsIdx := strings.Index(result, "payloads")
	scopeIdx := strings.Index(result, "scope")

	if payloadsIdx == -1 || scopeIdx == -1 {
		t.Fatalf("missing blocks in output:\n%s", result)
	}

	if payloadsIdx > scopeIdx {
		t.Errorf("expected payloads before scope (alphabetical), got:\n%s", result)
	}
}

func TestFormatDependsOnLast(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  depends_on = [jamfpro_script.test]
  name       = "test"
  category_id = "5"
}
`
	result := formatResource(t, src)

	catIdx := strings.Index(result, "category_id")
	nameIdx := strings.Index(result, "name")
	depsIdx := strings.Index(result, "depends_on")

	if catIdx == -1 || nameIdx == -1 || depsIdx == -1 {
		t.Fatalf("missing attributes in output:\n%s", result)
	}

	if catIdx >= nameIdx || nameIdx >= depsIdx {
		t.Errorf("expected depends_on last, got:\n%s", result)
	}
}

func TestFormatLifecycleBlockLast(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  lifecycle {
    ignore_changes = [name]
  }
  scope {
    all_computers = true
  }
  name = "test"
}
`
	result := formatResource(t, src)

	nameIdx := strings.Index(result, "name")
	scopeIdx := strings.Index(result, "scope")
	lifecycleIdx := strings.Index(result, "lifecycle")

	if nameIdx == -1 || scopeIdx == -1 || lifecycleIdx == -1 {
		t.Fatalf("missing elements in output:\n%s", result)
	}

	if nameIdx >= scopeIdx || scopeIdx >= lifecycleIdx {
		t.Errorf("expected order: name < scope < lifecycle, got:\n%s", result)
	}
}

func TestFormatRecursiveNesting(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  name = "test"
  scope {
    exclusions {
      network_segment_ids = ["8"]
      computer_group_ids  = ["51"]
    }
  }
}
`
	result := formatResource(t, src)

	// Inside exclusions, computer_group_ids should come before network_segment_ids
	cgIdx := strings.Index(result, "computer_group_ids")
	nsIdx := strings.Index(result, "network_segment_ids")

	if cgIdx == -1 || nsIdx == -1 {
		t.Fatalf("missing attributes in output:\n%s", result)
	}

	if cgIdx > nsIdx {
		t.Errorf("expected computer_group_ids before network_segment_ids in nested block, got:\n%s", result)
	}
}

func TestFormatSameTypeBlocksPreserveOrder(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  name = "test"
  payloads {
    scripts {
      id = "first"
    }
    scripts {
      id = "second"
    }
  }
}
`
	result := formatResource(t, src)

	firstIdx := strings.Index(result, "first")
	secondIdx := strings.Index(result, "second")

	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("missing script blocks in output:\n%s", result)
	}

	if firstIdx > secondIdx {
		t.Errorf("expected same-type blocks to preserve order, got:\n%s", result)
	}
}

func TestFormatCompleteOrdering(t *testing.T) {
	src := `
resource "jamfpro_policy" "test" {
  depends_on = [jamfpro_script.test]
  scope {
    all_computers = true
  }
  name = "test"
  for_each = var.policies
  lifecycle {
    ignore_changes = [name]
  }
  category_id = "5"
  payloads {
    scripts {
      id = "1"
    }
  }
}
`
	result := formatResource(t, src)

	// Expected order: for_each, [blank], category_id, name, [blank], payloads, scope, [blank], lifecycle, [blank], depends_on
	positions := map[string]int{
		"for_each":    strings.Index(result, "for_each"),
		"category_id": strings.Index(result, "category_id"),
		"name":        strings.Index(result, `name`),
		"payloads":    strings.Index(result, "payloads"),
		"scope":       strings.Index(result, "scope"),
		"lifecycle":   strings.Index(result, "lifecycle"),
		"depends_on":  strings.Index(result, "depends_on"),
	}

	for k, v := range positions {
		if v == -1 {
			t.Fatalf("missing %q in output:\n%s", k, result)
		}
	}

	checks := []struct {
		before, after string
	}{
		{"for_each", "category_id"},
		{"for_each", "name"},
		{"category_id", "name"},
		{"name", "payloads"},
		{"payloads", "scope"},
		{"scope", "lifecycle"},
		{"lifecycle", "depends_on"},
	}

	for _, c := range checks {
		if positions[c.before] >= positions[c.after] {
			t.Errorf("expected %s before %s, got:\n%s", c.before, c.after, result)
		}
	}
}
