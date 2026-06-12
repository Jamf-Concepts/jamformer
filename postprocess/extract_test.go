// Copyright 2026, Jamf Software LLC

package postprocess

import "testing"

func TestShouldSkipProfile(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantSkip   bool
		wantReason string
	}{
		{
			name:     "empty payload",
			payload:  "",
			wantSkip: false,
		},
		{
			name:     "whitespace only",
			payload:  "   \n  ",
			wantSkip: false,
		},
		{
			name: "regular unsigned profile",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>PayloadOrganization</key><string>Acme Corp</string>
  <key>PayloadIdentifier</key><string>com.acme.wifi.office</string>
</dict></plist>`,
			wantSkip: false,
		},
		{
			name:       "non-XML (binary/base64 DER)",
			payload:    "MIAGCSqGSIb3DQEHAqCAMIACAQExCzAJBgUrDgMCGgUAMIAGCSqGSIb3DQEH",
			wantSkip:   true,
			wantReason: "signed profile (non-XML payload)",
		},
		{
			name: "Jamf Protect plan profile (com.jamf.protect.* identifier)",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>PayloadOrganization</key><string>Jamf Protect</string>
  <key>PayloadIdentifier</key><string>com.jamf.protect.effa7f47-65c9-4bb5-94de-c6bb04646268</string>
  <key>PayloadContent</key><array></array>
</dict></plist>`,
			wantSkip:   true,
			wantReason: "vendor-managed profile (PayloadIdentifier: com.jamf.protect.effa7f47-65c9-4bb5-94de-c6bb04646268)",
		},
		{
			name: "Jamf Security Cloud / Jamf Trust profile (com.jamf.trust.* in sub-payload)",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>PayloadOrganization</key><string>London South East Colleges Limited</string>
  <key>PayloadIdentifier</key><string>5D90E895-1A74-44D7-98EC-5D9E591FD4F7</string>
  <key>PayloadContent</key><array>
    <dict>
      <key>PayloadIdentifier</key><string>com.jamf.trust.profile.jit.certificate.7bd6af2a-f2b7-2d9a-c9ab-073c18255e34</string>
      <key>PayloadType</key><string>com.apple.security.root</string>
    </dict>
  </array>
</dict></plist>`,
			wantSkip:   true,
			wantReason: "vendor-managed profile (PayloadIdentifier: com.jamf.trust.profile.jit.certificate.7bd6af2a-f2b7-2d9a-c9ab-073c18255e34)",
		},
		{
			name: "Jamf Trust bootstrap payload (com.jamf.trust.pa.bootstrap)",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>PayloadOrganization</key><string>Ozone Financial Technology Limited</string>
  <key>PayloadIdentifier</key><string>8718CF6C-2223-4AF6-A58A-CADC684D7637</string>
  <key>PayloadContent</key><array>
    <dict>
      <key>PayloadIdentifier</key><string>com.jamf.trust.pa.bootstrap</string>
    </dict>
  </array>
</dict></plist>`,
			wantSkip:   true,
			wantReason: "vendor-managed profile (PayloadIdentifier: com.jamf.trust.pa.bootstrap)",
		},
		{
			name: "profile with Jamf sub-payload org but no vendor identifier",
			payload: `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>PayloadOrganization</key><string>Acme Corp</string>
  <key>PayloadIdentifier</key><string>com.acme.mdm.restrictions</string>
  <key>PayloadContent</key><array>
    <dict>
      <key>PayloadOrganization</key><string>Jamf</string>
      <key>PayloadIdentifier</key><string>com.apple.applicationaccess.acme</string>
    </dict>
  </array>
</dict></plist>`,
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := ShouldSkipProfile(tt.payload)
			if skip != tt.wantSkip {
				t.Errorf("ShouldSkipProfile() skip = %v, want %v", skip, tt.wantSkip)
			}
			if tt.wantSkip && reason != tt.wantReason {
				t.Errorf("ShouldSkipProfile() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
