// Copyright 2026, Jamf Software LLC

package terraform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Ctx is the context used for all terraform operations. Set this to a
// signal-aware context so that Ctrl+C cancels in-flight terraform commands
// and terminates the provider subprocess. Defaults to context.Background().
var Ctx = context.Background()

// Verbose controls whether terraform command output is shown.
var Verbose bool

// Quiet suppresses progress messages (used when the spinner is active).
var Quiet bool

// AllowDevOverrides, when true, preserves the user's CLI config (including
// provider dev_overrides). By default jamformer sets TF_CLI_CONFIG_FILE to an
// empty file so dev overrides cannot interfere with registry providers.
var AllowDevOverrides bool

// Parallelism controls the -parallelism value passed to terraform plan.
// Defaults to 1 to avoid overwhelming the Jamf API during config generation.
var Parallelism = 1

// ProgressFunc is called during GenerateConfig with (current, total) as each
// resource is read by terraform. Set to nil to disable. Only set this in
// interactive mode when a spinner is active.
var ProgressFunc func(current, total int)

// progressWriter counts "Read complete" occurrences in terraform stderr output
// and calls callback(current, total) on each hit. It optionally forwards all
// bytes to sink (used when Verbose = true to preserve terminal output).
type progressWriter struct {
	sink     io.Writer
	buf      []byte
	current  int
	total    int
	callback func(int, int)
}

// refreshMarker is the per-resource progress line terraform emits during
// terraform plan -generate-config-out. Each imported resource prints exactly
// one "Refreshing state..." line as it is read from the provider.
var refreshMarker = []byte("Refreshing state...")

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	data := append(pw.buf, p...)
	for {
		idx := bytes.Index(data, refreshMarker)
		if idx < 0 {
			break
		}
		pw.current++
		if pw.callback != nil {
			pw.callback(pw.current, pw.total)
		}
		data = data[idx+len(refreshMarker):]
	}
	// Retain tail bytes to handle markers split across Write calls
	tail := len(refreshMarker) - 1
	if len(data) > tail {
		pw.buf = append(pw.buf[:0], data[len(data)-tail:]...)
	} else {
		pw.buf = append(pw.buf[:0], data...)
	}
	if pw.sink != nil {
		return pw.sink.Write(p)
	}
	return len(p), nil
}

// TerraformVersionConstraint is the version constraint for the Terraform binary
// that jamformer downloads. It pins to the latest 1.14.x release.
const TerraformVersionConstraint = "~> 1.14.0"

// EnsureTerraform downloads the latest Terraform 1.14.x binary to a temporary
// directory and returns the path. The binary is placed in
// os.TempDir()/jamformer-terraform/ so it persists across runs within the same
// OS session but is cleaned up on reboot.
func EnsureTerraform() (string, error) {
	cache := filepath.Join(os.TempDir(), "jamformer-terraform")
	binaryName := "terraform"
	if runtime.GOOS == "windows" {
		binaryName = "terraform.exe"
	}
	cachedPath := filepath.Join(cache, binaryName)

	// Reuse if already downloaded this session
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	if !Quiet {
		fmt.Println("Downloading Terraform...")
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}

	constraints, err := goversion.NewConstraint(TerraformVersionConstraint)
	if err != nil {
		return "", fmt.Errorf("parsing terraform version constraint: %w", err)
	}

	installer := &releases.LatestVersion{
		Product:     product.Terraform,
		Constraints: constraints,
		InstallDir:  cache,
	}

	ctx, cancel := context.WithTimeout(Ctx, 5*time.Minute)
	defer cancel()

	path, err := installer.Install(ctx)
	if err != nil {
		return "", fmt.Errorf("installing terraform: %w", err)
	}

	if !Quiet {
		fmt.Printf("Terraform installed to %s\n", path)
	}
	return path, nil
}

