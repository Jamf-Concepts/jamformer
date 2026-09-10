// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
)

// ResourceDef.Scopes is written out by hand on every entry in the Resources
// table while the SDK carries the same answer: MethodPrivileges.Scopes, the
// scope kinds each endpoint accepts, arrived in jamfplatform-go-sdk v0.22.0 and
// permissions.go already reads the registries that hold it.
//
// The privilege half of the table is SDK-derived and pinned by
// TestReadMethodsResolve. The scope half is a literal, so a spec ingest that
// narrows or widens an API family reaches jamformer as nothing at all: a
// narrowed family goes on being queried until the provider refuses it at
// configure time and takes the whole run down, and a widened one stays skipped
// with a report line saying the scope cannot reach it. This test is the missing
// half of that pinning.
//
// It is a cross-check, not a derivation. The table is deliberately wider than
// the spec in one place, and the allowlist below is why.

// scopeWidening records a resource whose table entry is deliberately wider than
// the SDK's declared scope set, because the gateway goes on serving a scope the
// published specification withdrew.
//
// It is the jamformer-side counterpart of gatewayWidenings in the provider's
// internal/providerdata/scopes.go, and carries the same obligation: an entry is
// an assertion about the gateway, evidenced on the wire on a stated date, and it
// is deleted rather than edited when either half of its justification goes. The
// spec catching up is caught here, by
// TestResourceScopesMatchTheSDKDeclaredScopes rejecting a widening the registry
// now declares outright. The gateway following the withdrawal cannot be caught
// from here — it needs a probe — so each entry names the probe to re-run rather
// than describing one.
type scopeWidening struct {
	// filterKey is the ResourceDef the widening applies to.
	filterKey string
	// scope is the kind the table adds on top of what the SDK declares.
	scope ScopeKind
	// why states the wire evidence and where to re-probe it.
	why string
}

// scopeWidenings are the resources whose spec-declared scope set understates
// what the gateway serves.
//
// One entry, and it is the Platform API GA's tenant withdrawal
// (public-apis-oas#437): the device-groups spec stripped X-Tenant-Id while the
// gateway went on serving it.
//
// Blueprints, Compliance Benchmarks and AI Governance withdrew tenant on the
// same GA build and are deliberately absent, so do not add them by symmetry:
// Jamf Account's permission picker does not offer their capabilities at all to a
// tenant-scoped integration, so no tenant credential can hold them whatever the
// route does. Narrowing those three takes away no configuration a practitioner
// could have written. The provider's own gatewayWidenings comment records that
// reasoning in full, and is the place to read before touching this list.
var scopeWidenings = []scopeWidening{
	{
		filterKey: "device_groups",
		scope:     ScopeTenant,
		why: "GET /device-groups/v1/groups answered 200 with 53 rows under X-Tenant-Id on " +
			"2026-09-04, alongside a served control in the same invocation. Re-probe by running " +
			"the SDK's TestAcceptance_TenantScopePlatformSpecsStillServed rather than " +
			"reconstructing it; the provider mirrors this as the \"Platform device groups\" entry " +
			"in internal/providerdata/scopes.go.",
	},
}

// TestResourceScopesMatchTheSDKDeclaredScopes cross-checks every hand-written
// ResourceDef.Scopes against the scope set the SDK declares for that resource's
// read endpoints, allowing only the widenings named above.
func TestResourceScopesMatchTheSDKDeclaredScopes(t *testing.T) {
	var skipped []string

	for _, r := range Resources {
		methods, mapped := readMethods[r.FilterKey]
		if !mapped {
			// No read path to check against. TestEveryResourceHasReadPermissions
			// insists the type is listed in unmappedReads with a reason, and the
			// skip list is bounded below so a new one cannot quietly escape.
			skipped = append(skipped, r.FilterKey)
			continue
		}

		declared, err := sdkDeclaredScopes(methods)
		if err != nil {
			t.Errorf("%s (%s): %v", r.TFType, r.FilterKey, err)
			continue
		}

		want := slices.Clone(declared)
		for _, w := range scopeWidenings {
			if w.filterKey != r.FilterKey {
				continue
			}
			if slices.Contains(declared, w.scope) {
				t.Errorf("%s: the %s widening is redundant — the SDK registry now declares %s "+
					"outright. Delete the entry; the resolved set does not change either way, and "+
					"a redundant entry reads at the table as though this scope rested on "+
					"jamformer's own wire evidence.", r.FilterKey, w.scope, w.scope)
				continue
			}
			want = append(want, w.scope)
		}

		got := tableScopeKinds(r.Scopes)
		if !sameScopeKinds(got, want) {
			t.Errorf("%s (%s): Scopes reaches %s, but the SDK declares %s%s.\n"+
				"Either the table is stale — a spec ingest moved this family, so update "+
				"ResourceDef.Scopes — or the gateway is serving a scope the spec withdrew, in "+
				"which case add a scopeWidenings entry naming the probe that showed it.",
				r.TFType, r.FilterKey, scopeKindNames(got), scopeKindNames(declared), widenedSuffix(r.FilterKey))
		}
	}

	// The skip list is asserted rather than merely tolerated: a resource with no
	// read path escapes this check entirely, so the set has to stay small and
	// named. Both current entries publish no GET the SDK can source a scope
	// from, which is exactly why they carry an unmappedReads reason.
	wantSkipped := []string{"login_page_settings", "volume_purchasing_notification"}
	slices.Sort(skipped)
	if !slices.Equal(skipped, wantSkipped) {
		t.Errorf("the cross-check skipped %v, want exactly %v — a resource with no SDK read "+
			"method is unchecked here, so add it to unmappedReads with a reason and to this "+
			"list, or map it in readMethods", skipped, wantSkipped)
	}
	for _, key := range skipped {
		if _, listed := unmappedReads[key]; !listed {
			t.Errorf("%s is skipped by the scope cross-check but carries no unmappedReads reason", key)
		}
	}
}

