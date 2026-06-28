// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"
	"os"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/platform"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// platformProvider implements Provider for the Jamf-Concepts/jamfplatform
// provider (OAuth2 only; tenant-scoped).
type platformProvider struct{}

func (platformProvider) Name() string           { return "jamfplatform" }
func (platformProvider) ProviderSource() string { return terraform.ProviderSourceJamfPlatform }
func (platformProvider) TypeToFileMap() map[string]string {
	return platform.TypeToFileMap()
}

func (platformProvider) DiscoverAndGenerate(env EnvConfig, opts *Options) (*PerEnvResult, error) {
	tempDir, err := os.MkdirTemp("", "jamformer-"+env.Name+"-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	ir, err := platform.RunDiscoveryAndGenerate(&platform.PipelineOptions{
		OutputDir:            tempDir,
		BaseURL:              env.URL,
		ClientID:             env.ClientID,
		ClientSecret:         env.ClientSecret,
		TenantID:             env.TenantID,
		SelectedResources:    opts.SelectedResources,
		SkipReferences:       false, // references must be resolved for diffing
		SkipPackageDownloads: opts.SkipPackageDownloads,
		ProviderVersion:      opts.ProviderVersion,
		Quiet:                opts.Quiet,
		Verbose:              opts.Verbose,
		StatusFunc:           opts.StatusFunc,
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	schemas, _ := ir.ProviderSchemas.(*tfjson.ProviderSchemas)

	// Apply the validation auto-fix in place (temp dir still init'd) so the merged
	// module and env roots inherit a plannable config. injectRequiredWriteOnly has
	// not run yet here, so the fix deliberately leaves write-only attributes alone.
	applyValidationFixes(tempDir, schemas)

	return &PerEnvResult{
		EnvName:   env.Name,
		Registry:  ir.Registry,
		Resources: platformResourceRefs(ir.Resources),
		OutputDir: tempDir,
		ProcessOptions: &postprocess.ProcessOptions{
			TypeToFileMap:           platform.TypeToFileMap(),
			Rules:                   platform.DefaultRules(),
			ExtractSpecs:            platform.ExtractSpecs(),
			InjectRequiredWriteOnly: true,
			PlatformPackageFiles:    ir.PackageFiles,
			ProviderSchemas:         schemas,
		},
		// The jamfplatform provider has no token_refresh_buffer_period attribute.
		TokenRefreshPeriod: 0,
	}, nil
}

// platformResourceRefs maps platform's discovered resources into the
// provider-agnostic ResourceRef slice.
func platformResourceRefs(discovered []platform.DiscoveredResource) []ResourceRef {
	refs := make([]ResourceRef, 0, len(discovered))
	for _, d := range discovered {
		refs = append(refs, ResourceRef{
			TFType: d.TFType,
			Label:  d.Label,
			Name:   d.Name,
			JamfID: d.JamfID,
		})
	}
	return refs
}

func (platformProvider) ModuleProvidersBlock(versionLine string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    jamfplatform = {
      source = "Jamf-Concepts/jamfplatform"%s
    }
  }
}
`, versionLine)
}

func (platformProvider) EnvProviderHeader(env EnvConfig, versionLine string, _ int) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    jamfplatform = {
      source = "Jamf-Concepts/jamfplatform"%s
    }
  }
}

provider "jamfplatform" {
  base_url      = var.jamfplatform_base_url
  client_id     = var.jamfplatform_client_id
  client_secret = var.jamfplatform_client_secret
  tenant_id     = var.jamfplatform_tenant_id
}
`, versionLine)
}

func (platformProvider) EnvAuthVariables(env EnvConfig) string {
	return fmt.Sprintf(`variable "jamfplatform_base_url" {
  description = "Jamf Platform API gateway base URL (e.g. https://us.apigw.jamf.com)"
  type        = string
  default     = %q
}

variable "jamfplatform_client_id" {
  description = "Jamf Platform API client ID"
  type        = string
  sensitive   = true
}

variable "jamfplatform_client_secret" {
  description = "Jamf Platform API client secret"
  type        = string
  sensitive   = true
}

variable "jamfplatform_tenant_id" {
  description = "Jamf Platform tenant ID"
  type        = string
  default     = %q
}

`, env.URL, env.TenantID)
}
