// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	sdkaccount "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/account"
	sdkaigov "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/aigovernance"
	sdkblueprints "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/blueprints"
	sdkbenchmarks "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	sdkdevicegroups "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devicegroups"
	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	sdkproclassic "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	sdksecuritycloud "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// The permissions a jamformer run needs are the READ permissions of the
// endpoints the export reads. They are not hand-copied: each resource type
// names the SDK method whose endpoint backs its listing, and the privilege
// identifiers and accepted scopes are read out of the SDK's own generated
// Privileges registries. An SDK bump therefore updates this map's output, and
// TestReadMethodsResolve fails if a named method disappears rather than letting
// a stale privilege ship.
//
// Only read is covered. Applying the generated configuration to a fresh tenant
// needs create/update as well, which is a different question from "what does
// this export require", and one the provider's own documentation answers per
// resource.

// privilegeSets are the SDK's generated per-package registries, keyed by the
// package name used in readMethod.
var privilegeSets = map[string]map[string]jamfplatform.MethodPrivileges{
	"pro":           sdkpro.Privileges,
	"proclassic":    sdkproclassic.Privileges,
	"securitycloud": sdksecuritycloud.Privileges,
	"account":       sdkaccount.Privileges,
	"aigovernance":  sdkaigov.Privileges,
	"blueprints":    sdkblueprints.Privileges,
	"benchmarks":    sdkbenchmarks.Privileges,
	"devicegroups":  sdkdevicegroups.Privileges,
}

// readMethod names one SDK method backing a resource type's read path.
type readMethod struct {
	Pkg    string // key into privilegeSets
	Method string // generated SDK method name
}

