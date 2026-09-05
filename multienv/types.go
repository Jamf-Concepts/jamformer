// Package multienv supports exporting multiple Jamf environments into a
// Terraform project with a module + per-environment directory structure,
// designed for a long-lived branch workflow where each branch represents an
// environment (e.g. staging → main for promotion). It is provider-agnostic:
// the per-provider specifics (discovery/generation, reference rules, provider
// blocks, credential resolution) are supplied through the Provider interface.
// Copyright 2026, Jamf Software LLC

package multienv

import (
	"github.com/Jamf-Concepts/jamformer/platform"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
)

// EnvConfig holds credentials and URL for a single Jamf environment.
type EnvConfig struct {
	Name         string // e.g. "dev", "staging", "prod"
	URL          string // Jamf Pro instance URL, or Jamf Platform base_url
	AuthMethod   string // "basic" or "oauth2" (Jamf Platform is always "oauth2")
	Username     string
	Password     string
	ClientID     string
	ClientSecret string
	// EnvironmentID and TenantID are the two Jamf Platform scope identifiers,
	// mutually exclusive. Both empty is organization scope. Ignored by the
	// jamfpro provider, which has no notion of either.
	EnvironmentID string
	TenantID      string
}

// PlatformScope resolves this environment's Jamf Platform scope.
func (e EnvConfig) PlatformScope() platform.Scope {
	switch {
	case e.EnvironmentID != "":
		return platform.Scope{Kind: platform.ScopeEnvironment, ID: e.EnvironmentID}
	case e.TenantID != "":
		return platform.Scope{Kind: platform.ScopeTenant, ID: e.TenantID}
	}
	return platform.Scope{Kind: platform.ScopeOrganization}
}

// Options holds all parameters for multi-environment export.
type Options struct {
	Provider             string // "jamfpro" (default) or "jamfplatform"
	Envs                 []EnvConfig
	SourceEnv            string // name of source-of-truth env (default: first in Envs)
	OutputDir            string
	SelectedResources    map[string]bool
	SkipReferences       bool
	SkipImportBlocks     bool
	SkipPackageDownloads bool
	ProviderVersion      string
	Quiet                bool
	Verbose              bool
	ResourcesFlag        string
	ExcludeFlag          string
	StatusFunc           func(string, int, int)
}

// PerEnvResult holds the pipeline output for a single environment. It is
// produced by Provider.DiscoverAndGenerate and consumed by the merge pipeline.
type PerEnvResult struct {
	EnvName   string
	Registry  *registry.Registry
	Resources []ResourceRef // flat list of discovered resources for matching
	OutputDir string        // temp working directory for this env

	// ProcessOptions is the provider-specific post-processing configuration
	// (TypeToFileMap, Rules, ExtractSpecs, provider schema, etc.) built during
	// discovery. The merge pipeline clones it and overrides SupportFilesPrefix
	// and SkipReferences per call.
	ProcessOptions *postprocess.ProcessOptions

	// TokenRefreshPeriod is the provider's token refresh buffer in seconds, or 0
	// if the provider has no such attribute. Written into the env-root provider
	// block when non-zero.
	TokenRefreshPeriod int
}

// MatchedResource represents a resource found across multiple environments.
type MatchedResource struct {
	ResourceType string
	Label        string
	Name         string            // human-readable Jamf name
	IDs          map[string]string // env name → Jamf ID
	Present      []string          // which envs have this resource
	AllEnvs      bool              // true if present in every environment
}

// AttrDiff represents an attribute whose value differs across environments.
type AttrDiff struct {
	ResourceType string            // e.g. "jamfpro_policy"
	Label        string            // e.g. "install_rosetta"
	AttrName     string            // e.g. "priority"
	Values       map[string]string // env name → attribute value expression
	VarName      string            // generated variable name
	VarType      string            // HCL variable type, e.g. "string", "list(string)"
}

// SupportFileClass indicates whether a support file is identical across all
// environments (shared) or differs (divergent).
type SupportFileClass int

const (
	// SupportFileShared means the file is identical across all environments
	// and belongs in the shared module directory.
	SupportFileShared SupportFileClass = iota

	// SupportFileDivergent means the file differs across environments and
	// each environment gets its own copy.
	SupportFileDivergent
)

// ClassifiedFile holds the classification result for a single support file.
type ClassifiedFile struct {
	// RelPath is the path relative to the support_files/<env>/ directory,
	// e.g. "scripts/install_agent.sh"
	RelPath string

	// Class indicates whether the file is shared or divergent.
	Class SupportFileClass
}

// ModuleVar represents a variable that the module declares and each
// environment's main.tf supplies. It covers divergent support file content,
// file(var.X) token paths, and write-only secrets the provider never returns.
type ModuleVar struct {
	Name         string // variable name, e.g. "script_install_agent_contents"
	Description  string // human-readable description
	ResourceType string // e.g. "jamfpro_script"
	Label        string // e.g. "install_agent"
	AttrName     string // e.g. "script_contents"
	FilePath     string // relative path to the divergent file, e.g. "scripts/install_agent.sh"
	Sensitive    bool   // mark the declared variable sensitive (write-only secrets)
}
