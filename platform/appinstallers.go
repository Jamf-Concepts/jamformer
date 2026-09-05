// Copyright 2026, Jamf Software LLC

package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// appTitleLister is the slice of the federated pro SDK client this file needs,
// kept narrow so tests can supply a fake.
type appTitleLister interface {
	ListAppInstallerTitlesV1(ctx context.Context, sort []string, filter string) ([]sdkpro.AppTitle, error)
}

// aiListEvent decodes the terraform query "list_resource_found" event for an
// App Installer deployment. app_title_id is computed, so
// `-generate-config-out` drops it and the event stream is the only place the
// generated config's own title reference survives.
type aiListEvent struct {
	Type              string `json:"type"`
	ListResourceFound struct {
		ResourceType string `json:"resource_type"`
		Identity     struct {
			ID string `json:"id"`
		} `json:"identity"`
		ResourceObject struct {
			AppTitleID   string `json:"app_title_id"`
			AppTitleName string `json:"app_title_name"`
			Name         string `json:"name"`
		} `json:"resource_object"`
	} `json:"list_resource_found"`
}

// HydrateAppInstallerTitles fills in app_title_name on every
// jamfplatform_pro_app_installer block that the provider read back as null.
//
// The provider marks app_title_name Required but its list resource returns
// null for it, so the generated block cannot plan — `terraform query` exits
// non-zero and, without this, an App Installer on the tenant would fail the
// whole export. The deployment's app_title_id is in the query event stream, so
// the name is resolved from the App Installers catalogue, which is the same
// place the provider should be resolving it.
//
// Where that chain breaks — the catalogue read fails, the title has been
// withdrawn from the catalogue since the deployment was created, or the
// provider did not surface the computed app_title_id in the event payload —
// the deployment's own name is used as a fallback: Jamf Pro defaults a
// deployment's name to the title name, so it is usually right and is always
// better than a null the provider rejects outright.
//
// "Usually right" is not "right", though, and a wrong app_title_name names a
// different app than the object the block imports. So the three outcomes are
// counted separately rather than summed: filled is a title the catalogue (or
// an app_title_name the provider actually returned) answered for, guessed is a
// title taken from the deployment name, and unresolved counts blocks that
// needed a title and got none — those keep their null for the validation pass
// to report. Callers are expected to surface guessed and unresolved rather
// than presenting the total as resolved.
//
// A missing events file is not an error, and neither is an unreachable
// catalogue: both degrade into guessed/unresolved counts.
func HydrateAppInstallerTitles(ctx context.Context, c appTitleLister, generatedFile, eventsFile string) (filled, guessed, unresolved int, err error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, 0, 0, fmt.Errorf("parsing generated file: %s", diags.Error())
	}

	// Which blocks actually need a name, keyed by import identity.
	needed := map[string]*hclwrite.Body{}
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != tAppInstaller {
			continue
		}
		attr := block.Body().GetAttribute("app_title_name")
		if attr == nil || !isNullExpr(attr) {
			continue
		}
		needed[labels[1]] = block.Body()
	}
	if len(needed) == 0 {
		return 0, 0, 0, nil
	}

	// The generated label is what the import block ties to an identity, and at
	// this point in the pipeline labels have not been renamed yet, so the
	// import blocks in the same file give label → deployment id.
	labelToID := map[string]string{}
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		if id := importIdentityID(block); id != "" {
			if to := importTargetLabel(block, tAppInstaller); to != "" {
				labelToID[to] = id
			}
		}
	}

	titleIDByDeployment, titleNameByDeployment, nameByDeployment, err := appInstallerTitlesFromEvents(eventsFile)
	if err != nil {
		return 0, 0, 0, err
	}

	catalogue := map[string]string{}
	if c != nil {
		titles, listErr := c.ListAppInstallerTitlesV1(ctx, nil, "")
		if listErr != nil {
			// The catalogue is the only authoritative source of a title name,
			// so losing it is not progress chatter to hide behind -verbose: it
			// changes what lands in the export, every block falling through to
			// the deployment-name guess. Report it whatever the verbosity.
			fmt.Printf("  Warning: could not read the App Installers catalogue: %v\n", listErr)
			fmt.Printf("           App Installer titles fall back to each deployment's own name, which may not be the title name\n")
		}
		for _, t := range titles {
			if t.ID != "" && t.TitleName != "" {
				catalogue[t.ID] = t.TitleName
			}
		}
	}

	for label, body := range needed {
		deploymentID := labelToID[label]

		// Authoritative, in order: an app_title_name the provider itself
		// returned in the event payload (it does not today, but if that is
		// ever fixed it beats a second lookup), then the catalogue entry for
		// the deployment's computed app_title_id.
		name := titleNameByDeployment[deploymentID]
		authoritative := name != ""
		if !authoritative {
			if titleID := titleIDByDeployment[deploymentID]; titleID != "" {
				if t := catalogue[titleID]; t != "" {
					name, authoritative = t, true
				}
			}
		}
		if name == "" {
			name = nameByDeployment[deploymentID]
		}
		if name == "" {
			// Nothing to write. Leaving the null in place is deliberate: the
			// validation pass reports a Required null, whereas an invented
			// value would import silently under the wrong app's name.
			unresolved++
			continue
		}
		body.SetAttributeValue("app_title_name", cty.StringVal(name))
		if authoritative {
			filled++
		} else {
			guessed++
		}
	}

	// A guessed-only run still has to write: the guess is what makes the block
	// plannable at all.
	if filled+guessed == 0 {
		return filled, guessed, unresolved, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, 0, 0, fmt.Errorf("writing generated file: %w", err)
	}
	return filled, guessed, unresolved, nil
}

