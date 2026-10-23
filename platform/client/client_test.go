// Copyright 2026, Jamf Software LLC

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
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

// The SDK's default inter-request interval gates request starts at 100ms, a
// 10 req/s ceiling. The clients jamformer builds turn it off (gateway
// rate-limit pacing stays on regardless), so serial requests against a gateway
// that reports no rate limit are not spaced at all. At the SDK default the
// requests below would take at least 1.9s; unpaced they take milliseconds.
func TestClientsAreNotPacedBetweenRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":0,"results":[]}`))
	}))
	defer srv.Close()

	pc := NewProClient(srv.URL, "id", "secret", Scope{EnvironmentID: "e1"})
	const requests = 20
	start := time.Now()
	for range requests {
		if _, err := pc.ListPackagesV1(context.Background(), nil, ""); err != nil {
			t.Fatalf("ListPackagesV1: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("%d requests took %v; the client is still paced between requests", requests, elapsed)
	}
}

// A gateway that accepts a request and never answers must not hang the export:
// the SDK sets no response-header timeout by default (since v1.3.0) and
// jamformer's contexts carry no deadline, so the client's own bound is the only
// thing that ends the wait.
func TestStalledGatewayTimesOut(t *testing.T) {
	prev := responseHeaderTimeout
	responseHeaderTimeout = 200 * time.Millisecond
	t.Cleanup(func() { responseHeaderTimeout = prev })

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(6 * time.Second): // fail the test rather than hang it
		}
	}))
	defer srv.Close()
	defer close(release)

	pc := NewProClient(srv.URL, "id", "secret", Scope{EnvironmentID: "e1"})
	start := time.Now()
	_, err := pc.ListPackagesV1(context.Background(), nil, "")
	if err == nil {
		t.Fatal("ListPackagesV1 against a stalled gateway returned no error")
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("stalled request took %v to fail; the response-header timeout is not applied", elapsed)
	}
}
