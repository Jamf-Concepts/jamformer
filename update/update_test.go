// Copyright 2026, Jamf Software LLC

package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
// be wrong advice. classifyPath takes the path as an argument, so each branch
// can be put in front of the layout it claims to recognise instead of the
// layout the test binary happens to sit in.
func TestClassifyPathRecognisesEachInstallLayout(t *testing.T) {
	// Every case clears the three variables the classifier reads before setting
	// the one it is about. Without that, whichever of them the developer's own
	// shell exports decides the answer: HOMEBREW_PREFIX is set on any machine
	// with Homebrew, and it is consulted before the go install and source
	// branches.
	clearEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("HOMEBREW_PREFIX", "")
		t.Setenv("GOBIN", "")
		t.Setenv("GOPATH", "")
		// The default ~/go/bin branch reads the home directory, so it is pointed
		// somewhere with nothing in it.
		t.Setenv("HOME", t.TempDir())
	}

	t.Run("a binary beside a go.mod is a source build", func(t *testing.T) {
		clearEnv(t)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if got := classifyPath(filepath.Join(dir, "jamformer")); got != InstallSource {
			t.Errorf("got %s, want source build — a contributor running a locally built "+
				"binary would be told to download a release instead of rebuilding", got)
		}
	})

	t.Run("a Cellar path component is Homebrew on any prefix", func(t *testing.T) {
		clearEnv(t)
		if got := classifyPath("/somewhere/else/Cellar/jamformer/1.2.3/bin/jamformer"); got != InstallHomebrew {
			t.Errorf("got %s, want homebrew", got)
		}
	})

	t.Run("HOMEBREW_PREFIX is honoured for a relocated prefix", func(t *testing.T) {
		clearEnv(t)
		prefix := t.TempDir()
		t.Setenv("HOMEBREW_PREFIX", prefix)
		if got := classifyPath(filepath.Join(prefix, "bin", "jamformer")); got != InstallHomebrew {
			t.Errorf("got %s, want homebrew", got)
		}
	})

	t.Run("GOBIN is a go install", func(t *testing.T) {
		clearEnv(t)
		gobin := t.TempDir()
		t.Setenv("GOBIN", gobin)
		if got := classifyPath(filepath.Join(gobin, "jamformer")); got != InstallGoInstall {
			t.Errorf("got %s, want go install", got)
		}
	})

	t.Run("GOPATH/bin is a go install", func(t *testing.T) {
		clearEnv(t)
		gopath := t.TempDir()
		t.Setenv("GOPATH", gopath)
		if got := classifyPath(filepath.Join(gopath, "bin", "jamformer")); got != InstallGoInstall {
			t.Errorf("got %s, want go install", got)
		}
	})

	t.Run("a system bin directory is a package install", func(t *testing.T) {
		clearEnv(t)
		if got := classifyPath("/usr/local/bin/jamformer"); got != InstallPackage {
			t.Errorf("got %s, want package", got)
		}
	})

	t.Run("anything else is unknown", func(t *testing.T) {
		clearEnv(t)
		// A bare temp directory matches no branch: no Cellar component, no
		// prefix, no GOBIN or GOPATH, and no go.mod beside it.
		if got := classifyPath(filepath.Join(t.TempDir(), "jamformer")); got != InstallUnknown {
			t.Errorf("got %s, want unknown — the releases page is the only safe advice here", got)
		}
	})
}

