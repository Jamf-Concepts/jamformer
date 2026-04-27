// Copyright 2026, Jamf Software LLC

package download

import "testing"

func TestSanitizeECImageFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple name", in: "BrandingLogo", want: "BrandingLogo.png"},
		{name: "already has .png", in: "logo.png", want: "logo.png"},
		{name: "already has .PNG uppercase", in: "logo.PNG", want: "logo.PNG"},
		{name: "mixed case .Png", in: "logo.Png", want: "logo.Png"},
		{name: "spaces preserved", in: "My Logo", want: "My Logo.png"},
		{name: "slashes replaced", in: "path/to/image", want: "path_to_image.png"},
		{name: "backslashes replaced", in: "path\\to\\image", want: "path_to_image.png"},
		{name: "colons replaced", in: "image:v2", want: "image_v2.png"},
		{name: "asterisks replaced", in: "star*name", want: "star_name.png"},
		{name: "question marks replaced", in: "what?", want: "what_.png"},
		{name: "quotes replaced", in: `say "hello"`, want: "say _hello_.png"},
		{name: "angle brackets replaced", in: "a<b>c", want: "a_b_c.png"},
		{name: "pipes replaced", in: "a|b", want: "a_b.png"},
		{name: "multiple special chars", in: `a/b\c:d*e?f"g<h>i|j`, want: "a_b_c_d_e_f_g_h_i_j.png"},
		{name: "empty string fallback", in: "", want: "enrollment_image.png"},
		{name: "only dots", in: "...", want: "enrollment_image.png"},
		{name: "leading trailing dots stripped", in: ".hidden.", want: "hidden.png"},
		{name: "leading trailing spaces stripped", in: "  spaced  ", want: "spaced.png"},
		{name: "trailing dot before png check", in: "name.", want: "name.png"},
		{name: "dot png in middle", in: "file.png.bak", want: "file.png.bak.png"},
		{name: "only whitespace", in: "   ", want: "enrollment_image.png"},
		{name: "all special chars", in: `/*?"<>|\:`, want: "_________.png"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeECImageFilename(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeECImageFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
