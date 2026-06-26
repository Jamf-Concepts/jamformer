// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// ResourceDef describes a single Jamf Platform resource type.
type ResourceDef struct {
	FilterKey         string // key for -include-resources / -exclude-resources
	DisplayName       string // human-readable name for prompts and output
	TFType            string // Terraform resource type name
	OutputFile        string // filename in the output directory
	IsSingleton       bool   // true for tenant-wide settings imported by a fixed ID
	SingletonImportID string // import ID for singletons (e.g. "singleton")
}

// nativeResources are the Jamf Platform Services resources (not Jamf Pro
// federated objects).
var nativeResources = []ResourceDef{
	{FilterKey: "blueprints", DisplayName: "Blueprints", TFType: "jamfplatform_blueprints_blueprint", OutputFile: "blueprints.tf"},
	{FilterKey: "compliance_benchmarks", DisplayName: "Compliance Benchmarks", TFType: "jamfplatform_cbengine_benchmark", OutputFile: "compliance_benchmarks.tf"},
	{FilterKey: "device_groups", DisplayName: "Device Groups", TFType: "jamfplatform_device_group", OutputFile: "device_groups.tf"},
}

// listablePro are the Jamf Pro objects federated into the Platform provider that
// are discoverable via `terraform query` list resources. Each maps to a
// jamfplatform_<suffix> resource type.
var listablePro = []struct{ suffix, display string }{
	{"pro_account", "Jamf Pro Accounts"},
	{"pro_account_group", "Account Groups"},
	{"pro_advanced_computer_search", "Advanced Computer Searches"},
	{"pro_advanced_mobile_device_search", "Advanced Mobile Device Searches"},
	{"pro_advanced_user_search", "Advanced User Searches"},
	{"pro_advanced_volume_purchasing_content_search", "Advanced VPP Content Searches"},
	{"pro_allowed_file_extension", "Allowed File Extensions"},
	{"pro_api_client", "API Clients"},
	{"pro_api_role", "API Roles"},
	{"pro_app_installer", "App Installers"},
	{"pro_app_request_form_field", "App Request Form Fields"},
	{"pro_automated_device_enrollment", "Automated Device Enrollments"},
	{"pro_building", "Buildings"},
	{"pro_category", "Categories"},
	{"pro_class", "Classes"},
	{"pro_cloud_identity_provider", "Cloud Identity Providers"},
	{"pro_computer_extension_attribute", "Computer Extension Attributes"},
	{"pro_computer_invitation", "Computer Invitations"},
	{"pro_computer_prestage_enrollment", "Computer PreStage Enrollments"},
	{"pro_department", "Departments"},
	{"pro_directory_binding", "Directory Bindings"},
	{"pro_disk_encryption_configuration", "Disk Encryption Configurations"},
	{"pro_dock_item", "Dock Items"},
	{"pro_ebook", "eBooks"},
	{"pro_enrollment_customization", "Enrollment Customizations"},
	{"pro_file_share_distribution_point", "File Share Distribution Points"},
	{"pro_ibeacon", "iBeacons"},
	{"pro_inventory_preload_record", "Inventory Preload Records"},
	{"pro_ldap_server", "LDAP Servers"},
	{"pro_licensed_software", "Licensed Software"},
	{"pro_mac_app_store_app", "Mac App Store Apps"},
	{"pro_macos_configuration_profile", "macOS Configuration Profiles"},
	{"pro_mobile_device_app", "Mobile Device Apps"},
	{"pro_mobile_device_configuration_profile", "Mobile Device Configuration Profiles"},
	{"pro_mobile_device_enrollment_profile", "Mobile Device Enrollment Profiles"},
	{"pro_mobile_device_extension_attribute", "Mobile Device Extension Attributes"},
	{"pro_mobile_device_invitation", "Mobile Device Invitations"},
	{"pro_mobile_device_prestage_enrollment", "Mobile Device PreStage Enrollments"},
	{"pro_mobile_device_provisioning_profile", "Mobile Device Provisioning Profiles"},
	{"pro_network_segment", "Network Segments"},
	{"pro_package", "Packages"},
	{"pro_patch_external_source", "Patch External Sources"},
	{"pro_patch_policy", "Patch Policies"},
	{"pro_patch_software_title", "Patch Software Titles"},
	{"pro_pki_json_web_token_configuration", "PKI JSON Web Token Configurations"},
	{"pro_policy", "Policies"},
	{"pro_printer", "Printers"},
	{"pro_removable_mac_address", "Removable MAC Addresses"},
	{"pro_restricted_software", "Restricted Software"},
	{"pro_return_to_service", "Return to Service Configurations"},
	{"pro_script", "Scripts"},
	{"pro_site", "Sites"},
	{"pro_supervision_identity", "Supervision Identities"},
	{"pro_user_extension_attribute", "User Extension Attributes"},
	{"pro_user_group", "User Groups"},
	{"pro_volume_purchasing_location", "Volume Purchasing Locations"},
	{"pro_volume_purchasing_notification", "Volume Purchasing Notifications"},
	{"pro_vpp_assignment", "VPP Assignments"},
	{"pro_vpp_invitation", "VPP Invitations"},
	{"pro_webhook", "Webhooks"},
}

