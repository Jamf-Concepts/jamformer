// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestReadMethodsResolve is the guard that keeps the permissions map honest
// across SDK bumps: every method named in readMethods must still exist in the
// SDK's generated registry. A renamed or withdrawn endpoint fails here instead
// of silently dropping a resource's permissions from PERMISSIONS.md.
func TestReadMethodsResolve(t *testing.T) {
	for key, methods := range readMethods {
		for _, m := range methods {
			set, ok := privilegeSets[m.Pkg]
			if !ok {
				t.Errorf("%s: unknown SDK package %q", key, m.Pkg)
				continue
			}
			if _, ok := set[m.Method]; !ok {
				t.Errorf("%s: SDK package %s has no method %q — it may have been renamed or withdrawn", key, m.Pkg, m.Method)
			}
		}
	}
	for key, methods := range syntheticReads {
		for _, m := range methods {
			if _, ok := privilegeSets[m.Pkg][m.Method]; !ok {
				t.Errorf("%s (synthetic): SDK package %s has no method %q", key, m.Pkg, m.Method)
			}
		}
	}
}

// TestEveryResourceHasReadPermissions insists on total coverage: a resource
// type is either mapped to an SDK read method or explicitly listed in
// unmappedReads with a reason. Adding a resource without either fails here.
func TestEveryResourceHasReadPermissions(t *testing.T) {
	for _, r := range Resources {
		_, mapped := readMethods[r.FilterKey]
		_, listed := unmappedReads[r.FilterKey]
		if !mapped && !listed {
			t.Errorf("%s (%s): no read method mapped and not listed in unmappedReads — "+
				"add an entry to readMethods, or to unmappedReads with the reason", r.TFType, r.FilterKey)
		}
		if mapped && listed {
			t.Errorf("%s: appears in both readMethods and unmappedReads", r.FilterKey)
		}
	}
}

// TestNoStaleReadMethods catches the reverse drift: an entry for a resource
// that no longer exists, left behind when a type is removed (pro_api_client and
// pro_api_role were both removed at the Platform API GA).
func TestNoStaleReadMethods(t *testing.T) {
	known := map[string]bool{"jamf_connect": true} // SDK-discovered, no Resources entry
	for _, r := range Resources {
		known[r.FilterKey] = true
	}
	for key := range readMethods {
		if !known[key] {
			t.Errorf("readMethods has an entry for %q, which is not a known resource type", key)
		}
	}
	for key := range unmappedReads {
		if !known[key] {
			t.Errorf("unmappedReads has an entry for %q, which is not a known resource type", key)
		}
	}
}

func TestResourcePermissionsResolvesCapabilities(t *testing.T) {
	tests := []struct {
		key     string
		wantCap string
	}{
		{"device_groups", "device-groups:read"},
		{"blueprints", "blueprints:read"},
		{"compliance_benchmarks", "compliance-benchmarks:read"},
		{"ai_governance_policies", "ai-policies:read"},
		{"account_sso_domains", "sso-domains:read"},
		{"account_sso_connections", "sso-connections:read"},
		{"security_cloud_ztna_apps", "ztna:read"},
		{"security_cloud_uem_connect", "uem-connect:read"},
		{"security_cloud_dns_search_domain", "search-domains:read"},
		{"security_cloud_dns_hostname_mappings", "custom-hostname-mappings:read"},
		{"policy", "policies:read"},
		{"script", "scripts:read"},
	}
	for _, tc := range tests {
		perms, reason := ResourcePermissions(tc.key)
		if reason != "" {
			t.Errorf("%s: unexpected reason %q", tc.key, reason)
			continue
		}
		var found bool
		for _, p := range perms {
			if p.Capability == tc.wantCap {
				found = true
			}
		}
		if !found {
			var got []string
			for _, p := range perms {
				got = append(got, p.Capability)
			}
			t.Errorf("%s: want capability %s, got %v", tc.key, tc.wantCap, got)
		}
	}
}

