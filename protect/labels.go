// Copyright 2026, Jamf Software LLC

package protect

import "github.com/Jamf-Concepts/jamformer/postprocess"

// nameAttrForType returns the attribute name used to derive a friendly label
// for the given resource type. Most resources use "name"; users use "email".
func nameAttrForType(resourceType string) string {
	switch resourceType {
	case "jamfprotect_user":
		return "email"
	default:
		return "name"
	}
}

// RenameLabels rewrites auto-generated labels (like "all_0") in the generated
// HCL file to friendly names derived from resource attributes.
func RenameLabels(generatedFile string) error {
	return postprocess.RenameLabels(generatedFile, nameAttrForType)
}
