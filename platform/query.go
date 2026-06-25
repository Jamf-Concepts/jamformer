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

// GenerateQueryFile writes a .tfquery.hcl file with list blocks for selected
// listable resource types. If selectedResources is nil, all listable resource
// types are included. List blocks are emitted in Resources-table order for
// deterministic output.
func GenerateQueryFile(outputDir string, selectedResources map[string]bool) error {
	var queryLines []string
	for _, r := range ListableResources() {
		if selectedResources != nil && !selectedResources[r.FilterKey] {
			continue
		}
		queryLines = append(queryLines, fmt.Sprintf(`list %q "all" {
  provider = jamfplatform
  limit    = 10000
}`, r.TFType))
	}

	if len(queryLines) > 0 {
		content := strings.Join(queryLines, "\n\n") + "\n"
		if err := os.WriteFile(filepath.Join(outputDir, "query.tfquery.hcl"), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing query file: %w", err)
		}
	}

	return nil
}