// A patch software title resolves its source_id from both patch source
// catalogues, so the export needs their read permissions too. Without them the
// read fails 403 and blames source_id, which is the failure the GA upgrade
// guide calls out.
func TestPatchSoftwareTitleRequiresSourceCatalogues(t *testing.T) {
	perms, reason := ResourcePermissions("patch_software_title")
	if reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	want := map[string]bool{
		"patch-management-software-titles:read": false,
		"patch-external-source:read":            false,
		"patch-internal-source:read":            false,
	}
	for _, p := range perms {
		if _, ok := want[p.Capability]; ok {
			want[p.Capability] = true
		}
	}
	for cap, found := range want {
		if !found {
			t.Errorf("missing required capability %s", cap)
		}
	}
}

func TestRequiredCapabilitiesIsScopeAware(t *testing.T) {
	org := RequiredCapabilities(ScopeOrganization, nil, true)
	// Organization scope reaches the Jamf Account family and nothing else, so
	// its capability set is exactly the SSO pair.
	if len(org) != 2 {
		t.Errorf("organization scope: want 2 capabilities, got %d (%v)", len(org), org)
	}
	for _, c := range org {
		if !strings.HasPrefix(c, "sso-") {
			t.Errorf("organization scope resolved a non-account capability: %s", c)
		}
	}

	env := RequiredCapabilities(ScopeEnvironment, nil, true)
	tenant := RequiredCapabilities(ScopeTenant, nil, true)
	if len(env) <= len(tenant) {
		t.Errorf("environment scope should require more capabilities than tenant scope (env=%d tenant=%d)", len(env), len(tenant))
	}
	// Blueprints, benchmarks and AI policies are environment-only.
	for _, cap := range []string{"blueprints:read", "compliance-benchmarks:read", "ai-policies:read"} {
		if !contains(env, cap) {
			t.Errorf("environment scope missing %s", cap)
		}
		if contains(tenant, cap) {
			t.Errorf("tenant scope must not require %s", cap)
		}
	}
	// No account capability is reachable from either.
	for _, cap := range []string{"sso-domains:read", "sso-connections:read"} {
		if contains(env, cap) || contains(tenant, cap) {
			t.Errorf("%s must only be reachable at organization scope", cap)
		}
	}
}

func contains(hay []string, needle string) bool {
	return slices.Contains(hay, needle)
}

// TestEveryExtraReadHasReadPermissions is TestEveryResourceHasReadPermissions
// for the reads no Resources entry covers. Dropping a synthetic type's methods
// or the JCDS pair from the map silently publishes a capability set that cannot
// complete the export that produced it, which is exactly the failure the
// per-resource guard above prevents for the table.
func TestEveryExtraReadHasReadPermissions(t *testing.T) {
	for _, key := range []string{tIcon, tBrandingImage, tJamfConnect} {
		methods, mapped := syntheticReads[key]
		if !mapped {
			t.Errorf("%s: no syntheticReads entry", key)
			continue
		}
		if len(methods) > 0 {
			continue
		}
		// An empty method list is only honest with a note saying why nothing
		// is read through a privileged endpoint.
		if _, noted := syntheticReadNotes[key]; !noted {
			t.Errorf("%s: syntheticReads entry is empty and syntheticReadNotes has no reason for it", key)
		}
	}

	if len(packageDownloadReads) == 0 {
		t.Fatal("packageDownloadReads is empty — package downloads are on by default and read the JCDS file store")
	}
	perms, reason := permissionsForMethods(packageDownloadReads)
	if reason != "" {
		t.Fatalf("packageDownloadReads resolved no capability: %s", reason)
	}
	if !hasCapability(perms, "jamf-cloud-distribution-service-files:read") {
		t.Errorf("packageDownloadReads must require the JCDS file capability, got %v", perms)
	}
}

