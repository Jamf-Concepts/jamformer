// Copyright 2026, Jamf Software LLC

package platform

import "github.com/Jamf-Concepts/jamformer/postprocess"

// nameAttrForType returns the attribute name used to derive a friendly label
// for the given resource type.
func nameAttrForType(resourceType string) string {
	switch resourceType {
	case "jamfplatform_cbengine_benchmark":
		return "title"
	default:
		return "name"
	}
}

// RenameLabels rewrites auto-generated labels (like "all_0") in the generated
// HCL file to friendly names derived from resource attributes.
func RenameLabels(generatedFile string) error {
	return postprocess.RenameLabels(generatedFile, nameAttrForType)
}