// readMethods maps a resource's filter key to the SDK method(s) an export reads
// it through. Several resources need more than one: a listing that returns
// identity only is followed by a per-item read, and where the two endpoints
// carry different privileges both are required.
//
// Where a resource is served by the Classic API the entry names the proclassic
// package, matching how the provider reaches it.
var readMethods = map[string][]readMethod{
	// --- Native Jamf Platform Services ---
	"blueprints":            {{"blueprints", "ListBlueprints"}, {"blueprints", "ListBlueprintComponents"}},
	"compliance_benchmarks": {{"benchmarks", "ListBenchmarks"}, {"benchmarks", "ListBaselines"}},
	"device_groups":         {{"devicegroups", "ListDeviceGroups"}},

	// --- Jamf AI Governance ---
	"ai_governance_policies": {{"aigovernance", "ListPolicies"}, {"aigovernance", "ListTools"}},

	// --- Jamf Account (organization scope) ---
	"account_sso_domains":     {{"account", "ListDomains"}},
	"account_sso_connections": {{"account", "ListConnections"}},

	// --- Jamf Security Cloud ---
	"security_cloud_device_groups":         {{"securitycloud", "ListDeviceGroupsV2"}},
	"security_cloud_ztna_gateways":         {{"securitycloud", "ListZtnaGatewaysV1"}},
	"security_cloud_ztna_grouped_gateways": {{"securitycloud", "ListZtnaGroupedGatewaysV1"}},
	"security_cloud_ztna_apps":             {{"securitycloud", "ListZtnaAppsV1"}},
	"security_cloud_dns_zones":             {{"securitycloud", "ListDnsZonesV1"}},
	"security_cloud_uem_connect":           {{"securitycloud", "ListUemConnectorsV1"}},
	"security_cloud_dns_search_domain":     {{"securitycloud", "GetDnsSearchDomainV1"}},
	"security_cloud_dns_hostname_mappings": {{"securitycloud", "GetDnsCustomHostnameMappingsV1"}},

	// --- Federated Jamf Pro: listable objects ---
	"account":                                   {{"pro", "ListAccountsV1"}},
	"account_group":                             {{"pro", "ListAccountGroupsV1"}},
	"advanced_computer_search":                  {{"proclassic", "ListAdvancedComputerSearches"}},
	"advanced_mobile_device_search":             {{"proclassic", "ListAdvancedMobileDeviceSearches"}},
	"advanced_user_search":                      {{"proclassic", "ListAdvancedUserSearches"}},
	"advanced_volume_purchasing_content_search": {{"pro", "ListAdvancedUserContentSearchesV1"}},
	"allowed_file_extension":                    {{"proclassic", "ListAllowedFileExtensions"}},
	"app_installer":                             {{"pro", "ListAppInstallerDeploymentsV1"}, {"pro", "ListAppInstallerTitlesV1"}},
	"app_request_form_field":                    {{"pro", "ListAppRequestFormInputFieldsV1"}},
	"automated_device_enrollment":               {{"pro", "ListDeviceEnrollmentsV1"}},
	"building":                                  {{"pro", "ListBuildingsV1"}},
	"category":                                  {{"pro", "ListCategoriesV1"}},
	"class":                                     {{"proclassic", "ListClasses"}},
	"cloud_identity_provider":                   {{"pro", "ListCloudIdpV1"}},
	"computer_extension_attribute":              {{"pro", "ListComputerExtensionAttributesV1"}},
	"computer_invitation":                       {{"proclassic", "ListComputerInvitations"}},
	"computer_prestage_enrollment":              {{"pro", "ListComputerPrestagesV3"}},
	"department":                                {{"pro", "ListDepartmentsV1"}},
	"directory_binding":                         {{"proclassic", "ListDirectoryBindings"}},
	"disk_encryption_configuration":             {{"proclassic", "ListDiskEncryptionConfigurations"}},
	"dock_item":                                 {{"proclassic", "ListDockItems"}},
	"ebook":                                     {{"pro", "ListEbooksV1"}},
	"enrollment_customization":                  {{"pro", "ListEnrollmentCustomizationsV2"}},
	"file_share_distribution_point":             {{"pro", "ListDistributionPointsV1"}},
	"ibeacon":                                   {{"proclassic", "ListIBeacons"}},
	"inventory_preload_record":                  {{"pro", "ListInventoryPreloadRecordsV2"}},
	"ldap_server":                               {{"pro", "ListLdapServersV1"}},
	"licensed_software":                         {{"proclassic", "ListLicensedSoftware"}},
	"mac_app_store_app":                         {{"proclassic", "ListMacApplications"}},
	"macos_configuration_profile":               {{"proclassic", "ListOSXConfigurationProfiles"}},
	"mobile_device_app":                         {{"proclassic", "ListMobileDeviceApplications"}},
	"mobile_device_configuration_profile":       {{"proclassic", "ListMobileDeviceConfigurationProfiles"}},
	"mobile_device_enrollment_profile":          {{"proclassic", "ListMobileDeviceEnrollmentProfiles"}},
	"mobile_device_extension_attribute":         {{"pro", "ListMobileDeviceExtensionAttributesV1"}},
	"mobile_device_invitation":                  {{"proclassic", "ListMobileDeviceInvitations"}},
	"mobile_device_prestage_enrollment":         {{"pro", "ListMobileDevicePrestagesV3"}},
	"mobile_device_provisioning_profile":        {{"proclassic", "ListMobileDeviceProvisioningProfiles"}},
	"network_segment":                           {{"proclassic", "ListNetworkSegments"}},
	"package":                                   {{"pro", "ListPackagesV1"}},
	"patch_external_source":                     {{"proclassic", "ListPatchExternalSources"}},
	"patch_policy":                              {{"pro", "ListPatchPoliciesV2"}},
	// A title resolves its source_id from the tenant's patch source catalogues,
	// so listing one needs both source lists alongside the titles themselves.
	// Without them the read fails 403 and reports only that source_id could not
	// be determined.
	"patch_software_title":             {{"pro", "ListPatchSoftwareTitleConfigurationsV3"}, {"proclassic", "ListPatchExternalSources"}, {"proclassic", "ListPatchInternalSources"}},
	"pki_json_web_token_configuration": {{"proclassic", "ListJsonWebTokenConfigurations"}},
	"policy":                           {{"proclassic", "ListPolicies"}},
	"printer":                          {{"proclassic", "ListPrinters"}},
	"removable_mac_address":            {{"proclassic", "ListRemovableMacAddresses"}},
	"restricted_software":              {{"proclassic", "ListRestrictedSoftware"}},
	"return_to_service":                {{"pro", "ListReturnToServiceConfigurationsV1"}},
	"script":                           {{"pro", "ListScriptsV1"}},
	"site":                             {{"pro", "ListSitesV1"}},
	"supervision_identity":             {{"pro", "ListSupervisionIdentitiesV1"}},
	"user_extension_attribute":         {{"proclassic", "ListUserExtensionAttributes"}},
	"user_group":                       {{"proclassic", "ListUserGroups"}},
	"volume_purchasing_location":       {{"pro", "ListVolumePurchasingLocationsV1"}},
	"vpp_assignment":                   {{"proclassic", "ListVPPAssignments"}},
	"vpp_invitation":                   {{"proclassic", "ListVPPInvitations"}},
	"webhook":                          {{"proclassic", "ListWebhooks"}},

	// --- Federated Jamf Pro: settings singletons ---
	"access_management_settings":             {{"pro", "GetEnrollmentAccessManagementV4"}},
	"activation_code":                        {{"proclassic", "GetActivationCode"}},
	"app_installer_settings":                 {{"pro", "GetAppInstallerGlobalSettingsV1"}},
	"app_request_settings":                   {{"pro", "GetAppRequestSettingsV1"}},
	"cloud_distribution_point":               {{"pro", "GetCloudDistributionPointV1"}},
	"computer_check_in_settings":             {{"proclassic", "GetComputerCheckIn"}},
	"computer_inventory_collection_settings": {{"pro", "GetComputerInventoryCollectionSettingsV2"}},
	"gsx_connection_settings":                {{"pro", "GetGSXConnectionV1"}},
	"impact_alert_notification_settings":     {{"pro", "GetImpactAlertNotificationSettingsV1"}},
	"jamf_parent_settings":                   {{"pro", "GetParentAppSettingsV1"}},
	"jamf_protect":                           {{"pro", "GetJamfProtectSettingsV1"}},
	"jamf_teacher_settings":                  {{"pro", "GetTeacherAppSettingsV1"}},
	"local_admin_password_settings":          {{"pro", "GetLocalAdminPasswordSettingsV2"}},
	"macos_onboarding":                       {{"pro", "GetOnboardingV1"}},
	"managed_software_update":                {{"pro", "GetManagedSoftwareUpdateFeatureToggleV1"}},
	"mdm_profile_settings":                   {{"pro", "GetDeviceCommunicationSettingsV1"}},
	"re_enrollment_settings":                 {{"pro", "GetReenrollmentSettingsV1"}},
	"self_service_branding_ios":              {{"pro", "ListIOSBrandingConfigurationsV1"}},
	"self_service_branding_macos":            {{"pro", "ListMacOSBrandingConfigurationsV1"}},
	"self_service_macos_settings":            {{"pro", "GetSelfServiceSettingsV1"}},
	"self_service_plus_settings":             {{"pro", "GetSelfServicePlusSettingsV1"}},
	"service_discovery_enrollment":           {{"pro", "GetServiceDiscoveryEnrollmentWellKnownSettingsV1"}},
	"smtp_server":                            {{"pro", "GetSmtpServerV2"}},
	"sso_failover_url":                       {{"pro", "GetSsoFailoverV1"}},
	"sso_settings":                           {{"pro", "GetSsoSettingsV3"}},
	"user_initiated_enrollment_settings":     {{"pro", "GetEnrollmentSettingsV4"}},

	// --- Jamf Connect (SDK-discovered; no list resource) ---
	"jamf_connect": {{"pro", "ListJamfConnectConfigProfilesV1"}},
}