// The synthetic types have no Resources entry, so ResourcePermissions is the
// only way they reach PERMISSIONS.md. Before this fallback existed syntheticReads
// was read by nothing but a test.
func TestResourcePermissionsFallsBackToSyntheticReads(t *testing.T) {
	perms, reason := ResourcePermissions(tJamfConnect)
	if reason != "" {
		t.Fatalf("%s: unexpected reason %q", tJamfConnect, reason)
	}
	if !hasCapability(perms, "jamf-connect-deployments:read") {
		t.Errorf("%s: want jamf-connect-deployments:read, got %v", tJamfConnect, perms)
	}

	// Icons read through no privileged endpoint at all, and must say so rather
	// than fall through to "no read path is recorded", which reads as a gap in
	// the map.
	if perms, reason := ResourcePermissions(tIcon); len(perms) != 0 || reason == "" {
		t.Errorf("%s: want a reason and no capabilities, got %v / %q", tIcon, perms, reason)
	}
}

// An integration granted exactly the capability set in PERMISSIONS.md has to be
// able to repeat the export. Without these, Jamf Connect discovery and the JCDS
// downloads 403 on the second run, each degrading to a warning, and the export
// silently comes out smaller.
func TestRequiredCapabilitiesCoversSyntheticAndDownloadReads(t *testing.T) {
	const jcds = "jamf-cloud-distribution-service-files:read"

	withDownloads := RequiredCapabilities(ScopeTenant, nil, true)
	for _, cap := range []string{"jamf-connect-deployments:read", "self-service:read", jcds} {
		if !contains(withDownloads, cap) {
			t.Errorf("tenant scope with package downloads is missing %s", cap)
		}
	}

	// -skip-package-downloads reads no file bytes, so claiming the capability
	// would over-grant.
	if noDownloads := RequiredCapabilities(ScopeTenant, nil, false); contains(noDownloads, jcds) {
		t.Errorf("%s must not be claimed when package downloads are disabled", jcds)
	}

	// Organization scope reaches none of the federated pro surface, so it
	// performs none of these reads and must claim none of them.
	org := RequiredCapabilities(ScopeOrganization, nil, true)
	for _, cap := range []string{"jamf-connect-deployments:read", jcds} {
		if contains(org, cap) {
			t.Errorf("organization scope must not require %s", cap)
		}
	}

	// A selection that excludes packages excludes their download too.
	sel := map[string]bool{"policy": true, "jamf_connect": true}
	if got := RequiredCapabilities(ScopeTenant, sel, true); contains(got, jcds) {
		t.Errorf("%s must not be claimed when packages are not selected", jcds)
	}
}

// PERMISSIONS.md itself must carry the rows, not just the capability block —
// the per-resource table is how a reader checks the claim.
func TestWritePermissionsFileListsExtraReads(t *testing.T) {
	dir := t.TempDir()
	if err := WritePermissionsFile(dir, Scope{Kind: ScopeTenant, ID: "t1"}, nil, true); err != nil {
		t.Fatalf("WritePermissionsFile: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "PERMISSIONS.md"))
	if err != nil {
		t.Fatalf("reading PERMISSIONS.md: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		tIcon,
		tBrandingImage,
		tJamfConnect,
		"JCDS file download",
		"jamf-cloud-distribution-service-files:read",
		"/v1/jcds/files",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("PERMISSIONS.md does not mention %q", want)
		}
	}

	skipDir := t.TempDir()
	if err := WritePermissionsFile(skipDir, Scope{Kind: ScopeTenant, ID: "t1"}, nil, false); err != nil {
		t.Fatalf("WritePermissionsFile: %v", err)
	}
	skipped, err := os.ReadFile(filepath.Join(skipDir, "PERMISSIONS.md"))
	if err != nil {
		t.Fatalf("reading PERMISSIONS.md: %v", err)
	}
	if strings.Contains(string(skipped), "jamf-cloud-distribution-service-files:read") {
		t.Error("PERMISSIONS.md claims the JCDS capability on a -skip-package-downloads run")
	}
}

func hasCapability(perms []Permission, cap string) bool {
	for _, p := range perms {
		if p.Capability == cap {
			return true
		}
	}
	return false
}
