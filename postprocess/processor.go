// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/zclconf/go-cty/cty"
)

// Quiet suppresses progress messages.
var Quiet bool

// ProcessOptions holds configuration for post-processing.
type ProcessOptions struct {
	// TypeToFileMap maps TF resource type → output filename. Required.
	TypeToFileMap map[string]string
	// Rules defines the cross-resource reference rewriting rules.
	Rules []ReferenceRule
	// PackageFiles maps package filename -> relative path for downloaded packages.
	PackageFiles map[string]string
	// PackageInfo maps package_name -> filename from discovery.
	PackageInfo map[string]string
	// IconURLs maps icon Jamf ID -> CDN URL for icon_file_web_source.
	IconURLs map[string]string
	// EnrollmentCustomizationImageFiles maps enrollment customization Jamf ID -> relative path.
	EnrollmentCustomizationImageFiles map[string]string
	// ExtractSpecs declaratively drives string-attribute extraction to support
	// files (scripts, profile payloads, app configs). Used by providers whose
	// resources expose these as nested object attributes (e.g. jamfplatform).
	// The jamfpro pipeline uses dedicated inline extraction instead.
	ExtractSpecs []ExtractSpec
	// SupportDirs lists extra directories (relative to support_files/) to create
	// as recommended locations for user-supplied files (e.g. token directories).
	SupportDirs []string
	// SkipReferences disables cross-resource reference resolution, leaving raw ID values.
	SkipReferences bool
	// SplitByCategory splits categorised resource types into per-category output files.
	SplitByCategory bool
	// ProviderSchemas is the parsed provider schema from tfexec (for null stripping).
	ProviderSchemas *tfjson.ProviderSchemas
	// SupportFilesPrefix, when set, is inserted into support file paths:
	// support_files/<prefix>/scripts/ instead of support_files/scripts/.
	// Used by multi-env mode to separate files per environment.
	SupportFilesPrefix string
	// InjectRequiredWriteOnly wires Required WriteOnly attributes the server never
	// returns to sensitive Terraform variables (and seeds their _wo_version
	// companions with 1) so a generated config validates. Used by jamfplatform.
	InjectRequiredWriteOnly bool
}

