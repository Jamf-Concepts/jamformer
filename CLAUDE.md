# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o jamformer .        # Build the binary
go test ./...                   # Run all tests
go test ./naming/               # Run tests for a specific package
go test ./naming/ -run TestSanitize  # Run a single test
golangci-lint run ./...         # Run linter
```

**Always run `go test ./...` and `golangci-lint run ./...` after making changes.** Fix any linter errors (unchecked error returns, staticcheck suggestions, etc.) before considering a change complete. Tests cover registry lookups, HCL reference rewriting (all resource types and scope paths), import file generation (both auth methods), null stripping, script extraction helpers, and filename sanitisation.

**Always add or update tests when creating new resources, reference rules, or modifying existing behaviour.** Key test files and what to update:
- `pro/resources_test.go` — Update `TestTypeToFileMap` expected list and `TestDefaultRulesCount` expected types when adding new resources or reference rules
- `postprocess/references_test.go` — Add reference rewriting tests for new resource types (category, site, scope, and any custom references); update `setupRegistry()` with entries for new types
- `importgen/generator_test.go` — Update `TestWriteImportFileAllResourceTypes` to include new import file entries

## Architecture

jamformer is a CLI tool that converts a Jamf instance into a structured Terraform project. It supports four providers, selected via `-provider` (default: `jamfplatform`):

- **Jamf Platform** (default) — Uses `terraform query` (Terraform 1.14+) with `list` blocks for resource discovery and HCL generation via the `Jamf-Concepts/jamfplatform` provider, then post-processes the output. As of provider v0.18.x this federates the **full Jamf Pro surface** as `jamfplatform_pro_*` (63 listable + ~27 singleton settings) alongside the native Platform Services resources (blueprints, compliance benchmarks, device groups).
- **Jamf Protect** — Uses `terraform query` (Terraform 1.14+) with `list` blocks for resource discovery and HCL generation via the `Jamf-Concepts/jamfprotect` provider, then post-processes the output.
- **JSC (Jamf Service Connection)** — Uses `terraform apply` against data sources for resource discovery (the JSC provider does not implement list resources), then `terraform plan -generate-config-out` for HCL generation, then post-processes the output. Basic auth only (username/password).
- **Jamf Pro** — The community-maintained `deploymenttheory/jamfpro` provider. Uses the Jamf Pro API for resource discovery, `terraform plan -generate-config-out` for HCL generation, then post-processes the output.

### Jamf Pro Pipeline (pro/pipeline.go → RunPipeline)

1. **client** — Authenticates to Jamf Pro using `go-api-sdk-jamfpro` (basic auth or OAuth2 client credentials)
2. **discovery** — Queries the Jamf API for each resource type, registers each in the **registry**, and produces `[]Resource` (JamfID, Name, Label). Discovery runs in dependency order: sites → buildings → categories → departments → scripts → extension attributes → packages → dock items → printers → network segments → smart groups → static groups → macOS profiles → policies → icons → enrollment customizations → computer prestages → advanced computer searches → app installers → mac applications → device enrollments → volume purchasing locations → restricted software → mobile device groups → mobile device profiles → mobile device prestages → mobile device EAs → advanced mobile device searches → API integrations → API roles → accounts → webhooks → account groups → disk encryption configs → allowed file extensions → LDAP servers → mobile device applications → user groups → self-service branding → advanced user searches. Singleton settings (16 types) are added after discovery without API calls. Icons are discovered in a second phase after policies/profiles/apps have listed, since icon IDs are extracted from those referencing resources. Supports `-resources` filter to selectively discover only specific types.
3. **importgen** — Writes `provider.tf`, `variables.tf`, `terraform.tfvars`, and per-type `*_import.tf` files with `import {}` blocks. Generated files match the auth method used (basic or oauth2).
4. **terraform** — Runs `terraform init` then `terraform plan -generate-config-out=generated.tf` via tfexec. Auto-removes failing import blocks and retries when the provider can't read specific resources.
5. **postprocess** — Parses the generated HCL and:
   - Strips null values from optional attributes (using provider schema from tfexec's `ProvidersSchema()`)
   - Removes `category_id = -1` (uncategorised) to prevent drift
   - Rewrites literal Jamf IDs to cross-resource Terraform references via **registry** lookups
   - Extracts `script_contents` to `support_files/scripts/` with `file()` references
   - Extracts extension attribute scripts to `support_files/extension_attributes/`
   - Extracts profile payloads to `support_files/macos_configuration_profiles/`
   - Resolves `icon_file_web_source` with CDN URLs (icons are not downloaded locally) and adds `lifecycle { ignore_changes }` to prevent destroy/create on first apply
   - Downloads enrollment customization branding images and resolves `enrollment_customization_image_source`
   - Extracts device enrollment tokens to `support_files/device_enrollment_tokens/` (.p7m)
   - Extracts volume purchasing tokens to `support_files/volume_purchasing_tokens/` (.vpptoken)
   - Extracts mobile device application app configurations to `support_files/app_configurations/` (.xml)
   - Injects required attributes the provider doesn't return (`redeploy_on_update`, `payload_validate`)
   - Resolves `package_file_source` via discovery metadata
   - Splits the monolithic generated.tf into per-resource-type files
   - Runs `terraform validate` to detect conditionally invalid attributes (e.g. attributes only valid for a specific `cdn_type`) and auto-removes them

### Jamf Protect Pipeline (protect/pipeline.go → RunPipeline)

1. **importgen** — Writes `provider.tf`, `variables.tf`, `terraform.tfvars` for the Jamf Protect provider (OAuth2 only)
2. **protect** — Generates `query.tfquery.hcl` with `list {}` blocks for 13 listable resource types + `singletons_import.tf` with `import {}` blocks for 3 singleton resources
3. **terraform** — Runs `terraform init`, then `terraform query -generate-config-out=generated.tf` (list resources), then `terraform plan -generate-config-out=singletons_generated.tf` (singletons) — all via tfexec
4. **protect** — Renames auto-generated labels (`all_0`, `all_1`) to friendly names derived from resource `name`/`email` attributes, then populates **registry** from import blocks in the generated HCL
5. **postprocess** — Strips null attributes, rewrites literal IDs to cross-resource Terraform references, splits into per-type files (no script/profile/package extraction for Protect)

### Jamf Platform Pipeline (platform/pipeline.go → RunPipeline)

1. **importgen** — Writes `provider.tf`, `variables.tf`, `terraform.tfvars` for the Jamf Platform provider (OAuth2 only, uses `base_url`)
2. **platform** — Generates `query.tfquery.hcl` with `list {}` blocks for all listable resource types (3 native + 60 `jamfplatform_pro_*`), derived from the `Resources` table via `ListableResources()`
3. **terraform** — Runs `terraform init`, then `terraform query -generate-config-out=generated.tf` (with a JSON event log captured via `QueryWithEvents`)
3b. **platform** — `DiscoverJamfConnect()` enumerates the Jamf Connect config-profile links via the federated `pro` SDK (`ListJamfConnectConfigProfilesV1`) and writes `jamf_connect_import.tf` (one import block per link, friendly label from the profile name, `id` = the config-profile id). `jamfplatform_pro_jamf_connect` has no list resource — it adopts an existing macOS configuration profile carrying a Jamf Connect payload — so it is not query-discoverable; the import blocks are materialised by the generate-config-out plan in step 4. Requires `JAMF_TENANT_ID` (pro endpoints are tenant-scoped, like package downloads). `jamfplatform_pro_jamf_connect` is added to `TypeToFileMap` (→ `pro_jamf_connect.tf`) and `ValidFilterNames` (filter key `jamf_connect`) despite having no `Resources` entry
4. **platform** — `WriteSingletonImports()` writes `singletons_import.tf` (import id `singleton`) for the ~27 settings resources, then `terraform plan -generate-config-out` materialises their config — **and any Jamf Connect import blocks from step 3b** — and merges it into `generated.tf` (the plan runs when either singletons or Jamf Connect links were written). Discovery and the singleton plan both drive the spinner progress bar: discovery counts `list_complete` events (one per list type) against the selected-type count via `terraform.QueryProgressFunc`; the singleton plan reuses `terraform.ProgressFunc` (the per-import "Refreshing state..." counter)
5. **download** — When package downloads are enabled (default; `-skip-package-downloads` opts out) and a tenant ID is set, `platform/download.Packages` fetches package files resident in the Jamf Cloud Distribution Service (JCDS) via the federated `pro` SDK client (`ListJCDSFilesV1` for the resident set, then `GetJCDSFileDownloadURLV1` per file → presigned CloudFront GET). Only JCDS-resident files are fetchable; catalog packages whose bytes live on another distribution point return 404 and are skipped. JCDS is tenant-scoped, so downloads require `JAMF_TENANT_ID`
6. **platform** — Renames auto-generated labels to friendly names via `RenameLabelsWithComposer` (benchmarks use `title`, packages use `display_name`, the objecty `pro_*` types read nested `general.name`, others top-level `name`; `jamfplatform_device_group` folds `device_type` into the label so computer/mobile groups that share a name don't collide), populates **registry** from import blocks, and registers synthetic `jamfplatform_device_group#computer` / `#mobile` subtypes keyed by `jamf_pro_id` (recovered from query events; computed so absent from generated HCL) so classic scope `computer_group_ids` / `mobile_device_group_ids` resolve to `.jamf_pro_id`
6c. **platform** — `GenerateIcons()` synthesises `jamfplatform_pro_icon` resources from the `self_service_icon` references hydrated onto policies (icons have no list resource): one resource per unique icon with `icon_file_source` = the icon's CDN `uri` + `lifecycle { ignore_changes = [icon_file_source] }` + an import block keyed by icon id; registers each icon so `self_service_icon.id` rewrites to the new resource (via a `DefaultRules` entry) and drops the server-computed `uri`/`filename` echoes (`postprocess.RemoveNestedAttrs`). `jamfplatform_pro_icon` is added to `TypeToFileMap` (→ `pro_icon.tf`) despite having no `Resources` entry
7. **postprocess** — Strips null attributes; rewrites literal IDs to cross-resource references through **nested object attributes** (`scope = { targets = { ... } }`, `general = { category_id }`, list-of-objects payloads) using `ReferenceRule.AttrPath`/`ElementAttr`; extracts `general.payloads` to `.mobileconfig`, `script_contents`/`script` to scripts, `app_configuration.preferences` to `.xml`, `printer.ppd_contents` to `.ppd`, and `mobile_device_provisioning_profile.profile_data` to `.mobileprovision` via `ProcessOptions.ExtractSpecs` (the last two use `FileKindRaw` + `ExtractSpec.Ext`); skips vendor-managed/signed profiles; **skips non-creatable blueprint drafts** (empty `device_groups` — `POST /blueprints` rejects an empty scope, mirrored by the provider's `SizeAtLeast(1)` validator); for downloaded packages, switches `jamfplatform_pro_package` to JCDS **upload mode** (`package_file_source = "${path.module}/..."` path string, with the conflicting server-supplied hash attributes `sha3_512`/`sha256`/`md5`/`hash_type`/`hash_value` removed — `package_file_source` `ConflictsWith` them); packages without a downloaded file stay as metadata + server hashes (the provider applies them without a file); splits into per-type files

Note: the `jamfplatform_pro_*` surface uses plugin-framework **nested attributes** (object expressions), not HCL nested blocks. The reference-rewriting engine (`postprocess/rewrite.go` `withLeafBody`) and the file-extraction engine (`postprocess/extractspec.go`) traverse these by re-parsing object-expression bytes, reusing the idiom in `schema.go`. `jamf_pro_id` recovery from query events requires the provider to surface it in the `list_resource_found` `resource_object` payload — confirm against a live tenant.

### JSC Pipeline (jsc/pipeline.go → RunPipeline)

1. **importgen** — Writes `provider.tf`, `variables.tf`, `terraform.tfvars` for the JSC provider (basic auth only, credentials passed via `terraform.tfvars` rather than env vars)
2. **jsc** — Generates a discovery config file with `data` blocks for 4 listable resource types (activation profiles, Entra IdP connections, hostname mappings, access policies)
3. **terraform** — Runs `terraform init` then `terraform apply` to populate the data sources, parses state JSON to recover discovered resource IDs, then cleans up the discovery files
4. **jsc** — Writes per-type `*_import.tf` files (with singleton `secure_policy_import.tf`) and registers each resource in the **registry**
5. **terraform** — Runs `terraform plan -generate-config-out=generated.tf`
6. **postprocess** — Strips null attributes, rewrites references, splits into per-type files, runs validation auto-fix

### Key packages

- **pro** — Top-level package with `ResourceDef` table (single source of truth for filter keys, display names, TF types, output filenames), `DefaultRules()` (cross-reference rules), `RunPipeline()` (orchestrates the full Jamf Pro export), and `RunDiscoveryAndGenerate()` (steps 1–6 returning `IntermediateResult` for reuse by multi-env). Sub-packages:
  - **pro/client** — Authenticates to Jamf Pro using `go-api-sdk-jamfpro` (basic auth or OAuth2 client credentials).
  - **pro/discovery** — Queries the Jamf Pro API for each resource type, registers each in the **registry**, and produces `[]Resource` (JamfID, Name, Label).
  - **pro/download** — Downloads package files and enrollment customization images from Jamf Pro.
- **protect** — Handles Protect-specific logic: `Resources` table (single source of truth), `DefaultRules()` (cross-reference rules), `RunPipeline()` (orchestrates the full Jamf Protect export), `GenerateQueryFile()` creates `.tfquery.hcl` with `list` blocks and singleton import blocks, `RenameLabels()` renames auto-generated labels to friendly names from resource attributes, `PopulateRegistryFromGenerated()` extracts ID→address mappings from `identity { id = "..." }` blocks in terraform query output.
- **platform** — Handles Platform-specific logic: `Resources` table (single source of truth), `DefaultRules()` (cross-reference rules), `RunPipeline()` (orchestrates the full Jamf Platform export), `GenerateQueryFile()` creates `.tfquery.hcl` with `list` blocks, `RenameLabels()` renames auto-generated labels (benchmarks use `title` attribute, others use `name`), `PopulateRegistryFromGenerated()` extracts ID→address mappings from `identity { id = "..." }` blocks.
- **jsc** — Handles JSC-specific logic: `Resources` table, `DefaultRules()`, `RunPipeline()` (orchestrates the full JSC export), `generateDiscoveryConfig()` writes data-source blocks, `parseDiscoveryState()` reads the Terraform state to recover discovered resource IDs and labels, and `writeJSCImportFiles()` writes per-type import files. JSC uses `terraform apply` for discovery because the provider does not implement list resources.
- **terraform** — Wraps all terraform operations via `tfexec`: `EnsureTerraform()` downloads the binary to a temp directory, `Init()` runs `terraform init`, `Apply()` runs `terraform apply`, `GenerateConfig()` runs `terraform plan -generate-config-out` with retry logic, `Query()` runs `terraform query` via `tf.QueryJSON()`, `ProvidersSchema()` loads typed provider schema, `Validate()` runs `terraform validate` and returns structured diagnostics, `FormatDir()` runs `terraform fmt`, `ResolvedProviderVersion()` reads the selected version from `.terraform.lock.hcl`. All functions accept provider credentials as `map[string]string` env vars — no provider-specific credential types.
- **importgen** — Writes `provider.tf`, `variables.tf`, `terraform.tfvars`, and per-type import files. Has separate `Credentials` (Jamf Pro, with auth method branching), `ProtectCredentials` (Jamf Protect, OAuth2 only), `PlatformCredentials` (Jamf Platform, OAuth2 only), and `JSCCredentials` (JSC, basic auth only) types because the generated HCL differs by provider. Provider credential variables (`client_id`, `client_secret`, `password`) are marked `sensitive = true` in `variables.tf` for all providers.
- **registry** — Thread-safe map of `(resourceType, jamfID) → terraformAddress`. Used by discovery (to register) and postprocess (to resolve references). `ResolveAny()` tries multiple resource types for fields like `computer_group_ids` that can reference either smart or static groups.
- **naming** — `Sanitize()` converts Jamf object names to valid TF labels. `Tracker` handles label collisions by appending `_2`, `_3`, etc. `StripScriptExtension()` removes file extensions from script names before label generation.
- **postprocess** — Split into focused files: `processor.go` (orchestration), `extract.go` (script/profile/token/appconfig file extraction), `schema.go` (provider schema parsing and null stripping, accepts `*tfjson.ProviderSchemas`), `rewrite.go` (reference rewriting), `hcl.go` (HCL token utilities), `format.go` (attribute/block ordering), `validate.go` (auto-fix validation errors via `terraform validate`), `labels.go` (`RenameLabels()` used by the Protect/Platform/JSC pipelines to rewrite auto-generated labels like `all_0` to friendly names), `category.go` (splits categorised resources into per-category output files when `-split-by-category` is set). `references.go` defines the `ReferenceRule` type; rules are defined per-provider in `pro/resources.go`, `protect/resources.go`, `platform/resources.go`, and `jsc/resources.go`.
- **compact** — Provider-agnostic post-processing that consolidates simple, uniform resource types into a single `for_each` + `locals` block. A resource type is eligible when the output file contains ≥2 resource blocks, all blocks share the same attribute names, and none have nested blocks (other than `lifecycle`). Gated by `-compact`; include/exclude via `-compact-include` / `-compact-exclude`.
- **secrets** — Post-generation secret scanning using gitleaks (MIT licensed). `scanner.go` runs gitleaks with default rules plus Jamf-specific rules (HCL passwords, plist secrets, LDAP/SMTP/WiFi credentials) against the output directory; the `jamf-plist-password` regex uses `(?is)` flags (case-insensitive + dotall) to match multiline XML/mobileconfig files where `<key>` and `<string>` are on separate lines. `report.go` prints findings grouped by resource address and attribute name. `remediate.go` moves secrets to sensitive Terraform variables: for `.tf` files, if the secret is the entire quoted value it is replaced with `var.<name>`, otherwise (embedded in a larger string like `"enrollauthtoken=SECRET"`) it uses HCL string interpolation `"enrollauthtoken=${var.name}"`; secrets that can't be replaced are skipped with a warning. For `support_files/`, the first finding in a file converts it to a `.tpl` template with `templatefile()` (escaping existing `${}` as `$${}` for shell scripts), and subsequent findings in the same file update the `.tpl` in place and add their variable to the existing `templatefile()` variable map. Variable names are lowercased via `sanitizeVarName`. After remediation, `terraform.FormatDir()` runs for consistent formatting. In interactive mode, users choose `[a]ll` to remediate everything, `[s]elect individually` to walk through each finding, or `[N]one` to skip. Scanned after all pipelines complete; controlled by `-skip-secret-scan` flag.

- **multienv** — Multi-environment support producing a module + per-environment directory structure for long-lived branch workflows. `pipeline.go` orchestrates running `pro.RunDiscoveryAndGenerate()` per-env into temp dirs, then assembling output. `match.go` matches resources across envs by `(type, label)`. `diff.go` compares attribute values and extracts differences to variables. `module.go` classifies support files (shared vs divergent via SHA-256 hash), assembles the `modules/jamf/` directory, splits partial-env resources into `*_<env>_only.tf` files, rewrites divergent file references to module variables, and generates `modules/jamf/variables.tf`. `envroot.go` generates per-environment root directories (`environments/<env>/`) with `main.tf` (provider + module call), `backend.tf` (placeholder), `variables.tf`, `terraform.tfvars`, `imports.tf` (with `module.jamf.` prefix), and divergent support file copies. Designed for git branching workflows where each branch represents an environment.

### Adding a new Jamf Pro discoverable resource type

1. Add a discovery function in `pro/discovery/` (follow `dock_items.go` for Classic API or `categories.go` for Jamf Pro API)
2. Add the field to `Results` struct in `pro/discovery/discovery.go`
3. Call it from `DiscoverAll()` in dependency order
4. Add a `ResourceDef` entry to `pro/resources.go` `Resources` table (this is the single source of truth for filter key, display name, TF type, and output filename)
5. Add entry to the `importFiles` table in `importgen/generator.go`
6. Add `printCount` line in `pro/pipeline.go` for the new type
7. Add reference rules to `pro.DefaultRules()` in `pro/resources.go` if the new type is referenced by other resources
8. Add any special post-processing (like script extraction) in `postprocess/processor.go`
9. Add tests in the relevant `_test.go` files (see test guidance above)
10. Update README.md

### Adding a new Jamf Pro singleton settings resource

1. Add a `ResourceDef` entry to `pro/resources.go` `Resources` table with `IsSingleton: true` and `SingletonImportID` set to the provider's fixed import ID (e.g. `"jamfpro_smtp_server_singleton"`)
2. No discovery function needed — singletons are populated in `pro/pipeline.go` from the `Resources` table
3. The `importgen` package handles singleton import file generation automatically via `resources.Singletons`
4. Add tests in the relevant `_test.go` files (update `TestTypeToFileMap`, `TestSingletonCount`)
5. Update README.md

### Adding a new Jamf Protect resource type

1. Add a `ResourceDef` entry to `protect/resources.go` `Resources` table
2. If listable: add to `listableResourceTypes` in `protect/query.go`; if singleton: add to `singletonResources`
3. Add reference rules to `protect.DefaultRules()` in `protect/resources.go` if the new type is referenced by other resources
4. Add tests in the relevant `_test.go` files
5. Update README.md (resource table, output structure, cross-reference table)

### Adding a new Jamf Platform resource type

1. Add an entry to `listablePro` (or `singletonPro`, or `nativeResources`) in `platform/resources.go` — `buildResources()` derives the `Resources` table, `TypeToFileMap`, `ValidFilterNames`, `ListableResources()`, and `SingletonResources()` from these. `GenerateQueryFile` (query.go) emits list blocks from `ListableResources()`, so no separate map to update
2. Add reference rules to `platform.DefaultRules()` if the new type is referenced by other resources — use `AttrPath` for nested object attributes (e.g. `["general"]`, `["scope","targets"]`) and `ElementAttr` for list-of-objects (e.g. `scripts = [ { id } ]`); route computer/mobile group scope to `DeviceGroupComputerType` / `DeviceGroupMobileType` with `TargetAttr: "jamf_pro_id"`
3. Add an `ExtractSpec` to `platform.ExtractSpecs()` if the type carries a long string (script/payload/app-config) to extract to a support file. For document content that isn't a script/`.mobileconfig`/`.xml`, use `FileKindRaw` with an explicit `Ext` (e.g. `.ppd`, `.mobileprovision`)
4. If the resource uses a non-`name` attribute for labels, update `nameAttrForType()` in `platform/labels.go`
5. Add tests in the relevant `_test.go` files (update the count expectations in `resources_test.go`)
6. Update README.md (resource table, output structure, cross-reference table)

### Adding a new JSC resource type

1. Add a `ResourceDef` entry to `jsc/resources.go` `Resources` table
2. Add a discovery data-source block to `generateDiscoveryConfig()` in `jsc/discovery.go` and extend `parseDiscoveryState()` + `DiscoveryResults` to capture the new type
3. Add an entry to `writeJSCImportFiles()` in `jsc/pipeline.go` (or `writeSingletonImportFile` for singletons)
4. Add reference rules to `jsc.DefaultRules()` if cross-referenced
5. Add tests in the relevant `_test.go` files
6. Update README.md

## Important details

- The Jamf SDK's `InstanceDomain` needs the full URL with `https://` scheme
- The SDK log level is set to `"error"` to suppress noisy auth token messages
- OAuth2 token lifetime: `pro/client` probes `/api/oauth/token` to determine `expires_in`, sets `TokenRefreshBufferPeriod` to `lifetime/2` (min 5s). The value is stored in a package-level var and exported via `TokenRefreshBufferPeriod()`. It's set as `token_refresh_buffer_period_seconds` in the generated provider block (both the temp block used during `terraform plan` and the final user-facing block). The provider has no env var fallback for this attribute — it must be in the provider block.
- Classic API resources (sites, dock items, printers, network segments, smart groups, static groups, macOS profiles, policies) use int IDs converted to strings
- Jamf Pro API resources (buildings, categories, departments, scripts, extension attributes, packages) use string IDs natively
- Import block `to` fields use raw `hclwrite.Tokens` with `TokenIdent` (type 9), not string values
- `category_id = -1` and `site_id = -1` are both removed before reference rewriting; `-1` tokenises as minus + number so raw expression bytes are checked
- `hclsyntax.ParseConfig` is used to properly evaluate HCL string expressions; manual unescaping breaks bash constructs like `\n` inside strings
- Go build constraint: files ending `_ios.go` are treated as `GOOS=ios` only — use a different naming pattern (e.g. `self_service_ios_branding.go` instead of `self_service_branding_ios.go`)
- Credentials (JAMF_USERNAME, JAMF_PASSWORD, JAMF_CLIENT_ID, JAMF_CLIENT_SECRET) are sourced from environment variables or interactive prompts only — no CLI flags — to avoid leaking secrets in shell history and process listings
- Non-credential CLI flags can be set via env vars (JAMF_URL, JAMFORMER_OUTPUT, JAMFORMER_VERBOSE, JAMFORMER_SKIP_PACKAGE_DOWNLOADS, JAMFORMER_TERRAFORM_PATH, JAMFORMER_RESOURCES, JAMFORMER_EXCLUDE, JAMFORMER_SKIP_REFERENCES, JAMFORMER_SKIP_IMPORT_BLOCKS, JAMFORMER_PROVIDER, JAMFORMER_PROVIDER_VERSION, JAMFORMER_ALLOW_DEV_OVERRIDES, JAMFORMER_SKIP_SECRET_SCAN, JAMFORMER_COMPACT, JAMFORMER_COMPACT_INCLUDE, JAMFORMER_COMPACT_EXCLUDE, JAMFORMER_SPLIT_BY_CATEGORY, JAMFORMER_PARALLELISM, JAMFORMER_MULTI_ENV, JAMFORMER_SOURCE_ENV); flags take priority
- `JAMF_TENANT_ID` sets the Jamf Platform tenant ID (optional; passed as `JAMFPLATFORM_TENANT_ID` to the provider and written to `provider.tf`, `variables.tf`, `terraform.tfvars` in the output — omitted entirely when not set)
- Multi-env mode uses env-name-suffixed credentials: e.g. `-multi-env dev,prod` requires `JAMF_URL_DEV`/`JAMF_URL_PROD` plus matching OAuth2 or basic auth vars per env
- `-include-resources` accepts space-separated friendly names to filter discovery (commas also accepted); when a dependency type isn't selected, references stay as raw IDs
- Icon discovery depends on listing policies/profiles/apps to extract icon IDs; lightweight listing (with label generation) is used when those types aren't selected. Icon resource labels are derived from the referencing resource's label (e.g. `jamfpro_icon.install_chrome` for an icon used by `jamfpro_policy.install_chrome`). Icons are not downloaded locally — post-processing sets `icon_file_web_source` to the CDN URL and adds `lifecycle { ignore_changes = [icon_file_web_source] }` to prevent destroy/create on first apply (the provider treats icon source attributes as ForceNew)
- Interactive prompts appear when required credentials are missing — passwords/secrets use hidden input via `golang.org/x/term`
- Non-interactive (no TTY) environments fail fast with clear error if credentials are missing — safe for CI
- Terraform is automatically downloaded to `os.TempDir()/jamformer-terraform/` (ephemeral, cleaned on reboot); `-terraform-path` / `JAMFORMER_TERRAFORM_PATH` overrides this to use a pre-installed binary
- All terraform commands are executed via `tfexec` (init, plan, query, providers schema, fmt) — no direct `os/exec` shell-outs in the terraform package
- Terraform output is suppressed by default; use `-verbose` to show it (stderr is still captured and shown on failure)
- `tfexec` manages `TF_IN_AUTOMATION` and `TF_INPUT` internally — do not pass these via `SetEnv` or they will be rejected; inherited env vars managed by tfexec are filtered out before calling `SetEnv`
- When testing against a live instance, always use `-skip-package-downloads` to avoid downloading large files from the Cloud Distribution Point
- `-provider jamfprotect`, `-provider jsc`, or `-provider jamfpro` (or `JAMFORMER_PROVIDER` env var) selects the pipeline; default is `jamfplatform`. The interactive picker lists Platform → Protect → JSC → Pro, with Pro flagged as the Deployment Theory community provider
- Jamf Protect and Jamf Platform only support OAuth2; JSC only supports basic auth; each rejects the wrong auth mode with a fatal error
- Protect and Platform use `terraform query` (Terraform 1.14+) instead of API-based discovery — no custom SDK client needed
- JSC uses `terraform apply` against data sources for discovery (the JSC provider does not implement list resources); credentials are passed through `terraform.tfvars` rather than env vars
- Protect/Platform import blocks from `terraform query` use `identity = { id = "..." }` format; Protect singletons use flat `id = "..."`
- Protect URL shorthand: `tenant` → `tenant.protect.jamfcloud.com`; Platform uses regional API gateway URLs directly (no shorthand expansion)
- Protect and JSC skip: script extraction, profile extraction, package downloads, icon handling, category_id/-1 removal
- Jamf Platform downloads package files from the Jamf Cloud Distribution Service (JCDS) when `JAMF_TENANT_ID` is set and `-skip-package-downloads` is not passed (only JCDS-resident files are fetchable; the rest stay as metadata + server hashes). It skips category_id/-1 removal, but does extract scripts/profiles/app-configs (plus printer PPDs and mobile-device provisioning profiles), synthesises `jamfplatform_pro_icon` resources from policy `self_service_icon` references, and discovers `jamfplatform_pro_jamf_connect` config-profile links via the federated `pro` SDK (no list resource — also requires `JAMF_TENANT_ID`) (see the Platform pipeline above)
- `-provider-version` pins a specific provider version; otherwise the version resolved by `terraform init` is read from `.terraform.lock.hcl` and written back into `provider.tf`
- `-allow-dev-overrides` lets local `dev_overrides` in `~/.terraformrc` take effect; the pipeline tolerates the "provider not found" error that `terraform init` produces in that mode
- `appendToFile` in `secrets/remediate.go` uses `os.O_CREATE|os.O_APPEND|os.O_WRONLY` so the file is created if it doesn't exist
