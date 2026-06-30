// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// vendorPayloadIDPrefixes lists PayloadIdentifier prefixes that identify
// vendor-managed profiles. These profiles contain short-lived certificates
// and service tokens bound to a specific Jamf tenant; re-applying stale
// exported versions would break the originating product's functionality.
//
// Detection uses PayloadIdentifier (not PayloadOrganization) because:
//   - Jamf Protect uses com.jamf.protect.* in its programmatically-created profiles.
//   - Jamf Security Cloud / Jamf Trust use com.jamf.trust.* in their signed profiles
//     but carry the customer's org name at the top level, so org-name matching
//     would give false negatives.
var vendorPayloadIDPrefixes = []string{
	"com.jamf.protect.",
	"com.jamf.trust.",
}

// rePayloadID matches any PayloadIdentifier value in an Apple plist XML.
var rePayloadID = regexp.MustCompile(`(?i)<key>PayloadIdentifier</key>\s*<string>([^<]+)</string>`)

// ShouldSkipProfile reports whether a configuration profile should be excluded
// from the Terraform output. Returns (true, reason) for:
//   - Jamf Protect profiles: any PayloadIdentifier starts with com.jamf.protect.*
//   - Jamf Security Cloud / Jamf Trust profiles: any PayloadIdentifier starts with com.jamf.trust.*
//   - Non-XML payloads: signed binary profiles stored as base64-encoded DER.
func ShouldSkipProfile(payload string) (bool, string) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return false, ""
	}
	if !strings.HasPrefix(trimmed, "<?xml") && !strings.HasPrefix(trimmed, "<plist") {
		return true, "signed profile (non-XML payload)"
	}
	for _, m := range rePayloadID.FindAllStringSubmatch(trimmed, -1) {
		id := strings.ToLower(strings.TrimSpace(m[1]))
		for _, prefix := range vendorPayloadIDPrefixes {
			if strings.HasPrefix(id, prefix) {
				return true, "vendor-managed profile (PayloadIdentifier: " + m[1] + ")"
			}
		}
	}
	return false, ""
}

// extractScriptContents pulls the script_contents attribute out of a resource
// block, writes it to a file in the given directory, and replaces the attribute
// with a file() function call. The relDir is the relative path from the output
// directory to the scripts directory (e.g. "support_files/scripts").
//
// Thin wrapper over extractStringAttr (the shared implementation).
func extractScriptContents(body *hclwrite.Body, scriptsDir, relDir string, fileNames map[string]int) error {
	_, err := extractStringAttr(body, ExtractSpec{
		AttrName: "script_contents",
		FileKind: FileKindScript,
	}, scriptsDir, relDir, fileNames)
	return err
}

// extractProfilePayloads pulls the payloads attribute out of a macOS configuration
// profile resource block, writes it to a .mobileconfig file in
// support_files/macos_configuration_profiles/ (or mobile_device_configuration_profiles/
// for mobile device profiles), and replaces the attribute with a file() function call.
// subDir must be the directory name under support_files (e.g. "macos_configuration_profiles").
func extractProfilePayloads(body *hclwrite.Body, profilesDir, relBase, subDir string, fileNames map[string]int) error {
	_, err := extractStringAttr(body, ExtractSpec{
		AttrName: "payloads",
		FileKind: FileKindMobileconfig,
	}, profilesDir, filepath.Join(relBase, subDir), fileNames)
	return err
}

// extractAppConfiguration pulls the preferences attribute out of the
// app_configuration block in a mobile_device_application resource, writes it to
// an XML file in support_files/app_configurations/, and replaces the attribute
// with a file() function call.
func extractAppConfiguration(body *hclwrite.Body, configsDir, relBase string, fileNames map[string]int) error {
	_, err := extractStringAttr(body, ExtractSpec{
		BlockPath: []string{"app_configuration"},
		AttrName:  "preferences",
		FileKind:  FileKindXML,
	}, configsDir, filepath.Join(relBase, "app_configurations"), fileNames)
	return err
}

// extractTokenToFile extracts a token attribute (e.g. encoded_token, service_token)
// from a resource block, writes it to a file in the given directory, and replaces
// the attribute with a file() function call. If the token value is empty/null,
// a placeholder file() reference is created for the user to fill in.
// TokenVar describes a variable generated for a token file path.
type TokenVar struct {
	VarName     string // e.g. "dep_token_path_my_enrollment"
	Description string // e.g. "Path to DEP token file (.p7m) for 'My Enrollment'"
}

// extractTokenVar replaces a token attribute with file(var.xxx) and returns
// the variable definition. The token directories are kept as the recommended
// location for users to place their files downloaded from Apple Business/School
// Manager. Returns nil if the resource has no name attribute.
func extractTokenVar(body *hclwrite.Body, attrName, varPrefix, label, ext string) *TokenVar {
	nameAttr := body.GetAttribute("name")
	if nameAttr == nil {
		return nil
	}

	resourceName := ExtractStringValue(nameAttr)
	if resourceName == "" {
		return nil
	}

	varName := varPrefix + label

	// Replace attribute with file(var.xxx)
	fileRef := fmt.Sprintf("file(var.%s)", varName)
	body.SetAttributeRaw(attrName, hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
	})

	return &TokenVar{
		VarName:     varName,
		Description: fmt.Sprintf("Path to token file (%s) for %q", ext, resourceName),
	}
}
