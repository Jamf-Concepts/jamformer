// Copyright 2026, Jamf Software LLC

package multienv

import (
	"fmt"
	"os"
	"path/filepath"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/Jamf-Concepts/jamformer/importgen"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/pro"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/Jamf-Concepts/jamformer/terraform"
)

// Quiet suppresses progress messages.
var Quiet bool

// RunPipeline orchestrates the multi-environment export:
//  1. Run discovery + terraform plan for each environment (into temp dirs)
//  2. Match resources across environments by (type, label)
//  3. Postprocess the source-of-truth environment's output
//  4. Extract support files for all environments
//  5. Classify support files as shared vs divergent
//  6. Assemble module directory and split partial-env resources
//  7. Diff attributes and extract differences to variables
//  8. Generate module variables
//  9. Generate per-environment root directories
//  10. Cleanup and format
func RunPipeline(opts *Options) error {
	logStep := func(format string, args ...any) {
		if !opts.Quiet {
			fmt.Printf(format+"\n", args...)
		}
	}

	sourceEnv := opts.SourceEnv
	if sourceEnv == "" {
		sourceEnv = opts.Envs[0].Name
	}
	envNames := make([]string, len(opts.Envs))
	for i, e := range opts.Envs {
		envNames[i] = e.Name
	}

	// 1. Run per-env pipelines into temp directories
	envResults := make(map[string]*PerEnvResult, len(opts.Envs))
	var tempDirs []string
	defer func() {
		for _, d := range tempDirs {
			_ = os.RemoveAll(d)
		}
	}()
	for _, env := range opts.Envs {
		logStep("Running pipeline for environment %q (%s)...", env.Name, env.URL)
		result, err := runPerEnv(env, opts)
		if err != nil {
			return fmt.Errorf("environment %q: %w", env.Name, err)
		}
		envResults[env.Name] = result
		tempDirs = append(tempDirs, result.OutputDir)
	}

	// 2. Match resources across environments
	logStep("Matching resources across %d environments...", len(opts.Envs))
	matches := MatchResources(envResults, envNames)

	allEnvCount := 0
	partialCount := 0
	for _, m := range matches {
		if m.AllEnvs {
			allEnvCount++
		} else {
			partialCount++
		}
	}
	logStep("  %d resources in all environments", allEnvCount)
	if partialCount > 0 {
		logStep("  %d resources in some environments only", partialCount)
	}

	// 3. Postprocess source-of-truth environment into output dir
	logStep("Post-processing source environment %q...", sourceEnv)
	sourceResult := envResults[sourceEnv]
	if sourceResult == nil {
		return fmt.Errorf("source environment %q not found in results", sourceEnv)
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Copy generated.tf and import files from source env to output
	sourceGenerated := filepath.Join(sourceResult.OutputDir, "generated.tf")
	outputGenerated := filepath.Join(opts.OutputDir, "generated.tf")
	data, err := os.ReadFile(sourceGenerated)
	if err != nil {
		return fmt.Errorf("reading source generated.tf: %w", err)
	}
	if err := os.WriteFile(outputGenerated, data, 0644); err != nil {
		return fmt.Errorf("writing generated.tf: %w", err)
	}

	// Copy import files from source env
	importFiles, _ := filepath.Glob(filepath.Join(sourceResult.OutputDir, "*_import.tf"))
	for _, f := range importFiles {
		importData, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(opts.OutputDir, filepath.Base(f)), importData, 0644); err != nil {
			return fmt.Errorf("copying import file %s: %w", filepath.Base(f), err)
		}
	}

	// Load provider schema from source env
	var schemas *tfjson.ProviderSchemas
	if sourceIR, ok := envResults[sourceEnv]; ok {
		s, _ := terraform.ProvidersSchema(sourceIR.OutputDir)
		schemas = s
	}

	// Postprocess source env with support files prefixed to support_files/<sourceEnv>/
	if err := postprocess.Process(opts.OutputDir, outputGenerated, sourceResult.Registry, &postprocess.ProcessOptions{
		TypeToFileMap:      pro.TypeToFileMap(),
		Rules:              pro.DefaultRules(),
		SkipReferences:     opts.SkipReferences,
		ProviderSchemas:    schemas,
		SupportFilesPrefix: sourceEnv,
	}); err != nil {
		return fmt.Errorf("post-processing: %w", err)
	}
	_ = os.Remove(outputGenerated)

	// 4. Extract support files for non-source environments
	for _, env := range opts.Envs {
		if env.Name == sourceEnv {
			continue
		}
		result := envResults[env.Name]
		if result == nil {
			continue
		}
		envGenerated := filepath.Join(result.OutputDir, "generated.tf")
		if _, err := os.Stat(envGenerated); os.IsNotExist(err) {
			continue
		}
		logStep("  Extracting support files for %q...", env.Name)
		if err := extractEnvSupportFiles(opts.OutputDir, env.Name, envGenerated, result.Registry); err != nil {
			logStep("  Warning: could not extract support files for %q: %v", env.Name, err)
		}
	}

	// 5. Classify support files (shared vs divergent)
	logStep("Classifying support files...")
	classified, err := classifySupportFiles(opts.OutputDir, sourceEnv, envNames)
	if err != nil {
		return fmt.Errorf("classifying support files: %w", err)
	}
	var sharedFiles, divergentFiles []ClassifiedFile
	for _, cf := range classified {
		if cf.Class == SupportFileShared {
			sharedFiles = append(sharedFiles, cf)
		} else {
			divergentFiles = append(divergentFiles, cf)
		}
	}
	if len(classified) > 0 {
		logStep("  %d shared, %d divergent support files", len(sharedFiles), len(divergentFiles))
	}

	// 6. Assemble module directory
	logStep("Assembling module directory...")
	if err := assembleModule(opts.OutputDir, sourceEnv, classified); err != nil {
		return fmt.Errorf("assembling module: %w", err)
	}

	moduleDir := filepath.Join(opts.OutputDir, "modules", "jamf")

	// Write required_providers in the module so Terraform knows the provider source
	pinnedVersion := opts.ProviderVersion
	resolvedVersion := terraform.ResolvedProviderVersion(sourceResult.OutputDir, terraform.ProviderSourceJamfPro)
	if err := generateModuleProviders(moduleDir, pinnedVersion, resolvedVersion); err != nil {
		return fmt.Errorf("generating module providers: %w", err)
	}

	// 6b. Split partial-env resources into labeled files
	if partialCount > 0 {
		logStep("  Separating %d environment-specific resources...", partialCount)
		if err := splitPartialEnvResources(moduleDir, matches, pro.TypeToFileMap()); err != nil {
			return fmt.Errorf("splitting partial-env resources: %w", err)
		}
	}

	// 7. Diff attributes across environments
	// Must run BEFORE divergent file rewriting so DiffResources can detect
	// file() refs and skip them (divergent script_contents, profile payloads, etc.)
	logStep("Comparing resource attributes across environments...")
	diffs, err := DiffResources(moduleDir, envResults, matches, sourceEnv)
	if err != nil {
		return fmt.Errorf("diffing resources: %w", err)
	}
	if len(diffs) > 0 {
		logStep("  %d attributes differ across environments (extracted to variables)", len(diffs))
	}

	// Rewrite divergent file references to module variables
	// Runs AFTER diffing so file() refs are still visible to DiffResources
	var fileVars []ModuleVar
	if len(divergentFiles) > 0 {
		logStep("  Rewriting divergent file references...")
		fv, err := rewriteDivergentFileRefs(moduleDir, divergentFiles)
		if err != nil {
			return fmt.Errorf("rewriting divergent file refs: %w", err)
		}
		fileVars = fv
	}

	// Pick up file(var.X) patterns created by postprocessing (e.g. token paths)
	// that aren't covered by the divergent file rewriting above.
	fileVars = append(fileVars, scanFileVarRefs(moduleDir)...)

	// 8. Generate module variables
	logStep("Generating module variables...")
	if err := generateModuleVariables(moduleDir, diffs, fileVars); err != nil {
		return fmt.Errorf("generating module variables: %w", err)
	}

	// 9. Generate per-environment root directories
	logStep("Generating environment directories...")

	// Get token refresh period from source env
	tokenRefreshPeriod := 0
	if sourceResult.ImportCreds != nil {
		tokenRefreshPeriod = sourceResult.ImportCreds.TokenRefreshBufferPeriod
	}

	for _, env := range opts.Envs {
		if err := generateEnvRoot(opts.OutputDir, env, matches, diffs, divergentFiles, fileVars, pinnedVersion, resolvedVersion, tokenRefreshPeriod); err != nil {
			return fmt.Errorf("generating env root for %s: %w", env.Name, err)
		}
	}

	// Remove import blocks if requested
	if opts.SkipImportBlocks {
		for _, env := range opts.Envs {
			envImports := filepath.Join(opts.OutputDir, "environments", env.Name, "imports.tf")
			_ = os.Remove(envImports)
		}
		logStep("  Removed import blocks (-skip-import-blocks)")
	}

	// Clean up workspace-era files from the output root (import files, provider.tf, etc.)
	cleanupOutputRoot(opts.OutputDir)

	// Write .gitignore
	if err := importgen.WriteGitignore(opts.OutputDir); err != nil {
		logStep("  Warning: could not write .gitignore: %v", err)
	}

	// 10. Clean up and format
	collapseBlankLines(moduleDir)
	terraform.FormatDir(moduleDir)
	for _, env := range opts.Envs {
		terraform.FormatDir(filepath.Join(opts.OutputDir, "environments", env.Name))
	}

	return nil
}

