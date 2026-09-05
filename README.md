# jamformer

<p align="center">
  <img src="docs/splash.png" alt="jamformer splash screen" width="600">
</p>

> **Read this first.** jamformer is an **enablement, education, and acceleration tool** for teams adopting Jamf with Terraform. It is **not** a tool that produces production-ready Terraform code, and it is not a drop-in "export my Jamf instance to prod IaC" button.
>
> What it is good for:
>
> - Seeing what's actually in your Jamf instance, expressed in the Terraform providers' own resource model
> - Learning how each Jamf object maps to a Terraform resource — attributes, naming, cross-references, lifecycle quirks
> - Giving engineers and architects a realistic, resource-accurate starting point to refactor and harden into their own IaC
> - Bootstrapping proofs-of-concept, demos, workshops, and migration planning sessions
>
> What it is **not**:
>
> - A production code generator. The output will need human review, refactoring, secret handling, module extraction, naming conventions, and provider-drift fixes before it is safe to manage real infrastructure.
> - A substitute for learning Terraform or the Jamf providers. It accelerates the learning curve; it does not remove it.
>
> Treat every file it emits as a first draft.

A CLI tool that converts a Jamf instance into a structured Terraform project. It discovers resources via the Jamf API (or `terraform query` for Protect/Platform), generates Terraform import blocks, uses `terraform plan -generate-config-out` to produce HCL via the appropriate provider, then post-processes the output to add cross-resource references and organise resources into per-type files. The goal is to hand you a realistic scaffold to learn from and refine — not a finished product.

