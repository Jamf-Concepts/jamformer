// Copyright 2026, Jamf Software LLC

package postprocess

// ReferenceRule defines how to resolve a literal ID in a generated resource
// to a cross-resource Terraform reference.
type ReferenceRule struct {
	// ResourceType is the TF resource type containing the reference (e.g. "jamfpro_policy")
	ResourceType string

	// BlockPath is the HCL block nesting path to the attribute.
	// e.g. ["payloads", "scripts"] means the "id" attr lives inside payloads { scripts { id = ... } }
	BlockPath []string

	// AttrName is the attribute within the block that holds the raw ID (e.g. "id")
	AttrName string

	// TargetTypes are the TF resource types to look up in the registry.
	// Tried in order; first match wins.
	TargetTypes []string

	// TargetAttr is the attribute on the target resource to reference (e.g. "id")
	TargetAttr string

	// IsList indicates the attribute is a list of IDs rather than a single ID.
	IsList bool
}
