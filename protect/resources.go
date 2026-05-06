// Copyright 2026, Jamf Software LLC

package protect

import "github.com/Jamf-Concepts/jamformer/postprocess"

// ResourceDef describes a single Jamf Protect resource type.
type ResourceDef struct {
	FilterKey   string // key for -include-resources / -exclude-resources
	DisplayName string // human-readable name for prompts and output
	TFType      string // Terraform resource type name
	OutputFile  string // filename in the output directory
}

// Resources is the ordered list of all supported Jamf Protect resource types.
// Order matches the interactive selection prompt order.
var Resources = []ResourceDef{
	{FilterKey: "roles", DisplayName: "Roles", TFType: "jamfprotect_role", OutputFile: "roles.tf"},
	{FilterKey: "groups", DisplayName: "Groups", TFType: "jamfprotect_group", OutputFile: "groups.tf"},
	{FilterKey: "users", DisplayName: "Users", TFType: "jamfprotect_user", OutputFile: "users.tf"},
	{FilterKey: "api_clients", DisplayName: "API Clients", TFType: "jamfprotect_api_client", OutputFile: "api_clients.tf"},
	{FilterKey: "analytics", DisplayName: "Analytics", TFType: "jamfprotect_analytic", OutputFile: "analytics.tf"},
	{FilterKey: "analytics_managed", DisplayName: "Jamf Managed Analytics", TFType: "jamfprotect_analytic_managed", OutputFile: "analytics_managed.tf"},
	{FilterKey: "analytic_sets", DisplayName: "Analytic Sets", TFType: "jamfprotect_analytic_set", OutputFile: "analytic_sets.tf"},
	{FilterKey: "exception_sets", DisplayName: "Exception Sets", TFType: "jamfprotect_exception_set", OutputFile: "exception_sets.tf"},
	{FilterKey: "action_configurations", DisplayName: "Action Configurations", TFType: "jamfprotect_action_configuration", OutputFile: "action_configurations.tf"},
	{FilterKey: "telemetry", DisplayName: "Telemetry", TFType: "jamfprotect_telemetry", OutputFile: "telemetry.tf"},
	{FilterKey: "unified_logging_filters", DisplayName: "Unified Logging Filters", TFType: "jamfprotect_unified_logging_filter", OutputFile: "unified_logging_filters.tf"},
	{FilterKey: "custom_prevent_lists", DisplayName: "Custom Prevent Lists", TFType: "jamfprotect_custom_prevent_list", OutputFile: "custom_prevent_lists.tf"},
	{FilterKey: "removable_storage_control_sets", DisplayName: "Removable Storage Control Sets", TFType: "jamfprotect_removable_storage_control_set", OutputFile: "removable_storage_control_sets.tf"},
	{FilterKey: "plans", DisplayName: "Plans", TFType: "jamfprotect_plan", OutputFile: "plans.tf"},
	{FilterKey: "change_management", DisplayName: "Change Management (singleton)", TFType: "jamfprotect_change_management", OutputFile: "change_management.tf"},
	{FilterKey: "data_forwarding", DisplayName: "Data Forwarding (singleton)", TFType: "jamfprotect_data_forwarding", OutputFile: "data_forwarding.tf"},
	{FilterKey: "data_retention", DisplayName: "Data Retention (singleton)", TFType: "jamfprotect_data_retention", OutputFile: "data_retention.tf"},
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

// DefaultRules returns the reference rules for Jamf Protect resource types.
func DefaultRules() []postprocess.ReferenceRule {
	return []postprocess.ReferenceRule{
		// Group -> Role references
		{
			ResourceType: "jamfprotect_group",
			AttrName:     "role_ids",
			TargetTypes:  []string{"jamfprotect_role"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// User -> Role references
		{
			ResourceType: "jamfprotect_user",
			AttrName:     "role_ids",
			TargetTypes:  []string{"jamfprotect_role"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// User -> Group references
		{
			ResourceType: "jamfprotect_user",
			AttrName:     "group_ids",
			TargetTypes:  []string{"jamfprotect_group"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// API Client -> Role references
		{
			ResourceType: "jamfprotect_api_client",
			AttrName:     "role_ids",
			TargetTypes:  []string{"jamfprotect_role"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Analytic Set -> Analytic / Managed Analytic references
		{
			ResourceType: "jamfprotect_analytic_set",
			AttrName:     "analytics",
			TargetTypes:  []string{"jamfprotect_analytic", "jamfprotect_analytic_managed"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Plan -> Action Configuration reference
		{
			ResourceType: "jamfprotect_plan",
			AttrName:     "action_configuration",
			TargetTypes:  []string{"jamfprotect_action_configuration"},
			TargetAttr:   "id",
		},
		// Plan -> Exception Set references
		{
			ResourceType: "jamfprotect_plan",
			AttrName:     "exception_sets",
			TargetTypes:  []string{"jamfprotect_exception_set"},
			TargetAttr:   "id",
			IsList:       true,
		},
		// Plan -> Telemetry reference
		{
			ResourceType: "jamfprotect_plan",
			AttrName:     "telemetry",
			TargetTypes:  []string{"jamfprotect_telemetry"},
			TargetAttr:   "id",
		},
		// Plan -> Removable Storage Control Set reference
		{
			ResourceType: "jamfprotect_plan",
			AttrName:     "removable_storage_control_set",
			TargetTypes:  []string{"jamfprotect_removable_storage_control_set"},
			TargetAttr:   "id",
		},
		// Plan -> Analytic Set references
		{
			ResourceType: "jamfprotect_plan",
			AttrName:     "analytic_sets",
			TargetTypes:  []string{"jamfprotect_analytic_set"},
			TargetAttr:   "id",
			IsList:       true,
		},
	}
}
