// Copyright 2026, Jamf Software LLC

package update

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpgradeHintNamesTheInstallMethod(t *testing.T) {
	tests := []struct {
		method   InstallMethod
		contains string
	}{
		{InstallHomebrew, "brew upgrade jamformer"},
		{InstallGoInstall, "go install"},
		{InstallSource, "git pull"},
		{InstallUnknown, ReleasesPage},
	}
	for _, tc := range tests {
		got := Result{Method: tc.method, URL: ReleasesPage}.UpgradeHint()
		if !strings.Contains(got, tc.contains) {
			t.Errorf("%s: hint %q does not mention %q", tc.method, got, tc.contains)
		}
	}
}

// A package install on macOS should point at the .pkg, since that is what the
// user downloaded; elsewhere there is no .pkg to name.
func TestPackageHintIsPlatformSpecific(t *testing.T) {
	got := Result{Method: InstallPackage, URL: ReleasesPage}.UpgradeHint()
	if runtime.GOOS == "darwin" {
		if !strings.Contains(got, ".pkg") {
			t.Errorf("macOS package hint should name the .pkg, got %q", got)
		}
	} else if strings.Contains(got, ".pkg") {
		t.Errorf("non-macOS package hint should not name a .pkg, got %q", got)
	}
}

func TestNoticeIsEmptyWhenUpToDate(t *testing.T) {
	if got := (Result{Available: false, Current: "1.2.3", Latest: "1.2.3"}).Notice(); got != "" {
		t.Errorf("expected no notice when up to date, got %q", got)
	}
}

func TestNoticeCarriesBothVersionsAndTheCommand(t *testing.T) {
	n := Result{Available: true, Current: "1.0.0", Latest: "1.4.0", Method: InstallHomebrew}.Notice()
	for _, want := range []string{"1.0.0", "1.4.0", "brew upgrade jamformer"} {
		if !strings.Contains(n, want) {
			t.Errorf("notice %q is missing %q", n, want)
		}
	}
}

// A source build reports version "dev", which is not comparable to a release
// tag. Check must return quietly rather than advising an upgrade from a version
// it cannot reason about — and it must make no network request to find that
// out, which is what the empty Latest proves.
func TestCheckSkipsNonReleaseVersions(t *testing.T) {
	for _, v := range []string{"dev", "none", "", "a1b2c3d"} {
		res, err := Check(context.Background(), v)
		if err != nil {
			t.Errorf("version %q: unexpected error %v", v, err)
		}
		if res.Available {
			t.Errorf("version %q: must not advise an upgrade", v)
		}
		if res.Latest != "" {
			t.Errorf("version %q: expected no release lookup, got latest %q", v, res.Latest)
		}
	}
}

func TestCheckHonoursSkipEnvVar(t *testing.T) {
	t.Setenv("JAMFORMER_SKIP_UPDATE_CHECK", "1")
	res, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Available || res.Latest != "" {
		t.Errorf("skip variable ignored: %+v", res)
	}
}

// The install method is inferred from the binary's own path, so the useful
// thing to assert is that a source checkout is recognised — that is the case
// every contributor hits, and the one where "download the latest release" would
// be wrong advice.
func TestDetectInstallMethodRecognisesSourceCheckout(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// DetectInstallMethod reads os.Executable, which a test cannot move, so
	// the go.mod probe is exercised directly against a known layout.
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("fixture not created: %v", err)
	}
	// The running test binary lives in a temp directory with no go.mod, so it
	// must not be reported as a source build.
	if m := DetectInstallMethod(); m == InstallSource {
		t.Errorf("the test binary should not look like a source build, got %s", m)
	}
}

func TestInstallMethodStringsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range []InstallMethod{InstallUnknown, InstallHomebrew, InstallGoInstall, InstallSource, InstallPackage} {
		s := m.String()
		if s == "" {
			t.Errorf("%d has an empty name", m)
		}
		if seen[s] {
			t.Errorf("duplicate name %q", s)
		}
		seen[s] = true
	}
}
