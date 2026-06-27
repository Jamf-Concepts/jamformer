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

	// Numeric indicates the source attribute is number-typed (e.g. an Int64
	// profile_id) while the target attribute (TargetAttr) is a string. The
	// resolved reference is wrapped in tonumber() so the assignment type-checks.
	// Only honoured for single-attribute (non-list, non-element) rules.
	Numeric bool

	// IsList indicates the attribute is a list of IDs rather than a single ID.
	IsList bool

	// ElementAttr, when set, indicates AttrName is a list-of-objects attribute
	// (e.g. scripts = [ { id = "44" }, ... ]) and ElementAttr (e.g. "id") is the
	// field rewritten on each element.
	ElementAttr string

	// DiscriminatorAttr, when set (with ElementAttr), names a sibling field on
	// each list element whose value selects the target type via DiscriminatorMap.
	// Used for polymorphic references where one ID field points at different
	// resource types depending on a type tag (e.g. macOS onboarding items whose
	// entity_id targets a policy, profile, or app per self_service_entity_type).
	DiscriminatorAttr string

	// DiscriminatorMap maps a DiscriminatorAttr value to the TF resource type to
	// resolve the element's ID against. An unmapped value leaves the ID untouched.
	DiscriminatorMap map[string]string
}
