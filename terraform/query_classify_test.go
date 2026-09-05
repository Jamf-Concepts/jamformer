// Copyright 2026, Jamf Software LLC

package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// classifyQueryResult decides whether a non-zero `terraform query` exit is a
// recoverable partial result or a dead run, and the diagnostics it attaches are
// the only account of what terraform refused. These cases cover the three
// outcomes plus the success path.
func TestClassifyQueryResult(t *testing.T) {
	const diags = "Error: Missing required argument\n\n  on generated.tf line 12"
	runErr := errors.New("exit status 1")

	t.Run("populated file is a partial result carrying the diagnostics", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "generated.tf")
		if err := os.WriteFile(out, []byte("resource \"x\" \"y\" {}\n"), 0644); err != nil {
			t.Fatal(err)
		}

		err := classifyQueryResult(runErr, out, diags)
		var partial *PartialQueryError
		if !errors.As(err, &partial) {
			t.Fatalf("want *PartialQueryError, got %T (%v)", err, err)
		}
		if partial.Diagnostics != diags {
			t.Errorf("diagnostics = %q, want the provider's text", partial.Diagnostics)
		}
		// The exit status must still be reachable: downgrading to a partial
		// result should not discard what the decision was based on.
		if !errors.Is(err, runErr) {
			t.Error("PartialQueryError does not unwrap to the original run error")
		}
		if !strings.Contains(err.Error(), diags) {
			t.Error("Error() omits the diagnostics")
		}
	})

	t.Run("empty diagnostics fall back to the exit status", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "generated.tf")
		if err := os.WriteFile(out, []byte("resource \"x\" \"y\" {}\n"), 0644); err != nil {
			t.Fatal(err)
		}

		var partial *PartialQueryError
		if !errors.As(classifyQueryResult(runErr, out, ""), &partial) {
			t.Fatal("want *PartialQueryError")
		}
		if partial.Diagnostics != runErr.Error() {
			t.Errorf("diagnostics = %q, want the exit status as the last resort", partial.Diagnostics)
		}
	})

	t.Run("zero-byte file is fatal", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "generated.tf")
		if err := os.WriteFile(out, nil, 0644); err != nil {
			t.Fatal(err)
		}

		err := classifyQueryResult(runErr, out, diags)
		if _, ok := errors.AsType[*PartialQueryError](err); ok {
			t.Fatal("an empty output file generated no config; the run must fail")
		}
		if !errors.Is(err, runErr) || !strings.Contains(err.Error(), diags) {
			t.Errorf("fatal error must wrap the run error and carry the diagnostics, got %v", err)
		}
	})

	t.Run("absent file is fatal", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "never-written.tf")

		err := classifyQueryResult(runErr, out, "")
		if _, ok := errors.AsType[*PartialQueryError](err); ok {
			t.Fatal("no output file at all means nothing was generated; the run must fail")
		}
		if !errors.Is(err, runErr) {
			t.Errorf("fatal error must wrap the run error, got %v", err)
		}
	})

	t.Run("nil run error is success", func(t *testing.T) {
		if err := classifyQueryResult(nil, filepath.Join(t.TempDir(), "absent.tf"), ""); err != nil {
			t.Errorf("want nil, got %v", err)
		}
	})
}
