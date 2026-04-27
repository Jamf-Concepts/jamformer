// Copyright 2026, Jamf Software LLC

package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// EnrollmentCustomizationImageFile holds the mapping from an enrollment customization
// Jamf ID to its downloaded image file path.
type EnrollmentCustomizationImageFile struct {
	JamfID   string
	FileName string
	FilePath string // Relative path from the output directory
}

// EnrollmentCustomizationImageInfo holds the metadata needed to download an image.
type EnrollmentCustomizationImageInfo struct {
	ID      string
	Name    string
	IconURL string
}

type ecImageWork struct {
	idStr string
	info  EnrollmentCustomizationImageInfo
}

// EnrollmentCustomizationImages downloads branding images to
// support_files/enrollment_customization_images/ and returns the mappings for post-processing.
// progressFn is called after each download completes with (current, total) counts.
func EnrollmentCustomizationImages(outputDir string, infoMap map[string]EnrollmentCustomizationImageInfo, progressFn func(int, int)) ([]EnrollmentCustomizationImageFile, error) {
	imagesDir := filepath.Join(outputDir, "support_files", "enrollment_customization_images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("creating enrollment customization images directory: %w", err)
	}

	total := len(infoMap)
	var completed atomic.Int32

	var mu sync.Mutex
	var files []EnrollmentCustomizationImageFile

	work := make([]ecImageWork, 0, total)
	for idStr, info := range infoMap {
		work = append(work, ecImageWork{idStr: idStr, info: info})
	}

	const workers = 5
	var wg sync.WaitGroup
	ch := make(chan ecImageWork, len(work))

	for _, w := range work {
		ch <- w
	}
	close(ch)

	for range workers {
		wg.Go(func() {
			for w := range ch {
				n := int(completed.Add(1))
				result := downloadECImage(w, imagesDir, n, total)
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

func downloadECImage(w ecImageWork, imagesDir string, n, total int) *EnrollmentCustomizationImageFile {
	if w.info.IconURL == "" {
		return nil
	}

	fileName := sanitizeECImageFilename(w.info.Name)
	destPath := filepath.Join(imagesDir, fileName)
	relPath := filepath.Join("support_files", "enrollment_customization_images", fileName)

	if _, err := os.Stat(destPath); err == nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s (already downloaded)\n", n, total, fileName)
		}
		return &EnrollmentCustomizationImageFile{JamfID: w.idStr, FileName: fileName, FilePath: relPath}
	}

	if !Quiet {
		fmt.Printf("  [%d/%d] Downloading %s...\n", n, total, fileName)
	}

	if err := downloadFile(w.info.IconURL, destPath); err != nil {
		if !Quiet {
			fmt.Printf("  [%d/%d] %s failed (%v)\n", n, total, fileName, err)
		}
		_ = os.Remove(destPath)
		return nil
	}

	if !Quiet {
		if fi, err := os.Stat(destPath); err == nil {
			fmt.Printf("  [%d/%d] %s done (%s)\n", n, total, fileName, formatSize(fi.Size()))
		}
	}

	return &EnrollmentCustomizationImageFile{JamfID: w.idStr, FileName: fileName, FilePath: relPath}
}

func sanitizeECImageFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	result := replacer.Replace(name)
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")

	if result == "" {
		result = "enrollment_image"
	}

	if !strings.HasSuffix(strings.ToLower(result), ".png") {
		result += ".png"
	}

	return result
}