// singletonPro are the tenant-wide Jamf Pro settings federated into the Platform
// provider. They are not list resources; each is imported by the fixed ID
// "singleton" and its config recovered via `terraform plan -generate-config-out`.
var singletonPro = []struct{ suffix, display string }{
	{"pro_access_management_settings", "Access Management Settings"},
	{"pro_activation_code", "Activation Code"},
	{"pro_app_installer_settings", "App Installer Settings"},
	{"pro_app_request_settings", "App Request Settings"},
	{"pro_cloud_distribution_point", "Cloud Distribution Point"},
	{"pro_computer_check_in_settings", "Computer Check-In Settings"},
	{"pro_computer_inventory_collection_settings", "Computer Inventory Collection Settings"},
	{"pro_gsx_connection_settings", "GSX Connection Settings"},
	{"pro_impact_alert_notification_settings", "Impact Alert Notification Settings"},
	{"pro_jamf_parent_settings", "Jamf Parent Settings"},
	{"pro_jamf_protect", "Jamf Protect Registration"},
	{"pro_jamf_teacher_settings", "Jamf Teacher Settings"},
	{"pro_local_admin_password_settings", "Local Admin Password (LAPS) Settings"},
	{"pro_login_page_settings", "Login Page Settings"},
	{"pro_macos_onboarding", "macOS Onboarding"},
	{"pro_managed_software_update", "Managed Software Updates"},
	{"pro_mdm_profile_settings", "MDM Profile Settings"},
	{"pro_re_enrollment_settings", "Re-enrollment Settings"},
	{"pro_self_service_branding_ios", "Self Service iOS Branding"},
	{"pro_self_service_branding_macos", "Self Service macOS Branding"},
	{"pro_self_service_macos_settings", "Self Service macOS Settings"},
	{"pro_self_service_plus_settings", "Self Service Plus Settings"},
	{"pro_service_discovery_enrollment", "Service Discovery Enrollment"},
	{"pro_smtp_server", "SMTP Server"},
	{"pro_sso_failover_url", "SSO Failover URL"},
	{"pro_sso_settings", "SSO Settings"},
	{"pro_user_initiated_enrollment_settings", "User-Initiated Enrollment Settings"},
}

// Resources is the ordered list of all supported Jamf Platform resource types:
// native Platform Services resources, then federated Jamf Pro list resources,
// then federated Jamf Pro singleton settings.
var Resources = buildResources()

