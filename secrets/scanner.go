// Copyright 2026, Jamf Software LLC

package secrets

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

// Finding wraps a gitleaks finding with jamformer-specific metadata.
type Finding struct {
	report.Finding

	// ResourceAddress is the Terraform resource address (e.g. "jamfpro_webhook.slack_alerts")
	ResourceAddress string

	// AttrName is the HCL attribute that contains the secret (e.g. "authentication_password")
	AttrName string

	// InSupportFiles is true when the finding is inside support_files/
	InSupportFiles bool

	// SupportFileRef is the .tf file attribute that references this support file
	// via file() (e.g. "support_files/app_configurations/Jamf_Trust.xml")
	SupportFileRef string
}

// Scan walks the output directory and returns secret findings enriched with
// resource address and attribute information. When quiet is true, gitleaks
// warnings (e.g. "skipping file: too large") are suppressed.
func Scan(dir string, quiet bool) ([]Finding, error) {
	// Suppress gitleaks zerolog warnings in quiet mode (they go to stderr)
	prevLevel := zerolog.GlobalLevel()
	if quiet {
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}
	defer zerolog.SetGlobalLevel(prevLevel)

	detector, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, err
	}

	// Skip files that are expected to contain user-provided secrets
	detector.Config.Allowlists = append(detector.Config.Allowlists, &config.Allowlist{
		Description: "Jamformer: skip credential files managed by the user",
		Paths: []*regexp.Regexp{
			regexp.MustCompile(`terraform\.tfvars$`),
			regexp.MustCompile(`variables\.tf$`),
			regexp.MustCompile(`provider\.tf$`),
		},
	})

	addJamfRules(&detector.Config)

	rawFindings, err := detector.DetectSource(
		context.Background(),
		&sources.Files{
			Config:         &detector.Config,
			FollowSymlinks: false,
			MaxFileSize:    100_000_000,
			Path:           dir,
			Sema:           detector.Sema,
		},
	)
	if err != nil {
		return nil, err
	}

	findings := make([]Finding, 0, len(rawFindings))
	for _, rf := range rawFindings {
		f := Finding{Finding: rf}
		f.InSupportFiles = strings.Contains(rf.File, "support_files")

		if strings.HasSuffix(rf.File, ".tf") {
			f.ResourceAddress, f.AttrName = enrichFromTF(rf.File, rf.StartLine)
		}

		if f.InSupportFiles {
			// Compute relative path from output dir for display
			if rel, err := filepath.Rel(dir, rf.File); err == nil {
				f.SupportFileRef = rel
			}
		}

		findings = append(findings, f)
	}

	return findings, nil
}

// enrichFromTF reads a .tf file and determines which resource block and
// attribute contains the finding at the given line number.
func enrichFromTF(filePath string, line int) (resourceAddr, attrName string) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)

	var currentResource string
	lineNum := 0

	resourceRe := regexp.MustCompile(`^resource\s+"([^"]+)"\s+"([^"]+)"`)
	attrRe := regexp.MustCompile(`^\s+(\w+)\s*=\s*`)

	for scanner.Scan() {
		lineNum++
		text := scanner.Text()

		// Track which resource block we're in
		if m := resourceRe.FindStringSubmatch(text); m != nil {
			currentResource = m[1] + "." + m[2]
		}

		// When we reach the finding line, extract the attribute name
		if lineNum == line {
			resourceAddr = currentResource
			if m := attrRe.FindStringSubmatch(text); m != nil {
				attrName = m[1]
			}
			return
		}
	}

	return "", ""
}

// addJamfRules supplements the default gitleaks config with rules for secrets
// commonly found in Jamf-managed resources.
func addJamfRules(cfg *config.Config) {
	jamfRules := []config.Rule{
		{
			RuleID:      "jamf-hcl-password",
			Description: "Password value in HCL attribute",
			Regex:       regexp.MustCompile(`(?i)(?:password|secret|credential|passphrase|auth_token)\s*=\s*"([^"]{8,})"`),
			SecretGroup: 1,
			Keywords:    []string{"password", "secret", "credential", "passphrase", "auth_token"},
		},
		{
			RuleID:      "jamf-plist-password",
			Description: "Password or secret in plist/XML configuration",
			Regex:       regexp.MustCompile(`(?is)<key>(?:password|secret|api_key|apikey|auth_key|token|credential)</key>\s*<string>([^<]{4,})</string>`),
			SecretGroup: 1,
			Keywords:    []string{"password", "secret", "api_key", "apikey", "auth_key", "token", "credential"},
		},
		{
			RuleID:      "jamf-ldap-bind-password",
			Description: "LDAP bind password",
			Regex:       regexp.MustCompile(`(?i)(?:bind_password|ldap_password|directory_password)\s*=\s*"([^"]{4,})"`),
			SecretGroup: 1,
			Keywords:    []string{"bind_password", "ldap_password", "directory_password"},
		},
		{
			RuleID:      "jamf-smtp-password",
			Description: "SMTP server password",
			Regex:       regexp.MustCompile(`(?i)smtp_password\s*=\s*"([^"]{4,})"`),
			SecretGroup: 1,
			Keywords:    []string{"smtp_password"},
		},
		{
			RuleID:      "jamf-wifi-password",
			Description: "WiFi or VPN password in configuration profile",
			Regex:       regexp.MustCompile(`(?i)<key>(?:wifi_password|shared_secret|vpn_password|preshared_key)</key>\s*(?:\\n)?<string>([^<]{4,})</string>`),
			SecretGroup: 1,
			Keywords:    []string{"wifi_password", "shared_secret", "vpn_password", "preshared_key"},
		},
		{
			// Catches vendor-specific key names (e.g. LicenseKey, BearerToken) that the
			// keyword-based jamf-plist-password rule won't match. Scoped to app_configurations/
			// to limit false positives from bundle IDs and other low-entropy config values.
			// No Keywords: gitleaks' prefilter trie is built at detector creation so post-init
			// additions have no effect; Path alone is sufficient to scope this rule.
			RuleID:      "jamf-appconfig-high-entropy",
			Description: "High-entropy secret under vendor-specific key in app configuration XML",
			Regex:       regexp.MustCompile(`(?is)<key>[^<]{1,64}</key>\s*<string>([^<]{8,})</string>`),
			SecretGroup: 1,
			Entropy:     4.0,
			Path:        regexp.MustCompile(`support_files[/\\]app_configurations`),
		},
	}

	for _, rule := range jamfRules {
		cfg.Rules[rule.RuleID] = rule
		cfg.OrderedRules = append(cfg.OrderedRules, rule.RuleID)
		for _, kw := range rule.Keywords {
			cfg.Keywords[kw] = struct{}{}
		}
	}
}
