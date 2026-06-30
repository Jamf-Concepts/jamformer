// Copyright 2026, Jamf Software LLC

// Package download fetches package files for the Jamf Platform pipeline from the
// Jamf Cloud Distribution Service (JCDS) via the federated Jamf Platform gateway.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
)

// Quiet suppresses progress messages.
var Quiet bool

// Packages downloads package files that are resident in the Jamf Cloud
// Distribution Service to support_files/packages/ and returns a map of
// fileName → relative path (from the output directory) for post-processing.
//
// Only files actually uploaded to JCDS are fetchable; catalog packages whose
// bytes live on another distribution point return 404 and are skipped. The
// fileName is the join key: post-processing matches each jamfplatform_pro_package
// by its file_name attribute and, when downloaded, sets package_file_source to a
// file() reference (upload mode).
//
// progressFn is called after each download attempt with (current, total).
func Packages(ctx context.Context, pc *sdkpro.Client, outputDir string, progressFn func(int, int)) (map[string]string, error) {
	pkgsDir := filepath.Join(outputDir, "support_files", "packages")
	if err := os.MkdirAll(pkgsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating packages directory: %w", err)
	}

	// Resolve which package filenames have bytes resident in JCDS. The JCDS
	// listing endpoint is the fast path (one call); if it is unavailable we fall
	// back to attempting every package filename and skipping 404s.
	var jcdsSet map[string]bool
	//nolint:staticcheck // deprecated, but the only JCDS listing API the SDK exposes; still functional (wire-probed 2026-06-26)
	if files, err := pc.ListJCDSFilesV1(ctx); err == nil {
		jcdsSet = make(map[string]bool, len(files))
		for _, f := range files {
			jcdsSet[f.FileName] = true
		}
	} else if !Quiet {
		fmt.Printf("  Could not list JCDS files (%v); will probe each package filename\n", err)
	}

	pkgs, err := pc.ListPackagesV1(ctx, nil, "")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	// Build a deduplicated work queue of fetchable filenames.
	seen := make(map[string]bool)
	var work []string
	for _, p := range pkgs {
		safe, ok := safePackageFileName(p.FileName)
		if !ok {
			continue
		}
		if jcdsSet != nil && !jcdsSet[safe] {
			continue
		}
		if seen[safe] {
			continue
		}
		seen[safe] = true
		work = append(work, safe)
	}

	total := len(work)
	var completed atomic.Int32
	var mu sync.Mutex
	files := make(map[string]string)

	const workers = 3
	var wg sync.WaitGroup
	ch := make(chan string, total)
	for _, w := range work {
		ch <- w
	}
	close(ch)

	for range workers {
		wg.Go(func() {
			for fileName := range ch {
				n := int(completed.Add(1))
				relPath := downloadPackage(ctx, pc, fileName, pkgsDir, n, total)
				if relPath != "" {
					mu.Lock()
					files[fileName] = relPath
					mu.Unlock()
				}
				if progressFn != nil {
					progressFn(n, total)
				}
			}
		})
	}
	wg.Wait()
	return files, nil
}

// downloadPackage downloads a single JCDS file and returns its relative path, or
// "" when the file is not fetchable (not resident in JCDS / download error).
func downloadPackage(ctx context.Context, pc *sdkpro.Client, fileName, pkgsDir string, n, total int) string {
	destPath := filepath.Join(pkgsDir, fileName)
	relPath := filepath.Join("support_files", "packages", fileName)

	if _, err := os.Stat(destPath); err == nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s (already downloaded)\n", n, total, fileName)
		}
		return relPath
	}

	//nolint:staticcheck // deprecated, but the only JCDS download-URL API the SDK exposes; still functional (wire-probed 2026-06-26)
	dl, err := pc.GetJCDSFileDownloadURLV1(ctx, fileName)
	if err != nil {
		var apiErr *jamfplatform.APIResponseError
		if errors.As(err, &apiErr) && apiErr.HasStatus(http.StatusNotFound) {
			if !Quiet {
				fmt.Printf("  [%d/%d] %s skipped (not resident in JCDS)\n", n, total, fileName)
			}
			return ""
		}
		if !Quiet {
			fmt.Printf("  [%d/%d] %s skipped (download URL error: %v)\n", n, total, fileName, err)
		}
		return ""
	}

	if !Quiet {
		fmt.Printf("  [%d/%d] Downloading %s...\n", n, total, fileName)
	}
	if err := downloadFile(ctx, dl.URI, destPath); err != nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s failed (%v)\n", n, total, fileName, err)
		}
		_ = os.Remove(destPath)
		return ""
	}

	if !Quiet {
		if info, statErr := os.Stat(destPath); statErr == nil {
			fmt.Printf("  [%d/%d] %s done (%s)\n", n, total, fileName, formatSize(info.Size()))
		}
	}
	return relPath
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

// safePackageFileName reduces an API-supplied filename to a single path
// component and rejects values that could escape the destination directory.
func safePackageFileName(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	base := filepath.Base(name)
	if base != name || base == "." || base == ".." {
		return "", false
	}
	return base, true
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
