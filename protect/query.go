// Copyright 2026, Jamf Software LLC

package protect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// Quiet suppresses progress messages.
var Quiet bool

// listableResourceTypes maps filter keys to Terraform resource type names.
var listableResourceTypes = map[string]string{
	"action_configurations":          "jamfprotect_action_configuration",
	"analytics":                      "jamfprotect_analytic",
	"analytics_managed":              "jamfprotect_analytic_managed",
	"analytic_sets":                  "jamfprotect_analytic_set",
	"api_clients":                    "jamfprotect_api_client",
	"custom_prevent_lists":           "jamfprotect_custom_prevent_list",
	"exception_sets":                 "jamfprotect_exception_set",
	"groups":                         "jamfprotect_group",
	"plans":                          "jamfprotect_plan",
	"removable_storage_control_sets": "jamfprotect_removable_storage_control_set",
	"roles":                          "jamfprotect_role",
	"telemetry":                      "jamfprotect_telemetry",
	"unified_logging_filters":        "jamfprotect_unified_logging_filter",
	"unified_logging_filter_sets":    "jamfprotect_unified_logging_filter_set",
	"users":                          "jamfprotect_user",
}

// singletonResources maps filter keys to their resource type and import ID.
var singletonResources = map[string]struct {
	ResourceType string
	ImportID     string
}{
	"change_management": {"jamfprotect_change_management", "change_management_singleton"},
	"data_forwarding":   {"jamfprotect_data_forwarding", "data_forwarding_singleton"},
	"data_retention":    {"jamfprotect_data_retention", "data_retention_singleton"},
}

// GenerateQueryFile writes a .tfquery.hcl file with list blocks for selected
// resource types. Singleton import blocks are written separately via
// WriteSingletonImports (called after terraform query completes, so the
// import blocks don't interfere with the query step).
// If selectedResources is nil, all resource types are included.
func GenerateQueryFile(outputDir string, selectedResources map[string]bool) error {
	var queryLines []string
	for filterKey, resourceType := range listableResourceTypes {
		if selectedResources != nil && !selectedResources[filterKey] {
			continue
		}
		queryLines = append(queryLines, fmt.Sprintf(`list %q "all" {
  provider = jamfprotect
  limit    = 10000
}`, resourceType))
	}

	if len(queryLines) > 0 {
		content := strings.Join(queryLines, "\n\n") + "\n"
		if err := os.WriteFile(filepath.Join(outputDir, "query.tfquery.hcl"), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing query file: %w", err)
		}
	}

	return nil
}

// WriteSingletonImports writes import blocks for singleton resources to
// singletons_import.tf. Must be called after terraform query completes.
// skipDataForwarding excludes data_forwarding when pre-discovery found no
// forwarding services enabled.
func WriteSingletonImports(outputDir string, selectedResources map[string]bool, skipDataForwarding bool) error {
	return writeSingletonImports(outputDir, selectedResources, skipDataForwarding)
}

// writeSingletonImports writes import blocks for singleton resources.
func writeSingletonImports(outputDir string, selectedResources map[string]bool, skipDataForwarding bool) error {
	f := hclwrite.NewEmptyFile()
	body := f.Body()
	count := 0

	for filterKey, singleton := range singletonResources {
		if selectedResources != nil && !selectedResources[filterKey] {
			continue
		}
		if skipDataForwarding && filterKey == "data_forwarding" {
			if !Quiet {
				fmt.Println("  Skipping unconfigured data_forwarding singleton")
			}
			continue
		}

		if count > 0 {
			body.AppendNewline()
		}

		block := body.AppendNewBlock("import", nil)
		blockBody := block.Body()

		toTokens := hclwrite.Tokens{
			{Type: 9, Bytes: fmt.Appendf(nil, "%s.singleton", singleton.ResourceType)}, // TokenIdent = 9
		}
		blockBody.SetAttributeRaw("to", toTokens)
		blockBody.SetAttributeValue("id", cty.StringVal(singleton.ImportID))
		count++
	}

	if count == 0 {
		return nil
	}

	return os.WriteFile(filepath.Join(outputDir, "singletons_import.tf"), f.Bytes(), 0644)
}
