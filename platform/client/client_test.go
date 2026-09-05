// Copyright 2026, Jamf Software LLC

package client

import (
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
)

// The scope a Scope resolves to is asserted through the SDK client it is used to
// build, not through the length of the option slice options() returns. A count
// only says how many options were produced; Client.Scope() says which scope the
// SDK actually bound and which ID it will send, and ScopeKind.ScopeHeader()
// names the header that carries it. Those are the properties the gateway sees,
// and the ones a wrong arm of the switch would get wrong while still returning
// one option.
//
// No request is made: NewClient only constructs the transport, so the header a
// scope selects can be read off it without credentials or a network.
func TestScopeSelectsTheSDKScopeHeader(t *testing.T) {
	tests := []struct {
		name       string
		scope      Scope
		wantKind   jamfplatform.ScopeKind
		wantID     string
		wantHeader string
	}{
		{
			name:       "environment scope sends X-Environment-Id",
			scope:      Scope{EnvironmentID: "e1"},
			wantKind:   jamfplatform.ScopeEnvironment,
			wantID:     "e1",
			wantHeader: "X-Environment-Id",
		},
		{
			name:       "tenant scope sends X-Tenant-Id",
			scope:      Scope{TenantID: "t1"},
			wantKind:   jamfplatform.ScopeTenant,
			wantID:     "t1",
			wantHeader: "X-Tenant-Id",
		},
		{
			// Sending nothing is the whole mechanism for organization scope:
			// the gateway resolves the organization from the access token, so a
			// scope header — either one — would target something else entirely.
			name:       "organization scope sends no scope header at all",
			scope:      Scope{},
			wantKind:   jamfplatform.ScopeOrganization,
			wantID:     "",
			wantHeader: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := jamfplatform.NewClient("https://us.api.jamfcloud.com", "id", "secret", tc.scope.options()...)
			kind, id := c.Scope()
			if kind != tc.wantKind {
				t.Errorf("scope kind = %v, want %v", kind, tc.wantKind)
			}
			if id != tc.wantID {
				t.Errorf("scope ID = %q, want %q", id, tc.wantID)
			}
			if h := kind.ScopeHeader(); h != tc.wantHeader {
				t.Errorf("scope header = %q, want %q", h, tc.wantHeader)
			}
		})
	}
}

// The two IDs are mutually exclusive upstream — platform.ResolveScope rejects
// the pair before a Scope is ever built — so options() is never handed both in
// production. It is asserted here anyway to pin which arm wins if that guard is
// ever bypassed: environment, the preferred scope, rather than the legacy one.
func TestEnvironmentWinsIfBothIDsAreSomehowSet(t *testing.T) {
	c := jamfplatform.NewClient("https://us.api.jamfcloud.com", "id", "secret",
		Scope{EnvironmentID: "e1", TenantID: "t1"}.options()...)
	kind, id := c.Scope()
	if kind != jamfplatform.ScopeEnvironment || id != "e1" {
		t.Errorf("got scope %v/%q, want environment/e1 — the preferred scope must win", kind, id)
	}
}

// A scope contributing an option at all is what distinguishes the two
// header-bearing scopes from organization scope, which must contribute none.
// The count is a weaker check than the header assertions above, so it is kept
// separate: it is the one that fails if an arm of the switch stops appending
// rather than appending the wrong thing.
func TestOnlyHeaderBearingScopesContributeOptions(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		want  int
	}{
		{"environment", Scope{EnvironmentID: "e1"}, 1},
		{"tenant", Scope{TenantID: "t1"}, 1},
		{"organization", Scope{}, 0},
	}
	for _, tc := range tests {
		if got := len(tc.scope.options()); got != tc.want {
			t.Errorf("%s scope produced %d SDK options, want %d", tc.name, got, tc.want)
		}
	}
}
