// Copyright 2026, Jamf Software LLC

package naming

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Disable Bluetooth", "disable_bluetooth"},
		{"Install Firefox - Staff", "install_firefox_staff"},
		{"Firefox (Staff) — v2.1", "firefox_staff_v2_1"},
		{"365 Business", "_365_business"},
		{"  lots   of   spaces  ", "lots_of_spaces"},
		{"CamelCaseName", "camelcasename"},
		{"already_valid", "already_valid"},
		{"!!!special!!!", "special"},
		{"", "unnamed"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Sanitize(tt.input)
			if got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripScriptExtension(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gen-script-policy-RunCommand.sh", "gen-script-policy-RunCommand"},
		{"compliance_benchmark.zsh", "compliance_benchmark"},
		{"my_script.py", "my_script"},
		{"backup.rb", "backup"},
		{"Windows Script.ps1", "Windows Script"},
		{"no_extension", "no_extension"},
		{"tricky.shell", "tricky.shell"}, // .shell is not a known extension
		{"", ""},
		{"just.SH", "just"}, // case insensitive match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StripScriptExtension(tt.input)
			if got != tt.want {
				t.Errorf("StripScriptExtension(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTrackerDeduplication(t *testing.T) {
	tracker := NewTracker()
	rt := "jamfpro_script"

	label1 := tracker.Label(rt, "My Script")
	label2 := tracker.Label(rt, "My Script")
	label3 := tracker.Label(rt, "My Script")

	if label1 != "my_script" {
		t.Errorf("first label = %q, want %q", label1, "my_script")
	}
	if label2 != "my_script_2" {
		t.Errorf("second label = %q, want %q", label2, "my_script_2")
	}
	if label3 != "my_script_3" {
		t.Errorf("third label = %q, want %q", label3, "my_script_3")
	}
}

func TestTrackerDeduplication_NaturalSuffixCollision(t *testing.T) {
	tracker := NewTracker()
	rt := "jamfpro_icon"

	// "Home Assistant" sanitizes to "home_assistant"
	// "Home Assistant" again deduplicates to "home_assistant_2"
	// "Home Assistant 2" naturally sanitizes to "home_assistant_2" — must not collide
	label1 := tracker.Label(rt, "Home Assistant")
	label2 := tracker.Label(rt, "Home Assistant")
	label3 := tracker.Label(rt, "Home Assistant 2")

	if label1 != "home_assistant" {
		t.Errorf("first = %q, want %q", label1, "home_assistant")
	}
	if label2 != "home_assistant_2" {
		t.Errorf("second = %q, want %q", label2, "home_assistant_2")
	}
	if label3 != "home_assistant_2_2" {
		t.Errorf("third = %q, want %q (must skip already-used home_assistant_2)", label3, "home_assistant_2_2")
	}
}
