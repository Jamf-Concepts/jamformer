// Copyright 2026, Jamf Software LLC

// Package update checks whether a newer jamformer release is available and
// works out how to tell the user to upgrade.
//
// Three things keep this from getting in the way. The check runs in the
// background from a short-lived goroutine, so a slow or unreachable network
// never delays a run; its result is cached on disk so a request goes out at
// most once a day; and the advice it prints names the upgrade command for the
// way this binary was actually installed, because "a new version is available"
// with no next step is a notification, not help.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
)

const (
	// repo is the GitHub repository releases are published to.
	repo = "Jamf-Concepts/jamformer"
	// releasesAPI is the unauthenticated latest-release endpoint. It is rate
	// limited per IP, which the on-disk cache keeps us well inside.
	releasesAPI = "https://api.github.com/repos/" + repo + "/releases/latest"
	// ReleasesPage is where a user without a package manager goes.
	ReleasesPage = "https://github.com/" + repo + "/releases/latest"

	// cacheTTL is how long a check result is reused. A day is short enough to
	// surface a release promptly and long enough that repeated runs — a CI job
	// looping over environments, say — make one request between them.
	cacheTTL = 24 * time.Hour

	// requestTimeout bounds the HTTP request. The check is advisory, so it is
	// better to give up than to hold anything up.
	requestTimeout = 3 * time.Second
)

// InstallMethod is how this binary got onto the machine, which decides the
// upgrade instruction.
type InstallMethod int

const (
	// InstallUnknown covers anything not positively identified. The releases
	// page is always correct advice, so this is a safe default rather than a
	// failure.
	InstallUnknown InstallMethod = iota
	// InstallHomebrew is a binary under a Homebrew prefix or Cellar.
	InstallHomebrew
	// InstallGoInstall is a binary in GOBIN / GOPATH/bin.
	InstallGoInstall
	// InstallSource is a binary sitting in a source checkout.
	InstallSource
	// InstallPackage is a system path a macOS .pkg or a manual copy installs
	// to.
	InstallPackage
)

// Result carries the outcome of a check.
type Result struct {
	// Available is true only when a strictly newer release was found.
	Available bool
	// Current and Latest are the two versions compared, without a leading "v".
	Current string
	Latest  string
	// URL is the release's page.
	URL string
	// Method is how this binary appears to have been installed.
	Method InstallMethod
}

// UpgradeHint returns the command or action to upgrade, phrased for the
// detected install method.
func (r Result) UpgradeHint() string {
	switch r.Method {
	case InstallHomebrew:
		return "brew update && brew upgrade jamformer"
	case InstallGoInstall:
		return "go install github.com/" + repo + "@latest"
	case InstallSource:
		return "git pull && go build -o jamformer ."
	case InstallPackage:
		if runtime.GOOS == "darwin" {
			return "download the latest .pkg from " + r.URL
		}
		return "download the latest release from " + r.URL
	default:
		return "download the latest release from " + r.URL
	}
}

// Notice renders the one-line advisory, or "" when there is nothing to say.
func (r Result) Notice() string {
	if !r.Available {
		return ""
	}
	return fmt.Sprintf("A new version of jamformer is available: %s → %s\n  Upgrade: %s",
		r.Current, r.Latest, r.UpgradeHint())
}

// release is the subset of the GitHub release payload we read.
type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Prerel  bool   `json:"prerelease"`
}

// cached is what we persist between runs.
type cached struct {
	CheckedAt time.Time `json:"checked_at"`
	TagName   string    `json:"tag_name"`
	HTMLURL   string    `json:"html_url"`
}

// Check looks for a newer release than current. It returns an error only for
// the caller's information: every caller treats a failed check as "no notice"
// rather than a problem, because an update check is not part of the job the
// user asked for.
//
// The check is skipped, with no error, when the current version is not a
// release version (a `go build` from source reports "dev", and there is
// nothing meaningful to compare), or when JAMFORMER_SKIP_UPDATE_CHECK is set.
func Check(ctx context.Context, current string) (Result, error) {
	res := Result{Current: strings.TrimPrefix(current, "v"), Method: DetectInstallMethod()}

	if os.Getenv("JAMFORMER_SKIP_UPDATE_CHECK") != "" {
		return res, nil
	}
	currentVer, err := goversion.NewVersion(res.Current)
	if err != nil {
		// "dev", "none", a git hash: not a release, so not comparable.
		return res, nil
	}

	tag, url, err := latestRelease(ctx)
	if err != nil {
		return res, err
	}
	res.Latest = strings.TrimPrefix(tag, "v")
	res.URL = url
	if res.URL == "" {
		res.URL = ReleasesPage
	}

	latestVer, err := goversion.NewVersion(res.Latest)
	if err != nil {
		return res, fmt.Errorf("unparseable release tag %q: %w", tag, err)
	}
	res.Available = latestVer.GreaterThan(currentVer)
	return res, nil
}

