// Copyright 2026, Jamf Software LLC

package platform

import (
	"strings"
	"testing"
)

// clearScopeEnv unsets every variable ResolveScope consults, so a case states
// its whole input. Without it the answer depends on the developer's shell: an
// exported JAMF_TENANT_ID turns the "environment only" case into the conflict
// case, and an exported JAMFPLATFORM_ENVIRONMENT_ID turns the organization case
// into an environment one.
func clearScopeEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"JAMF_ENVIRONMENT_ID", "JAMFPLATFORM_ENVIRONMENT_ID",
		"JAMF_TENANT_ID", "JAMFPLATFORM_TENANT_ID",
		"JAMF_ORGANIZATION_ID", "JAMFPLATFORM_ORGANIZATION_ID",
	} {
		t.Setenv(name, "")
	}
}

// ResolveScope is the one place the run's scope is decided, and getting it wrong
// is quiet rather than loud: a run that resolves organization scope instead of
// environment scope authenticates, queries the two Jamf Account types, exports
// them, and reports success — having skipped the other 97. So the table below
// covers each arm of the switch and each way an identifier can arrive, and
// asserts the resolved kind and ID rather than only that no error came back.
func TestResolveScope(t *testing.T) {
	tests := []struct {
		name string
		// argEnv and argTenant are the explicit parameters; env is what the
		// process environment holds.
		argEnv, argTenant string
		env               map[string]string
		wantKind          ScopeKind
		wantID            string
		// wantErrContains is non-empty for the conflict cases. The message is
		// what tells a user which variable to unset, so a fragment of it is
		// asserted rather than merely that an error occurred.
		wantErrContains string
	}{
		{
			name:     "an environment ID alone is environment scope",
			env:      map[string]string{"JAMF_ENVIRONMENT_ID": "env-1"},
			wantKind: ScopeEnvironment,
			wantID:   "env-1",
		},
		{
			name:     "a tenant ID alone is the legacy tenant scope",
			env:      map[string]string{"JAMF_TENANT_ID": "tenant-1"},
			wantKind: ScopeTenant,
			wantID:   "tenant-1",
		},
		{
			// Not a fallback or a default: setting neither identifier is the
			// documented way to select organization scope, and it is how the
			// provider itself selects it.
			name:     "neither identifier is organization scope, carrying no ID",
			wantKind: ScopeOrganization,
			wantID:   "",
		},
		{
			name:     "an explicitly named organization ID is still organization scope",
			env:      map[string]string{"JAMF_ORGANIZATION_ID": "org-1"},
			wantKind: ScopeOrganization,
			// The organization ID is never sent — it exists only to state the
			// choice — so it must not end up in the resolved scope.
			wantID: "",
		},
		{
			name: "both identifiers together are rejected",
			env: map[string]string{
				"JAMF_ENVIRONMENT_ID": "env-1",
				"JAMF_TENANT_ID":      "tenant-1",
			},
			wantErrContains: "both an environment ID and a tenant ID are set",
		},
		{
			name: "an environment ID with an organization ID is rejected",
			env: map[string]string{
				"JAMF_ENVIRONMENT_ID":  "env-1",
				"JAMF_ORGANIZATION_ID": "org-1",
			},
			wantErrContains: "an environment ID and an organization ID are both set",
		},
		{
			name: "a tenant ID with an organization ID is rejected",
			env: map[string]string{
				"JAMF_TENANT_ID":       "tenant-1",
				"JAMF_ORGANIZATION_ID": "org-1",
			},
			wantErrContains: "a tenant ID and an organization ID are both set",
		},
		{
			// A shell already set up to run `terraform` exports the provider's
			// own names; jamformer must not ask for a second set.
			name:     "the provider's JAMFPLATFORM_ENVIRONMENT_ID is honoured",
			env:      map[string]string{"JAMFPLATFORM_ENVIRONMENT_ID": "env-2"},
			wantKind: ScopeEnvironment,
			wantID:   "env-2",
		},
		{
			name:     "the provider's JAMFPLATFORM_TENANT_ID is honoured",
			env:      map[string]string{"JAMFPLATFORM_TENANT_ID": "tenant-2"},
			wantKind: ScopeTenant,
			wantID:   "tenant-2",
		},
		{
			name: "jamformer's own name wins over the provider's",
			env: map[string]string{
				"JAMF_ENVIRONMENT_ID":         "env-mine",
				"JAMFPLATFORM_ENVIRONMENT_ID": "env-theirs",
			},
			wantKind: ScopeEnvironment,
			wantID:   "env-mine",
		},
		{
			name: "jamformer's own tenant name wins over the provider's",
			env: map[string]string{
				"JAMF_TENANT_ID":         "tenant-mine",
				"JAMFPLATFORM_TENANT_ID": "tenant-theirs",
			},
			wantKind: ScopeTenant,
			wantID:   "tenant-mine",
		},
		{
			// An exported-but-blank variable is how a shell that once set a
			// scope looks after it stops; treating it as set would resolve a
			// scope with an empty ID and fail at the gateway instead.
			name:     "a whitespace-only value is treated as unset",
			env:      map[string]string{"JAMF_ENVIRONMENT_ID": "   "},
			wantKind: ScopeOrganization,
			wantID:   "",
		},
		{
			name: "a whitespace-only value falls through to the provider's name",
			env: map[string]string{
				"JAMF_ENVIRONMENT_ID":         "  \t ",
				"JAMFPLATFORM_ENVIRONMENT_ID": "env-3",
			},
			wantKind: ScopeEnvironment,
			wantID:   "env-3",
		},
		{
			// A whitespace-only tenant variable must not turn an environment
			// run into the conflict case.
			name: "a whitespace-only tenant value is not a conflict",
			env: map[string]string{
				"JAMF_ENVIRONMENT_ID": "env-1",
				"JAMF_TENANT_ID":      " ",
			},
			wantKind: ScopeEnvironment,
			wantID:   "env-1",
		},
		{
			name:     "an explicit parameter is used without consulting the environment",
			argEnv:   "env-arg",
			env:      map[string]string{"JAMF_ENVIRONMENT_ID": "env-ignored"},
			wantKind: ScopeEnvironment,
			wantID:   "env-arg",
		},
		{
			name:      "an explicit tenant parameter conflicts with an environment variable",
			argTenant: "tenant-arg",
			env:       map[string]string{"JAMF_ENVIRONMENT_ID": "env-1"},
			// The parameters and the environment are one input, not two: a
			// tenant passed on the command line still conflicts with an
			// environment ID that only the shell knows about.
			wantErrContains: "both an environment ID and a tenant ID are set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearScopeEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got, err := ResolveScope(tc.argEnv, tc.argTenant)
			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected a conflict error, got scope %s/%q", got.Kind, got.ID)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrContains)
				}
				// A rejected run must not carry a usable scope out of the
				// error path, in case a caller checks the value first.
				if got.ID != "" {
					t.Errorf("a rejected resolution returned an ID: %q", got.ID)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %s, want %s", got.Kind, tc.wantKind)
			}
			if got.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tc.wantID)
			}
			// The two accessors are what the client and importgen read, and
			// each must be empty for every kind but its own — an environment ID
			// leaking out of TenantID would write the wrong provider attribute.
			if want := tc.wantID; tc.wantKind == ScopeEnvironment && got.EnvironmentID() != want {
				t.Errorf("EnvironmentID() = %q, want %q", got.EnvironmentID(), want)
			}
			if tc.wantKind != ScopeEnvironment && got.EnvironmentID() != "" {
				t.Errorf("EnvironmentID() = %q on a %s scope, want empty", got.EnvironmentID(), got.Kind)
			}
			if want := tc.wantID; tc.wantKind == ScopeTenant && got.TenantID() != want {
				t.Errorf("TenantID() = %q, want %q", got.TenantID(), want)
			}
			if tc.wantKind != ScopeTenant && got.TenantID() != "" {
				t.Errorf("TenantID() = %q on a %s scope, want empty", got.TenantID(), got.Kind)
			}
		})
	}
}