// unmappedReads records the resource types whose read path no SDK method
// covers, with the reason. Keeping them here rather than absent from
// readMethods means TestEveryResourceHasReadPermissions can insist on total
// coverage: a new resource type is either mapped or deliberately listed, never
// silently missing its permissions.
var unmappedReads = map[string]string{
	"login_page_settings": "the login page settings endpoint publishes no GET operation in any " +
		"ingested spec, so the SDK exposes no read method to source a privilege from.",
	"volume_purchasing_notification": "Volume Purchasing notifications are read through an " +
		"endpoint absent from the SDK's generated registries; the provider reaches them directly.",
}

// syntheticReads maps the resource types jamformer synthesises rather than
// lists — they have no list resource of their own, and their bytes come from an
// endpoint reached while processing the resource that references them.
//
// These are keyed by TF type, not by filter key: having no Resources entry,
// they have no filter key to be keyed by. ResourcePermissions falls back to
// this map so the per-resource table in PERMISSIONS.md carries a row for each,
// and extraReads folds them into the published capability set — without that
// an integration granted exactly the file's contents cannot repeat the export
// that produced it.
var syntheticReads = map[string][]readMethod{
	// Icons are discovered from the self_service_icon references hydrated onto
	// policies, and their bytes are served from the CDN rather than an API.
	tIcon: nil,
	// Branding images are downloaded per referenced id. DownloadBrandingImageV1
	// is the endpoint the bytes come from, but the spec declares no privilege
	// for it, so the capability an integration actually has to grant is the
	// branding listing's — which is what is named here.
	tBrandingImage: {{"pro", "ListMacOSBrandingConfigurationsV1"}},
	// Jamf Connect adopts an existing macOS configuration profile.
	tJamfConnect: {{"pro", "ListJamfConnectConfigProfilesV1"}},
}