// latestRelease returns the latest release tag and URL, from cache when it is
// fresh enough and from the API otherwise.
func latestRelease(ctx context.Context) (tag, url string, err error) {
	if c, ok := readCache(); ok && time.Since(c.CheckedAt) < cacheTTL {
		return c.TagName, c.HTMLURL, nil
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "jamformer-update-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// A repository with no published release answers 404. That is a normal
	// state, not a fault, so it is cached like any other answer to stop every
	// run retrying.
	if resp.StatusCode == http.StatusNotFound {
		writeCache(cached{CheckedAt: time.Now()})
		return "", "", errors.New("no published release")
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("release check returned %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	if rel.Draft || rel.Prerel || rel.TagName == "" {
		// /releases/latest already excludes drafts and prereleases, but a
		// stable release is what we advise upgrading to either way.
		writeCache(cached{CheckedAt: time.Now()})
		return "", "", errors.New("no stable release")
	}

	writeCache(cached{CheckedAt: time.Now(), TagName: rel.TagName, HTMLURL: rel.HTMLURL})
	return rel.TagName, rel.HTMLURL, nil
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jamformer", "update-check.json"), nil
}

func readCache() (cached, bool) {
	path, err := cachePath()
	if err != nil {
		return cached{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cached{}, false
	}
	var c cached
	if err := json.Unmarshal(b, &c); err != nil {
		return cached{}, false
	}
	// An empty tag is a cached "nothing published"; still a valid answer.
	return c, !c.CheckedAt.IsZero()
}

// writeCache is best-effort: a read-only or missing cache directory makes the
// check uncached, not broken.
func writeCache(c cached) {
	path, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0644)
}

// DetectInstallMethod works out how this binary was installed, from where it
// sits on disk. Every branch is a guess about a filesystem layout, so the
// unknown case has to stay useful: it points at the releases page, which is
// right however the binary arrived.
func DetectInstallMethod() InstallMethod {
	exe, err := os.Executable()
	if err != nil {
		return InstallUnknown
	}
	// Homebrew installs a symlink in <prefix>/bin pointing into the Cellar, so
	// the real path is what identifies it.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)

	// Homebrew: a Cellar path component is conclusive on any prefix, including
	// a relocated or linuxbrew one.
	if strings.Contains(exe, string(filepath.Separator)+"Cellar"+string(filepath.Separator)) ||
		strings.HasPrefix(exe, "/opt/homebrew/") ||
		strings.HasPrefix(exe, "/home/linuxbrew/") {
		return InstallHomebrew
	}
	if prefix := os.Getenv("HOMEBREW_PREFIX"); prefix != "" && strings.HasPrefix(exe, prefix+string(filepath.Separator)) {
		return InstallHomebrew
	}

	// go install: GOBIN, or GOPATH/bin, or the default ~/go/bin.
	if gobin := os.Getenv("GOBIN"); gobin != "" && dir == filepath.Clean(gobin) {
		return InstallGoInstall
	}
	for _, gopath := range filepath.SplitList(os.Getenv("GOPATH")) {
		if gopath != "" && dir == filepath.Join(gopath, "bin") {
			return InstallGoInstall
		}
	}
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Join(home, "go", "bin") {
		return InstallGoInstall
	}

	// A source build leaves the binary next to the code it was built from.
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return InstallSource
	}

	// A macOS .pkg, or a manual copy, lands in a system bin directory.
	switch dir {
	case "/usr/local/bin", "/usr/bin", "/opt/jamf/bin", "/usr/local/jamf/bin":
		return InstallPackage
	}

	return InstallUnknown
}

// String names the method, for -version output and tests.
func (m InstallMethod) String() string {
	switch m {
	case InstallHomebrew:
		return "homebrew"
	case InstallGoInstall:
		return "go install"
	case InstallSource:
		return "source build"
	case InstallPackage:
		return "package"
	default:
		return "unknown"
	}
}
