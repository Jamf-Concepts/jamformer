// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/postprocess"
)

// ResourceRef is a flat (type, label, name, id) record for a single resource,
// used to match resources across environments by (type, label) and to write
// per-env import blocks. Each provider produces these from its own discovery.
type ResourceRef struct {
	TFType string
	Label  string
	Name   string
	JamfID string
}

// Provider abstracts the per-provider specifics the merge pipeline needs:
// running discovery + generation for one environment, the reference/extraction
// configuration for post-processing, and the provider-specific Terraform blocks
// for the module and environment roots.
type Provider interface {
	// Name is the provider identifier ("jamfpro", "jamfplatform").
	Name() string

	// ProviderSource is the registry source used to read the resolved version
	// from .terraform.lock.hcl (e.g. "deploymenttheory/jamfpro").
	ProviderSource() string

	// DiscoverAndGenerate runs discovery + terraform plan/query for one
	// environment into a fresh temp directory and returns a populated
	// PerEnvResult (the caller owns cleanup of result.OutputDir).
	DiscoverAndGenerate(env EnvConfig, opts *Options) (*PerEnvResult, error)

	// TypeToFileMap maps TF resource type → output filename.
	TypeToFileMap() map[string]string

	// ModuleProvidersBlock returns the required_providers block written to
	// modules/jamf/providers.tf. versionLine is the formatted version constraint
	// (may be empty).
	ModuleProvidersBlock(versionLine string) string

	// EnvProviderHeader returns the terraform{} + provider{} blocks for an
	// environment root's main.tf — everything that precedes the module call.
	EnvProviderHeader(env EnvConfig, versionLine string, tokenRefreshPeriod int) string

	// EnvAuthVariables returns the connection variable declarations for an
	// environment root's variables.tf (URL/auth/credentials).
	EnvAuthVariables(env EnvConfig) string
}

// applyValidationFixes runs the provider's validation auto-fix against a
// freshly-generated (still init'd) working directory, in place on its
// generated.tf: it removes conditionally-invalid attributes and rewrites
// Required attributes the server returned null to sensitive var.<name>
// references. The single-env pipelines run this in their tail; the multi-env
// merge needs it applied per-env before generated.tf is copied and split, so
// the module and every environment root inherit a clean, plannable config. The
// rewritten var.<name> references are picked up later by scanWriteOnlyVarRefs
// and declared on the module + each env root. Best-effort: validation problems
// are logged by the post-processor, not fatal.
func applyValidationFixes(dir string, schemas *tfjson.ProviderSchemas) {
	if schemas == nil {
		return
	}
	ps := postprocess.LoadProviderSchema(schemas)
	if ps == nil {
		return
	}
	if _, err := postprocess.FixValidationErrors(dir, ps); err != nil && !Quiet {
		fmt.Printf("  Warning: validation auto-fix failed for %s: %v\n", dir, err)
	}
}

// providerFor returns the Provider implementation for the given provider name.
func providerFor(name string) (Provider, error) {
	switch name {
	case "", "jamfpro":
		return proProvider{}, nil
	case "jamfplatform":
		return platformProvider{}, nil
	default:
		return nil, fmt.Errorf("multi-env does not support provider %q", name)
	}
}
