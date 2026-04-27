// Copyright 2026, Jamf Software LLC

package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeVarName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"jamfpro_webhook.alerts", "jamfpro_webhook_alerts"},
		{"My File (1).xml", "my_file_1_xml"},
		{"UPPER_CASE", "upper_case"},
		{"123start", "_123start"},
		{"a--b__c", "a_b_c"},
		{"___leading___", "leading"},
		{"normal_name", "normal_name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeVarName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeVarName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildVariableBlock(t *testing.T) {
	tests := []struct {
		name    string
		varName string
		finding Finding
		wantIn  string
	}{
		{
			name:    "with resource address",
			varName: "webhook_password",
			finding: Finding{ResourceAddress: "jamfpro_webhook.alerts"},
			wantIn:  "Sensitive value from jamfpro_webhook.alerts",
		},
		{
			name:    "with support file ref",
			varName: "app_config_secret",
			finding: Finding{SupportFileRef: "support_files/app_configurations/Test.xml"},
			wantIn:  "Sensitive value from support_files/app_configurations/Test.xml",
		},
		{
			name:    "generic fallback",
			varName: "some_secret",
			finding: Finding{},
			wantIn:  `description = "Sensitive value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVariableBlock(tt.varName, &tt.finding)
			if got == "" {
				t.Fatal("expected non-empty output")
			}
			if !contains(got, tt.wantIn) {
				t.Errorf("missing %q in:\n%s", tt.wantIn, got)
			}
			if !contains(got, "sensitive   = true") {
				t.Error("variable block should be sensitive")
			}
		})
	}
}

func TestBuildVarName(t *testing.T) {
	tests := []struct {
		name    string
		finding Finding
		want    string
	}{
		{
			name:    "resource address with attr",
			finding: Finding{ResourceAddress: "jamfpro_webhook.alerts", AttrName: "password"},
			want:    "jamfpro_webhook_alerts_password",
		},
		{
			name:    "support file ref",
			finding: Finding{SupportFileRef: "support_files/app_configurations/MyApp.xml", AttrName: "secret"},
			want:    "myapp_secret",
		},
		{
			name:    "no attr falls back to secret",
			finding: Finding{ResourceAddress: "jamfpro_webhook.alerts"},
			want:    "jamfpro_webhook_alerts_secret",
		},
		{
			name:    "empty finding returns empty",
			finding: Finding{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used := make(map[string]bool)
			got := buildVarName(&tt.finding, used)
			if got != tt.want {
				t.Errorf("buildVarName() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("deduplication", func(t *testing.T) {
		used := make(map[string]bool)
		f := Finding{ResourceAddress: "jamfpro_webhook.alerts", AttrName: "password"}
		first := buildVarName(&f, used)
		second := buildVarName(&f, used)
		if first == second {
			t.Errorf("duplicate names should differ: %q == %q", first, second)
		}
		if second != first+"_2" {
			t.Errorf("second name should have _2 suffix, got %q", second)
		}
	})
}

func TestAppendToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := appendToFile(path, "first\n"); err != nil {
		t.Fatal(err)
	}
	if err := appendToFile(path, "second\n"); err != nil {
		t.Fatal(err)
	}

	content, _ := os.ReadFile(path)
	if string(content) != "first\nsecond\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