// cleanupOutputRoot removes files from the output root that were produced by
// postprocessing but now live in the module or env directories.
func cleanupOutputRoot(outputDir string) {
	// Remove leftover import files, provider.tf, variables.tf, tfvars from root
	patterns := []string{"*_import.tf", "*.tfvars"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(outputDir, pattern))
		for _, f := range files {
			_ = os.Remove(f)
		}
	}
	for _, name := range []string{"provider.tf", "variables.tf", "terraform.tfvars", "locals_env.tf"} {
		_ = os.Remove(filepath.Join(outputDir, name))
	}

	// Clean up remaining support_files directories (divergent files have been copied to env dirs)
	_ = os.RemoveAll(filepath.Join(outputDir, "support_files"))
}

// runPerEnv executes the Jamf Pro pipeline for a single environment into a
// temp directory, returning the intermediate results.
func runPerEnv(env EnvConfig, opts *Options) (*PerEnvResult, error) {
	tempDir, err := os.MkdirTemp("", "jamformer-"+env.Name+"-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	pipelineOpts := &pro.PipelineOptions{
		OutputDir:            tempDir,
		URL:                  env.URL,
		AuthMethod:           env.AuthMethod,
		Username:             env.Username,
		Password:             env.Password,
		ClientID:             env.ClientID,
		ClientSecret:         env.ClientSecret,
		SelectedResources:    opts.SelectedResources,
		SkipReferences:       false, // we need references resolved for diffing
		SkipPackageDownloads: opts.SkipPackageDownloads,
		ProviderVersion:      opts.ProviderVersion,
		Quiet:                opts.Quiet,
		Verbose:              opts.Verbose,
		ResourcesFlag:        opts.ResourcesFlag,
		ExcludeFlag:          opts.ExcludeFlag,
		StatusFunc:           opts.StatusFunc,
	}

	ir, err := pro.RunDiscoveryAndGenerate(pipelineOpts)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}

	return &PerEnvResult{
		EnvName:     env.Name,
		Registry:    ir.Registry,
		Resources:   ir.Resources,
		OutputDir:   tempDir,
		ImportCreds: ir.ImportCreds,
	}, nil
}