// syntheticReadNotes explains a synthetic type that reads through no API
// endpoint at all, so its empty method list is reported as the reason it needs
// no capability rather than falling through to "no read path is recorded" —
// which would read as an omission from the map.
var syntheticReadNotes = map[string]string{
	tIcon: "icon bytes are served from the Jamf CDN, and the ids come from the " +
		"policies that reference them, so no capability of its own is required",
}

// packageDownloadReads are the endpoints a package download reads through. They
// are held apart from readMethods["package"] for two reasons: the JCDS file
// store carries its own capability (`packages:read` authorises the catalogue
// listing, not the file bytes), and the download is optional — claiming it on a
// -skip-package-downloads run would over-grant, which is the failure this file
// exists to prevent. extraReads adds them when downloads are enabled.
var packageDownloadReads = []readMethod{
	{"pro", "ListJCDSFilesV1"},
	{"pro", "GetJCDSFileDownloadURLV1"},
}

// Permission is one resolved permission requirement for a resource type.
type Permission struct {
	// Capability is the GA capability permission in {capability}:{action}
	// form, e.g. "device-groups:read". This is what an integration grants.
	Capability string
	// Legacy is the retired Jamf Pro privilege name, e.g. "Read Buildings".
	// Populated for the Jamf Pro family only; other families publish none.
	Legacy []string
	// Scopes are the scope kinds the backing endpoint accepts.
	Scopes []jamfplatform.ScopeKind
	// Method and Path identify the endpoint the requirement came from, so a
	// reader can check the claim rather than take it on trust.
	Method string
	Path   string
}

// ResourcePermissions returns the read permissions an export of this resource
// type requires, de-duplicated and sorted by capability. The key is a filter
// key for a type in the Resources table, or a TF type for one of the synthetic
// types, which have no filter key. The second return is a reason when no
// permission could be resolved: either the type is deliberately unmapped (see
// unmappedReads) or the SDK declares no privilege for its endpoint, which is
// NOT the same as none being required.
func ResourcePermissions(filterKey string) ([]Permission, string) {
	methods, ok := readMethods[filterKey]
	if !ok {
		methods, ok = syntheticReads[filterKey]
	}
	if !ok {
		if reason, listed := unmappedReads[filterKey]; listed {
			return nil, reason
		}
		return nil, "no read path is recorded for this resource type"
	}
	if len(methods) == 0 {
		if note, noted := syntheticReadNotes[filterKey]; noted {
			return nil, note
		}
	}
	return permissionsForMethods(methods)
}

