// Copyright 2026, Jamf Software LLC

package postprocess

import "testing"

func TestBuildExtractFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		kind    FileKind
		content string
		want    string
	}{
		{"mobileconfig no existing ext", "FileVault Settings", FileKindMobileconfig, "", "FileVault Settings.mobileconfig"},
		{"mobileconfig already has ext", "Allow ClearPass.mobileconfig", FileKindMobileconfig, "", "Allow ClearPass.mobileconfig"},
		{"mobileconfig mixed-case ext", "Profile.MobileConfig", FileKindMobileconfig, "", "Profile.MobileConfig"},
		{"xml no existing ext", "app config", FileKindXML, "", "app config.xml"},
		{"xml already has ext", "app config.xml", FileKindXML, "", "app config.xml"},
		{"script with shebang adds ext", "install chrome", FileKindScript, "#!/bin/bash\necho hi", "install chrome.sh"},
		{"script already has ext", "install.sh", FileKindScript, "#!/bin/bash", "install.sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExtractFileName(tt.input, tt.kind, tt.content, map[string]int{})
			if got != tt.want {
				t.Errorf("buildExtractFileName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildExtractFileNameCollision(t *testing.T) {
	fileNames := map[string]int{}
	first := buildExtractFileName("Same Name", FileKindMobileconfig, "", fileNames)
	second := buildExtractFileName("Same Name", FileKindMobileconfig, "", fileNames)
	if first != "Same Name.mobileconfig" {
		t.Errorf("first = %q", first)
	}
	if second != "Same Name_2.mobileconfig" {
		t.Errorf("second = %q, want collision suffix before extension", second)
	}
}
