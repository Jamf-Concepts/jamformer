// Copyright 2026, Jamf Software LLC

package download

import "testing"

func TestSafePackageFileName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "plain pkg", input: "App-1.0.pkg", want: "App-1.0.pkg", wantOK: true},
		{name: "spaces allowed", input: "My App 2.pkg", want: "My App 2.pkg", wantOK: true},
		{name: "unicode allowed", input: "café.pkg", want: "café.pkg", wantOK: true},
		{name: "empty", input: "", wantOK: false},
		{name: "dot", input: ".", wantOK: false},
		{name: "dotdot", input: "..", wantOK: false},
		{name: "forward traversal", input: "../../etc/passwd", wantOK: false},
		{name: "backslash traversal", input: `..\..\windows\system32`, wantOK: false},
		{name: "leading slash", input: "/etc/passwd", wantOK: false},
		{name: "embedded slash", input: "sub/dir/file.pkg", wantOK: false},
		{name: "embedded backslash", input: `sub\file.pkg`, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := safePackageFileName(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("safePackageFileName(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("safePackageFileName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero bytes", bytes: 0, want: "0 B"},
		{name: "small bytes", bytes: 512, want: "512 B"},
		{name: "one byte", bytes: 1, want: "1 B"},
		{name: "1023 bytes", bytes: 1023, want: "1023 B"},
		{name: "exactly 1 KB", bytes: 1024, want: "1.0 KB"},
		{name: "1.5 KB", bytes: 1536, want: "1.5 KB"},
		{name: "999 KB", bytes: 999 * 1024, want: "999.0 KB"},
		{name: "exactly 1 MB", bytes: 1 << 20, want: "1.0 MB"},
		{name: "10.5 MB", bytes: int64(10.5 * float64(1<<20)), want: "10.5 MB"},
		{name: "500 MB", bytes: 500 * (1 << 20), want: "500.0 MB"},
		{name: "exactly 1 GB", bytes: 1 << 30, want: "1.0 GB"},
		{name: "2.5 GB", bytes: int64(2.5 * float64(1<<30)), want: "2.5 GB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSize(tc.bytes)
			if got != tc.want {
				t.Errorf("formatSize(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}
