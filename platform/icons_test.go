// Copyright 2026, Jamf Software LLC

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
)

func TestGenerateIcons(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")

	// Two policies share icon 124; one policy has no icon (empty object).
	src := `resource "jamfplatform_pro_policy" "install_chrome" {
  self_service = {
    use_for_self_service = true
    self_service_icon = {
      filename = "Chrome.png"
      id       = "124"
      uri      = "https://cdn.example/icon/abc"
    }
  }
}

resource "jamfplatform_pro_policy" "update_chrome" {
  self_service = {
    self_service_icon = {
      filename = "Chrome.png"
      id       = "124"
      uri      = "https://cdn.example/icon/abc"
    }
  }
}

resource "jamfplatform_pro_policy" "no_icon" {
  self_service = {
    self_service_icon = {}
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg := registry.New()
	n, err := GenerateIcons(generatedFile, reg)
	if err != nil {
		t.Fatalf("GenerateIcons: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 unique icon, got %d", n)
	}

	addr, ok := reg.Resolve("jamfplatform_pro_icon", "124")
	if !ok || addr != "jamfplatform_pro_icon.chrome" {
		t.Fatalf("expected icon 124 registered to jamfplatform_pro_icon.chrome, got %q (ok=%v)", addr, ok)
	}

	out, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)

	if !strings.Contains(body, `resource "jamfplatform_pro_icon" "chrome"`) {
		t.Errorf("expected icon resource block, got:\n%s", body)
	}
	if !strings.Contains(body, `icon_file_source = "https://cdn.example/icon/abc"`) {
		t.Errorf("expected icon_file_source set to the CDN uri, got:\n%s", body)
	}
	if !strings.Contains(body, "ignore_changes") {
		t.Errorf("expected lifecycle ignore_changes, got:\n%s", body)
	}
	if !strings.Contains(body, `id = "124"`) {
		t.Errorf("expected import block with id 124, got:\n%s", body)
	}
	// uri/filename echoes dropped from the policy self_service_icon; id kept.
	if strings.Contains(body, `uri      = "https://cdn.example/icon/abc"`) || strings.Contains(body, `uri = "https://cdn.example/icon/abc"`) {
		t.Errorf("expected self_service_icon.uri dropped from policies, got:\n%s", body)
	}
	if strings.Contains(body, `filename = "Chrome.png"`) {
		t.Errorf("expected self_service_icon.filename dropped from policies, got:\n%s", body)
	}
	if !strings.Contains(body, `id       = "124"`) && !strings.Contains(body, `id = "124"`) {
		t.Errorf("expected self_service_icon.id retained on policies, got:\n%s", body)
	}
}

func TestGenerateIconsNoIcons(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")
	src := `resource "jamfplatform_pro_policy" "p" {
  general = {
    name = "x"
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := GenerateIcons(generatedFile, registry.New())
	if err != nil {
		t.Fatalf("GenerateIcons: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 icons, got %d", n)
	}
}

func TestIconLabelBase(t *testing.T) {
	tests := []struct {
		filename, id, want string
	}{
		{"AdobePhotoshop2025-512x512.png", "5", "adobephotoshop2025_512x512"},
		{"", "9", "icon_9"},
		{".png", "3", "icon_3"},
	}
	for _, tt := range tests {
		if got := iconLabelBase(tt.filename, tt.id); got != tt.want {
			t.Errorf("iconLabelBase(%q,%q) = %q, want %q", tt.filename, tt.id, got, tt.want)
		}
	}
}