// permissionsForMethods resolves a set of SDK read methods into the capability
// requirements an integration must grant to call them, de-duplicated by
// capability and sorted. Shared by the per-resource lookup and by the reads
// that belong to no resource type at all (see extraReads).
func permissionsForMethods(methods []readMethod) ([]Permission, string) {
	byCapability := map[string]Permission{}
	var undeclared []string
	for _, m := range methods {
		set, ok := privilegeSets[m.Pkg]
		if !ok {
			continue
		}
		mp, ok := set[m.Method]
		if !ok {
			continue
		}
		if len(mp.Scoped) == 0 {
			undeclared = append(undeclared, m.Method)
			continue
		}
		for _, cap := range mp.Scoped {
			// Only read actions matter to an export. A write action reaching
			// this map means the endpoint's own privilege set names one (the
			// LAPS settings GET is such a case), and it is carried through
			// rather than dropped, because the endpoint does require it.
			existing, seen := byCapability[cap]
			if seen {
				existing.Legacy = mergeSorted(existing.Legacy, mp.Legacy)
				byCapability[cap] = existing
				continue
			}
			byCapability[cap] = Permission{
				Capability: cap,
				Legacy:     mergeSorted(nil, mp.Legacy),
				Scopes:     mp.Scopes,
				Method:     mp.Method,
				Path:       mp.Path,
			}
		}
	}

	if len(byCapability) == 0 {
		if len(undeclared) > 0 {
			return nil, fmt.Sprintf("the spec declares no privilege for %s; this is not the same as none being required",
				strings.Join(undeclared, ", "))
		}
		return nil, "no read path is recorded for this resource type"
	}

	out := make([]Permission, 0, len(byCapability))
	for _, p := range byCapability {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Capability < out[j].Capability })
	return out, ""
}

func mergeSorted(dst, src []string) []string {
	seen := map[string]bool{}
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range src {
		if !seen[v] {
			dst = append(dst, v)
			seen[v] = true
		}
	}
	sort.Strings(dst)
	return dst
}

// extraRead is a read the export performs that iterating the Resources table
// cannot find: a type jamformer synthesises rather than lists, or a download
// path hanging off a type whose listing capability does not authorise it.
type extraRead struct {
	// Label names the row in PERMISSIONS.md. For a synthetic type that is the
	// TF type; for a download path it is the TF type plus the path, because
	// that type already has a row of its own from the table loop.
	Label   string
	Methods []readMethod
	// Reason, when set, replaces the resolved capabilities in the table — the
	// read goes through no privileged endpoint.
	Reason string
}

// extraReads returns the reads this run performs outside the Resources table,
// so PERMISSIONS.md publishes a set that can actually complete the export that
// produced it. Two classes land here:
//
//   - the synthetic types (Self Service icons, branding images, Jamf Connect),
//     which have no list resource and so no Resources entry — the table loop
//     never reaches them, and the Jamf Connect capability in particular is the
//     one whose absence silently drops every jamfplatform_pro_jamf_connect
//     resource from the next export;
//   - the JCDS download path, which reads the file store rather than the
//     package catalogue and carries a separate capability for it.
//
// All of them ride on the federated pro surface, so an organization-scoped run
// performs none and must claim none.
func extraReads(scope ScopeKind, selected map[string]bool, packageDownloads bool) []extraRead {
	if scope == ScopeOrganization {
		return nil
	}
	// A nil selection means no filter at all; the pipeline has normally
	// already replaced it with the scoped set by this point.
	wanted := func(key string) bool { return selected == nil || selected[key] }

	// Icons and branding images are synthesised from whatever referenced them
	// and are not independently selectable, so they are always in play.
	out := []extraRead{
		{Label: tIcon, Methods: syntheticReads[tIcon], Reason: syntheticReadNotes[tIcon]},
		{Label: tBrandingImage, Methods: syntheticReads[tBrandingImage]},
	}
	if wanted("jamf_connect") {
		out = append(out, extraRead{Label: tJamfConnect, Methods: syntheticReads[tJamfConnect]})
	}
	if packageDownloads && wanted("package") {
		out = append(out, extraRead{
			Label:   tPackage + " (JCDS file download)",
			Methods: packageDownloadReads,
		})
	}
	return out
}

