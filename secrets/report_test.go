// Copyright 2026, Jamf Software LLC

package secrets

import "testing"

func TestRedactLongerSecrets(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   string
	}{
		{"15 chars", "SuperS3cretP@ss", "Supe*******P@ss"},
		{"9 chars", "123456789", "1234*6789"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redact(tt.secret)
			if got != tt.want {
				t.Errorf("redact(%q) = %q, want %q", tt.secret, got, tt.want)
			}
		})
	}
}
