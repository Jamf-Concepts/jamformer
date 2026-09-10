// Copyright 2026, Jamf Software LLC

package main

import (
	"strings"
	"testing"
)

func TestRenderBar(t *testing.T) {
	tests := []struct {
		current, total, width int
		want                  string
	}{
		{0, 10, 10, "░░░░░░░░░░"},
		{5, 10, 10, "█████░░░░░"},
		{10, 10, 10, "██████████"},
		{0, 0, 10, ""}, // zero total
		{3, 10, 20, "██████░░░░░░░░░░░░░░"},
		{10, 10, 20, "████████████████████"},
		{1, 3, 9, "███░░░░░░"},
		{100, 10, 10, "██████████"}, // overflow clamped
	}

	for _, tt := range tests {
		got := renderBar(tt.current, tt.total, tt.width)
		if got != tt.want {
			t.Errorf("renderBar(%d, %d, %d) = %q, want %q", tt.current, tt.total, tt.width, got, tt.want)
		}
	}
}

// TestApplyPlatformEnvFallbacksOnlyForPlatform is the regression guard for the
// case that made this a function instead of an inline block: the fallbacks used
// to run before the interactive provider picker, so a shell exporting
// JAMFPLATFORM_CLIENT_SECRET had it sent to whichever host was picked next.
func TestApplyPlatformEnvFallbacksOnlyForPlatform(t *testing.T) {
	for _, provider := range []string{"jamfpro", "jamfprotect", "jsc"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv("JAMFPLATFORM_CLIENT_ID", "platform-id")
			t.Setenv("JAMFPLATFORM_CLIENT_SECRET", "platform-secret")
			t.Setenv("JAMFPLATFORM_BASE_URL", "https://us.api.jamfcloud.com")
			t.Setenv("JAMFPLATFORM_ENVIRONMENT_ID", "env-1")
			t.Setenv("JAMFPLATFORM_TENANT_ID", "tenant-1")

			var url, clientID, clientSecret, environmentID, tenantID string
			applyPlatformEnvFallbacks(provider, &url, &clientID, &clientSecret, &environmentID, &tenantID)

			for name, got := range map[string]string{
				"url": url, "clientID": clientID, "clientSecret": clientSecret,
				"environmentID": environmentID, "tenantID": tenantID,
			} {
				if got != "" {
					t.Errorf("provider %q: %s = %q, want empty (no JAMFPLATFORM_* value may reach a non-Platform run)", provider, name, got)
				}
			}
		})
	}
}

func TestApplyPlatformEnvFallbacksAppliesForPlatform(t *testing.T) {
	t.Setenv("JAMFPLATFORM_CLIENT_ID", "platform-id")
	t.Setenv("JAMFPLATFORM_CLIENT_SECRET", "platform-secret")
	t.Setenv("JAMFPLATFORM_BASE_URL", "https://us.api.jamfcloud.com")
	t.Setenv("JAMFPLATFORM_ENVIRONMENT_ID", "env-1")
	t.Setenv("JAMFPLATFORM_TENANT_ID", "tenant-1")

	var url, clientID, clientSecret, environmentID, tenantID string
	applyPlatformEnvFallbacks("jamfplatform", &url, &clientID, &clientSecret, &environmentID, &tenantID)

	want := map[string]string{
		"url":           "https://us.api.jamfcloud.com",
		"clientID":      "platform-id",
		"clientSecret":  "platform-secret",
		"environmentID": "env-1",
		"tenantID":      "tenant-1",
	}
	got := map[string]string{
		"url": url, "clientID": clientID, "clientSecret": clientSecret,
		"environmentID": environmentID, "tenantID": tenantID,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, want %q", name, got[name], w)
		}
	}
}

// A JAMF_* value already resolved wins over the provider's own variable.
func TestApplyPlatformEnvFallbacksDoesNotOverride(t *testing.T) {
	t.Setenv("JAMFPLATFORM_CLIENT_ID", "platform-id")
	t.Setenv("JAMFPLATFORM_ENVIRONMENT_ID", "env-fallback")

	url, clientID, clientSecret := "", "jamf-id", ""
	environmentID, tenantID := "env-jamf", ""
	applyPlatformEnvFallbacks("jamfplatform", &url, &clientID, &clientSecret, &environmentID, &tenantID)

	if clientID != "jamf-id" {
		t.Errorf("clientID = %q, want %q", clientID, "jamf-id")
	}
	if environmentID != "env-jamf" {
		t.Errorf("environmentID = %q, want %q", environmentID, "env-jamf")
	}
}

func TestUnknownFilterKeyErrorNamesGARemoval(t *testing.T) {
	nameMap := map[string]string{"policy": "policy", "scripts": "scripts"}

	for _, key := range []string{"api_client", "api_role"} {
		err := unknownFilterKeyError("jamfplatform", key, nameMap)
		if err == nil {
			t.Fatalf("unknownFilterKeyError(jamfplatform, %q) = nil, want an error", key)
		}
		msg := err.Error()
		if !strings.Contains(msg, "no longer available") || !strings.Contains(msg, "Platform API GA") {
			t.Errorf("unknownFilterKeyError(jamfplatform, %q) = %q, want the GA removal explanation", key, msg)
		}
		if strings.Contains(msg, "Valid types") {
			t.Errorf("unknownFilterKeyError(jamfplatform, %q) = %q, want no valid-name dump for a retired key", key, msg)
		}
	}
}

// A retirement is per-provider: jamfpro still has api_roles, so "api_role"
// there is a near-miss that should show the valid names, not a GA removal.
func TestUnknownFilterKeyErrorRetirementIsPerProvider(t *testing.T) {
	nameMap := map[string]string{"api_roles": "api_roles", "policies": "policies"}

	for _, provider := range []string{"jamfpro", "jamfprotect", "jsc"} {
		err := unknownFilterKeyError(provider, "api_role", nameMap)
		if err == nil {
			t.Fatalf("unknownFilterKeyError(%q, api_role) = nil, want an error", provider)
		}
		if strings.Contains(err.Error(), "Platform API GA") {
			t.Errorf("unknownFilterKeyError(%q, api_role) = %q, want no Platform GA claim", provider, err.Error())
		}
		if !strings.Contains(err.Error(), "Valid types") {
			t.Errorf("unknownFilterKeyError(%q, api_role) = %q, want the valid-name list", provider, err.Error())
		}
	}
}

func TestUnknownFilterKeyErrorTypo(t *testing.T) {
	nameMap := map[string]string{"policy": "policy", "scripts": "scripts"}

	err := unknownFilterKeyError("jamfplatform", "polcy", nameMap)
	if err == nil {
		t.Fatal("unknownFilterKeyError on a typo = nil, want an error")
	}
	if !strings.Contains(err.Error(), "unknown resource type") || !strings.Contains(err.Error(), "Valid types") {
		t.Errorf("unknownFilterKeyError on a typo = %q, want the unknown-type message with valid names", err.Error())
	}
}
