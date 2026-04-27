// Copyright 2026, Jamf Software LLC

package secrets

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ReportOptions controls the formatting of the secret scan report.
type ReportOptions struct {
	Bold   string
	Dim    string
	Yellow string
	Reset  string
}

// PrintReport prints a human-readable summary of secret findings grouped by file,
// showing the resource address and attribute that contains each secret.
func PrintReport(findings []Finding, opts ReportOptions) {
	if len(findings) == 0 {
		return
	}

	// Split into .tf findings and support_files findings
	var tfFindings, sfFindings []Finding
	for _, f := range findings {
		if f.InSupportFiles {
			sfFindings = append(sfFindings, f)
		} else {
			tfFindings = append(tfFindings, f)
		}
	}

	fmt.Printf("\n%s%s!%s  Secret scan found %s%d%s potential secret(s):\n",
		opts.Yellow, opts.Bold, opts.Reset,
		opts.Bold, len(findings), opts.Reset)

	// Report .tf file findings grouped by resource
	if len(tfFindings) > 0 {
		fmt.Printf("\n  %sIn Terraform resources:%s\n\n", opts.Bold, opts.Reset)
		printTFFindings(tfFindings, opts)
	}

	// Report support_files findings
	if len(sfFindings) > 0 {
		fmt.Printf("\n  %sIn support files:%s\n\n", opts.Bold, opts.Reset)
		printSupportFileFindings(sfFindings, opts)
	}

	fmt.Printf("\n  %sReview these findings before committing to version control.%s\n\n", opts.Yellow, opts.Reset)
}

// printTFFindings prints findings from .tf files grouped by resource address.
func printTFFindings(findings []Finding, opts ReportOptions) {
	// Group by resource address
	type resourceGroup struct {
		addr     string
		file     string
		findings []Finding
	}

	byResource := make(map[string]*resourceGroup)
	var order []string

	for _, f := range findings {
		key := f.ResourceAddress
		if key == "" {
			key = f.File
		}
		if _, exists := byResource[key]; !exists {
			byResource[key] = &resourceGroup{addr: f.ResourceAddress, file: f.File}
			order = append(order, key)
		}
		byResource[key].findings = append(byResource[key].findings, f)
	}

	for _, key := range order {
		rg := byResource[key]
		displayFile := filepath.Base(rg.file)

		if rg.addr != "" {
			fmt.Printf("    %s%s%s  %s(%s)%s\n", opts.Bold, rg.addr, opts.Reset, opts.Dim, displayFile, opts.Reset)
		} else {
			fmt.Printf("    %s%s%s\n", opts.Bold, displayFile, opts.Reset)
		}

		for _, f := range rg.findings {
			attr := f.AttrName
			if attr == "" {
				attr = fmt.Sprintf("line %d", f.StartLine)
			}

			fmt.Printf("      %s%-30s%s %s  %s(%s)%s\n",
				opts.Bold, attr, opts.Reset,
				redact(f.Secret),
				opts.Dim, f.Description, opts.Reset)
		}
		fmt.Println()
	}
}

// printSupportFileFindings prints findings from support_files/.
func printSupportFileFindings(findings []Finding, opts ReportOptions) {
	// Group by file
	byFile := make(map[string][]Finding)
	var files []string
	for _, f := range findings {
		path := f.SupportFileRef
		if path == "" {
			path = filepath.Base(f.File)
		}
		if _, exists := byFile[path]; !exists {
			files = append(files, path)
		}
		byFile[path] = append(byFile[path], f)
	}
	sort.Strings(files)

	for _, file := range files {
		fmt.Printf("    %s%s%s\n", opts.Bold, file, opts.Reset)
		for _, f := range byFile[file] {
			fmt.Printf("      Line %-4d %s  %s(%s)%s\n",
				f.StartLine, redact(f.Secret),
				opts.Dim, f.Description, opts.Reset)
		}
		fmt.Println()
	}
}

// redact masks the middle of a secret string for display.
func redact(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}
