// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// One App Installer deployment, read back with the Required app_title_name as
// null (what the provider's list resource actually returns) and named
// something other than its catalogue title — the case where taking the
// deployment name as the title is visibly wrong rather than harmlessly right.
const aiGenerated = `resource "jamfplatform_pro_app_installer" "all_0" {
  name           = "Chrome - IT dept"
  app_title_name = null
  enabled        = true
}

import {
  to = jamfplatform_pro_app_installer.all_0
  identity = {
    id = "deploy-1"
  }
}
`

// The event stream is the only place the deployment's computed app_title_id
// survives, since -generate-config-out drops computed attributes.
const aiEventWithTitleID = `{"type":"list_resource_found","list_resource_found":{"resource_type":"jamfplatform_pro_app_installer","identity":{"id":"deploy-1"},"resource_object":{"app_title_id":"title-42","name":"Chrome - IT dept"}}}`

// The same deployment with no name to fall back on either.
const aiEventNameless = `{"type":"list_resource_found","list_resource_found":{"resource_type":"jamfplatform_pro_app_installer","identity":{"id":"deploy-1"},"resource_object":{"app_title_id":"title-99"}}}`

type stubAppTitleLister struct {
	titles []sdkpro.AppTitle
	err    error
	calls  int
}

func (s *stubAppTitleLister) ListAppInstallerTitlesV1(_ context.Context, _ []string, _ string) ([]sdkpro.AppTitle, error) {
	s.calls++
	return s.titles, s.err
}

func writeAIFixture(t *testing.T, generated string, eventLines ...string) (generatedFile, eventsFile string) {
	t.Helper()
	dir := t.TempDir()
	generatedFile = filepath.Join(dir, "generated.tf")
	if err := os.WriteFile(generatedFile, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	eventsFile = filepath.Join(dir, "query-events.jsonl")
	if err := os.WriteFile(eventsFile, []byte(strings.Join(eventLines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return generatedFile, eventsFile
}

// appTitleName reads the attribute back through hclwrite so the assertion is
// about the parsed value, not about how the bytes happen to be spaced.
func appTitleName(t *testing.T, generatedFile string) string {
	t.Helper()
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatal(err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("re-parsing generated file: %s", diags.Error())
	}
	for _, b := range f.Body().Blocks() {
		if b.Type() != "resource" || len(b.Labels()) < 2 || b.Labels()[0] != tAppInstaller {
			continue
		}
		attr := b.Body().GetAttribute("app_title_name")
		if attr == nil {
			return ""
		}
		return strings.TrimSpace(string(attr.Expr().BuildTokens(nil).Bytes()))
	}
	t.Fatalf("no %s block in %s", tAppInstaller, generatedFile)
	return ""
}

func TestHydrateAppInstallerTitlesFromCatalogue(t *testing.T) {
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, aiEventWithTitleID)
	c := &stubAppTitleLister{titles: []sdkpro.AppTitle{
		{ID: "title-42", TitleName: "Google Chrome"},
		{ID: "title-7", TitleName: "Slack"},
	}}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("HydrateAppInstallerTitles: %v", err)
	}
	if filled != 1 || guessed != 0 || unresolved != 0 {
		t.Fatalf("expected filled=1 guessed=0 unresolved=0, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != `"Google Chrome"` {
		t.Errorf("expected the catalogue title, got %s", got)
	}
}

// A provider that starts returning app_title_name itself is authoritative: it
// counts as filled, not as a guess.
func TestHydrateAppInstallerTitlesFromEventAppTitleName(t *testing.T) {
	event := `{"type":"list_resource_found","list_resource_found":{"resource_type":"jamfplatform_pro_app_installer","identity":{"id":"deploy-1"},"resource_object":{"app_title_name":"Google Chrome","name":"Chrome - IT dept"}}}`
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, event)

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), nil, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("HydrateAppInstallerTitles: %v", err)
	}
	if filled != 1 || guessed != 0 || unresolved != 0 {
		t.Fatalf("expected filled=1 guessed=0 unresolved=0, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != `"Google Chrome"` {
		t.Errorf("expected the event's app_title_name, got %s", got)
	}
}

// The title id is recoverable but the catalogue has no entry for it (withdrawn
// title): the deployment name is written, and it is counted as a guess so the
// caller can say so.
func TestHydrateAppInstallerTitlesCatalogueMissIsGuessed(t *testing.T) {
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, aiEventWithTitleID)
	c := &stubAppTitleLister{titles: []sdkpro.AppTitle{{ID: "title-7", TitleName: "Slack"}}}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("HydrateAppInstallerTitles: %v", err)
	}
	if filled != 0 || guessed != 1 || unresolved != 0 {
		t.Fatalf("expected filled=0 guessed=1 unresolved=0, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != `"Chrome - IT dept"` {
		t.Errorf("expected the deployment name as the fallback, got %s", got)
	}
}