> **📖 Full walkthrough:** [Adopting Terraform for Jamf with jamformer](https://concepts.jamf.com/en/guides/infrastructure-as-code/adopting-terraform-for-jamf-with-jamformer/) is the maintained, frequently-updated tutorial covering setup, usage, and workflows end to end. This README covers the essentials and the CLI reference; treat the guide as the source of truth for step-by-step how-to content.

## Supported Providers

| Provider | Flag | Auth | Discovery Method |
|---|---|---|---|
| [jamfplatform](https://github.com/Jamf-Concepts/terraform-provider-jamfplatform) **(default)** | `-provider jamfplatform` | OAuth2 only | `terraform query` (Terraform 1.14+) |
| [jamfprotect](https://github.com/Jamf-Concepts/terraform-provider-jamfprotect) | `-provider jamfprotect` | OAuth2 only | `terraform query` (Terraform 1.14+) |
| [jsc](https://github.com/Jamf-Concepts/terraform-provider-jsctfprovider) | `-provider jsc` | Local account or Jamf ID | Terraform data sources |
| [jamfpro](https://github.com/deploymenttheory/terraform-provider-jamfpro) — community provider by Deployment Theory | `-provider jamfpro` | Basic auth or OAuth2 | Jamf Pro API via SDK |

`jamfplatform` federates the full Jamf Pro resource surface (`jamfplatform_pro_*`) alongside native Platform Services resources (blueprints, compliance benchmarks, device groups), Jamf Security Cloud (`jamfplatform_security_cloud_*`), Jamf AI Governance (`jamfplatform_ai_governance_*`) and Jamf Account single sign-on (`jamfplatform_account_*`) — and is the default. `jamfpro` is the community-maintained provider by Deployment Theory and remains fully supported.

> The Jamf Platform API reached general availability on 3 September 2026. jamformer targets the GA gateway (`https://{region}.api.jamfcloud.com`) and provider `v0.30.0` or later. Beta API integration credentials are revoked, and `jamfplatform_pro_api_client` / `jamfplatform_pro_api_role` no longer exist — their endpoints were unpublished at GA. See [API integration scope](#api-integration-scope).

## Resource Coverage

Coverage is broad across all four providers and grows as each provider adds resources, so rather than duplicate an enumeration here that will drift out of date, jamformer exposes it directly:

```bash
./jamformer -list-resources                        # all providers
./jamformer -list-resources -provider jamfplatform  # a specific provider
```

This is always the authoritative, current list — it's generated from the same resource tables the pipeline runs against. See the [guide](https://concepts.jamf.com/en/guides/infrastructure-as-code/adopting-terraform-for-jamf-with-jamformer/) for a narrated tour of what's covered and how each resource type maps to Terraform.

## Prerequisites

- [Go 1.26+](https://go.dev/dl/) (to build)
- **Jamf Platform:** An API integration (OAuth2) registered in Jamf Account, with read permissions for what you intend to export. Its **scope** decides what it can reach — see [API integration scope](#api-integration-scope). Requires Terraform 1.14+.
- **Jamf Protect:** An API client (OAuth2) with appropriate privileges. Requires Terraform 1.14+.
- **Jamf Pro:** A user account with read/auditor access, or an API integration with appropriate privileges.
- **JSC:** A local account or Jamf ID with access to Jamf Security Cloud (radar.wandera.com). SSO/SAML accounts are not supported.

Terraform 1.15.x is automatically downloaded if not already installed (cached in a temp directory). Use `-terraform-path` to override with a pre-installed binary.

## Installation

```bash
git clone https://github.com/Jamf-Concepts/jamformer.git
cd jamformer
go build -o jamformer .
```

jamformer checks for a newer release on launch and, if there is one, prints the upgrade command for how the binary was installed — `brew upgrade`, `go install`, `git pull && go build`, or a link to the release. The check runs in the background, is cached for 24 hours, and never delays or blocks a run. Set `JAMFORMER_SKIP_UPDATE_CHECK=1` to turn it off; `jamformer -version` reports the install method it detected.

## Quick Start

`jamfplatform` is the default provider — no `-provider` flag needed:

```bash
export JAMF_CLIENT_ID='your-client-id'
export JAMF_CLIENT_SECRET='your-client-secret'
export JAMF_ENVIRONMENT_ID='your-environment-id'
./jamformer -url https://eu.api.jamfcloud.com
```

Jamf Platform is OAuth2-only, and the URL is your regional GA gateway host — `https://us.api.jamfcloud.com`, `https://eu.api.jamfcloud.com` or `https://apac.api.jamfcloud.com` — not a Jamf Pro instance hostname, and there's no shorthand expansion for it. Give the host alone: the gateway serves every API namespace at the root, and a path such as `/api` makes authentication fail.

The provider's own `JAMFPLATFORM_BASE_URL`, `JAMFPLATFORM_CLIENT_ID`, `JAMFPLATFORM_CLIENT_SECRET`, `JAMFPLATFORM_ENVIRONMENT_ID` and `JAMFPLATFORM_TENANT_ID` are accepted as fallbacks, so a shell already set up to run `terraform` needs no second set of exports.

### API integration scope

An API integration is registered at exactly one of three scopes, chosen in Jamf Account and **not changeable afterwards**. The scope decides what jamformer can export, so set the one your integration actually carries:

| Scope | Set | Reaches |
|---|---|---|
| **Environment** (preferred) | `JAMF_ENVIRONMENT_ID` | Everything except Jamf Account — the federated Jamf Pro surface, Security Cloud, device groups, and the environment-only families: Blueprints, Compliance Benchmarks, AI Governance |
| **Tenant** (legacy) | `JAMF_TENANT_ID` | Jamf Pro, Security Cloud and device groups. **Not** Blueprints, Compliance Benchmarks or AI Governance — the provider refuses those at configure time |
| **Organization** | neither (optionally `JAMF_ORGANIZATION_ID` to say so explicitly) | Only the Jamf Account SSO resources (`jamfplatform_account_*`), and it is the only scope that reaches them |

The two IDs are mutually exclusive; setting both is an error, as it is in the provider. jamformer reports the scope it resolved and names the resource types it is skipping because of it, so an export never quietly returns less than your tenant holds:

```
API integration scope: tenant (85f69825-…) — legacy; prefer an environment-scoped integration
  Skipping 5 resource type(s) this scope cannot reach:
    Blueprints, Compliance Benchmarks, AI Governance Policies, Jamf Account SSO Domains, Jamf Account SSO Connections
    Blueprints, Compliance Benchmarks and AI Governance need an environment-scoped integration.
```

Package downloads, Jamf Connect discovery and Self Service branding images all ride on the federated Jamf Pro surface, which environment and tenant scope both reach; organization scope reaches none of it.

Credentials are set via environment variables (`JAMF_CLIENT_ID`, `JAMF_CLIENT_SECRET`) to avoid leaking secrets in shell history and process listings. Run without them for interactive prompts. The URL can be passed as a flag or via `JAMF_URL`. Run `./jamformer -help credentials` for auth-method detection details.

### Other providers

Swap `-provider` to target Jamf Pro, Jamf Protect, or JSC instead — see [Supported Providers](#supported-providers) for auth requirements per provider.

```bash
# Jamf Pro (basic auth; shorthand URLs expand to <name>.jamfcloud.com)
export JAMF_USERNAME=admin
export JAMF_PASSWORD='yourpassword'
./jamformer -provider jamfpro -url yourinstance

# Jamf Protect (OAuth2; shorthand URLs expand to <tenant>.protect.jamfcloud.com)
export JAMF_CLIENT_ID='your-client-id'
export JAMF_CLIENT_SECRET='your-client-secret'
./jamformer -provider jamfprotect -url your-tenant

# JSC (basic auth or Jamf ID only)
export JAMF_USERNAME=your@email.com
export JAMF_PASSWORD='yourpassword'
./jamformer -provider jsc
```

## Credentials & Permissions

Credentials are sourced from environment variables or interactive prompts only (never CLI flags).

| Env Var | Description |
|---|---|
| `JAMF_CLIENT_ID` | API client ID (OAuth2 — Jamf Platform / Protect) |
| `JAMF_CLIENT_SECRET` | API client secret (OAuth2 — Jamf Platform / Protect) |
| `JAMF_ENVIRONMENT_ID` | Jamf Platform environment ID — the preferred scope (Jamf Platform only) |
| `JAMF_TENANT_ID` | Jamf Platform tenant ID — the legacy scope (Jamf Platform only) |
| `JAMF_ORGANIZATION_ID` | Set to select organization scope explicitly; never sent (Jamf Platform only) |
| `JAMF_USERNAME` | Jamf Pro / JSC username (basic auth) |
| `JAMF_PASSWORD` | Jamf Pro / JSC password (basic auth) |

jamformer needs **Read** on every object type it is asked to discover — it performs no writes.

- **Jamf Platform:** register an API integration in Jamf Account's **Platform API integrations** UI and grant the read permissions for what you intend to export. Permissions are organised by capability and action (`device-groups:read`, `policies:read`, …). Every export writes a **`PERMISSIONS.md`** into its output directory listing the exact capability set it required, per resource type, with the endpoint each requirement came from — so the next run, or a CI job, can be given precisely those permissions rather than over-granted. It is generated from the Jamf Platform Go SDK's own privilege registries, so it tracks the API rather than a hand-maintained table.
- **Jamf Protect:** create an OAuth2 API client with read access to every object type you intend to discover. Refer to the Jamf Protect admin documentation for current role names.
- **Jamf Pro:** the built-in `Auditor` user role (basic auth) or an `Auditor` privilege set (OAuth2) is the easiest setup and covers everything jamformer supports. For minimum privilege, grant `Read` on each object type you intend to discover — privilege names in the Jamf Pro role editor generally map 1:1 to the `-list-resources` output.
- **JSC:** a local account or Jamf ID with read access to every object type you intend to discover.

If a resource type comes back empty, or a `terraform plan -generate-config-out` step reports "provider couldn't read resource," it's almost always a missing read privilege — see [Troubleshooting](#troubleshooting).

## Flags

| Flag | Env Var | Description | Default |
|---|---|---|---|
| `-provider` | `JAMFORMER_PROVIDER` | Provider: `jamfplatform`, `jamfprotect`, `jsc`, or `jamfpro` | `jamfplatform` |
| `-url` | `JAMF_URL` | Jamf instance URL | |
| `-include-resources` | `JAMFORMER_RESOURCES` | Space-separated resource types to include (`-help filtering`) | all |
| `-exclude-resources` | `JAMFORMER_EXCLUDE` | Space-separated resource types to exclude (`-help filtering`) | |
| `-output` | `JAMFORMER_OUTPUT` | Output directory | `generated` |
| `-terraform-path` | `JAMFORMER_TERRAFORM_PATH` | Path to terraform binary (skip auto-download) | |
| `-skip-package-downloads` | `JAMFORMER_SKIP_PACKAGE_DOWNLOADS` | Skip downloading packages (Jamf Platform: JCDS; Jamf Pro: CDP) | `false` |
| `-skip-references` | `JAMFORMER_SKIP_REFERENCES` | Skip cross-resource reference resolution | `false` |
| `-skip-import-blocks` | `JAMFORMER_SKIP_IMPORT_BLOCKS` | Remove import blocks after generation | `false` |
| `-verbose` | `JAMFORMER_VERBOSE` | Show terraform command output | `false` |
| `-parallelism` | `JAMFORMER_PARALLELISM` | Concurrent Terraform provider reads during generation | `1` |
| `-provider-version` | `JAMFORMER_PROVIDER_VERSION` | Pin a specific provider version (`-help provider-version`) | latest, `>=` constraint |
| `-allow-dev-overrides` | `JAMFORMER_ALLOW_DEV_OVERRIDES` | Allow Terraform provider `dev_overrides` from CLI config (`-help dev-overrides`) | `false` |
| `-compact` | `JAMFORMER_COMPACT` | Consolidate simple resource types into `for_each` patterns (`-help compact`) | `false` |
| `-compact-include` | `JAMFORMER_COMPACT_INCLUDE` | Space-separated resource types to compact (default: all eligible) | |
| `-compact-exclude` | `JAMFORMER_COMPACT_EXCLUDE` | Space-separated resource types to exclude from compaction | |
| `-split-by-category` | `JAMFORMER_SPLIT_BY_CATEGORY` | Split categorised resource types into per-category output files | `false` |
| `-skip-secret-scan` | `JAMFORMER_SKIP_SECRET_SCAN` | Skip secret scanning of generated output (`-help secrets`) | `false` |
| `-multi-env` | `JAMFORMER_MULTI_ENV` | Space-separated environment names for multi-env export (`-help multi-env`) | |
| `-source-env` | `JAMFORMER_SOURCE_ENV` | Source-of-truth environment (default: first in list) | |
| `-list-resources` | | List valid resource filter names and exit | |
| `-credits` | | Show credits and acknowledgements | |
| `-version` / `-v` | | Print version and exit | |

Several flags have extended help built into the CLI — run `./jamformer -help <topic>` (e.g. `-help multi-env`, `-help compact`) for details and examples without leaving your terminal.

## Output

The tool generates a self-contained Terraform project in the output directory:

- `provider.tf`, `variables.tf`, `terraform.tfvars` — provider configuration (credentials are not written to tfvars for security)
- Per-type resource files — for Jamf Platform, the federated Jamf Pro surface uses a `pro_` prefix (e.g. `pro_policy.tf`, `pro_script.tf`), while native Platform resources keep the plain type name (`blueprints.tf`, `device_groups.tf`); other providers use the plain type name too (e.g. `policies.tf`, `scripts.tf`)
- Per-type import block files (e.g. `pro_policy_import.tf`, `policies_import.tf`), plus `singletons_import.tf` for singleton settings and, for Jamf Platform, `jamf_connect_import.tf`
- `support_files/` — extracted scripts, configuration profiles, app configurations, packages, and branding images; `device_enrollment_tokens/` and `volume_purchasing_tokens/` directories are created as the recommended location for token files
- `PERMISSIONS.md` (Jamf Platform) — the read capability set the export required, per resource type, with the endpoint behind each requirement

The generated `provider.tf` includes a minimum version constraint (`>= X.Y.Z`) based on the provider version that terraform downloaded. Use `-provider-version` to pin an exact version instead.

## After Generation

```bash
cd generated
terraform plan     # Review the import plan, check for provider errors
terraform apply    # Import resources into state (see warning below)
rm *_import.tf     # Remove import blocks (no longer needed)
```

**Review carefully before running `terraform apply`.** The generated configuration may contain provider-level plan errors (cross-attribute validators, missing blocks, etc.) that need manual fixing first. Always inspect the plan output and resolve any errors before applying. Remember: this tool gives you a starting point, not a finished product.

## Multi-Environment Support

> **⚠️ Experimental and highly advanced.** Intended for people already comfortable with Terraform modules and long-lived branch workflows. It produces output that is *more* of a scaffold than the single-environment mode — expect to edit the generated module, the per-env roots, and the variables extraction before any of it is usable.

`-multi-env "staging prod"` generates a Terraform project structured for a long-lived branch workflow: a shared module plus a per-environment root directory, designed for git branching strategies where each branch represents an environment. A single environment name is also accepted, producing the same module/environment scaffold for one instance.

Supported providers: **jamfplatform** (default) and **jamfpro**. Protect and JSC are not supported in multi-env mode. Credentials use an environment-name suffix (e.g. `JAMF_URL_PROD`, `JAMF_CLIENT_ID_PROD`); see `./jamformer -help multi-env` for the full credential and output-structure reference, and the [guide](https://concepts.jamf.com/en/guides/infrastructure-as-code/adopting-terraform-for-jamf-with-jamformer/) for a walkthrough of the branch promotion workflow.

## Validation Auto-Fix

After splitting the generated HCL into per-type files, jamformer runs `terraform validate` in a loop and auto-fixes schema-level errors — removing invalid or conflicting attributes, setting attributes to a value a validator requires, and resolving `null` Required attributes.

A `null` Required attribute is usually a value the API can't give back. **Write-only attributes** (passwords, tokens, and other create-only secrets) are a Terraform schema construct that's never persisted to state and never returned by a provider's Read, so the API has no way to round-trip them — they always import as `null`. jamformer detects these against the provider schema, rewires them to a sensitive Terraform variable, and seeds the paired `_wo_version` rotation attribute so the config still validates. Other Required attributes the API returns as `null` get a sensitive variable too if the schema marks them sensitive, or a type-appropriate zero value (`""`, `false`, `0`) otherwise. Either way, supply real values via `TF_VAR_*` environment variables or `terraform.tfvars` at apply time.

Re-run with `-verbose` to see exactly what was changed.

## Secret Scanning

After generation, jamformer scans the output for secrets using [gitleaks](https://github.com/gitleaks/gitleaks) (MIT licensed) plus Jamf-specific rules (HCL passwords, plist/XML secrets, LDAP/SMTP/WiFi credentials). In interactive mode you choose `[a]ll` to remediate automatically, `[s]elect` to walk through findings individually, or `[N]one` to skip. Remediation moves secrets to sensitive Terraform variables (`.tf` files) or converts affected support files to `.tpl` templates with `templatefile()`. Use `-skip-secret-scan` to disable. Run `./jamformer -help secrets` for the full mechanics.

## CI / Non-Interactive Usage

jamformer detects non-interactive environments and fails fast if credentials are missing.

```yaml
- name: Generate Terraform from Jamf Platform
  env:
    JAMF_URL: ${{ secrets.JAMF_URL }}              # e.g. https://eu.api.jamfcloud.com
    JAMF_CLIENT_ID: ${{ secrets.JAMF_CLIENT_ID }}
    JAMF_CLIENT_SECRET: ${{ secrets.JAMF_CLIENT_SECRET }}
    JAMF_ENVIRONMENT_ID: ${{ secrets.JAMF_ENVIRONMENT_ID }}
  run: ./jamformer -skip-package-downloads
```

## Troubleshooting

jamformer does not write persistent log files. All output goes to stdout/stderr. Re-run with `-verbose` to surface the full `terraform` command output instead of the spinner summary.

### Authentication failed

Verified at startup before any terraform step runs, so this fails fast.

- Confirm the right environment variables are set for the auth method you intend to use. Basic auth needs `JAMF_USERNAME` and `JAMF_PASSWORD`. OAuth2 needs `JAMF_CLIENT_ID` and `JAMF_CLIENT_SECRET`. Setting credentials for both at once is rejected.
- Jamf Platform and Jamf Protect accept OAuth2 only; JSC accepts basic auth only.
- Jamf Platform: `403 OWNERSHIP_FORBIDDEN` means the scope ID does not match how the integration was registered — an environment ID supplied for a tenant-scoped integration, or the reverse — even when both IDs belong to the same customer. `403 BAD_PERMISSIONS` means a missing read permission; the generated `PERMISSIONS.md` lists what the export needs. Authentication failing outright usually means pre-GA credentials, which are revoked (see [API integration scope](#api-integration-scope)).
- For OAuth2, the integration must have an active privilege set / role. A client with no privileges will authenticate successfully but fail on the first real call.
- If the URL is wrong (typo, missing region, or mismatched Protect tenant), you will usually see a network or TLS error rather than an auth error.

### "Provider couldn't read resource" during `terraform plan -generate-config-out`

Handled automatically: when the provider refuses to read a specific resource, the offending `import {}` block is removed and the step is retried until `terraform plan` succeeds or there is nothing left to retry. Re-run with `-verbose` to see which addresses were dropped.

The most common root cause is a missing read privilege. If only some resources of a type are dropped, the usual culprit is a provider bug with a specific attribute — file an issue (see Support below) with the `-verbose` output and the resource address.

### Token refresh errors / short-lived OAuth2 tokens

jamformer probes `/api/oauth/token` at startup to determine your integration's `expires_in`, then writes `token_refresh_buffer_period_seconds` into the generated `provider.tf`. If you rotate the API integration after generation and the new token lifetime differs, update that value manually (roughly half the new `expires_in`).

### Spinner shows nothing for a long time

Large instances (thousands of policies / icons / profiles) can take minutes to list. `-verbose` shows the underlying `terraform` commands. `-parallelism N` increases concurrent provider reads during generation.

### `terraform query` / Terraform 1.14 errors (Platform, Protect)

Platform and Protect use `terraform query`, which requires Terraform 1.14+. jamformer auto-downloads a compatible version; if you pinned a pre-1.14 binary with `-terraform-path`, remove the flag or upgrade it.

## Support

- **Issues and feature requests:** [https://github.com/Jamf-Concepts/jamformer/issues](https://github.com/Jamf-Concepts/jamformer/issues). Include the provider, the command you ran (redacted), the `-verbose` output, and the jamformer version (`jamformer -version`).
- **Questions and discussion:** `#jamformer` on the [MacAdmins Slack](https://macadmins.org/).
- **Step-by-step how-to:** the [guide](https://concepts.jamf.com/en/guides/infrastructure-as-code/adopting-terraform-for-jamf-with-jamformer/).

## Known Limitations

- **Not production-ready output** — The generated HCL is a starting point that will likely need review and refinement before managing real infrastructure.
- **Provider drift** — Some attributes may show as changes on `terraform plan` after import due to provider SDK defaults that don't round-trip. These are provider issues, not jamformer issues.
- **Icons are not downloaded locally** — Referenced via CDN URL with `lifecycle { ignore_changes }` to prevent destroy/create on first apply, across all providers that support icons.
- **Package downloads are best-effort** — Jamf Platform downloads only packages resident in the Jamf Cloud Distribution Service (JCDS); catalog packages whose bytes live elsewhere stay as metadata + server-supplied hashes. Jamf Pro downloads from the Cloud Distribution Point by default. Use `-skip-package-downloads` to skip in both cases.
- **Terraform 1.14+ required for Platform and Protect** — both use `terraform query` for discovery.
- **JSC auth** — requires a local account or Jamf ID; SSO/SAML is not supported.

For the many Jamf Platform–specific synthesis and reference-resolution behaviors (icons, Jamf Connect, branding images, blueprint conditions, smart-group criteria, compliance-benchmark artifact stripping, and more), see the [guide](https://concepts.jamf.com/en/guides/infrastructure-as-code/adopting-terraform-for-jamf-with-jamformer/) or the code comments in `platform/`.