// ProviderAttr picks the provider attribute importgen writes, and the empty
// string for organization scope is load-bearing rather than a gap: setting
// neither attribute is what selects that scope, so a name here would resolve
// the wrong one.
func TestScopeKindNamesAndProviderAttrs(t *testing.T) {
	tests := []struct {
		kind     ScopeKind
		wantName string
		wantAttr string
	}{
		{ScopeEnvironment, "environment", "environment_id"},
		{ScopeTenant, "tenant", "tenant_id"},
		{ScopeOrganization, "organization", ""},
	}
	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.wantName {
			t.Errorf("String() = %q, want %q", got, tc.wantName)
		}
		if got := tc.kind.ProviderAttr(); got != tc.wantAttr {
			t.Errorf("%s.ProviderAttr() = %q, want %q", tc.wantName, got, tc.wantAttr)
		}
	}
	// An unrecognised kind must name itself rather than pass for a real scope,
	// since the name reaches -help text and diagnostics.
	if got := ScopeKind(99).String(); got != "unknown" {
		t.Errorf("an unhandled kind rendered as %q, want %q", got, "unknown")
	}
	if got := ScopeKind(99).ProviderAttr(); got != "" {
		t.Errorf("an unhandled kind named the attribute %q, want none", got)
	}
}

// ReachesPro gates package downloads, Jamf Connect discovery and Self Service
// branding images. Only organization scope misses the federated pro surface;
// getting this wrong either skips those silently or attempts them against a
// credential that cannot reach them.
func TestReachesProCoversEveryScopeKind(t *testing.T) {
	tests := []struct {
		scope Scope
		want  bool
	}{
		{Scope{Kind: ScopeEnvironment, ID: "env-1"}, true},
		{Scope{Kind: ScopeTenant, ID: "tenant-1"}, true},
		{Scope{Kind: ScopeOrganization}, false},
	}
	for _, tc := range tests {
		if got := tc.scope.ReachesPro(); got != tc.want {
			t.Errorf("%s scope: ReachesPro() = %v, want %v", tc.scope.Kind, got, tc.want)
		}
	}
}