// DetectInstallMethod is now a thin wrapper over os.Executable and
// classifyPath, so the only thing left to assert about it is that it resolves at
// all rather than falling through to InstallUnknown because os.Executable
// failed. The test binary sits in a temp directory with no go.mod, so it must
// not look like a source build.
func TestDetectInstallMethodClassifiesTheRunningBinary(t *testing.T) {
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

// --- Check against a stubbed release endpoint -------------------------------
//
// Everything below drives Check through the two test seams in update.go. Before
// they existed, Check was reachable only on its two early-return paths — the
// non-release version and the skip variable — both of which return before the
// release lookup, so the comparison that decides whether a notice is printed at
// all had never executed. Inverting it to LessThan left the suite green.

// stubReleaseEndpoint points the release lookup at a local server for the
// duration of the test and returns the request counter, so a test can assert
// how many requests were actually made rather than only what came back.
func stubReleaseEndpoint(t *testing.T, handler http.HandlerFunc) *atomic.Int64 {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	prevURL, prevClient := releasesURL, httpClient
	releasesURL, httpClient = srv.URL, srv.Client()
	t.Cleanup(func() { releasesURL, httpClient = prevURL, prevClient })
	return &calls
}

// redirectCache points os.UserCacheDir at a temp directory so a test neither
// reads a real cached answer nor writes one, and returns that directory.
//
// Which variable does it is per-platform and documented on os.UserCacheDir:
// $HOME/Library/Caches on darwin, $XDG_CACHE_HOME (else $HOME/.cache) on other
// Unix, %LocalAppData% on Windows, $home/lib/cache on Plan 9. The redirect is
// verified rather than assumed, and a platform where it does not take is
// skipped with the reason instead of silently exercising the developer's real
// cache.
func redirectCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", dir)
	case "windows":
		t.Setenv("LocalAppData", dir)
	case "plan9":
		t.Setenv("home", dir)
	default:
		t.Setenv("XDG_CACHE_HOME", dir)
	}
	got, err := os.UserCacheDir()
	if err != nil || !strings.HasPrefix(got, dir) {
		t.Skipf("os.UserCacheDir could not be redirected on %s (got %q, %v) — "+
			"the cache assertions would otherwise run against the real cache", runtime.GOOS, got, err)
	}
	// Nothing else in the process should be reaching for a cached answer, but a
	// stale file from a previous run under the same temp path would make the
	// request counter meaningless, so start from empty.
	t.Setenv("JAMFORMER_SKIP_UPDATE_CHECK", "")
	return dir
}

// releaseJSON renders the subset of the GitHub release payload Check reads.
func releaseJSON(tag string, draft, prerelease bool) string {
	b, err := json.Marshal(map[string]any{
		"tag_name":   tag,
		"html_url":   "https://example.invalid/releases/" + tag,
		"draft":      draft,
		"prerelease": prerelease,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// The comparison is strictly greater-than: a notice is for a release the user
// does not have, and telling someone already on the latest version to upgrade
// is worse than saying nothing. An equal or older tag must produce no notice —
// the older case being what an inverted comparison turns into a notice.
func TestCheckAdvisesOnlyStrictlyNewerReleases(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		tag           string
		wantAvailable bool
	}{
		{"a newer release is offered", "1.0.0", "v1.4.0", true},
		{"the same version is not", "1.2.3", "v1.2.3", false},
		{"an older release is not", "2.0.0", "v1.9.9", false},
		{"a newer patch is offered", "1.2.3", "v1.2.4", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			redirectCache(t)
			stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(releaseJSON(tc.tag, false, false)))
			})

			res, err := Check(context.Background(), tc.current)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Available != tc.wantAvailable {
				t.Errorf("Available = %v, want %v (current %s, latest %s)",
					res.Available, tc.wantAvailable, tc.current, tc.tag)
			}
			if want := strings.TrimPrefix(tc.tag, "v"); res.Latest != want {
				t.Errorf("Latest = %q, want %q", res.Latest, want)
			}
			if res.Available && res.Notice() == "" {
				t.Error("an available upgrade must render a notice")
			}
			if !res.Available && res.Notice() != "" {
				t.Errorf("no upgrade is available, but a notice was rendered: %q", res.Notice())
			}
		})
	}
}

// A leading "v" is a tag convention, not part of the version, and the two sides
// of the comparison have to agree about that. Stripping it on one side only
// makes every comparison fail to parse.
func TestCheckToleratesTagsWithAndWithoutTheVPrefix(t *testing.T) {
	for _, tag := range []string{"v1.4.0", "1.4.0"} {
		redirectCache(t)
		stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(releaseJSON(tag, false, false)))
		})
		res, err := Check(context.Background(), "v1.0.0")
		if err != nil {
			t.Fatalf("tag %q: unexpected error: %v", tag, err)
		}
		if !res.Available || res.Latest != "1.4.0" {
			t.Errorf("tag %q: got available=%v latest=%q, want true/1.4.0", tag, res.Available, res.Latest)
		}
	}
}

