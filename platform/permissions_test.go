// Copyright 2026, Jamf Software LLC

package platform

import (
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
	org := RequiredCapabilities(ScopeOrganization, nil)
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

	env := RequiredCapabilities(ScopeEnvironment, nil)
	tenant := RequiredCapabilities(ScopeTenant, nil)
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
	for _, v := range hay {
		if v == needle {
			return true
		}
	}
	return false
}
