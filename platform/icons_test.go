// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamformer/registry"
	sdkpro "github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
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
	n, err := GenerateIcons(context.Background(), nil, generatedFile, reg)
	if err != nil {
		t.Fatalf("GenerateIcons: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 unique icon, got %d", n)
	}

	addr, ok := reg.Resolve("jamfplatform_pro_icon", "124")
	if !ok || addr != "jamfplatform_pro_icon.policy_install_chrome" {
		t.Fatalf("expected icon 124 registered to jamfplatform_pro_icon.policy_install_chrome, got %q (ok=%v)", addr, ok)
	}

	out, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}
	body := string(out)

	if !strings.Contains(body, `resource "jamfplatform_pro_icon" "policy_install_chrome"`) {
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
	n, err := GenerateIcons(context.Background(), nil, generatedFile, registry.New())
	if err != nil {
		t.Fatalf("GenerateIcons: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 icons, got %d", n)
	}
}

func TestIconLabelBase(t *testing.T) {
	tests := []struct {
		hint, ref, filename, id, want string
	}{
		{"policy", "install_chrome", "Chrome.png", "5", "policy_install_chrome"},
		{"", "install_chrome", "", "5", "install_chrome"},
		{"", "", "AdobePhotoshop2025-512x512.png", "5", "adobephotoshop2025_512x512"},
		{"", "", "", "9", "icon_9"},
		{"", "", ".png", "3", "icon_3"},
	}
	for _, tt := range tests {
		if got := iconLabelBase(tt.hint, tt.ref, tt.filename, tt.id); got != tt.want {
			t.Errorf("iconLabelBase(%q,%q,%q,%q) = %q, want %q", tt.hint, tt.ref, tt.filename, tt.id, got, tt.want)
		}
	}
}

type fakeIconGetter map[string]string

func (f fakeIconGetter) GetIconV1(_ context.Context, id string) (*sdkpro.IconResponse, error) {
	u, ok := f[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &sdkpro.IconResponse{URL: u}, nil
}

// uri/filename are Computed-only, so -generate-config-out never emits them: the
// URL must come from the lookup, and an unresolvable icon must not produce an
// empty icon_file_source.
func TestGenerateIconsResolvesURIViaLookup(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")
	src := `resource "jamfplatform_pro_policy" "a" {
  self_service = {
    self_service_icon = {
      id = "4507"
    }
  }
}

resource "jamfplatform_pro_policy" "b" {
  self_service = {
    self_service_icon = {
      id = "999"
    }
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := GenerateIcons(context.Background(), fakeIconGetter{"4507": "https://cdn.example/icon/xyz"}, generatedFile, registry.New())
	if err != nil {
		t.Fatalf("GenerateIcons: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 resolvable icon, got %d", n)
	}
	out, _ := os.ReadFile(generatedFile)
	got := string(out)
	if !strings.Contains(got, `icon_file_source = "https://cdn.example/icon/xyz"`) {
		t.Errorf("missing resolved icon_file_source:\n%s", got)
	}
	if strings.Contains(got, `icon_file_source = ""`) || strings.Contains(got, "icon_999") {
		t.Errorf("unresolvable icon must not be emitted:\n%s", got)
	}
}

// Every resource type that carries an icon contributes one icon resource, named
// for the first referencing resource, with the id reference left for rewriting.
func TestGenerateIconsAllSourceTypes(t *testing.T) {
	dir := t.TempDir()
	generatedFile := filepath.Join(dir, "generated.tf")
	src := `resource "jamfplatform_pro_policy" "pol" {
  self_service = {
    self_service_icon = {
      id = "1"
    }
  }
}

resource "jamfplatform_pro_mac_app_store_app" "mac" {
  self_service = {
    self_service_icon = {
      id = "2"
    }
  }
}

resource "jamfplatform_pro_mobile_device_app" "mob" {
  self_service = {
    self_service_icon = {
      id = "3"
    }
  }
}

resource "jamfplatform_pro_ebook" "book" {
  self_service = {
    icon_id  = "4"
    icon_uri = "https://cdn.example/stale"
  }
}

resource "jamfplatform_pro_patch_policy" "patch" {
  user_interaction = {
    self_service_icon_id = "5"
  }
}
`
	if err := os.WriteFile(generatedFile, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	lookup := fakeIconGetter{"1": "https://cdn/1", "2": "https://cdn/2", "3": "https://cdn/3", "4": "https://cdn/4", "5": "https://cdn/5"}
	reg := registry.New()
	n, err := GenerateIcons(context.Background(), lookup, generatedFile, reg)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 icons, got %d", n)
	}
	for id, want := range map[string]string{
		"1": "jamfplatform_pro_icon.policy_pol",
		"2": "jamfplatform_pro_icon.mac_app_mac",
		"3": "jamfplatform_pro_icon.mobile_app_mob",
		"4": "jamfplatform_pro_icon.ebook_book",
		"5": "jamfplatform_pro_icon.patch_policy_patch",
	} {
		if got, _ := reg.Resolve("jamfplatform_pro_icon", id); got != want {
			t.Errorf("icon %s registered to %q, want %q", id, got, want)
		}
	}
	out, _ := os.ReadFile(generatedFile)
	if strings.Contains(string(out), "icon_uri") {
		t.Errorf("ebook icon_uri echo should be dropped:\n%s", out)
	}
}
