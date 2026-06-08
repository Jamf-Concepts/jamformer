// Copyright 2026, Jamf Software LLC

package postprocess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// extractScriptContents pulls the script_contents attribute out of a resource
// block, writes it to a file in the given directory, and replaces the attribute
// with a file() function call. The relDir is the relative path from the output
// directory to the scripts directory (e.g. "support_files/scripts").
func extractScriptContents(body *hclwrite.Body, scriptsDir, relDir string, fileNames map[string]int) error {
	contentsAttr := body.GetAttribute("script_contents")
	if contentsAttr == nil {
		return nil
	}

	// Get the script name from the "name" attribute to use as filename
	nameAttr := body.GetAttribute("name")
	if nameAttr == nil {
		return nil
	}

	scriptName := ExtractStringValue(nameAttr)
	if scriptName == "" {
		return nil
	}

	// Extract the actual script content string
	scriptContent := extractFullStringValue(contentsAttr)
	if scriptContent == "" {
		return nil
	}

	// Determine file extension from script content
	ext := guessScriptExtension(scriptContent)

	// Build filename from script name, handling collisions
	baseFileName := sanitizeFilename(scriptName)
	if !strings.HasSuffix(strings.ToLower(baseFileName), ext) {
		baseFileName += ext
	}

	fileNames[baseFileName]++
	if fileNames[baseFileName] > 1 {
		nameWithoutExt := strings.TrimSuffix(baseFileName, ext)
		baseFileName = fmt.Sprintf("%s_%d%s", nameWithoutExt, fileNames[baseFileName], ext)
	}

	// Write script content to file
	scriptPath := filepath.Join(scriptsDir, baseFileName)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0644); err != nil {
		return fmt.Errorf("writing script file %s: %w", baseFileName, err)
	}

	// Replace script_contents with file() reference (always use forward slashes in HCL)
	relPath := filepath.ToSlash(filepath.Join(relDir, baseFileName))
	fileRef := fmt.Sprintf(`file("${path.module}/%s")`, relPath)
	body.SetAttributeRaw("script_contents", hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
	})

	return nil
}

// extractProfilePayloads pulls the payloads attribute out of a macOS configuration
// profile resource block, writes it to a .mobileconfig file in
// support_files/macos_configuration_profiles/ (or mobile_device_configuration_profiles/
// for mobile device profiles), and replaces the attribute with a file() function call.
// subDir must be the directory name under support_files (e.g. "macos_configuration_profiles").
func extractProfilePayloads(body *hclwrite.Body, profilesDir, relBase, subDir string, fileNames map[string]int) error {
	payloadsAttr := body.GetAttribute("payloads")
	if payloadsAttr == nil {
		return nil
	}

	nameAttr := body.GetAttribute("name")
	if nameAttr == nil {
		return nil
	}

	profileName := ExtractStringValue(nameAttr)
	if profileName == "" {
		return nil
	}

	payloadContent := extractFullStringValue(payloadsAttr)
	if payloadContent == "" {
		return nil
	}

	baseFileName := sanitizeFilename(profileName) + ".mobileconfig"

	fileNames[baseFileName]++
	if fileNames[baseFileName] > 1 {
		nameWithoutExt := strings.TrimSuffix(baseFileName, ".mobileconfig")
		baseFileName = fmt.Sprintf("%s_%d.mobileconfig", nameWithoutExt, fileNames[baseFileName])
	}

	profilePath := filepath.Join(profilesDir, baseFileName)
	if err := os.WriteFile(profilePath, []byte(payloadContent), 0644); err != nil {
		return fmt.Errorf("writing profile file %s: %w", baseFileName, err)
	}

	relPath := filepath.ToSlash(filepath.Join(relBase, subDir, baseFileName))
	fileRef := fmt.Sprintf(`file("${path.module}/%s")`, relPath)
	body.SetAttributeRaw("payloads", hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
	})

	return nil
}

// extractAppConfiguration pulls the preferences attribute out of the
// app_configuration block in a mobile_device_application resource, writes it to
// an XML file in support_files/app_configurations/, and replaces the attribute
// with a file() function call.
func extractAppConfiguration(body *hclwrite.Body, configsDir, relBase string, fileNames map[string]int) error {
	appConfigBlock := body.FirstMatchingBlock("app_configuration", nil)
	if appConfigBlock == nil {
		return nil
	}

	prefsAttr := appConfigBlock.Body().GetAttribute("preferences")
	if prefsAttr == nil {
		return nil
	}

	nameAttr := body.GetAttribute("name")
	if nameAttr == nil {
		return nil
	}

	appName := ExtractStringValue(nameAttr)
	if appName == "" {
		return nil
	}

	prefsContent := extractFullStringValue(prefsAttr)
	if prefsContent == "" {
		return nil
	}

	baseFileName := sanitizeFilename(appName) + ".xml"

	fileNames[baseFileName]++
	if fileNames[baseFileName] > 1 {
		nameWithoutExt := strings.TrimSuffix(baseFileName, ".xml")
		baseFileName = fmt.Sprintf("%s_%d.xml", nameWithoutExt, fileNames[baseFileName])
	}

	configPath := filepath.Join(configsDir, baseFileName)
	if err := os.WriteFile(configPath, []byte(prefsContent), 0644); err != nil {
		return fmt.Errorf("writing app configuration file %s: %w", baseFileName, err)
	}

	relPath := filepath.ToSlash(filepath.Join(relBase, "app_configurations", baseFileName))
	fileRef := fmt.Sprintf(`file("${path.module}/%s")`, relPath)
	appConfigBlock.Body().SetAttributeRaw("preferences", hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(fileRef)},
	})

	return nil
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