// newTF creates a tfexec.Terraform instance for the given work directory.
func newTF(workDir, terraformPath string) (*tfexec.Terraform, error) {
	tf, err := tfexec.NewTerraform(workDir, terraformPath)
	if err != nil {
		return nil, fmt.Errorf("initializing terraform: %w", err)
	}

	if Verbose {
		tf.SetStdout(os.Stdout)
		tf.SetStderr(os.Stderr)
	}

	return tf, nil
}

// terraformPath is set by SetTerraformPath and used by Init/GenerateConfig.
var terraformPath string

// SetTerraformPath sets the terraform binary path for all subsequent operations.
func SetTerraformPath(path string) {
	terraformPath = path
}

// Init runs terraform init in the given directory.
func Init(workDir string) error {
	tf, err := newTF(workDir, terraformPath)
	if err != nil {
		return err
	}

	if !AllowDevOverrides {
		env := mergeProviderEnv(nil)
		if err := tf.SetEnv(env); err != nil {
			return fmt.Errorf("setting terraform env: %w", err)
		}
	}

	if err := tf.Init(Ctx, tfexec.Upgrade(false)); err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	return nil
}

// Apply runs terraform apply in the given directory. This is used for
// read-only operations like populating data sources for resource discovery.
func Apply(workDir string, providerEnv map[string]string) error {
	tf, err := newTF(workDir, terraformPath)
	if err != nil {
		return err
	}

	env := mergeProviderEnv(providerEnv)
	if err := tf.SetEnv(env); err != nil {
		return fmt.Errorf("setting terraform env: %w", err)
	}

	if err := tf.Apply(Ctx); err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	return nil
}

// resourceAddrPattern matches "with <resource_type>.<label>," in terraform error output.
var resourceAddrPattern = regexp.MustCompile(`with\s+(\S+\.\S+),`)

// importTargetPattern matches "import target <resource_type>.<label> does not exist"
// in terraform error output when a provider doesn't support import for a resource.
var importTargetPattern = regexp.MustCompile(`import target\s+(\S+\.\S+)\s+does not exist`)

// GenerateConfig runs terraform plan with -generate-config-out to produce HCL
// from the import blocks. providerEnv contains provider-specific environment
// variables (e.g. JAMFPRO_INSTANCE_FQDN, JAMFPROTECT_URL) that are merged
// with the current process environment before running terraform.
//
// If the provider errors on specific resources (preventing Terraform from
// writing the output file), those import blocks are removed and the plan
// is re-run. This handles provider bugs where certain resources can't be read.
func GenerateConfig(workDir, outputFile string, providerEnv map[string]string) error {
	tf, err := newTF(workDir, terraformPath)
	if err != nil {
		return err
	}

	env := mergeProviderEnv(providerEnv)
	if err := tf.SetEnv(env); err != nil {
		return fmt.Errorf("setting terraform env: %w", err)
	}

	// Count total import blocks to show progress
	importCount, _ := countImportBlocks(workDir)
	if importCount > 0 && !Quiet {
		fmt.Printf("  Reading %d resources (this may take a while)...\n", importCount)
	}
	if ProgressFunc != nil {
		ProgressFunc(0, importCount)
	}

	const maxAttempts = 50
	for attempt := range maxAttempts {
		// Wire progress tracking into terraform stdout for this attempt.
		// "Read complete" progress lines go to stdout during terraform plan.
		// importCount may decrease across retries as failing blocks are removed.
		if ProgressFunc != nil {
			currentTotal, _ := countImportBlocks(workDir)
			pw := &progressWriter{
				total:    currentTotal,
				callback: ProgressFunc,
			}
			if Verbose {
				pw.sink = os.Stdout
			}
			tf.SetStdout(pw)
		}

		_, err := tf.Plan(Ctx,
			tfexec.GenerateConfigOut(outputFile),
			tfexec.Parallelism(Parallelism),
		)
		if err == nil {
			return nil
		}

		// Output file written despite error — post-processor can fix it
		if _, statErr := os.Stat(outputFile); statErr == nil {
			if !Quiet {
				fmt.Println("  Some resources generated with warnings — these will be resolved during post-processing.")
			}
			return nil
		}

		// No output file — try to identify and skip the failing resource
		errMsg := err.Error()
		var matches [][]string
		matches = append(matches, resourceAddrPattern.FindAllStringSubmatch(errMsg, -1)...)
		matches = append(matches, importTargetPattern.FindAllStringSubmatch(errMsg, -1)...)
		if len(matches) == 0 {
			return fmt.Errorf("terraform plan failed: %w", err)
		}

		removed := false
		for _, match := range matches {
			addr := match[1]
			if removeErr := removeImportBlock(workDir, addr); removeErr == nil {
				if !Quiet {
					fmt.Printf("  Skipping %s (provider error)\n", addr)
				}
				removed = true
			}
		}
		if !removed {
			return fmt.Errorf("terraform plan failed: %w", err)
		}

		if !Quiet {
			fmt.Printf("  Retrying (attempt %d)...\n", attempt+2)
		}

		// Clean up partial output before re-running
		_ = os.Remove(outputFile)
	}

	return fmt.Errorf("terraform plan failed after removing failing resources")
}

