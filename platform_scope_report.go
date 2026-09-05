// Copyright 2026, Jamf Software LLC

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/platform"
)

// isPlatformProvider reports whether the -provider value selects Jamf Platform,
// which is also the default when the flag is unset.
func isPlatformProvider(providerFlag string) bool {
	v := providerFlag
	if v == "" {
		v = os.Getenv("JAMFORMER_PROVIDER")
	}
	return v == "" || v == "jamfplatform"
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// reportPlatformScope tells the user which scope the run is using and, more
// usefully, what that scope cannot reach.
//
// Getting this wrong is the most common Platform API GA failure: an integration
// carries one scope, chosen at registration and unchangeable, and a resource
// outside it is refused by the provider at configure time. Naming the skipped
// families up front beats an export that quietly returns less than the tenant
// holds, or one that dies on the first list block it cannot serve.
func reportPlatformScope(scope platform.Scope, selected map[string]bool, quiet bool) {
	if quiet {
		return
	}

	switch scope.Kind {
	case platform.ScopeEnvironment:
		fmt.Printf("API integration scope: %senvironment%s (%s)\n", uBold, uReset, scope.ID)
	case platform.ScopeTenant:
		fmt.Printf("API integration scope: %stenant%s (%s) — legacy; prefer an environment-scoped integration\n",
			uBold, uReset, scope.ID)
	case platform.ScopeOrganization:
		fmt.Printf("API integration scope: %sorganization%s — no environment or tenant ID set\n", uBold, uReset)
	}

	// Only mention what the user actually asked for. A resource excluded by the
	// filter is not a scope problem, and naming it as one is noise.
	var blocked []string
	for _, r := range platform.UnreachableForScope(scope.Kind) {
		if selected != nil && !selected[r.FilterKey] {
			continue
		}
		blocked = append(blocked, r.DisplayName)
	}
	if len(blocked) == 0 {
		return
	}

	fmt.Printf("  %sSkipping %d resource type(s) this scope cannot reach:%s\n", uDim, len(blocked), uReset)
	// A long list adds nothing beyond the first few names and the count.
	const show = 6
	shown := blocked
	if len(shown) > show {
		shown = shown[:show]
	}
	fmt.Printf("  %s  %s", uDim, strings.Join(shown, ", "))
	if len(blocked) > show {
		fmt.Printf(", and %d more", len(blocked)-show)
	}
	fmt.Printf("%s\n", uReset)

	switch scope.Kind {
	case platform.ScopeTenant:
		fmt.Printf("  %s  Blueprints, Compliance Benchmarks and AI Governance need an environment-scoped integration.%s\n", uDim, uReset)
	case platform.ScopeOrganization:
		fmt.Printf("  %s  Organization scope reaches only the Jamf Account SSO resources. Set an environment ID for the rest.%s\n", uDim, uReset)
	case platform.ScopeEnvironment:
		fmt.Printf("  %s  The Jamf Account resources need an organization-scoped integration (no environment or tenant ID).%s\n", uDim, uReset)
	}
}