// Process reads the generated HCL file, rewrites ID references, extracts
// script contents to files, and splits into per-type output files.
func Process(outputDir, generatedFile string, reg *registry.Registry, opts *ProcessOptions) error {
	if opts == nil {
		opts = &ProcessOptions{}
	}
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return fmt.Errorf("reading generated file: %w", err)
	}

	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	typeMap := opts.TypeToFileMap
	rules := opts.Rules

	// Determine which resource types should be split into per-category files.
	// Only active when SplitByCategory is set and categories were discovered
	// (otherwise everything would land in *_uncategorised.tf).
	var catSplitTypes map[string]bool
	if opts.SplitByCategory && reg.HasType("jamfpro_category") {
		catSplitTypes = categorySplitTypes(rules)
	}
	categoryFileMap := make(map[string]*hclwrite.File)    // "type:category" → file
	labelCategories := make(map[string]map[string]string) // resourceType → (label → categoryLabel)

	// Determine if Jamf Pro-specific processing is needed (support_files, category/site removal)
	isJamfPro := false
	for typeName := range typeMap {
		if strings.HasPrefix(typeName, "jamfpro_") {
			isJamfPro = true
			break
		}
	}

	// Load provider schema to determine required vs optional attributes
	var schema *ProviderSchema
	if opts.ProviderSchemas != nil {
		schema = LoadProviderSchema(opts.ProviderSchemas)
	}

	// Create support_files output directories (Jamf Pro only)
	var scriptsDir, extensionAttrsDir, profilesDir, mobileDeviceProfilesDir, appConfigsDir string
	var scriptFileNames, eaFileNames, profileFileNames, mobileDeviceProfileFileNames, appConfigFileNames map[string]int
	var tokenVars []TokenVar
	// Build support_files base path, optionally with an env prefix for multi-env mode
	supportBase := filepath.Join(outputDir, "support_files")
	if opts.SupportFilesPrefix != "" {
		supportBase = filepath.Join(outputDir, "support_files", opts.SupportFilesPrefix)
	}

	if isJamfPro {
		scriptsDir = filepath.Join(supportBase, "scripts")
		if err := os.MkdirAll(scriptsDir, 0755); err != nil {
			return fmt.Errorf("creating scripts directory: %w", err)
		}
		extensionAttrsDir = filepath.Join(supportBase, "extension_attributes")
		if err := os.MkdirAll(extensionAttrsDir, 0755); err != nil {
			return fmt.Errorf("creating extension attributes directory: %w", err)
		}
		profilesDir = filepath.Join(supportBase, "macos_configuration_profiles")
		if err := os.MkdirAll(profilesDir, 0755); err != nil {
			return fmt.Errorf("creating macOS profiles directory: %w", err)
		}
		mobileDeviceProfilesDir = filepath.Join(supportBase, "mobile_device_configuration_profiles")
		if err := os.MkdirAll(mobileDeviceProfilesDir, 0755); err != nil {
			return fmt.Errorf("creating mobile device profiles directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(supportBase, "device_enrollment_tokens"), 0755); err != nil {
			return fmt.Errorf("creating ADE tokens directory: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(supportBase, "volume_purchasing_tokens"), 0755); err != nil {
			return fmt.Errorf("creating VPP tokens directory: %w", err)
		}
		appConfigsDir = filepath.Join(supportBase, "app_configurations")
		if err := os.MkdirAll(appConfigsDir, 0755); err != nil {
			return fmt.Errorf("creating app configurations directory: %w", err)
		}

		scriptFileNames = make(map[string]int)
		eaFileNames = make(map[string]int)
		profileFileNames = make(map[string]int)
		mobileDeviceProfileFileNames = make(map[string]int)
		appConfigFileNames = make(map[string]int)
	}

	// Relative path base for file() references in HCL (e.g. "support_files" or "support_files/dev")
	supportRelBase := "support_files"
	if opts.SupportFilesPrefix != "" {
		supportRelBase = filepath.Join("support_files", opts.SupportFilesPrefix)
	}

	// Spec-driven extraction (e.g. jamfplatform): create the output subdirectory
	// for each spec plus any recommended SupportDirs, and track collision-safe
	// filenames per subdirectory.
	specFileNames := make(map[string]map[string]int) // OutputSubdir -> (filename -> count)
	for _, spec := range opts.ExtractSpecs {
		if specFileNames[spec.OutputSubdir] == nil {
			specFileNames[spec.OutputSubdir] = make(map[string]int)
			if err := os.MkdirAll(filepath.Join(supportBase, spec.OutputSubdir), 0755); err != nil {
				return fmt.Errorf("creating %s directory: %w", spec.OutputSubdir, err)
			}
		}
	}
	for _, dir := range opts.SupportDirs {
		if err := os.MkdirAll(filepath.Join(supportBase, dir), 0755); err != nil {
			return fmt.Errorf("creating %s directory: %w", dir, err)
		}
	}

	// Group resource blocks by type
	fileMap := make(map[string]*hclwrite.File)
	for typeName := range typeMap {
		fileMap[typeName] = hclwrite.NewEmptyFile()
	}

	// Separate file map for import blocks (Protect/Platform generate these in
	// generated.tf). Import blocks are collected and written after the resource
	// pass so we can drop imports whose target resource was skipped.
	importFileMap := make(map[string]*hclwrite.File)
	for typeName := range typeMap {
		importFileMap[typeName] = hclwrite.NewEmptyFile()
	}
	var pendingImports []*hclwrite.Block  // import blocks awaiting skip filtering
	skippedAddrs := make(map[string]bool) // resource addresses dropped by a skip rule

	// Count resources per type for progress output.
	typeCounts := make(map[string]int)
	for _, block := range f.Body().Blocks() {
		if block.Type() == "resource" {
			labels := block.Labels()
			if len(labels) >= 2 {
				typeCounts[labels[0]]++
			}
		}
	}

	// Sensitive variables synthesised for Required WriteOnly attributes the server
	// never returns; appended to variables.tf/terraform.tfvars after the block pass.
	var requiredVars []requiredVar
	requiredVarNames := make(map[string]bool)

	processedTypes := make(map[string]bool)
	for _, block := range f.Body().Blocks() {
		// Collect import blocks; they are written after the resource pass so
		// imports targeting a skipped resource can be dropped.
		if block.Type() == "import" {
			pendingImports = append(pendingImports, block)
			continue
		}

		if block.Type() != "resource" {
			continue
		}

		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		resourceType := labels[0]

		// Print progress when we encounter a new resource type
		if !processedTypes[resourceType] {
			processedTypes[resourceType] = true
			if filename, ok := typeMap[resourceType]; ok {
				typeName := strings.TrimSuffix(filename, ".tf")
				if !Quiet {
					fmt.Printf("  Processing %s (%d resources)...\n", typeName, typeCounts[resourceType])
				}
			}
		}

		// Remove optional null attributes (requires schema)
		if schema != nil {
			stripNullAttributes(block.Body(), resourceType, "", schema)
		}

		// Determine category label for per-category file splitting. Must happen
		// before -1 removal strips the attribute and before reference rewriting.
		var categoryLabel string
		if catSplitTypes[resourceType] {
			if attr := block.Body().GetAttribute("category_id"); attr != nil {
				exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if exprBytes == "-1" || exprBytes == "\"-1\"" {
					categoryLabel = "uncategorised"
				}
			} else {
				categoryLabel = "uncategorised"
			}
		}

		// Jamf Pro-specific: remove category_id = -1 and site_id = -1
		if isJamfPro {
			// Remove category_id = -1 (uncategorized) before reference rewriting —
			// the provider maps -1 to category ID 1 on plan, causing drift.
			// -1 is tokenized as minus + number, so we check the raw expression bytes.
			if attr := block.Body().GetAttribute("category_id"); attr != nil {
				exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
				if exprBytes == "-1" || exprBytes == "\"-1\"" {
					block.Body().RemoveAttribute("category_id")
				}
			}

			// Remove site_id = -1 (no site) — the provider maps -1 to 1 on plan,
			// causing drift. Removing it lets the provider use its default.
			// Exception: prestage enrollments require site_id (even as -1).
			if resourceType != "jamfpro_computer_prestage_enrollment" && resourceType != "jamfpro_mobile_device_prestage_enrollment" {
				if attr := block.Body().GetAttribute("site_id"); attr != nil {
					exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
					if exprBytes == "-1" || exprBytes == "\"-1\"" {
						block.Body().RemoveAttribute("site_id")
					}
				}

				if attr := block.Body().GetAttribute("enrollment_site_id"); attr != nil {
					exprBytes := strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
					if exprBytes == "-1" || exprBytes == "\"-1\"" {
						block.Body().RemoveAttribute("enrollment_site_id")
					}
				}
			}
		}

		// Apply reference rules to this block (unless skipped)
		if !opts.SkipReferences {
			for _, rule := range rules {
				if rule.ResourceType != resourceType {
					continue
				}
				rewriteBlock(block.Body(), rule.BlockPath, rule, reg)
			}
		}

		// Extract category label from rewritten reference (if not already
		// determined as uncategorised from -1 removal above)
		if catSplitTypes[resourceType] && categoryLabel == "" {
			categoryLabel = extractCategoryLabel(block.Body(), reg)
			if categoryLabel == "" {
				categoryLabel = "uncategorised"
			}
		}

		// Spec-driven skip pass: drop vendor-managed/signed resources entirely.
		skipResource := false
		for _, spec := range opts.ExtractSpecs {
			if spec.ResourceType != resourceType || spec.SkipFn == nil {
				continue
			}
			if skip, reason := spec.SkipFn(specContent(block.Body(), spec)); skip {
				nameAttrName := spec.NameAttr
				if nameAttrName == "" {
					nameAttrName = "name"
				}
				name := labels[1]
				if nm := readLeafString(block.Body(), nil, spec.NameAttrPath, nameAttrName); nm != "" {
					name = nm
				}
				if !Quiet {
					fmt.Printf("  Skipping %s %q: %s\n", resourceType, name, reason)
				}
				_ = terraform.RemoveImportBlock(outputDir, resourceType+"."+labels[1])
				skippedAddrs[resourceType+"."+labels[1]] = true
				skipResource = true
				break
			}
		}
		if skipResource {
			continue
		}

		// Wire Required WriteOnly secrets the server never returns to sensitive
		// variables (and seed their _wo_version companions) so the config validates.
		if opts.InjectRequiredWriteOnly && schema != nil {
			requiredVars = append(requiredVars, injectRequiredWriteOnly(block.Body(), resourceType, labels[1], schema, requiredVarNames)...)
		}

		// Jamf Pro-specific resource processing (script/profile/package extraction)
		if isJamfPro && resourceType == "jamfpro_script" {
			if err := extractScriptContents(block.Body(), scriptsDir, filepath.Join(supportRelBase, "scripts"), scriptFileNames); err != nil {
				return fmt.Errorf("extracting script contents: %w", err)
			}
		}

		if isJamfPro && resourceType == "jamfpro_computer_extension_attribute" {
			if block.Body().GetAttribute("script_contents") != nil {
				if err := extractScriptContents(block.Body(), extensionAttrsDir, filepath.Join(supportRelBase, "extension_attributes"), eaFileNames); err != nil {
					return fmt.Errorf("extracting extension attribute script: %w", err)
				}
			}
		}

		if isJamfPro && resourceType == "jamfpro_package" {
			// Set package_file_source — required attribute that the provider's
			// Read doesn't return during generate-config-out
			if attr := block.Body().GetAttribute("package_file_source"); attr == nil || isNullValue(attr) {
				resolved := false

				// Look up filename from package_name via discovery info
				if nameAttr := block.Body().GetAttribute("package_name"); nameAttr != nil {
					pkgName := ExtractStringValue(nameAttr)
					if filename, ok := opts.PackageInfo[pkgName]; ok {
						if relPath, ok := opts.PackageFiles[filename]; ok {
							fileRef := fmt.Sprintf(`"${path.module}/%s"`, relPath)
							block.Body().SetAttributeRaw("package_file_source", hclwrite.Tokens{
								{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
							})
							resolved = true
						} else {
							block.Body().SetAttributeValue("package_file_source", cty.StringVal("# TODO: set path to "+filename))
							resolved = true
						}
					}
				}

				if !resolved {
					block.Body().SetAttributeValue("package_file_source", cty.StringVal("# TODO: set package file path"))
				}
			}
		}

		if isJamfPro && resourceType == "jamfpro_macos_configuration_profile_plist" {
			// Skip vendor-managed or signed profiles that cannot be re-applied via the API.
			if payloadsAttr := block.Body().GetAttribute("payloads"); payloadsAttr != nil {
				if payload := extractFullStringValue(payloadsAttr); payload != "" {
					if skip, reason := ShouldSkipProfile(payload); skip {
						profileName := labels[1]
						if nameAttr := block.Body().GetAttribute("name"); nameAttr != nil {
							profileName = ExtractStringValue(nameAttr)
						}
						fmt.Printf("  Skipping profile %q: %s\n", profileName, reason)
						addr := resourceType + "." + labels[1]
						_ = terraform.RemoveImportBlock(outputDir, addr)
						continue
					}
				}
			}

			// Fix redeploy_on_update — required attribute that the provider's
			// Read returns as null during generate-config-out
			if attr := block.Body().GetAttribute("redeploy_on_update"); attr == nil || isNullValue(attr) {
				block.Body().SetAttributeValue("redeploy_on_update", cty.StringVal("Newly Assigned"))
			}

			// Fix payload_validate — set to false to avoid validation failures
			// on imported profiles that may contain non-standard payloads
			if block.Body().GetAttribute("payload_validate") == nil {
				block.Body().SetAttributeValue("payload_validate", cty.False)
			}

			if err := extractProfilePayloads(block.Body(), profilesDir, supportRelBase, "macos_configuration_profiles", profileFileNames); err != nil {
				return fmt.Errorf("extracting profile payloads: %w", err)
			}
		}

		if isJamfPro && resourceType == "jamfpro_mobile_device_configuration_profile_plist" {
			// Skip vendor-managed or signed profiles that cannot be re-applied via the API.
			if payloadsAttr := block.Body().GetAttribute("payloads"); payloadsAttr != nil {
				if payload := extractFullStringValue(payloadsAttr); payload != "" {
					if skip, reason := ShouldSkipProfile(payload); skip {
						profileName := labels[1]
						if nameAttr := block.Body().GetAttribute("name"); nameAttr != nil {
							profileName = ExtractStringValue(nameAttr)
						}
						fmt.Printf("  Skipping profile %q: %s\n", profileName, reason)
						addr := resourceType + "." + labels[1]
						_ = terraform.RemoveImportBlock(outputDir, addr)
						continue
					}
				}
			}

			if attr := block.Body().GetAttribute("redeploy_on_update"); attr == nil || isNullValue(attr) {
				block.Body().SetAttributeValue("redeploy_on_update", cty.StringVal("Newly Assigned"))
			}

			// Fix payload_validate — set to false to avoid validation failures
			// on imported profiles that may contain non-standard payloads
			if block.Body().GetAttribute("payload_validate") == nil {
				block.Body().SetAttributeValue("payload_validate", cty.False)
			}

			if err := extractProfilePayloads(block.Body(), mobileDeviceProfilesDir, supportRelBase, "mobile_device_configuration_profiles", mobileDeviceProfileFileNames); err != nil {
				return fmt.Errorf("extracting mobile device profile payloads: %w", err)
			}
		}

		if isJamfPro && resourceType == "jamfpro_icon" {
			// Use icon_file_web_source with the CDN URL instead of icon_file_path.
			if len(labels) >= 2 {
				resolved := false
				if opts.IconURLs != nil {
					for iconID, url := range opts.IconURLs {
						if ref, ok := reg.Resolve("jamfpro_icon", iconID); ok {
							if ref == fmt.Sprintf("jamfpro_icon.%s", labels[1]) {
								block.Body().SetAttributeValue("icon_file_web_source", cty.StringVal(url))
								block.Body().RemoveAttribute("icon_file_path")
								resolved = true
								break
							}
						}
					}
				}
				if !resolved {
					if attr := block.Body().GetAttribute("icon_file_path"); attr == nil || isNullValue(attr) {
						block.Body().SetAttributeValue("icon_file_path", cty.StringVal("# TODO: set path to icon PNG file"))
					}
				}
			}

			// The provider treats icon source attributes as ForceNew — any value
			// set on an imported icon triggers destroy/create. ignore_changes
			// prevents this on first apply; users remove it when they want to
			// update an icon.
			lifecycle := block.Body().AppendNewBlock("lifecycle", nil)
			lifecycle.Body().SetAttributeRaw("ignore_changes", hclwrite.Tokens{
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenIdent, Bytes: []byte("icon_file_web_source")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
			})
		}

		if isJamfPro && resourceType == "jamfpro_device_enrollments" {
			if tv := extractTokenVar(block.Body(), "encoded_token", "dep_token_path_", labels[1], ".p7m"); tv != nil {
				tokenVars = append(tokenVars, *tv)
			}
		}

		if isJamfPro && resourceType == "jamfpro_enrollment_customization" {
			if attr := block.Body().GetAttribute("enrollment_customization_image_source"); attr == nil || isNullValue(attr) {
				if len(labels) >= 2 {
					resolved := false
					if opts.EnrollmentCustomizationImageFiles != nil {
						for ecID, relPath := range opts.EnrollmentCustomizationImageFiles {
							if ref, ok := reg.Resolve("jamfpro_enrollment_customization", ecID); ok {
								if ref == fmt.Sprintf("jamfpro_enrollment_customization.%s", labels[1]) {
									fileRef := fmt.Sprintf(`"${path.module}/%s"`, relPath)
									block.Body().SetAttributeRaw("enrollment_customization_image_source", hclwrite.Tokens{
										{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
									})
									resolved = true
									break
								}
							}
						}
					}
					if !resolved {
						block.Body().SetAttributeValue("enrollment_customization_image_source", cty.StringVal("# TODO: set path to enrollment customization image (PNG/GIF, 180x180px recommended)"))
					}
				}
			}
		}

		if isJamfPro && resourceType == "jamfpro_volume_purchasing_locations" {
			if tv := extractTokenVar(block.Body(), "service_token", "vpp_token_path_", labels[1], ".vpptoken"); tv != nil {
				tokenVars = append(tokenVars, *tv)
			}
		}

		if isJamfPro && resourceType == "jamfpro_mobile_device_application" {
			if err := extractAppConfiguration(block.Body(), appConfigsDir, supportRelBase, appConfigFileNames); err != nil {
				return fmt.Errorf("extracting app configuration: %w", err)
			}
		}

		// Spec-driven extraction pass (e.g. jamfplatform): write nested string
		// attributes to support files and replace them with file() references.
		for _, spec := range opts.ExtractSpecs {
			if spec.ResourceType != resourceType {
				continue
			}
			absDir := filepath.Join(supportBase, spec.OutputSubdir)
			relDir := filepath.Join(supportRelBase, spec.OutputSubdir)
			if _, err := extractStringAttr(block.Body(), spec, absDir, relDir, specFileNames[spec.OutputSubdir]); err != nil {
				return fmt.Errorf("extracting %s for %s: %w", spec.AttrName, resourceType, err)
			}
		}

		// Append to the appropriate file (per-category or per-type)
		if catSplitTypes[resourceType] {
			key := resourceType + ":" + categoryLabel
			outFile, ok := categoryFileMap[key]
			if !ok {
				outFile = hclwrite.NewEmptyFile()
				categoryFileMap[key] = outFile
			}
			outFile.Body().AppendNewline()
			appendBlock(outFile.Body(), block)

			if labelCategories[resourceType] == nil {
				labelCategories[resourceType] = make(map[string]string)
			}
			labelCategories[resourceType][labels[1]] = categoryLabel
		} else if outFile, ok := fileMap[resourceType]; ok {
			outFile.Body().AppendNewline()
			appendBlock(outFile.Body(), block)
		}
	}

	// Append sensitive variable declarations for any Required WriteOnly attributes
	// that were wired to var.<name> above.
	if err := appendRequiredVars(outputDir, requiredVars); err != nil {
		return err
	}

	// Write per-type files
	for typeName, outFile := range fileMap {
		filename, ok := typeMap[typeName]
		if !ok {
			continue
		}

		content := outFile.Bytes()
		if len(strings.TrimSpace(string(content))) == 0 {
			continue
		}

		if err := os.WriteFile(filepath.Join(outputDir, filename), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
	}

	// Distribute collected import blocks into per-type import files, dropping any
	// whose target resource was skipped (e.g. a vendor-managed profile).
	for _, block := range pendingImports {
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		toBytes := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		if skippedAddrs[toBytes] {
			continue
		}
		parts := strings.SplitN(toBytes, ".", 2)
		if len(parts) < 1 {
			continue
		}
		if outFile, ok := importFileMap[parts[0]]; ok {
			outFile.Body().AppendNewline()
			appendBlock(outFile.Body(), block)
		}
	}

	// Write per-type import files (for import blocks found in generated.tf)
	for typeName, outFile := range importFileMap {
		filename, ok := typeMap[typeName]
		if !ok {
			continue
		}

		content := outFile.Bytes()
		if len(strings.TrimSpace(string(content))) == 0 {
			continue
		}

		importFilename := strings.TrimSuffix(filename, ".tf") + "_import.tf"
		if err := os.WriteFile(filepath.Join(outputDir, importFilename), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", importFilename, err)
		}
	}

	// Write per-category resource files
	for key, outFile := range categoryFileMap {
		parts := strings.SplitN(key, ":", 2)
		resourceType, catLabel := parts[0], parts[1]

		baseFilename, ok := typeMap[resourceType]
		if !ok {
			continue
		}

		content := outFile.Bytes()
		if len(strings.TrimSpace(string(content))) == 0 {
			continue
		}

		baseName := strings.TrimSuffix(baseFilename, ".tf")
		filename := baseName + "_" + catLabel + ".tf"
		if err := os.WriteFile(filepath.Join(outputDir, filename), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
	}

	// Split import files for categorized resource types
	for resourceType, labelCats := range labelCategories {
		baseFilename, ok := typeMap[resourceType]
		if !ok {
			continue
		}
		if err := splitImportFileByCategory(outputDir, baseFilename, labelCats); err != nil {
			return fmt.Errorf("splitting import file: %w", err)
		}
	}

	// Append token path variables to variables.tf
	if len(tokenVars) > 0 {
		appendTokenVars(outputDir, tokenVars)
	}

	// Final pass: run terraform fmt for consistent alignment and indentation
	terraform.FormatDir(outputDir)

	return nil
}

// appendTokenVars adds token file path variable definitions to variables.tf.
func appendTokenVars(outputDir string, vars []TokenVar) {
	varFile := filepath.Join(outputDir, "variables.tf")
	existing, _ := os.ReadFile(varFile) // ignore error: file may not exist yet
	content := string(existing)

	var newVars []byte
	for _, v := range vars {
		varDecl := fmt.Sprintf("variable %q", v.VarName)
		if strings.Contains(content, varDecl) {
			continue
		}
		block := fmt.Sprintf("\nvariable %q {\n  description = %q\n  type        = string\n}\n",
			v.VarName, v.Description)
		newVars = append(newVars, []byte(block)...)
	}

	if len(newVars) > 0 {
		f, err := os.OpenFile(varFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			if !Quiet {
				fmt.Printf("  Warning: could not write token variables to %s: %v\n", varFile, err)
			}
			return
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(newVars); err != nil && !Quiet {
			fmt.Printf("  Warning: could not write token variables to %s: %v\n", varFile, err)
		}
	}
}
