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

// RenameLabelsWithEvents is like RenameLabels but uses display_name values
// captured from the terraform query JSON event stream as a fallback for
// resource types whose name attribute is not present in the generated HCL
// (e.g. jamfprotect_analytic_managed).
func RenameLabelsWithEvents(generatedFile string, idToName map[string]map[string]string) error {
	return postprocess.RenameLabelsWithFallback(generatedFile, nameAttrForType, idToName)
}
