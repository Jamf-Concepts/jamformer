// Copyright 2026, Jamf Software LLC

package download

import "testing"

func TestSafePackageFileName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"app.pkg", "app.pkg", true},
		{"my-pkg-1.2.3.pkg", "my-pkg-1.2.3.pkg", true},
		{"", "", false},
		{"../../etc/passwd", "", false},
		{"sub/dir.pkg", "", false},
		{`win\dir.pkg`, "", false},
		{".", "", false},
		{"..", "", false},
	}
	for _, tc := range cases {
		got, ok := safePackageFileName(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("safePackageFileName(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1 << 20, "5.0 MB"},
		{3 * 1 << 30, "3.0 GB"},
	}
	for _, tc := range cases {
		if got := formatSize(tc.in); got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
