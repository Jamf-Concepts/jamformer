// Copyright 2026, Jamf Software LLC

package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeProviderEnv(t *testing.T) {
	t.Run("provider vars included", func(t *testing.T) {
		env := mergeProviderEnv(map[string]string{
			"JAMFPRO_INSTANCE_FQDN": "example.jamfcloud.com",
		})
		if env["JAMFPRO_INSTANCE_FQDN"] != "example.jamfcloud.com" {
			t.Errorf("expected provider var to be included, got %q", env["JAMFPRO_INSTANCE_FQDN"])
		}
	})

	t.Run("tfexec managed vars excluded", func(t *testing.T) {
		managed := []string{"TF_IN_AUTOMATION", "TF_INPUT", "TF_LOG", "TF_LOG_PATH", "TF_LOG_CORE", "TF_LOG_PROVIDER"}
		for _, key := range managed {
			t.Setenv(key, "should-be-excluded")
		}

		env := mergeProviderEnv(nil)
		for _, key := range managed {
			if _, ok := env[key]; ok {
				t.Errorf("tfexec-managed var %s should be excluded", key)
			}
		}
	})

	t.Run("provider vars take precedence over inherited env", func(t *testing.T) {
		t.Setenv("MY_VAR", "from-env")
		env := mergeProviderEnv(map[string]string{"MY_VAR": "from-provider"})
		if env["MY_VAR"] != "from-provider" {
			t.Errorf("expected provider var to take precedence, got %q", env["MY_VAR"])
		}
	})

	t.Run("inherited env vars included", func(t *testing.T) {
		t.Setenv("SOME_OTHER_VAR", "inherited")
		env := mergeProviderEnv(nil)
		if env["SOME_OTHER_VAR"] != "inherited" {
			t.Errorf("expected inherited env var, got %q", env["SOME_OTHER_VAR"])
		}
	})

	t.Run("AllowDevOverrides suppresses TF_CLI_CONFIG_FILE", func(t *testing.T) {
		AllowDevOverrides = false
		defer func() { AllowDevOverrides = false }()

		env := mergeProviderEnv(nil)
		if env["TF_CLI_CONFIG_FILE"] == "" {
			t.Error("expected TF_CLI_CONFIG_FILE to be set when AllowDevOverrides=false")
		}

		AllowDevOverrides = true
		env = mergeProviderEnv(nil)
		if _, ok := env["TF_CLI_CONFIG_FILE"]; ok {
			t.Error("expected TF_CLI_CONFIG_FILE to be absent when AllowDevOverrides=true")
		}
	})
}

func TestCountImportBlocks(t *testing.T) {
	t.Run("counts across multiple files", func(t *testing.T) {
		dir := t.TempDir()

		file1 := `import {
  to = jamfpro_category.general
  id = "1"
}

import {
  to = jamfpro_category.testing
  id = "2"
}
`
		file2 := `import {
  to = jamfpro_script.setup
  id = "3"
}
`
		if err := os.WriteFile(filepath.Join(dir, "categories_import.tf"), []byte(file1), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scripts_import.tf"), []byte(file2), 0644); err != nil {
			t.Fatal(err)
		}

		count, err := countImportBlocks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Errorf("expected 3 import blocks, got %d", count)
		}
	})

	t.Run("ignores non-import files", func(t *testing.T) {
		dir := t.TempDir()

		if err := os.WriteFile(filepath.Join(dir, "provider.tf"), []byte(`import {}`), 0644); err != nil {
			t.Fatal(err)
		}

		count, err := countImportBlocks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("expected 0 import blocks, got %d", count)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		count, err := countImportBlocks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})
}