// Query runs terraform query with -generate-config-out to discover and generate
// HCL from list blocks and import blocks. providerEnv contains provider-specific
// environment variables.
//
// Note: we use os/exec directly instead of tfexec.QueryJSON because QueryJSON
// uses a bufio.Scanner with the default 64KB max token size. Providers that
// return large resources (e.g. 150KB+ JSON lines) cause the scanner to fail
// silently, deadlocking the pipe. Since we only need the generated HCL file
// (not the JSON stream), running the command directly avoids this issue.
func Query(workDir, outputFile string, providerEnv map[string]string) error {
	return queryInternal(workDir, outputFile, "", providerEnv)
}

// QueryWithEvents runs terraform query with -json -generate-config-out and
// streams the JSON event log (stdout) to eventsFile. This is needed when
// resource labels must be derived from list_resource_found events because
// the resource block lacks a usable name attribute (e.g. when the provider
// marks name as read-only/computed and -generate-config-out omits it).
func QueryWithEvents(workDir, outputFile, eventsFile string, providerEnv map[string]string) error {
	return queryInternal(workDir, outputFile, eventsFile, providerEnv)
}

func queryInternal(workDir, outputFile, eventsFile string, providerEnv map[string]string) error {
	args := []string{"query", "-no-color", "-generate-config-out=" + outputFile}
	if eventsFile != "" {
		args = append(args, "-json")
	}

	cmd := exec.CommandContext(Ctx, terraformPath, args...)
	cmd.Dir = workDir

	env := mergeProviderEnv(providerEnv)
	cmdEnv := make([]string, 0, len(env))
	for k, v := range env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	cmd.Env = cmdEnv

	var stderr strings.Builder
	var eventsOut *os.File
	if eventsFile != "" {
		f, err := os.Create(eventsFile)
		if err != nil {
			return fmt.Errorf("creating events file: %w", err)
		}
		defer func() { _ = f.Close() }()
		eventsOut = f
	}

	if Verbose {
		if eventsOut != nil {
			cmd.Stdout = io.MultiWriter(eventsOut, os.Stdout)
		} else {
			cmd.Stdout = os.Stdout
		}
		cmd.Stderr = os.Stderr
	} else {
		if eventsOut != nil {
			cmd.Stdout = eventsOut
		}
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("terraform query failed: %w\n%s", err, stderr.String())
		}
		return fmt.Errorf("terraform query failed: %w", err)
	}
	return nil
}

// ProvidersSchema runs terraform providers schema -json and returns the parsed result.
func ProvidersSchema(workDir string) (*tfjson.ProviderSchemas, error) {
	tf, err := newTF(workDir, terraformPath)
	if err != nil {
		return nil, err
	}

	schemas, err := tf.ProvidersSchema(Ctx)
	if err != nil {
		return nil, fmt.Errorf("terraform providers schema: %w", err)
	}
	return schemas, nil
}

