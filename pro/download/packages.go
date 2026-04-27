// Copyright 2026, Jamf Software LLC

package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

// Quiet suppresses progress messages.
var Quiet bool

// PackageFile holds the mapping from a Jamf package ID to its local file path.
type PackageFile struct {
	JamfID   string
	FileName string
	FilePath string // Relative path from the output directory
}

// packageWork represents a single package download task.
type packageWork struct {
	id       string
	fileName string
}

// Packages downloads all package files from the Cloud Distribution Point to support_files/packages/
// and returns the mappings for post-processing. Downloads run in parallel.
// progressFn is called after each download completes with (current, total) counts.
func Packages(client *jamfpro.Client, outputDir string, progressFn func(int, int)) ([]PackageFile, error) {
	pkgsDir := filepath.Join(outputDir, "support_files", "packages")
	if err := os.MkdirAll(pkgsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating packages directory: %w", err)
	}

	resp, err := client.GetPackages("", "")
	if err != nil {
		return nil, fmt.Errorf("listing packages: %w", err)
	}

	// Build work queue. API-supplied filenames are sanitised to prevent writes
	// escaping pkgsDir (defence against a malicious or corrupt FileName like
	// "../../etc/foo.pkg").
	var work []packageWork
	for _, p := range resp.Results {
		safe, ok := safePackageFileName(p.FileName)
		if !ok {
			if !Quiet && p.FileName != "" {
				fmt.Printf("  Skipping package ID %s: unsafe filename %q\n", p.ID, p.FileName)
			}
			continue
		}
		work = append(work, packageWork{id: p.ID, fileName: safe})
	}

	total := len(work)
	var completed atomic.Int32

	var mu sync.Mutex
	var files []PackageFile

	// Worker pool — 3 concurrent downloads (packages are large)
	const workers = 3
	var wg sync.WaitGroup
	ch := make(chan packageWork, len(work))

	for _, w := range work {
		ch <- w
	}
	close(ch)

	for range workers {
		wg.Go(func() {
			for w := range ch {
				n := int(completed.Add(1))
				result := downloadPackage(client, w, pkgsDir, n, total)
				if result != nil {
					mu.Lock()
					files = append(files, *result)
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

// downloadPackage handles a single package download and returns the result.
func downloadPackage(client *jamfpro.Client, w packageWork, pkgsDir string, n, total int) *PackageFile {
	destPath := filepath.Join(pkgsDir, w.fileName)
	relPath := filepath.Join("support_files", "packages", w.fileName)

	// Skip if already downloaded
	if _, err := os.Stat(destPath); err == nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s (already downloaded)\n", n, total, w.fileName)
		}
		return &PackageFile{JamfID: w.id, FileName: w.fileName, FilePath: relPath}
	}

	if !Quiet {
		fmt.Printf("  [%d/%d] Downloading %s...\n", n, total, w.fileName)
	}

	// Get presigned download URI from the Cloud Distribution Point
	jcdsFile, err := client.GetJCDS2PackageURIByName(w.fileName)
	if err != nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s skipped (download error: %v)\n", n, total, w.fileName, err)
		}
		return nil
	}

	if err := downloadFile(jcdsFile.URI, destPath); err != nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s failed (%v)\n", n, total, w.fileName, err)
		}
		_ = os.Remove(destPath)
		return nil
	}

	if !Quiet {
		if info, err := os.Stat(destPath); err == nil {
			fmt.Printf("  [%d/%d] %s done (%s)\n", n, total, w.fileName, formatSize(info.Size()))
		}
	}

	return &PackageFile{JamfID: w.id, FileName: w.fileName, FilePath: relPath}
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
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

// safePackageFileName reduces an API-supplied package filename to a single path
// component and rejects values that could escape the destination directory.
// Returns (name, true) when the name is safe to use, or ("", false) otherwise.
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