func TestRemoveImportBlock(t *testing.T) {
	t.Run("removes target block", func(t *testing.T) {
		dir := t.TempDir()
		content := `import {
  to = jamfpro_category.general
  id = "1"
}

import {
  to = jamfpro_category.testing
  id = "2"
}
`
		path := filepath.Join(dir, "categories_import.tf")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := removeImportBlock(dir, "jamfpro_category.general"); err != nil {
			t.Fatal(err)
		}

		result, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		s := string(result)
		if containsStr(s, "jamfpro_category.general") {
			t.Error("removed block should not appear in output")
		}
		if !containsStr(s, "jamfpro_category.testing") {
			t.Error("other block should be preserved")
		}
	})

	t.Run("resource not found returns nil", func(t *testing.T) {
		dir := t.TempDir()
		content := `import {
  to = jamfpro_category.general
  id = "1"
}
`
		if err := os.WriteFile(filepath.Join(dir, "categories_import.tf"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		err := removeImportBlock(dir, "jamfpro_category.nonexistent")
		if err != nil {
			t.Errorf("expected nil error for missing resource, got %v", err)
		}
	})

	t.Run("removes middle block from three", func(t *testing.T) {
		dir := t.TempDir()
		content := `import {
  to = jamfpro_category.a
  id = "1"
}

import {
  to = jamfpro_category.b
  id = "2"
}

import {
  to = jamfpro_category.c
  id = "3"
}
`
		path := filepath.Join(dir, "categories_import.tf")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := removeImportBlock(dir, "jamfpro_category.b"); err != nil {
			t.Fatal(err)
		}

		result, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		s := string(result)
		if containsStr(s, "jamfpro_category.b") {
			t.Error("removed block should not appear")
		}
		if !containsStr(s, "jamfpro_category.a") || !containsStr(s, "jamfpro_category.c") {
			t.Error("surrounding blocks should be preserved")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		err := removeImportBlock(dir, "jamfpro_category.x")
		if err != nil {
			t.Errorf("expected nil error for empty dir, got %v", err)
		}
	})
}

func TestEmptyCliConfigPath(t *testing.T) {
	path := emptyCliConfigPath()
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file should exist: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("config file should be empty, got %d bytes", info.Size())
	}

	// Should return same path on repeated calls
	path2 := emptyCliConfigPath()
	if path != path2 {
		t.Errorf("expected cached path %q, got %q", path, path2)
	}
}

func TestProgressWriter(t *testing.T) {
	t.Run("counts Refreshing state in single write", func(t *testing.T) {
		var calls []int
		pw := &progressWriter{
			total: 3,
			callback: func(current, _ int) {
				calls = append(calls, current)
			},
		}

		input := "jamfpro_policy.foo: Refreshing state... [id=1]\njamfpro_policy.bar: Refreshing state... [id=2]\n"
		pw.Write([]byte(input)) //nolint:errcheck

		if len(calls) != 2 {
			t.Fatalf("expected 2 callbacks, got %d", len(calls))
		}
		if calls[0] != 1 || calls[1] != 2 {
			t.Errorf("expected [1 2], got %v", calls)
		}
	})

	t.Run("counts Refreshing state split across writes", func(t *testing.T) {
		var count int
		pw := &progressWriter{
			total: 1,
			callback: func(current, _ int) {
				count = current
			},
		}

		// Split "Refreshing state..." across two Write calls
		pw.Write([]byte("jamfpro_script.x: Refreshing")) //nolint:errcheck
		pw.Write([]byte(" state... [id=abc]\n"))         //nolint:errcheck

		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	})

	t.Run("forwards bytes to sink", func(t *testing.T) {
		var sink strings.Builder
		pw := &progressWriter{
			sink:     &sink,
			total:    1,
			callback: func(_, _ int) {},
		}

		input := "jamfpro_policy.x: Refreshing state... [id=1]\n"
		pw.Write([]byte(input)) //nolint:errcheck

		if sink.String() != input {
			t.Errorf("sink got %q, want %q", sink.String(), input)
		}
	})

	t.Run("nil callback does not panic", func(t *testing.T) {
		pw := &progressWriter{total: 1}
		pw.Write([]byte("Refreshing state...\n")) //nolint:errcheck
		if pw.current != 1 {
			t.Errorf("expected current=1, got %d", pw.current)
		}
	})

	t.Run("multiple markers in one write", func(t *testing.T) {
		var count int
		pw := &progressWriter{
			total:    5,
			callback: func(c, _ int) { count = c },
		}

		input := "Refreshing state...\nRefreshing state...\nRefreshing state...\n"
		pw.Write([]byte(input)) //nolint:errcheck

		if count != 3 {
			t.Errorf("expected count 3, got %d", count)
		}
	})

	t.Run("no false positives when partial marker never completes", func(t *testing.T) {
		var count int
		pw := &progressWriter{
			total:    1,
			callback: func(c, _ int) { count = c },
		}

		// "Refreshing sta" + "le..." does not contain "Refreshing state..."
		pw.Write([]byte("Refreshing sta")) //nolint:errcheck
		pw.Write([]byte("le...\n"))        //nolint:errcheck

		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}
	})
}

func TestCtxDefault(t *testing.T) {
	if Ctx == nil {
		t.Fatal("Ctx should not be nil")
	}
	select {
	case <-Ctx.Done():
		t.Error("default Ctx should not be cancelled")
	default:
	}
}

func TestProgressWriterCallbackReceivesTotal(t *testing.T) {
	var gotTotal int
	pw := &progressWriter{
		total:    42,
		callback: func(_, total int) { gotTotal = total },
	}
	pw.Write([]byte("jamfpro_policy.foo: Refreshing state... [id=1]\n")) //nolint:errcheck
	if gotTotal != 42 {
		t.Errorf("expected total=42 in callback, got %d", gotTotal)
	}
}

func TestProgressWriterZeroTotal(t *testing.T) {
	type call struct{ current, total int }
	var calls []call
	pw := &progressWriter{
		total: 0,
		callback: func(c, t int) {
			calls = append(calls, call{c, t})
		},
	}
	pw.Write([]byte("jamfpro_policy.foo: Refreshing state... [id=1]\n")) //nolint:errcheck
	if len(calls) != 1 || calls[0] != (call{1, 0}) {
		t.Errorf("expected [(1,0)], got %v", calls)
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