// Validate runs terraform validate in the given directory and returns
// structured diagnostics. This is used after post-processing to detect
// and auto-fix validation errors (e.g. conditionally invalid attributes).
func Validate(workDir string) (*tfjson.ValidateOutput, error) {
	tf, err := newTF(workDir, terraformPath)
	if err != nil {
		return nil, err
	}

	if !AllowDevOverrides {
		env := mergeProviderEnv(nil)
		if err := tf.SetEnv(env); err != nil {
			return nil, fmt.Errorf("setting terraform env: %w", err)
		}
	}

	return tf.Validate(Ctx)
}

// FormatDir runs terraform fmt on the given directory. Errors are silently
// ignored since formatting is best-effort.
func FormatDir(dir string) {
	if terraformPath == "" {
		return
	}
	tf, err := newTF(dir, terraformPath)
	if err != nil {
		return
	}
	_ = tf.FormatWrite(Ctx)
}

// mergeProviderEnv merges provider-specific environment variables with the
// current process environment, excluding variables managed internally by tfexec.
// When AllowDevOverrides is false, it also sets TF_CLI_CONFIG_FILE to an empty
// config so that user dev_overrides in ~/.terraform.d/terraform.tfrc don't
// interfere with registry provider versions.
func mergeProviderEnv(providerVars map[string]string) map[string]string {
	tfexecManaged := map[string]bool{
		"TF_IN_AUTOMATION": true,
		"TF_INPUT":         true,
		"TF_LOG":           true,
		"TF_LOG_PATH":      true,
		"TF_LOG_CORE":      true,
		"TF_LOG_PROVIDER":  true,
	}

	env := make(map[string]string, len(providerVars))
	maps.Copy(env, providerVars)

	// Override the CLI config to suppress dev_overrides unless explicitly allowed
	if !AllowDevOverrides {
		env["TF_CLI_CONFIG_FILE"] = emptyCliConfigPath()
	}

	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			if tfexecManaged[parts[0]] {
				continue
			}
			if _, exists := env[parts[0]]; !exists {
				env[parts[0]] = parts[1]
			}
		}
	}
	return env
}

// emptyCliConfigPath returns the path to an empty Terraform CLI config file,
// creating it on first call. This is used to suppress dev_overrides.
// The file is placed in the jamformer-terraform cache directory so it is
// cleaned up on reboot along with the terraform binary.
var emptyCliConfigPath = func() func() string {
	var path string
	return func() string {
		if path != "" {
			return path
		}
		cacheDir := filepath.Join(os.TempDir(), "jamformer-terraform")
		_ = os.MkdirAll(cacheDir, 0755)
		cfgPath := filepath.Join(cacheDir, "empty.tfrc")
		if err := os.WriteFile(cfgPath, nil, 0644); err != nil {
			return "" // fall back to no override
		}
		path = cfgPath
		return path
	}
}()

// countImportBlocks counts the total number of import blocks across all *_import.tf files.
func countImportBlocks(workDir string) (int, error) {
	files, err := filepath.Glob(filepath.Join(workDir, "*_import.tf"))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		count += strings.Count(string(content), "import {")
	}
	return count, nil
}

// removeImportBlock removes the import block for a given resource address
// from the import files in workDir.
func removeImportBlock(workDir, resourceAddr string) error {
	files, err := filepath.Glob(filepath.Join(workDir, "*_import.tf"))
	if err != nil {
		return err
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Base(file), err)
		}

		if !strings.Contains(string(content), resourceAddr) {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var result []string
		skip := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "import {" {
				skip = false
			}
			if strings.Contains(line, resourceAddr) {
				skip = true
				// Remove lines back to (and including) the "import {" we already added
				for len(result) > 0 {
					removed := result[len(result)-1]
					result = result[:len(result)-1]
					if strings.TrimSpace(removed) == "import {" {
						break
					}
				}
				continue
			}
			if skip {
				if trimmed == "}" {
					skip = false
				}
				continue
			}
			result = append(result, line)
		}

		return os.WriteFile(file, []byte(strings.Join(result, "\n")), 0644)
	}

	return nil
}
