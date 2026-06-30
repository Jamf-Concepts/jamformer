// Copyright 2026, Jamf Software LLC

package platform

import (
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// nameAttrForType returns the top-level attribute name used to derive a friendly
// label for the given resource type. Many federated pro types name themselves
// with something other than "name" (display_name, title, username, …); those
// that nest their name (policy/profile general.name, ldap_server
// connection_settings.display_name) are handled in platformLabelName.
func nameAttrForType(resourceType string) string {
	switch resourceType {
	case "jamfplatform_cbengine_benchmark",
		"jamfplatform_pro_app_request_form_field":
		return "title"
	case "jamfplatform_pro_package",
		"jamfplatform_pro_api_role",
		"jamfplatform_pro_api_client",
		"jamfplatform_pro_account_group",
		"jamfplatform_pro_supervision_identity",
		"jamfplatform_pro_enrollment_customization",
		"jamfplatform_pro_computer_prestage_enrollment",
		"jamfplatform_pro_mobile_device_prestage_enrollment":
		return "display_name"
	case "jamfplatform_pro_account":
		return "username"
	case "jamfplatform_pro_allowed_file_extension":
		return "extension"
	case "jamfplatform_pro_removable_mac_address":
		return "mac_address"
	default:
		return "name"
	}
}

// platformLabelName composes the pre-sanitization friendly name for a resource
// block. The federated jamfplatform_pro_* "objecty" types (policy, profiles,
// apps, ebooks, …) carry their display name at nested general.name rather than a
// top-level attribute, so general.name is consulted first and the top-level
// attribute is the fallback for flat types (account, category, building, …).
// For jamfplatform_device_group it folds the device_type into the name so a
// computer group and a mobile group that share a name produce distinct labels
// (e.g. "All Staff" -> all_staff_computer / all_staff_mobile).
func platformLabelName(resourceType string, body *hclwrite.Body, _ func() string) string {
	attrName := nameAttrForType(resourceType)
	var name string
	if attrName == "name" {
		name = postprocess.ReadObjectAttrString(body, []string{"general"}, "name")
	}
	// ldap_server nests its display name under connection_settings.
	if name == "" && resourceType == "jamfplatform_pro_ldap_server" {
		name = postprocess.ReadObjectAttrString(body, []string{"connection_settings"}, "display_name")
	}
	if name == "" {
		if attr := body.GetAttribute(attrName); attr != nil {
			name = postprocess.ExtractStringValue(attr)
		}
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
