// Copyright 2026, Jamf Software LLC

package platform

import "github.com/Jamf-Concepts/jamformer/postprocess"

// ResourceDef describes a single Jamf Platform resource type.
type ResourceDef struct {
	FilterKey   string // key for -include-resources / -exclude-resources
	DisplayName string // human-readable name for prompts and output
	TFType      string // Terraform resource type name
	OutputFile  string // filename in the output directory
}

// Resources is the ordered list of all supported Jamf Platform resource types.
// Order matches the interactive selection prompt order.
var Resources = []ResourceDef{
	{FilterKey: "blueprints", DisplayName: "Blueprints", TFType: "jamfplatform_blueprints_blueprint", OutputFile: "blueprints.tf"},
	{FilterKey: "compliance_benchmarks", DisplayName: "Compliance Benchmarks", TFType: "jamfplatform_cbengine_benchmark", OutputFile: "compliance_benchmarks.tf"},
	{FilterKey: "device_groups", DisplayName: "Device Groups", TFType: "jamfplatform_device_group", OutputFile: "device_groups.tf"},
}

// TypeToFileMap returns a map of TF resource type → output filename,
// derived from the Resources table.
func TypeToFileMap() map[string]string {
	m := make(map[string]string, len(Resources))
	for _, r := range Resources {
		m[r.TFType] = r.OutputFile
	}
	return m
}

// ValidFilterNames returns a map of user-friendly filter names → canonical keys.
func ValidFilterNames() map[string]string {
	m := make(map[string]string, len(Resources))
	for _, r := range Resources {
		m[r.FilterKey] = r.FilterKey
	}
	return m
}

// DefaultRules returns the reference rules for Jamf Platform resource types.
func DefaultRules() []postprocess.ReferenceRule {
	return []postprocess.ReferenceRule{
		// Blueprint -> Device Group references
		{
			ResourceType: "jamfplatform_blueprints_blueprint",
			AttrName:     "device_groups",
			TargetTypes:  []string{"jamfplatform_device_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Compliance Benchmark -> Device Group reference
		{
			ResourceType: "jamfplatform_cbengine_benchmark",
			AttrName:     "target_device_group",
			TargetTypes:  []string{"jamfplatform_device_group"},
			TargetAttr:   "id",
		},
	}
}
