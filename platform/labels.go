// Copyright 2026, Jamf Software LLC

package platform

import (
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

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

// platformLabelName composes the pre-sanitization friendly name for a resource
// block. For jamfplatform_device_group it folds the device_type into the name so
// a computer group and a mobile group that share a name produce distinct labels
// (e.g. "All Staff" -> all_staff_computer / all_staff_mobile).
func platformLabelName(resourceType string, body *hclwrite.Body, _ func() string) string {
	var name string
	if attr := body.GetAttribute(nameAttrForType(resourceType)); attr != nil {
		name = postprocess.ExtractStringValue(attr)
	}
	if name == "" {
		return ""
	}
	if resourceType == "jamfplatform_device_group" {
		if dt := body.GetAttribute("device_type"); dt != nil {
			if v := postprocess.ExtractStringValue(dt); v != "" {
				// Sanitize converts the space to "_", yielding e.g. all_staff_computer.
				name = name + " " + v
			}
		}
	}
	return name
}

// RenameLabels rewrites auto-generated labels (like "all_0") in the generated
// HCL file to friendly names derived from resource attributes, folding
// device_type into jamfplatform_device_group labels to avoid computer/mobile
// collisions.
func RenameLabels(generatedFile string) error {
	return postprocess.RenameLabelsWithComposer(generatedFile, platformLabelName)
}
