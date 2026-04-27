// Copyright 2026, Jamf Software LLC

package discovery

import (
	"os"
	"strings"
	"testing"
)

func TestWritePanicLog(t *testing.T) {
	t.Run("creates file with correct content", func(t *testing.T) {
		stack := []byte("goroutine 1 [running]:\nmain.main()\n\t/app/main.go:10")
		path := writePanicLog("categories", "index out of range", stack)
		if path == "" {
			t.Fatal("expected non-empty file path")
		}
		defer func() { _ = os.Remove(path) }()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading panic log: %v", err)
		}
		content := string(data)

		if !strings.Contains(content, "categories") {
			t.Error("log should contain the resource type")
		}
		if !strings.Contains(content, "index out of range") {
			t.Error("log should contain the panic value")
		}
		if !strings.Contains(content, "goroutine 1 [running]") {
			t.Error("log should contain the stack trace")
		}
		if !strings.Contains(content, "jamformer panic during categories discovery") {
			t.Error("log should contain the header line")
		}
		if !strings.Contains(content, "time:") {
			t.Error("log should contain a timestamp")
		}
	})

	t.Run("filename contains jamformer-crash prefix", func(t *testing.T) {
		path := writePanicLog("scripts", "nil pointer", []byte("stack"))
		if path == "" {
			t.Fatal("expected non-empty file path")
		}
		defer func() { _ = os.Remove(path) }()

		if !strings.Contains(path, "jamformer-crash-") {
			t.Errorf("expected filename to contain 'jamformer-crash-', got %s", path)
		}
		if !strings.HasSuffix(path, ".log") {
			t.Errorf("expected filename to end with .log, got %s", path)
		}
	})

	t.Run("handles numeric panic value", func(t *testing.T) {
		path := writePanicLog("buildings", 42, []byte("trace"))
		if path == "" {
			t.Fatal("expected non-empty file path")
		}
		defer func() { _ = os.Remove(path) }()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading panic log: %v", err)
		}
		if !strings.Contains(string(data), "42") {
			t.Error("log should contain the numeric panic value")
		}
	})
}

func TestResultsZeroValue(t *testing.T) {
	var r Results

	if r.Sites != nil {
		t.Error("expected nil Sites")
	}
	if r.Singletons != nil {
		t.Error("expected nil Singletons")
	}
	if r.IconInfo != nil {
		t.Error("expected nil IconInfo")
	}
	if r.EnrollmentCustomizationInfo != nil {
		t.Error("expected nil EnrollmentCustomizationInfo")
	}
}
