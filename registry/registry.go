// Copyright 2026, Jamf Software LLC

package registry

import (
	"fmt"
	"sync"
)

// Registry tracks the mapping of Jamf resource type + ID to Terraform resource addresses.
// This enables resolving raw Jamf IDs in generated HCL to proper cross-resource references.
type Registry struct {
	mu   sync.RWMutex
	refs map[string]map[string]string // map[resourceType]map[jamfID]terraformAddress
}

func New() *Registry {
	return &Registry{
		refs: make(map[string]map[string]string),
	}
}

// Register records a mapping from a Jamf resource type + ID to a Terraform resource address.
// Example: Register("jamfpro_script", "42", "jamfpro_script.disable_bluetooth")
func (r *Registry) Register(tfResourceType, jamfID, tfAddress string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.refs[tfResourceType] == nil {
		r.refs[tfResourceType] = make(map[string]string)
	}
	r.refs[tfResourceType][jamfID] = tfAddress
}

// Unregister removes a mapping for a Jamf resource type + ID. After removal,
// Resolve returns false for it, so any reference to the (now absent) resource is
// left as a raw ID rather than rewritten to a dangling Terraform address. Used
// when post-processing strips resources (e.g. compliance-benchmark artifacts).
func (r *Registry) Unregister(tfResourceType, jamfID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if typeMap, ok := r.refs[tfResourceType]; ok {
		delete(typeMap, jamfID)
	}
}

// Resolve looks up a Terraform resource address for a given Jamf resource type + ID.
// Returns the address and true if found, or empty string and false if not.
func (r *Registry) Resolve(tfResourceType, jamfID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if typeMap, ok := r.refs[tfResourceType]; ok {
		if addr, ok := typeMap[jamfID]; ok {
			return addr, true
		}
	}
	return "", false
}

// ResolveAny tries to resolve a Jamf ID across multiple resource types.
// Returns the first match found. Useful for scope fields that can reference
// either smart or static groups.
func (r *Registry) ResolveAny(jamfID string, tfResourceTypes ...string) (string, bool) {
	for _, rt := range tfResourceTypes {
		if addr, ok := r.Resolve(rt, jamfID); ok {
			return addr, true
		}
	}
	return "", false
}

// HasType reports whether any resources of the given type have been registered.
func (r *Registry) HasType(tfResourceType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.refs[tfResourceType]) > 0
}

// AttrReference returns a full Terraform attribute reference string.
// Example: AttrReference("jamfpro_script", "42", "id") -> "jamfpro_script.disable_bluetooth.id"
func (r *Registry) AttrReference(tfResourceType, jamfID, attr string) (string, bool) {
	addr, ok := r.Resolve(tfResourceType, jamfID)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s.%s", addr, attr), true
}
