// Copyright 2026, Jamf Software LLC

package main

import "testing"

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