// extractEnvSupportFiles runs postprocessing for a non-source environment into
// a throwaway directory, then moves the extracted support files to the real output.
func extractEnvSupportFiles(outputDir, envName, generatedFile string, reg *registry.Registry) error {
	extractDir, err := os.MkdirTemp("", "jamformer-extract-"+envName+"-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()

	genData, err := os.ReadFile(generatedFile)
	if err != nil {
		return fmt.Errorf("reading generated.tf: %w", err)
	}

	extractGenFile := filepath.Join(extractDir, "generated.tf")
	if err := os.WriteFile(extractGenFile, genData, 0644); err != nil {
		return fmt.Errorf("writing generated.tf: %w", err)
	}

	// Run postprocess with extraction only (skip references, use env prefix)
	if err := postprocess.Process(extractDir, extractGenFile, reg, &postprocess.ProcessOptions{
		TypeToFileMap:      pro.TypeToFileMap(),
		Rules:              pro.DefaultRules(),
		SkipReferences:     true,
		SupportFilesPrefix: envName,
	}); err != nil {
		return fmt.Errorf("extracting support files: %w", err)
	}

	// Move extracted support files to the real output dir
	envSupportSrc := filepath.Join(extractDir, "support_files", envName)
	if _, err := os.Stat(envSupportSrc); os.IsNotExist(err) {
		return nil // no support files extracted for this env
	}

	envSupportDst := filepath.Join(outputDir, "support_files", envName)
	if err := os.MkdirAll(filepath.Dir(envSupportDst), 0755); err != nil {
		return fmt.Errorf("creating support dir: %w", err)
	}
	if err := os.Rename(envSupportSrc, envSupportDst); err != nil {
		return fmt.Errorf("moving support files: %w", err)
	}

	return nil
}