// A catalogue read that fails must not fail the export — a null Required
// attribute takes the whole plan down, so the guess is the lesser evil — but
// it must land in guessed, never in filled.
func TestHydrateAppInstallerTitlesCatalogueErrorStillFallsBack(t *testing.T) {
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, aiEventWithTitleID)
	c := &stubAppTitleLister{err: os.ErrPermission}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("a catalogue read failure should not fail hydration: %v", err)
	}
	if c.calls != 1 {
		t.Errorf("expected the catalogue to be read once, got %d calls", c.calls)
	}
	if filled != 0 || guessed != 1 || unresolved != 0 {
		t.Fatalf("expected filled=0 guessed=1 unresolved=0, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != `"Chrome - IT dept"` {
		t.Errorf("expected the deployment name as the fallback, got %s", got)
	}
}

// Nothing to write: the null stays for the validation pass to report rather
// than being replaced with something invented.
func TestHydrateAppInstallerTitlesUnresolved(t *testing.T) {
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, aiEventNameless)
	c := &stubAppTitleLister{titles: []sdkpro.AppTitle{{ID: "title-42", TitleName: "Google Chrome"}}}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("HydrateAppInstallerTitles: %v", err)
	}
	if filled != 0 || guessed != 0 || unresolved != 1 {
		t.Fatalf("expected filled=0 guessed=0 unresolved=1, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != "null" {
		t.Errorf("expected the null left in place, got %s", got)
	}
}

// A block the provider did answer for is not touched, and does not inflate any
// of the counters.
func TestHydrateAppInstallerTitlesLeavesPopulatedBlocks(t *testing.T) {
	populated := strings.Replace(aiGenerated, "app_title_name = null", `app_title_name = "Firefox"`, 1)
	generatedFile, eventsFile := writeAIFixture(t, populated, aiEventWithTitleID)
	c := &stubAppTitleLister{titles: []sdkpro.AppTitle{{ID: "title-42", TitleName: "Google Chrome"}}}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("HydrateAppInstallerTitles: %v", err)
	}
	if filled != 0 || guessed != 0 || unresolved != 0 {
		t.Fatalf("expected every counter at 0, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if c.calls != 0 {
		t.Errorf("expected no catalogue read when no block needs a title, got %d calls", c.calls)
	}
	if got := appTitleName(t, generatedFile); got != `"Firefox"` {
		t.Errorf("expected the existing value untouched, got %s", got)
	}
}

// A run with no event log (cached or resumed) loses the app_title_id, so the
// contract is a clean no-op with the block counted unresolved — not an error.
func TestHydrateAppInstallerTitlesMissingEventsFile(t *testing.T) {
	generatedFile, eventsFile := writeAIFixture(t, aiGenerated, aiEventWithTitleID)
	if err := os.Remove(eventsFile); err != nil {
		t.Fatal(err)
	}
	c := &stubAppTitleLister{titles: []sdkpro.AppTitle{{ID: "title-42", TitleName: "Google Chrome"}}}

	filled, guessed, unresolved, err := HydrateAppInstallerTitles(context.Background(), c, generatedFile, eventsFile)
	if err != nil {
		t.Fatalf("a missing events file should not be an error: %v", err)
	}
	if filled != 0 || guessed != 0 || unresolved != 1 {
		t.Fatalf("expected filled=0 guessed=0 unresolved=1, got filled=%d guessed=%d unresolved=%d", filled, guessed, unresolved)
	}
	if got := appTitleName(t, generatedFile); got != "null" {
		t.Errorf("expected the null left in place, got %s", got)
	}
}

// importTargetLabel is the join between a resource block and its deployment
// id, so a `to` shape it cannot parse silently loses the hydration for that
// block. These are the shapes the pipeline and multi-env assembly produce.
func TestImportTargetLabel(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "plain address",
			src:  "import {\n  to = jamfplatform_pro_app_installer.chrome\n  identity = { id = \"d1\" }\n}\n",
			want: "chrome",
		},
		{
			name: "module-prefixed address",
			src:  "import {\n  to = module.jamf.jamfplatform_pro_app_installer.chrome\n  identity = { id = \"d1\" }\n}\n",
			want: "chrome",
		},
		{
			name: "label with digits and underscores",
			src:  "import {\n  to = jamfplatform_pro_app_installer.all_0\n  identity = { id = \"d1\" }\n}\n",
			want: "all_0",
		},
		{
			name: "different resource type",
			src:  "import {\n  to = jamfplatform_pro_policy.chrome\n  identity = { id = \"d1\" }\n}\n",
			want: "",
		},
		{
			name: "type whose name only starts the same",
			src:  "import {\n  to = jamfplatform_pro_app_installer_deployment.chrome\n  identity = { id = \"d1\" }\n}\n",
			want: "",
		},
		{
			name: "no to attribute",
			src:  "import {\n  identity = { id = \"d1\" }\n}\n",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, diags := hclwrite.ParseConfig([]byte(tc.src), "import.tf", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatalf("parsing fixture: %s", diags.Error())
			}
			got := importTargetLabel(f.Body().Blocks()[0], tAppInstaller)
			if got != tc.want {
				t.Errorf("importTargetLabel = %q, want %q", got, tc.want)
			}
		})
	}
}
