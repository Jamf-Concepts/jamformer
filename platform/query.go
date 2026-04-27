// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Quiet suppresses progress messages.
var Quiet bool

// listableResourceTypes maps filter keys to Terraform resource type names.
var listableResourceTypes = map[string]string{
	"blueprints":            "jamfplatform_blueprints_blueprint",
	"compliance_benchmarks": "jamfplatform_cbengine_benchmark",
	"device_groups":         "jamfplatform_device_group",
}

// GenerateQueryFile writes a .tfquery.hcl file with list blocks for selected
// resource types. If selectedResources is nil, all resource types are included.
func GenerateQueryFile(outputDir string, selectedResources map[string]bool) error {
	var queryLines []string
	for filterKey, resourceType := range listableResourceTypes {
		if selectedResources != nil && !selectedResources[filterKey] {
			continue
		}
		queryLines = append(queryLines, fmt.Sprintf(`list %q "all" {
  provider = jamfplatform
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
