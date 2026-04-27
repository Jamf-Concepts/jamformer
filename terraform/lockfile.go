// Copyright 2026, Jamf Software LLC

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// providerVersionPattern matches the version line in .terraform.lock.hcl.
var providerVersionPattern = regexp.MustCompile(`^\s*version\s*=\s*"([^"]+)"`)

// providerBlockPattern matches a provider block header in .terraform.lock.hcl.
var providerBlockPattern = regexp.MustCompile(`^provider\s+"registry\.terraform\.io/([^"]+)"`)

// ResolvedProviderVersion reads .terraform.lock.hcl in workDir and returns
// the version that terraform resolved for the given provider source
// (e.g. "deploymenttheory/jamfpro"). Returns an empty string if the lock
// file doesn't exist or the provider isn't found.
func ResolvedProviderVersion(workDir, providerSource string) string {
	lockFile := filepath.Join(workDir, ".terraform.lock.hcl")
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return ""
	}

	normalised := strings.ToLower(providerSource)
	inBlock := false
	for line := range strings.SplitSeq(string(data), "\n") {
		if m := providerBlockPattern.FindStringSubmatch(line); m != nil {
			inBlock = strings.ToLower(m[1]) == normalised
			continue
		}
		if inBlock {
			if m := providerVersionPattern.FindStringSubmatch(line); m != nil {
				return m[1]
			}
			// End of block
			if strings.TrimSpace(line) == "}" {
				inBlock = false
			}
		}
	}
	return ""
}

// ProviderSources for each supported provider.
const (
	ProviderSourceJamfPro      = "deploymenttheory/jamfpro"
	ProviderSourceJamfProtect  = "Jamf-Concepts/jamfprotect"
	ProviderSourceJamfPlatform = "Jamf-Concepts/jamfplatform"
	ProviderSourceJSC          = "Jamf-Concepts/jsctfprovider"
)

// FormatVersionConstraint returns the version line for a required_providers block.
// If pinnedVersion is set, it produces an exact pin: version = "X.Y.Z".
// If resolvedVersion is set (and no pin), it produces: version = ">= X.Y.Z".
// If neither is set, it returns an empty string (no constraint).
func FormatVersionConstraint(pinnedVersion, resolvedVersion string) string {
	switch {
	case pinnedVersion != "":
		return fmt.Sprintf("\n      version = %q", pinnedVersion)
	case resolvedVersion != "":
		return fmt.Sprintf("\n      version = %q", ">= "+resolvedVersion)
	default:
		return ""
	}
}
