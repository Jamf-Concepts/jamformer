// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/platform/client"
)

// ScopeKind is the scope an API integration was registered with. It decides
// which construct families the credentials can reach, and which HTTP header the
// provider (and the SDK) sends to identify the caller's target.
//
// The Jamf Platform API reached GA on 3 September 2026. Scope now travels in a
// request header rather than a URL path, and three kinds exist. An integration
// carries exactly one: it is chosen at registration in Jamf Account and cannot
// be changed afterwards.
type ScopeKind int

const (
	// ScopeEnvironment targets a "Platform environment" — a group of tenants
	// across product types. It sends X-Environment-Id. This is the preferred
	// scope for new integrations and the only one that reaches Blueprints,
	// Compliance Benchmarks and AI Governance.
	ScopeEnvironment ScopeKind = iota
	// ScopeTenant targets a single Jamf Pro / Jamf School / Jamf Protect /
	// Jamf Security Cloud tenant. It sends X-Tenant-Id. Legacy: it costs one
	// integration per tenant per product, and cannot reach Blueprints,
	// Compliance Benchmarks or AI Governance at all.
	ScopeTenant
	// ScopeOrganization targets the Jamf Account organization. It sends no
	// scope header at all — the gateway resolves the organization from the
	// access token — and is selected by supplying neither an environment nor a
	// tenant ID. It is the only scope that reaches jamfplatform_account_*, and
	// reaches nothing else.
	ScopeOrganization
)

// String returns the scope's name as used in messages and the -help text.
func (k ScopeKind) String() string {
	switch k {
	case ScopeEnvironment:
		return "environment"
	case ScopeTenant:
		return "tenant"
	case ScopeOrganization:
		return "organization"
	}
	return "unknown"
}

// ProviderAttr returns the provider attribute that selects this scope, or ""
// for organization scope, which is selected by setting neither attribute.
func (k ScopeKind) ProviderAttr() string {
	switch k {
	case ScopeEnvironment:
		return "environment_id"
	case ScopeTenant:
		return "tenant_id"
	}
	return ""
}

// Scope is the resolved scope for a run: its kind plus the identifier the
// gateway is given (empty for organization scope).
type Scope struct {
	Kind ScopeKind
	ID   string
}

// EnvironmentID returns the environment ID, or "" when the scope is not an
// environment scope.
func (s Scope) EnvironmentID() string {
	if s.Kind == ScopeEnvironment {
		return s.ID
	}
	return ""
}

// TenantID returns the tenant ID, or "" when the scope is not a tenant scope.
func (s Scope) TenantID() string {
	if s.Kind == ScopeTenant {
		return s.ID
	}
	return ""
}

// ReachesPro reports whether this scope reaches the federated Jamf Pro surface.
// The pro endpoints are published at both environment and tenant scope, so
// everything but organization scope reaches them. Package downloads, Jamf
// Connect discovery and Self Service branding images all depend on this.
func (s Scope) ReachesPro() bool { return s.Kind != ScopeOrganization }

// ScopeSet is the set of scope kinds a resource type is reachable at. A run
// reaches the resource when its own scope is a member.
type ScopeSet uint8

const (
	// ScopeSetEnvOnly marks a family only an environment-scoped integration
	// reaches: Blueprints, Compliance Benchmarks, AI Governance. The provider
	// refuses these at configure time under any other scope.
	ScopeSetEnvOnly ScopeSet = 1 << iota
	// ScopeSetTenantOrEnv marks a family reachable under either environment or
	// tenant scope: the federated Jamf Pro surface, Jamf Security Cloud, and
	// the Platform inventory constructs (device groups included).
	ScopeSetTenantOrEnv
	// ScopeSetOrgOnly marks the Jamf Account family, which only an
	// organization-scoped integration reaches — and which is the only family
	// that scope reaches.
	ScopeSetOrgOnly
)

// Reachable reports whether a resource carrying this ScopeSet can be exported
// under the given scope.
func (s ScopeSet) Reachable(k ScopeKind) bool {
	switch k {
	case ScopeEnvironment:
		return s&(ScopeSetEnvOnly|ScopeSetTenantOrEnv) != 0
	case ScopeTenant:
		return s&ScopeSetTenantOrEnv != 0
	case ScopeOrganization:
		return s&ScopeSetOrgOnly != 0
	}
	return false
}

