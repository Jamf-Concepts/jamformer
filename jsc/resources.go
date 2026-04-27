// Copyright 2026, Jamf Software LLC

package jsc

import "github.com/Jamf-Concepts/jamformer/postprocess"

// ResourceDef describes a single JSC resource type.
type ResourceDef struct {
	FilterKey      string // key for -include-resources / -exclude-resources
	DisplayName    string // human-readable name for prompts and output
	TFType         string // Terraform resource type name
	OutputFile     string // filename in the output directory
	DataSource     string // data source type for discovery (empty for singletons)
	DataSourceAttr string // attribute in data source containing the list
	IsSingleton    bool   // true if this is a singleton resource
	SingletonID    string // import ID for singletons
}

// Resources is the ordered list of all supported JSC resource types.
var Resources = []ResourceDef{
	{
		FilterKey:      "activation_profiles",
		DisplayName:    "Activation Profiles",
		TFType:         "jsc_ap",
		OutputFile:     "activation_profiles.tf",
		DataSource:     "jsc_activation_profiles",
		DataSourceAttr: "profiles",
	},
	{
		FilterKey:      "entra_idps",
		DisplayName:    "Entra IdP Connections",
		TFType:         "jsc_entra_idp",
		OutputFile:     "entra_idps.tf",
		DataSource:     "jsc_entra_idps",
		DataSourceAttr: "connections",
	},
	{
		FilterKey:      "hostname_mappings",
		DisplayName:    "Hostname Mappings",
		TFType:         "jsc_hostnamemapping",
		OutputFile:     "hostname_mappings.tf",
		DataSource:     "jsc_hostnamemappings",
		DataSourceAttr: "mappings",
	},
	{
		FilterKey:      "access_policies",
		DisplayName:    "Access Policies",
		TFType:         "jsc_access_policy",
		OutputFile:     "access_policies.tf",
		DataSource:     "jsc_access_policies",
		DataSourceAttr: "policies",
	},
	{
		FilterKey:   "secure_policy",
		DisplayName: "Secure Policy (singleton)",
		TFType:      "jsc_secure_policy",
		OutputFile:  "secure_policy.tf",
		IsSingleton: true,
		SingletonID: "secure_policy",
	},
}

// TypeToFileMap returns a map of TF resource type → output filename.
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

// DiscoverableResources returns only resources that have data sources for discovery.
func DiscoverableResources() []ResourceDef {
	var result []ResourceDef
	for _, r := range Resources {
		if r.DataSource != "" {
			result = append(result, r)
		}
	}
	return result
}

// SingletonResources returns only singleton resources.
func SingletonResources() []ResourceDef {
	var result []ResourceDef
	for _, r := range Resources {
		if r.IsSingleton {
			result = append(result, r)
		}
	}
	return result
}

// DefaultRules returns the reference rules for JSC resource types.
func DefaultRules() []postprocess.ReferenceRule {
	// JSC resources currently don't have cross-references that need rewriting
	// Add rules here as needed when resource relationships are identified
	return []postprocess.ReferenceRule{}
}
