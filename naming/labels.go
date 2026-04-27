// Copyright 2026, Jamf Software LLC

package naming

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9_]`)
	multiUnderscore = regexp.MustCompile(`_{2,}`)
	leadingDigit    = regexp.MustCompile(`^[0-9]`)
)

// Tracker keeps track of used labels per resource type to avoid collisions.
// Not thread-safe — callers must synchronise access if used concurrently.
type Tracker struct {
	used map[string]map[string]bool // map[resourceType]map[label]true
}

func NewTracker() *Tracker {
	return &Tracker{
		used: make(map[string]map[string]bool),
	}
}

// Label converts a Jamf object name into a valid Terraform resource label
// and ensures uniqueness within the given resource type.
func (t *Tracker) Label(resourceType, name string) string {
	label := Sanitize(name)

	if t.used[resourceType] == nil {
		t.used[resourceType] = make(map[string]bool)
	}

	if t.used[resourceType][label] {
		base := label
		counter := 2
		for t.used[resourceType][fmt.Sprintf("%s_%d", base, counter)] {
			counter++
		}
		label = fmt.Sprintf("%s_%d", base, counter)
	}
	t.used[resourceType][label] = true

	return label
}

// scriptExtensions are common script file extensions to strip from labels.
var scriptExtensions = []string{
	".sh", ".bash", ".zsh",
	".py", ".py3",
	".rb",
	".pl", ".pm",
	".swift",
	".js",
	".ps1",
	".exp",
}

// StripScriptExtension removes a trailing script file extension from a name.
func StripScriptExtension(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range scriptExtensions {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

// Sanitize converts a human-readable name into a valid Terraform resource label.
func Sanitize(name string) string {
	// Lowercase
	label := strings.ToLower(name)

	// Replace common separators with underscore
	label = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, label)

	// Remove any remaining non-alphanumeric/underscore chars
	label = nonAlphanumeric.ReplaceAllString(label, "_")

	// Collapse multiple underscores
	label = multiUnderscore.ReplaceAllString(label, "_")

	// Trim leading/trailing underscores
	label = strings.Trim(label, "_")

	// Terraform labels can't start with a digit
	if leadingDigit.MatchString(label) {
		label = "_" + label
	}

	// Fallback for empty labels
	if label == "" {
		label = "unnamed"
	}

	return label
}