// A draft or prerelease is not what a user should be pointed at.
// /releases/latest already excludes both, so reaching this code means the API
// answered with one anyway — and the right response is no notice, not a notice
// naming a release nobody can download.
func TestCheckIgnoresDraftsAndPrereleases(t *testing.T) {
	tests := []struct {
		name             string
		draft, prerelase bool
	}{
		{"draft", true, false},
		{"prerelease", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			redirectCache(t)
			stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(releaseJSON("v9.9.9", tc.draft, tc.prerelase)))
			})

			res, _ := Check(context.Background(), "1.0.0")
			if res.Available {
				t.Errorf("a %s must not be advised as an upgrade", tc.name)
			}
			if res.Notice() != "" {
				t.Errorf("a %s rendered a notice: %q", tc.name, res.Notice())
			}
		})
	}
}

// A repository with no published release answers 404. That is a normal state,
// so the caller must get no notice — and specifically not the "unparseable
// release tag" error, which would blame the release for a release that does not
// exist.
func TestCheckHandlesARepositoryWithNoRelease(t *testing.T) {
	redirectCache(t)
	stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	res, err := Check(context.Background(), "1.0.0")
	if res.Available || res.Notice() != "" {
		t.Errorf("a missing release must produce no notice, got %+v", res)
	}
	if err == nil {
		t.Fatal("expected the caller to be told the lookup found nothing")
	}
	if strings.Contains(err.Error(), "unparseable release tag") {
		t.Errorf("a 404 was reported as a tag-parsing failure: %v", err)
	}
}

// The package documents a once-a-day request, and the cache is what keeps the
// unauthenticated endpoint's per-IP rate limit comfortable for anything that
// runs jamformer in a loop. Two checks inside the TTL must make one request.
func TestCheckCachesTheLookupWithinTheTTL(t *testing.T) {
	redirectCache(t)
	calls := stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("v1.4.0", false, false)))
	})

	first, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	second, err := Check(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("second check: %v", err)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("two checks inside the %s TTL made %d requests, want 1", cacheTTL, n)
	}
	// The cached answer has to be the same answer, or the cache is saving a
	// request by degrading the result.
	if first.Available != second.Available || first.Latest != second.Latest {
		t.Errorf("cached check disagrees with the fresh one: %+v vs %+v", first, second)
	}
}

// A cache older than the TTL is not a cache. Rewriting the stored timestamp
// past it is the only way to assert expiry without making a test wait a day.
func TestCheckRefetchesOnceTheCacheIsStale(t *testing.T) {
	redirectCache(t)
	calls := stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("v1.4.0", false, false)))
	})

	if _, err := Check(context.Background(), "1.0.0"); err != nil {
		t.Fatalf("first check: %v", err)
	}
	writeCache(cached{CheckedAt: time.Now().Add(-cacheTTL - time.Hour), TagName: "v1.4.0"})

	if _, err := Check(context.Background(), "1.0.0"); err != nil {
		t.Fatalf("second check: %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("a stale cache made %d requests in total, want 2", n)
	}
}

// The skip variable and a non-release version both have to return before the
// lookup, not merely discard its result: an update check that still calls out
// is an update check the user asked not to have.
func TestCheckMakesNoRequestWhenItShouldNotLook(t *testing.T) {
	t.Run("the skip variable is set", func(t *testing.T) {
		redirectCache(t)
		calls := stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(releaseJSON("v9.9.9", false, false)))
		})
		t.Setenv("JAMFORMER_SKIP_UPDATE_CHECK", "1")

		if _, err := Check(context.Background(), "1.0.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n := calls.Load(); n != 0 {
			t.Errorf("made %d requests despite JAMFORMER_SKIP_UPDATE_CHECK, want 0", n)
		}
	})

	t.Run("the current version is not a release", func(t *testing.T) {
		redirectCache(t)
		calls := stubReleaseEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(releaseJSON("v9.9.9", false, false)))
		})

		if _, err := Check(context.Background(), "dev"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n := calls.Load(); n != 0 {
			t.Errorf(`made %d requests for version "dev", want 0`, n)
		}
	})
}