func buildResources() []ResourceDef {
	r := make([]ResourceDef, 0, len(nativeResources)+len(listablePro)+len(singletonPro))
	r = append(r, nativeResources...)
	for _, e := range listablePro {
		r = append(r, ResourceDef{
			FilterKey:   strings.TrimPrefix(e.suffix, "pro_"),
			DisplayName: e.display,
			TFType:      "jamfplatform_" + e.suffix,
			OutputFile:  e.suffix + ".tf",
		})
	}
	for _, e := range singletonPro {
		r = append(r, ResourceDef{
			FilterKey:         strings.TrimPrefix(e.suffix, "pro_"),
			DisplayName:       e.display,
			TFType:            "jamfplatform_" + e.suffix,
			OutputFile:        e.suffix + ".tf",
			IsSingleton:       true,
			SingletonImportID: "singleton",
		})
	}
	return r
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

// ListableResources returns the resource defs discoverable via `terraform query`
// (everything that is not a singleton).
func ListableResources() []ResourceDef {
	var out []ResourceDef
	for _, r := range Resources {
		if !r.IsSingleton {
			out = append(out, r)
		}
	}
	return out
}

// SingletonResources returns the tenant-wide settings resource defs.
func SingletonResources() []ResourceDef {
	var out []ResourceDef
	for _, r := range Resources {
		if r.IsSingleton {
			out = append(out, r)
		}
	}
	return out
}

// countSelectedListableTypes returns how many listable (non-singleton) resource
// types will be queried, honouring the selection filter (nil = all). Used as the
// denominator for the discovery progress fraction.
func countSelectedListableTypes(selected map[string]bool) int {
	n := 0
	for _, r := range ListableResources() {
		if selected == nil || selected[r.FilterKey] {
			n++
		}
	}
	return n
}

// Synthetic registry types used to disambiguate the two flavours of
// jamfplatform_device_group (one TF type, separate Jamf Pro ID spaces). They are
// keyed by jamf_pro_id and resolve to a jamfplatform_device_group.<label> address
// whose .jamf_pro_id is referenced from classic-API scope blocks.
const (
	DeviceGroupComputerType = "jamfplatform_device_group#computer"
	DeviceGroupMobileType   = "jamfplatform_device_group#mobile"
)

// Federated Jamf Pro target type names.
const (
	tCategory    = "jamfplatform_pro_category"
	tSite        = "jamfplatform_pro_site"
	tBuilding    = "jamfplatform_pro_building"
	tDepartment  = "jamfplatform_pro_department"
	tNetworkSeg  = "jamfplatform_pro_network_segment"
	tiBeacon     = "jamfplatform_pro_ibeacon"
	tUserGroup   = "jamfplatform_pro_user_group"
	tScript      = "jamfplatform_pro_script"
	tPackage     = "jamfplatform_pro_package"
	tPrinter     = "jamfplatform_pro_printer"
	tDockItem    = "jamfplatform_pro_dock_item"
	tDirBinding  = "jamfplatform_pro_directory_binding"
	tDiskEnc     = "jamfplatform_pro_disk_encryption_configuration"
	tMacProfile  = "jamfplatform_pro_macos_configuration_profile"
	tADE         = "jamfplatform_pro_automated_device_enrollment"
	tEnrollCust  = "jamfplatform_pro_enrollment_customization"
	tVPPLocation = "jamfplatform_pro_volume_purchasing_location"
	tSupervision = "jamfplatform_pro_supervision_identity"
	tPatchTitle  = "jamfplatform_pro_patch_software_title"
)

// ExtractSpecs returns the string-attribute → support-file extraction specs for
// the federated Jamf Pro resources, whose content lives in plugin-framework
// nested attributes (object expressions).
func ExtractSpecs() []postprocess.ExtractSpec {
	return []postprocess.ExtractSpec{
		// Scripts: script_contents at the resource root.
		{
			ResourceType: "jamfplatform_pro_script",
			AttrName:     "script_contents",
			OutputSubdir: "scripts",
			FileKind:     postprocess.FileKindScript,
		},
		// Computer extension attributes: SCRIPT-type EAs carry the script in `script`.
		{
			ResourceType: "jamfplatform_pro_computer_extension_attribute",
			AttrName:     "script",
			OutputSubdir: "extension_attributes",
			FileKind:     postprocess.FileKindScript,
		},
		// macOS configuration profiles: .mobileconfig XML at general.payloads.
		{
			ResourceType: "jamfplatform_pro_macos_configuration_profile",
			AttrPath:     []string{"general"},
			AttrName:     "payloads",
			NameAttrPath: []string{"general"},
			OutputSubdir: "macos_configuration_profiles",
			FileKind:     postprocess.FileKindMobileconfig,
			SkipFn:       postprocess.ShouldSkipProfile,
		},
		// Mobile device configuration profiles: .mobileconfig XML at general.payloads.
		{
			ResourceType: "jamfplatform_pro_mobile_device_configuration_profile",
			AttrPath:     []string{"general"},
			AttrName:     "payloads",
			NameAttrPath: []string{"general"},
			OutputSubdir: "mobile_device_configuration_profiles",
			FileKind:     postprocess.FileKindMobileconfig,
			SkipFn:       postprocess.ShouldSkipProfile,
		},
		// Mobile device apps: managed-app configuration plist at app_configuration.preferences.
		{
			ResourceType: "jamfplatform_pro_mobile_device_app",
			AttrPath:     []string{"app_configuration"},
			AttrName:     "preferences",
			NameAttrPath: []string{"general"},
			OutputSubdir: "app_configurations",
			FileKind:     postprocess.FileKindXML,
		},
	}
}

// WriteSingletonImports writes singletons_import.tf with an import block per
// selected singleton settings resource (label "singleton", id "singleton"), so a
// subsequent `terraform plan -generate-config-out` can materialise their config.
// Returns false if no singleton import blocks were written.
func WriteSingletonImports(outputDir string, selectedResources map[string]bool) (bool, error) {
	f := hclwrite.NewEmptyFile()
	body := f.Body()
	count := 0
	for _, r := range SingletonResources() {
		if selectedResources != nil && !selectedResources[r.FilterKey] {
			continue
		}
		if count > 0 {
			body.AppendNewline()
		}
		block := body.AppendNewBlock("import", nil)
		block.Body().SetAttributeRaw("to", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(r.TFType + ".singleton")},
		})
		block.Body().SetAttributeValue("id", cty.StringVal(r.SingletonImportID))
		count++
	}
	if count == 0 {
		return false, nil
	}
	if err := os.WriteFile(filepath.Join(outputDir, "singletons_import.tf"), f.Bytes(), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// DefaultRules returns the cross-resource reference rules for Jamf Platform.
// The federated jamfplatform_pro_* surface uses plugin-framework nested
// attributes (object expressions), so rules use AttrPath / ElementAttr rather
// than the block-based BlockPath used by the jamfpro provider.
func DefaultRules() []postprocess.ReferenceRule {
	// Helper constructors keep this table compact and consistent.
	cat := func(rt string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: "category_id", TargetTypes: []string{tCategory}, TargetAttr: "id"}
	}
	site := func(rt, attr string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: attr, TargetTypes: []string{tSite}, TargetAttr: "id"}
	}
	single := func(rt, attr, target string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: attr, TargetTypes: []string{target}, TargetAttr: "id"}
	}
	list := func(rt, attr, target string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: attr, TargetTypes: []string{target}, TargetAttr: "id", IsList: true}
	}
	elem := func(rt, attr, elemAttr, target string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: attr, ElementAttr: elemAttr, TargetTypes: []string{target}, TargetAttr: "id"}
	}
	// Device-group scope list (computer or mobile), referenced by .jamf_pro_id.
	dg := func(rt, attr, dgType string, path ...string) postprocess.ReferenceRule {
		return postprocess.ReferenceRule{ResourceType: rt, AttrPath: path, AttrName: attr, TargetTypes: []string{dgType}, TargetAttr: "jamf_pro_id", IsList: true}
	}

	const (
		policy     = "jamfplatform_pro_policy"
		macProfile = tMacProfile
		mobProfile = "jamfplatform_pro_mobile_device_configuration_profile"
		mobApp     = "jamfplatform_pro_mobile_device_app"
		macApp     = "jamfplatform_pro_mac_app_store_app"
		restricted = "jamfplatform_pro_restricted_software"
		compPre    = "jamfplatform_pro_computer_prestage_enrollment"
		mobPre     = "jamfplatform_pro_mobile_device_prestage_enrollment"
		patchPol   = "jamfplatform_pro_patch_policy"
		patchTitle = tPatchTitle
		ade        = tADE
		vppLoc     = tVPPLocation
	)

	rules := []postprocess.ReferenceRule{
		// --- Native Platform Services resources (reference device groups by UUID) ---
		{ResourceType: "jamfplatform_blueprints_blueprint", AttrName: "device_groups", TargetTypes: []string{"jamfplatform_device_group"}, TargetAttr: "id", IsList: true},
		{ResourceType: "jamfplatform_cbengine_benchmark", AttrName: "target_device_group", TargetTypes: []string{"jamfplatform_device_group"}, TargetAttr: "id"},

		// --- Policy ---
		cat(policy, "general"),
		site(policy, "site_id", "general"),
		dg(policy, "computer_group_ids", DeviceGroupComputerType, "scope", "targets"),
		list(policy, "building_ids", tBuilding, "scope", "targets"),
		list(policy, "department_ids", tDepartment, "scope", "targets"),
		list(policy, "user_group_ids", tUserGroup, "scope", "targets"),
		dg(policy, "computer_group_ids", DeviceGroupComputerType, "scope", "exclusions"),
		list(policy, "building_ids", tBuilding, "scope", "exclusions"),
		list(policy, "department_ids", tDepartment, "scope", "exclusions"),
		list(policy, "user_group_ids", tUserGroup, "scope", "exclusions"),
		list(policy, "network_segment_ids", tNetworkSeg, "scope", "exclusions"),
		list(policy, "ibeacon_ids", tiBeacon, "scope", "exclusions"),
		list(policy, "network_segment_ids", tNetworkSeg, "scope", "limitations"),
		list(policy, "ibeacon_ids", tiBeacon, "scope", "limitations"),
		list(policy, "network_segment_ids", tNetworkSeg, "general", "network_limitations"),
		elem(policy, "scripts", "id", tScript, "scripts"),
		elem(policy, "packages", "id", tPackage, "packages"),
		elem(policy, "printers", "id", tPrinter, "printers"),
		elem(policy, "dock_items", "id", tDockItem, "dock_items"),
		elem(policy, "directory_bindings", "id", tDirBinding),
		single(policy, "disk_encryption_configuration_id", tDiskEnc, "disk_encryption"),
		single(policy, "remediate_disk_encryption_configuration_id", tDiskEnc, "disk_encryption"),
		elem(policy, "categories", "id", tCategory, "self_service"),

		// --- macOS configuration profile ---
		cat(macProfile, "general"),
		site(macProfile, "site_id", "general"),
		dg(macProfile, "computer_group_ids", DeviceGroupComputerType, "scope", "targets"),
		list(macProfile, "building_ids", tBuilding, "scope", "targets"),
		list(macProfile, "department_ids", tDepartment, "scope", "targets"),
		list(macProfile, "user_group_ids", tUserGroup, "scope", "targets"),
		dg(macProfile, "computer_group_ids", DeviceGroupComputerType, "scope", "exclusions"),
		list(macProfile, "building_ids", tBuilding, "scope", "exclusions"),
		list(macProfile, "department_ids", tDepartment, "scope", "exclusions"),
		list(macProfile, "user_group_ids", tUserGroup, "scope", "exclusions"),
		list(macProfile, "network_segment_ids", tNetworkSeg, "scope", "exclusions"),
		list(macProfile, "ibeacon_ids", tiBeacon, "scope", "exclusions"),
		list(macProfile, "network_segment_ids", tNetworkSeg, "scope", "limitations"),
		list(macProfile, "ibeacon_ids", tiBeacon, "scope", "limitations"),
		elem(macProfile, "categories", "id", tCategory, "self_service"),

		// --- Mobile device configuration profile ---
		cat(mobProfile, "general"),
		site(mobProfile, "site_id", "general"),
		dg(mobProfile, "mobile_device_group_ids", DeviceGroupMobileType, "scope", "targets"),
		list(mobProfile, "building_ids", tBuilding, "scope", "targets"),
		list(mobProfile, "department_ids", tDepartment, "scope", "targets"),
		list(mobProfile, "user_group_ids", tUserGroup, "scope", "targets"),
		dg(mobProfile, "mobile_device_group_ids", DeviceGroupMobileType, "scope", "exclusions"),
		list(mobProfile, "building_ids", tBuilding, "scope", "exclusions"),
		list(mobProfile, "department_ids", tDepartment, "scope", "exclusions"),
		list(mobProfile, "network_segment_ids", tNetworkSeg, "scope", "exclusions"),
		list(mobProfile, "network_segment_ids", tNetworkSeg, "scope", "limitations"),

		// --- Mobile device app ---
		cat(mobApp, "general"),
		site(mobApp, "site_id", "general"),
		dg(mobApp, "mobile_device_group_ids", DeviceGroupMobileType, "scope", "targets"),
		list(mobApp, "building_ids", tBuilding, "scope", "targets"),
		list(mobApp, "department_ids", tDepartment, "scope", "targets"),
		list(mobApp, "user_group_ids", tUserGroup, "scope", "targets"),
		dg(mobApp, "mobile_device_group_ids", DeviceGroupMobileType, "scope", "exclusions"),
		list(mobApp, "building_ids", tBuilding, "scope", "exclusions"),
		list(mobApp, "department_ids", tDepartment, "scope", "exclusions"),
		list(mobApp, "user_group_ids", tUserGroup, "scope", "exclusions"),
		list(mobApp, "network_segment_ids", tNetworkSeg, "scope", "exclusions"),
		list(mobApp, "network_segment_ids", tNetworkSeg, "scope", "limitations"),
		elem(mobApp, "self_service_categories", "id", tCategory, "self_service"),
		single(mobApp, "vpp_admin_account_id", vppLoc, "vpp"),

		// --- Mac App Store app ---
		cat(macApp, "general"),
		site(macApp, "site_id", "general"),
		dg(macApp, "computer_group_ids", DeviceGroupComputerType, "scope", "targets"),
		list(macApp, "building_ids", tBuilding, "scope", "targets"),
		list(macApp, "department_ids", tDepartment, "scope", "targets"),
		dg(macApp, "computer_group_ids", DeviceGroupComputerType, "scope", "exclusions"),
		single(macApp, "vpp_admin_account_id", vppLoc, "vpp"),

		// --- Restricted software (computer scope) ---
		site(restricted, "site_id"),
		dg(restricted, "computer_group_ids", DeviceGroupComputerType, "scope", "targets"),
		dg(restricted, "computer_group_ids", DeviceGroupComputerType, "scope", "exclusions"),

		// --- Scripts / packages / patch ---
		cat(tScript),
		cat(tPackage),
		cat(patchTitle),
		site(patchTitle, "site_id"),
		single(patchPol, "software_title_configuration_id", patchTitle),

		// --- Advanced searches / user groups (site) ---
		site("jamfplatform_pro_advanced_computer_search", "site_id"),
		site("jamfplatform_pro_advanced_mobile_device_search", "site_id"),
		site("jamfplatform_pro_advanced_user_search", "site_id"),
		site(tUserGroup, "site_id"),

		// --- Computer PreStage enrollment ---
		site(compPre, "enrollment_site_id"),
		single(compPre, "enrollment_customization_id", tEnrollCust),
		single(compPre, "device_enrollment_program_instance_id", ade),
		single(compPre, "psso_config_profile_id", tMacProfile),
		list(compPre, "prestage_installed_profile_ids", tMacProfile),
		list(compPre, "custom_package_ids", tPackage),
		single(compPre, "building_id", tBuilding, "location_information"),
		single(compPre, "department_id", tDepartment, "location_information"),

		// --- Mobile device PreStage enrollment ---
		site(mobPre, "enrollment_site_id"),
		single(mobPre, "enrollment_customization_id", tEnrollCust),
		single(mobPre, "device_enrollment_program_instance_id", ade),
		single(mobPre, "building_id", tBuilding, "location_information"),
		single(mobPre, "department_id", tDepartment, "location_information"),

		// --- Automated Device Enrollment / VPP location ---
		site(ade, "site_id"),
		single(ade, "supervision_identity_id", tSupervision),
		site(vppLoc, "site_id"),
	}

	return rules
}