// Describe renders a resource's scope set for -list-resources and for the
// skipped-types report, so it is what a user reads when a type they asked for
// was not exported. Each set is covered, environment first because that is the
// scope Jamf intends new integrations to carry.
func TestScopeSetDescribe(t *testing.T) {
	tests := []struct {
		set  ScopeSet
		want string
	}{
		{ScopeSetEnvOnly, "environment"},
		{ScopeSetTenantOrEnv, "environment, tenant"},
		{ScopeSetOrgOnly, "organization"},
	}
	for _, tc := range tests {
		if got := tc.set.Describe(); got != tc.want {
			t.Errorf("Describe() = %q, want %q", got, tc.want)
		}
	}
	// A zero set reaches nothing, and must render as nothing rather than as one
	// of the real scopes. TestEveryResourceDeclaresScopes keeps it out of the
	// table; this keeps it out of the output if it ever gets in.
	if got := ScopeSet(0).Describe(); got != "" {
		t.Errorf("the empty set rendered as %q, want empty", got)
	}
}

// clientScope is how the resolved scope reaches the SDK. Each kind must hand
// over exactly its own identifier and nothing else: crossing the two over is a
// 403 OWNERSHIP_FORBIDDEN even within one customer, and an organization scope
// carrying either ID would send a header the gateway does not expect.
func TestClientScopeCarriesOnlyItsOwnIdentifier(t *testing.T) {
	env := Scope{Kind: ScopeEnvironment, ID: "env-1"}.clientScope()
	if env.EnvironmentID != "env-1" || env.TenantID != "" {
		t.Errorf("environment scope produced %+v", env)
	}
	tenant := Scope{Kind: ScopeTenant, ID: "tenant-1"}.clientScope()
	if tenant.TenantID != "tenant-1" || tenant.EnvironmentID != "" {
		t.Errorf("tenant scope produced %+v", tenant)
	}
	org := Scope{Kind: ScopeOrganization}.clientScope()
	if org.EnvironmentID != "" || org.TenantID != "" {
		t.Errorf("organization scope must carry no identifier, produced %+v", org)
	}
}
