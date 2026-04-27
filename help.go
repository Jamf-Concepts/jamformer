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
Multi-Environment Support (Advanced)
─────────────────────────────────────
Export multiple Jamf Pro environments into a single Terraform project
using workspaces to target each environment independently.

This feature assumes familiarity with Terraform workspaces, state
management, and multi-environment workflows. If you're still getting
a single-instance project working, start there first.

  Prerequisites:
    Your environments should already have matching configurations (same
    policies, scripts, profiles). This is not a reconciliation tool for
    wildly different instances.

  Usage:
    jamformer -multi-env "dev staging prod"

  Credentials:
    Set per-env credentials with an environment name suffix:
      JAMF_URL_DEV, JAMF_CLIENT_ID_DEV, JAMF_CLIENT_SECRET_DEV
      JAMF_URL_PROD, JAMF_CLIENT_ID_PROD, JAMF_CLIENT_SECRET_PROD

  How It Works:
    1. Runs discovery + terraform plan against each environment
    2. Matches resources across environments by name
    3. Generates a merged project using terraform.workspace:
       - provider.tf uses local.env_urls[terraform.workspace]
       - Import blocks use local.imports[terraform.workspace]
       - Per-env tfvars for differing attribute values
       - Support files extracted per environment into support_files/<env>/

  Using Workspaces:
    terraform workspace new dev
    terraform workspace select dev
    terraform plan -var-file=dev.tfvars

  What Happens When Environments Differ:
    Resources are matched by name. When they don't line up:

    • Resource exists in all envs, same settings:
      One resource block, per-env import IDs. Nothing special needed.

    • Resource exists in all envs, different settings:
      Differing attribute values are extracted to variables. Each
      env's value goes into its tfvars file (dev.tfvars, prod.tfvars).
      The resource block uses var.<name> instead of a literal value.

    • Resource exists in some envs but not others:
      A count conditional is added so the resource is only created
      in workspaces where it exists:
        count = contains(keys(local.imports[terraform.workspace]), ...) ? 1 : 0

    • Resource exists in one env with a completely different name:
      It won't match. It appears as a partial-env resource in the env
      that has it, and is absent in the others.

    • Support files (scripts, profiles) differ between envs:
      Each env's files are extracted into support_files/<env>/.
      The file() references use terraform.workspace to select the
      right version. Edit in dev, copy to prod when ready.

    The more your environments differ, the more conditionals and
    variables the output will contain. For best results, keep your
    environments in sync before running multi-env export.

  Flags:
    -multi-env "dev staging prod"   Environment names (min 2, space/comma separated)
    -source-env prod                Source of truth (default: first listed)
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