// Describe renders the scopes in a ScopeSet for display, e.g.
// "environment, tenant".
func (s ScopeSet) Describe() string {
	var parts []string
	if s&ScopeSetEnvOnly != 0 || s&ScopeSetTenantOrEnv != 0 {
		parts = append(parts, "environment")
	}
	if s&ScopeSetTenantOrEnv != 0 {
		parts = append(parts, "tenant")
	}
	if s&ScopeSetOrgOnly != 0 {
		parts = append(parts, "organization")
	}
	return strings.Join(parts, ", ")
}

// ResolveScope determines the run's scope from explicit values, falling back to
// the environment. Both jamformer's own JAMF_* names and the provider's own
// JAMFPLATFORM_* names are accepted, so credentials already exported for
// `terraform` work unchanged; the JAMF_* name wins where both are set.
//
// The two identifiers are mutually exclusive, matching the provider, which
// rejects both together with a "Conflicting API Integration Scope" error.
// Supplying neither selects organization scope, which is how the provider
// itself selects it. JAMFPLATFORM_ORGANIZATION_ID is accepted as an explicit
// way to ask for organization scope: it is not a provider attribute and is
// never sent, but naming it beats leaving the choice implicit in the absence of
// two other variables.
func ResolveScope(environmentID, tenantID string) (Scope, error) {
	if environmentID == "" {
		environmentID = firstEnv("JAMF_ENVIRONMENT_ID", "JAMFPLATFORM_ENVIRONMENT_ID")
	}
	if tenantID == "" {
		tenantID = firstEnv("JAMF_TENANT_ID", "JAMFPLATFORM_TENANT_ID")
	}
	orgID := firstEnv("JAMF_ORGANIZATION_ID", "JAMFPLATFORM_ORGANIZATION_ID")

	switch {
	case environmentID != "" && tenantID != "":
		return Scope{}, fmt.Errorf("conflicting API integration scope: both an environment ID and a tenant ID are set, " +
			"but an integration targets one or the other. Unset JAMF_TENANT_ID / JAMFPLATFORM_TENANT_ID to use " +
			"environment scope (preferred), or unset the environment ID to use the legacy tenant scope")
	case environmentID != "":
		if orgID != "" {
			return Scope{}, fmt.Errorf("conflicting API integration scope: an environment ID and an organization ID are both set. " +
				"Organization scope reaches only the jamfplatform_account_* family and is selected by setting no " +
				"environment or tenant ID at all; unset one of the two")
		}
		return Scope{Kind: ScopeEnvironment, ID: environmentID}, nil
	case tenantID != "":
		if orgID != "" {
			return Scope{}, fmt.Errorf("conflicting API integration scope: a tenant ID and an organization ID are both set. " +
				"Organization scope reaches only the jamfplatform_account_* family and is selected by setting no " +
				"environment or tenant ID at all; unset one of the two")
		}
		return Scope{Kind: ScopeTenant, ID: tenantID}, nil
	default:
		// Neither identifier: organization scope. The gateway resolves the
		// organization from the access token, so no ID is carried.
		return Scope{Kind: ScopeOrganization}, nil
	}
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// clientScope converts a Scope into the platform/client package's own scope
// type, which mirrors this one so that the client package stays a leaf and does
// not import platform.
func (s Scope) clientScope() client.Scope {
	return client.Scope{EnvironmentID: s.EnvironmentID(), TenantID: s.TenantID()}
}

// scopedSelection narrows a resource selection to the types the given scope can
// reach. A nil selection means "everything", which becomes an explicit set of
// the reachable types rather than staying nil — downstream code reads a nil
// selection as no filter at all, and that would put unreachable list blocks
// back into the query file.
func scopedSelection(k ScopeKind, selected map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for _, r := range ResourcesForScope(k) {
		if selected == nil || selected[r.FilterKey] {
			out[r.FilterKey] = true
		}
	}
	// jamf_connect carries no Resources entry (it is SDK-discovered, not
	// query-listable) but is independently selectable, and rides on the
	// federated pro surface.
	if (selected == nil || selected["jamf_connect"]) && k != ScopeOrganization {
		out["jamf_connect"] = true
	}
	return out
}
