// Copyright 2026, Jamf Software LLC

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// helpTopics maps topic names to their extended help text.
var helpTopics = map[string]string{
	"credentials": `
Credentials
───────────
Credentials are sourced from environment variables or interactive prompts
only - never CLI flags - to avoid leaking secrets in shell history and
process listings.

  Environment Variables:
    JAMF_URL              Jamf instance URL
    JAMF_USERNAME         Jamf Pro / JSC username (basic auth)
    JAMF_PASSWORD         Jamf Pro / JSC password (basic auth)
    JAMF_CLIENT_ID        API client ID (OAuth2)
    JAMF_CLIENT_SECRET    API client secret (OAuth2)

  Auth Method Detection:
    If JAMF_CLIENT_ID is set    → OAuth2
    If JAMF_USERNAME is set     → Basic auth
    Both set                    → Error (pick one)

  Interactive Mode:
    When running in a terminal with missing credentials, jamformer
    prompts for URL and credentials. Passwords use hidden input.

  Non-Interactive Mode (CI):
    Fails fast with a clear error if credentials are missing.
    Set all required environment variables before running.
`,

	"multi-env": `
Multi-Environment Support (Advanced, Experimental)
───────────────────────────────────────────────────
Generates a Terraform project structured for a long-lived branch
workflow: a shared module plus a per-environment root directory,
where each git branch represents an environment (e.g. staging, main).

This produces more of a scaffold than the single-environment mode -
expect to edit the generated module, the per-env roots, and the
extracted variables before any of it is usable. If you're still
getting a single-instance project working, start there first.

  Supported providers:
    jamfplatform (default), jamfpro
    Jamf Protect and JSC are not supported in multi-env mode.

  Usage:
    jamformer -multi-env "staging prod"
    jamformer -multi-env "prod"              (single env also accepted -
                                               produces the module +
                                               one environments/<env>/ dir)

  Prerequisites:
    Your environments should have matching resource names where
    possible. Resources are matched across environments by name -
    duplicate names within a resource type may cause incorrect
    matching. Designed for environments intentionally kept in sync.

  Credentials:
    Set per-env credentials with an environment name suffix:
      JAMF_URL_STAGING, JAMF_CLIENT_ID_STAGING, JAMF_CLIENT_SECRET_STAGING
      JAMF_URL_PROD, JAMF_CLIENT_ID_PROD, JAMF_CLIENT_SECRET_PROD
    jamfplatform is OAuth2-only per env. Scope per environment:
    JAMF_ENVIRONMENT_ID_<ENV> (preferred) or JAMF_TENANT_ID_<ENV> (legacy);
    neither means organization scope. The two are mutually exclusive.
    (enables package / Jamf Connect / Self Service branding downloads).
    jamfpro accepts basic auth or OAuth2 per env.

  Output Structure:
    generated/
      modules/jamf/                  shared resource definitions
        policies.tf                  resources in ALL environments
        policies_staging_only.tf     resources only in staging
        variables.tf                 module input variables
        support_files/               files identical across environments
      environments/
        staging/
          main.tf                    provider config + module call
          backend.tf                 placeholder - configure your backend
          variables.tf               auth + pass-through variables
          terraform.tfvars
          imports.tf                 module.jamf.<type>.<label> imports
        prod/
          ... (same layout)

  How It Works:
    1. Runs discovery + terraform plan against each environment,
       independently
    2. Matches resources across environments by name
    3. Assembles modules/jamf/, extracting attributes that differ
       between environments to var.xxx
    4. Resources present in only some environments go into
       *_<env>_only.tf files inside the module
    5. Support files identical across environments live in the
       module; divergent files are copied per environment
    6. Each environments/<env>/ gets its own provider config,
       backend placeholder, variables, tfvars, and import blocks

  Branch Workflow:
    1. Commit the output and create a long-lived branch per
       environment (e.g. staging, main for prod)
    2. On each branch, configure backend.tf with your state backend
    3. On each branch, delete *_<other_env>_only.tf files from the
       module (e.g. on main, delete *_staging_only.tf)
    4. Set branch protection so main only accepts merges from staging
    5. Configure CI to run terraform apply in environments/<env>/
       when code lands on the corresponding branch
    6. Initial import per environment:
         cd environments/<env> && terraform init && terraform apply

    Feature branches are created from staging, merged to staging via
    PR, then promoted to main via PR. The shared module merges cleanly
    because it's identical across branches.

  Flags:
    -multi-env "staging prod"   Environment names (space/comma separated)
    -source-env prod            Source of truth (default: first listed)

  Not compatible with -compact mode.
`,

	"compact": `
Compact Mode
────────────
Consolidates simple, uniform resource types into for_each + locals
patterns, producing output closer to hand-written Terraform.

  Usage:
    jamformer -compact

  Without Compact:
    resource "jamfpro_category" "productivity" { name = "Productivity" }
    resource "jamfpro_category" "security"     { name = "Security" }

  With Compact:
    locals {
      categories = {
        productivity = { name = "Productivity" }
        security     = { name = "Security" }
      }
    }
    resource "jamfpro_category" "all" {
      for_each = local.categories
      name     = each.value.name
    }

  Eligibility (determined at runtime):
    - File contains 2+ resource blocks
    - All blocks share the same attribute names
    - No nested blocks (except lifecycle)

  Fine-Grained Control:
    -compact-include "categories buildings"   Only compact these types
    -compact-exclude "policies"               Compact everything except these

  References are automatically rewritten:
    jamfpro_category.productivity.id → jamfpro_category.all["productivity"].id

  Not compatible with -multi-env.
`,

	"secrets": `
Secret Scanning
───────────────
After generation, jamformer scans the output for secrets using gitleaks
with Jamf-specific rules.

  What It Detects:
    - HCL attributes containing passwords, secrets, credentials
    - Plist/XML secrets in configuration profiles (<key>Password</key>)
    - LDAP bind passwords, SMTP passwords, WiFi/VPN shared secrets
    - Private keys, API tokens, cloud credentials (200+ default rules)

  Interactive Remediation:
    When secrets are found, you choose:
      [a]ll      Remediate everything automatically
      [s]elect   Walk through each finding individually
      [N]one     Skip remediation (default)

  How Remediation Works:
    - .tf files: secret value replaced with var.<name> (sensitive = true)
    - Support files: converted to .tpl templates with templatefile()

  Flags:
    -skip-secret-scan    Skip scanning entirely (useful for CI)

  Tip:
    Even with -skip-secret-scan, run your own secret detection before
    committing to Git. The generated .gitignore excludes terraform.tfvars
    but other files may contain embedded secrets.
`,

	"filtering": `
Resource Filtering
──────────────────
By default, jamformer discovers all supported resource types. Use
filtering to import specific types or exclude types you don't need.

  Include Specific Types:
    jamformer -include-resources "policies scripts categories"

  Exclude Specific Types:
    jamformer -exclude-resources "packages icons"

  List Available Types:
    jamformer -list-resources
    jamformer -list-resources -provider jamfprotect

  Notes:
    - Names are case-insensitive
    - Space or comma separated
    - Cannot use -include-resources and -exclude-resources together
    - When a dependency type isn't included, references to those types
      stay as literal IDs (e.g. category_id = "5" instead of
      jamfpro_category.productivity.id)
`,

	"dev-overrides": `
Provider Dev Overrides
──────────────────────
By default, jamformer suppresses Terraform provider dev_overrides from
your CLI config (~/.terraform.d/terraform.tfrc). This prevents local
provider builds from interfering with registry provider resolution.

  Usage:
    jamformer -allow-dev-overrides

  When To Use:
    - You're developing or testing a local build of a Jamf provider
    - You want jamformer to use your dev_overrides provider instead
      of downloading from the Terraform registry

  How It Works:
    Without this flag, jamformer sets TF_CLI_CONFIG_FILE to an empty
    file, overriding any dev_overrides in your config. With this flag,
    your normal Terraform CLI config is preserved.

  Note:
    terraform init may fail with dev_overrides since the provider isn't
    in the registry. The JSC pipeline handles this gracefully.
`,

	"provider-version": `
Provider Version Management
───────────────────────────
By default, jamformer downloads the latest provider version and adds a
minimum version constraint (>= X.Y.Z) to the generated provider.tf.

  Default Behavior:
    1. Phase 1: No version constraint (terraform init gets latest)
    2. Phase 2: Reads .terraform.lock.hcl for resolved version
    3. Final provider.tf includes: version = ">= X.Y.Z"

  Pin a Specific Version:
    jamformer -provider-version 0.35.1

    This produces an exact pin: version = "0.35.1"

  Environment Variable:
    JAMFORMER_PROVIDER_VERSION=0.35.1

  The minimum constraint (>=) is recommended for most users - it
  allows upgrading the provider without regenerating. Pin a specific
  version if you need reproducible builds or are testing against a
  known-good provider release.
`,
}

// printHelpTopic prints extended help for a specific topic.
// Returns true if the topic was found and printed.
func printHelpTopic(topic string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))

	text, ok := helpTopics[topic]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown help topic %q.\n\nAvailable topics:\n", topic)
		printHelpTopicList()
		os.Exit(1)
	}

	fmt.Print(strings.TrimLeft(text, "\n"))
	return true
}

// printHelpTopicList prints the list of available help topics.
func printHelpTopicList() {
	var names []string
	for name := range helpTopics {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// First non-empty line of the help text is the title
		lines := strings.Split(strings.TrimLeft(helpTopics[name], "\n"), "\n")
		title := ""
		if len(lines) > 0 {
			title = strings.TrimSpace(lines[0])
		}
		fmt.Fprintf(os.Stderr, "  %-20s %s\n", name, title)
	}
}

// checkHelpArgs intercepts -help <topic> before flag.Parse runs.
// Returns true if a help topic was handled (caller should exit).
func checkHelpArgs() bool {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "-help" || arg == "--help" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				printHelpTopic(args[i+1])
				return true
			}
			// No topic specified - fall through to default flag.Usage
			return false
		}
	}
	return false
}