// appInstallerTitlesFromEvents reads what each App Installer deployment
// reported, keyed by deployment id: its computed app_title_id, any
// app_title_name the provider actually returned, and the deployment's own
// name. The last two are kept apart because a deployment name is only a guess
// at the title name, and the caller counts the two outcomes separately.
func appInstallerTitlesFromEvents(eventsFile string) (titleIDs, titleNames, names map[string]string, err error) {
	titleIDs, titleNames, names = map[string]string{}, map[string]string{}, map[string]string{}
	f, err := os.Open(eventsFile)
	if err != nil {
		// No event log (a cached or resumed run): the caller falls back to
		// whatever the catalogue and the config already carry.
		return titleIDs, titleNames, names, nil
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var ev aiListEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Type != "list_resource_found" || ev.ListResourceFound.ResourceType != tAppInstaller {
			continue
		}
		id := ev.ListResourceFound.Identity.ID
		if id == "" {
			continue
		}
		obj := ev.ListResourceFound.ResourceObject
		if obj.AppTitleName != "" {
			titleNames[id] = obj.AppTitleName
		}
		if obj.Name != "" {
			names[id] = obj.Name
		}
		if obj.AppTitleID != "" {
			titleIDs[id] = obj.AppTitleID
		}
	}
	return titleIDs, titleNames, names, scanner.Err()
}

// importTargetLabel returns the label an import block targets, when it targets
// the given resource type.
func importTargetLabel(block *hclwrite.Block, resourceType string) string {
	attr := block.Body().GetAttribute("to")
	if attr == nil {
		return ""
	}
	expr := string(attr.Expr().BuildTokens(nil).Bytes())
	prefix := resourceType + "."
	for i := 0; i+len(prefix) <= len(expr); i++ {
		if expr[i:i+len(prefix)] == prefix {
			label := expr[i+len(prefix):]
			// Trim trailing whitespace / newline the token stream carries.
			for len(label) > 0 && (label[len(label)-1] == '\n' || label[len(label)-1] == ' ' || label[len(label)-1] == '\r' || label[len(label)-1] == '\t') {
				label = label[:len(label)-1]
			}
			return label
		}
	}
	return ""
}

// isNullExpr reports whether an attribute is the bare literal null.
func isNullExpr(attr *hclwrite.Attribute) bool {
	expr := string(attr.Expr().BuildTokens(nil).Bytes())
	trimmed := ""
	for _, r := range expr {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			trimmed += string(r)
		}
	}
	return trimmed == "null"
}