// RequiredCapabilities returns the union of read capabilities needed to export
// the given selection, sorted. A nil selection means every resource type
// reachable at the given scope. packageDownloads reports whether the run
// downloads package files, which reads the JCDS file store on top of the
// package catalogue. The result is the permission set to tick in Jamf Account's
// picker for an integration that only has to run this export.
func RequiredCapabilities(scope ScopeKind, selected map[string]bool, packageDownloads bool) []string {
	seen := map[string]bool{}
	for _, r := range ResourcesForScope(scope) {
		if selected != nil && !selected[r.FilterKey] {
			continue
		}
		perms, _ := ResourcePermissions(r.FilterKey)
		for _, p := range perms {
			seen[p.Capability] = true
		}
	}
	for _, e := range extraReads(scope, selected, packageDownloads) {
		perms, _ := permissionsForMethods(e.Methods)
		for _, p := range perms {
			seen[p.Capability] = true
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// WritePermissionsFile writes PERMISSIONS.md into the output directory: the
// capability set the export required, and a per-resource breakdown. It ships
// with the generated project so the next person to run the export — or to
// apply it in CI — can create an integration with the right permissions
// instead of over-granting to make an unattributed 403 go away.
// packageDownloads reports whether the run fetched package files, which reads
// the JCDS file store under its own capability; a -skip-package-downloads run
// must not claim it.
func WritePermissionsFile(outputDir string, scope Scope, selected map[string]bool, packageDownloads bool) error {
	var b strings.Builder
	b.WriteString("# Required Jamf permissions\n\n")
	fmt.Fprintf(&b, "This project was exported with an API integration at **%s scope**.\n\n", scope.Kind)
	b.WriteString("Permissions are granted per capability and action in the Jamf Account " +
		"**Platform API integrations** UI. The identifiers below are the API capability names; " +
		"the picker presents each as a named permission with a checkbox per action.\n\n")
	b.WriteString("Everything here is a **read** permission: it is what this export required. " +
		"Applying the generated configuration needs create and update as well.\n\n")

	caps := RequiredCapabilities(scope.Kind, selected, packageDownloads)
	fmt.Fprintf(&b, "## Capability set (%d)\n\n```\n", len(caps))
	for _, c := range caps {
		b.WriteString(c + "\n")
	}
	b.WriteString("```\n\n## Per resource type\n\n")
	b.WriteString("| Resource type | Capabilities | Endpoint |\n|---|---|---|\n")

	row := func(label string, perms []Permission, reason string) {
		if reason != "" {
			fmt.Fprintf(&b, "| `%s` | _not recorded_ | %s |\n", label, reason)
			return
		}
		var capList, paths []string
		for _, p := range perms {
			capList = append(capList, "`"+p.Capability+"`")
			paths = append(paths, "`"+p.Path+"`")
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", label,
			strings.Join(capList, "<br>"), strings.Join(dedupe(paths), "<br>"))
	}

	for _, r := range ResourcesForScope(scope.Kind) {
		if selected != nil && !selected[r.FilterKey] {
			continue
		}
		perms, reason := ResourcePermissions(r.FilterKey)
		row(r.TFType, perms, reason)
	}

	// The reads the Resources table cannot describe: the synthesised types and
	// the JCDS download path. Without these rows the file publishes a set that
	// cannot repeat its own export — the next run's Jamf Connect discovery and
	// package downloads 403 and degrade to warnings, and the export quietly
	// comes out smaller.
	for _, e := range extraReads(scope.Kind, selected, packageDownloads) {
		perms, reason := permissionsForMethods(e.Methods)
		if e.Reason != "" {
			reason = e.Reason
		}
		row(e.Label, perms, reason)
	}

	b.WriteString("\nSourced from the Jamf Platform Go SDK's generated privilege registries, " +
		"which carry the `x-required-privileges` extensions of the Jamf OpenAPI specifications.\n")

	return os.WriteFile(filepath.Join(outputDir, "PERMISSIONS.md"), []byte(b.String()), 0644)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
