// Copyright 2026, Jamf Software LLC

package postprocess

// ReferenceRule defines how to resolve a literal ID in a generated resource
// to a cross-resource Terraform reference.
//
// Two traversal mechanisms are supported and may be combined (BlockPath is
// walked first, then AttrPath):
//   - BlockPath descends through HCL nested *blocks* (e.g. the jamfpro provider's
//     scope { ... } / payloads { scripts { ... } }), navigating live hclwrite blocks.
//   - AttrPath descends through plugin-framework nested *attributes* (object
//     expressions, e.g. the jamfplatform provider's scope = { targets = { ... } }),
//     re-parsing object-expression bytes at each level.
type ReferenceRule struct {
	// ResourceType is the TF resource type containing the reference (e.g. "jamfpro_policy")
	ResourceType string

	// BlockPath is the HCL block nesting path to the attribute.
	// e.g. ["payloads", "scripts"] means the "id" attr lives inside payloads { scripts { id = ... } }
	BlockPath []string

	// AttrPath is the nested object-attribute path to the container holding the
	// target attribute, e.g. ["scope", "targets"] for scope = { targets = { computer_group_ids = ... } }.
	// Walked after BlockPath. Empty for root-level attributes.
	AttrPath []string

	// AttrName is the attribute within the container that holds the raw ID(s)
	// (e.g. "id", "computer_group_ids"). When ElementAttr is set, AttrName names
	// the list-of-objects attribute and ElementAttr names the per-element field.
	AttrName string

	// TargetTypes are the TF resource types to look up in the registry.
	// Tried in order; first match wins.
	TargetTypes []string

	// TargetAttr is the attribute on the target resource to reference (e.g. "id")
	TargetAttr string

	// IsList indicates the attribute is a list of IDs rather than a single ID.
	IsList bool

	// ElementAttr, when set, indicates AttrName is a list-of-objects attribute
	// (e.g. scripts = [ { id = "44" }, ... ]) and ElementAttr (e.g. "id") is the
	// field rewritten on each element.
	ElementAttr string
}