// TestScopeWideningsCarryEvidence keeps the allowlist from decaying into a list
// of exceptions: an entry that names no resource widens nothing, silently, and
// an entry with no stated evidence cannot be re-probed — which means it will be
// deleted while still load-bearing, or kept long after it is wrong.
func TestScopeWideningsCarryEvidence(t *testing.T) {
	known := map[string]bool{}
	for _, r := range Resources {
		known[r.FilterKey] = true
	}
	for _, w := range scopeWidenings {
		if !known[w.filterKey] {
			t.Errorf("scopeWidenings names %q, which is not a resource in the table — a widening "+
				"for a key that matches nothing looks exactly like a family the gateway never "+
				"diverged on", w.filterKey)
		}
		if strings.TrimSpace(w.why) == "" {
			t.Errorf("the %s / %s widening carries no wire evidence", w.filterKey, w.scope)
		}
		if _, mapped := readMethods[w.filterKey]; !mapped {
			t.Errorf("scopeWidenings widens %q, which the cross-check skips for want of a read "+
				"method — the widening can never be checked", w.filterKey)
		}
	}
}

// sdkDeclaredScopes resolves the scope kinds a single credential can use to
// reach every one of a resource's read endpoints.
//
// Scopes is an ALTERNATIVES set per method — the endpoint is published at each
// kind listed and the caller picks one, because a client carries exactly one
// scope. A resource read through several methods on one client therefore needs
// the INTERSECTION across them, not the union: a scope that reaches only some
// of its endpoints fails the read.
func sdkDeclaredScopes(methods []readMethod) ([]ScopeKind, error) {
	var accepted []ScopeKind
	seen := false
	for _, m := range methods {
		mp, ok := privilegeSets[m.Pkg][m.Method]
		if !ok {
			// TestReadMethodsResolve owns this failure and reports it per
			// method; repeating it here would double every message.
			continue
		}
		kinds, err := scopeKindsFromSDK(m, mp.Scopes)
		if err != nil {
			return nil, err
		}
		if !seen {
			accepted, seen = kinds, true
			continue
		}
		accepted = slices.DeleteFunc(accepted, func(k ScopeKind) bool { return !slices.Contains(kinds, k) })
	}
	if !seen {
		return nil, fmt.Errorf("no read method resolved in the SDK registries")
	}
	if len(accepted) == 0 {
		return nil, fmt.Errorf("no single scope reaches every read endpoint — the specs behind " +
			"them disagree, so this resource cannot be exported by any one credential")
	}
	return accepted, nil
}

// scopeKindsFromSDK maps the SDK's scope kinds onto jamformer's, deduplicated.
//
// An unrecognised kind is an error rather than a default. The SDK models
// organization as its zero value, so falling through would map a fourth kind it
// grew onto organization scope — widening a resource to a credential its
// endpoint does not accept, which is the failure this cross-check exists to
// catch.
func scopeKindsFromSDK(m readMethod, sdkKinds []jamfplatform.ScopeKind) ([]ScopeKind, error) {
	if len(sdkKinds) == 0 {
		// The SDK's generator fails rather than emitting an empty set, so this
		// is a contract break upstream, not an absent requirement.
		return nil, fmt.Errorf("%s.%s declares no scope at all", m.Pkg, m.Method)
	}
	out := make([]ScopeKind, 0, len(sdkKinds))
	for _, s := range sdkKinds {
		var k ScopeKind
		switch s {
		case jamfplatform.ScopeOrganization:
			k = ScopeOrganization
		case jamfplatform.ScopeEnvironment:
			k = ScopeEnvironment
		case jamfplatform.ScopeTenant:
			k = ScopeTenant
		default:
			return nil, fmt.Errorf("%s.%s declares SDK scope kind %d, which jamformer has no "+
				"ScopeKind for — add one rather than letting it default", m.Pkg, m.Method, s)
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out, nil
}

// tableScopeKinds expands a ScopeSet into the kinds it makes a resource
// reachable at, which is the vocabulary the SDK's Scopes speaks.
func tableScopeKinds(s ScopeSet) []ScopeKind {
	var out []ScopeKind
	for _, k := range []ScopeKind{ScopeEnvironment, ScopeTenant, ScopeOrganization} {
		if s.Reachable(k) {
			out = append(out, k)
		}
	}
	return out
}

func sameScopeKinds(a, b []ScopeKind) bool {
	if len(a) != len(b) {
		return false
	}
	for _, k := range a {
		if !slices.Contains(b, k) {
			return false
		}
	}
	return true
}

func scopeKindNames(kinds []ScopeKind) string {
	if len(kinds) == 0 {
		return "nothing"
	}
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k.String())
	}
	return strings.Join(out, ", ")
}

// widenedSuffix names the allowed widening in a mismatch message, so a failure
// reports the set actually expected rather than leaving the reader to find the
// allowlist entry themselves.
func widenedSuffix(filterKey string) string {
	var extra []string
	for _, w := range scopeWidenings {
		if w.filterKey == filterKey {
			extra = append(extra, w.scope.String())
		}
	}
	if len(extra) == 0 {
		return ""
	}
	return " (plus the allowed widening to " + strings.Join(extra, ", ") + ")"
}
