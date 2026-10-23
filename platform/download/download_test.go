// Copyright 2026, Jamf Software LLC

package download

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortenTimeouts swaps in test-sized bounds and restores the real ones.
func shortenTimeouts(t *testing.T, header, stall time.Duration) {
	t.Helper()
	prevHeader, prevStall, prevClient := responseHeaderTimeout, stallTimeout, downloadClient
	responseHeaderTimeout, stallTimeout = header, stall
	downloadClient = newDownloadClient(header)
	t.Cleanup(func() {
		responseHeaderTimeout, stallTimeout, downloadClient = prevHeader, prevStall, prevClient
	})
}

// stallFor blocks the handler until the client gives up, or cap elapses so a
// missing bound fails the test rather than hanging it.
func stallFor(r *http.Request, limit time.Duration) {
	select {
	case <-r.Context().Done():
	case <-time.After(limit):
	}
}

func TestDownloadFileTimesOutWhenNoResponseArrives(t *testing.T) {
	shortenTimeouts(t, 200*time.Millisecond, 10*time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stallFor(r, 6*time.Second)
	}))
	defer srv.Close()

	start := time.Now()
	err := downloadFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "a.pkg"))
	if err == nil {
		t.Fatal("download from a server that never answers returned no error")
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("took %v to fail; the response-header timeout is not applied", elapsed)
	}
}

func TestDownloadFileFailsWhenTransferStalls(t *testing.T) {
	shortenTimeouts(t, 10*time.Second, 200*time.Millisecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		stallFor(r, 6*time.Second)
	}))
	defer srv.Close()

	start := time.Now()
	err := downloadFile(context.Background(), srv.URL, filepath.Join(t.TempDir(), "a.pkg"))
	if !errors.Is(err, errStalled) {
		t.Fatalf("err = %v, want errStalled", err)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("took %v to fail; the stall timeout is not applied", elapsed)
	}
}

// A slow transfer that keeps delivering bytes must finish however long it
// takes in total: the stall bound restarts on every chunk, and there is no
// overall deadline.
func TestDownloadFileCompletesSlowSteadyTransfer(t *testing.T) {
	shortenTimeouts(t, 10*time.Second, 200*time.Millisecond)
	const chunks = 8 // 8 × 100ms ≈ 4× the stall bound in total
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for range chunks {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "a.pkg")
	if err := downloadFile(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("slow but steady download failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("x", chunks); string(got) != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}
