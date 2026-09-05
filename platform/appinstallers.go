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
// Where the catalogue cannot answer — a title withdrawn from the catalogue
// since the deployment was created — the deployment's own name is used as a
// fallback: Jamf Pro defaults it to the title name, so it is usually right and
// is always better than a null the provider will reject.
//
// Returns the number of blocks filled in. A missing events file or an
// unreachable catalogue is not an error: it leaves the nulls in place for the
// validation pass to report.
func HydrateAppInstallerTitles(ctx context.Context, c appTitleLister, generatedFile, eventsFile string) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated file: %s", diags.Error())
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
		return 0, nil
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

	titleIDByDeployment, nameByDeployment, err := appInstallerTitlesFromEvents(eventsFile)
	if err != nil {
		return 0, err
	}

	catalogue := map[string]string{}
	if c != nil {
		titles, listErr := c.ListAppInstallerTitlesV1(ctx, nil, "")
		if listErr != nil {
			// The catalogue is an optimisation over the fallback, not a
			// requirement. Carry on with the deployment names.
			if !Quiet {
				fmt.Printf("  Warning: could not read the App Installers catalogue: %v\n", listErr)
			}
		}
		for _, t := range titles {
			if t.ID != "" && t.TitleName != "" {
				catalogue[t.ID] = t.TitleName
			}
		}
	}

	filled := 0
	for label, body := range needed {
		deploymentID := labelToID[label]
		name := ""
		if titleID := titleIDByDeployment[deploymentID]; titleID != "" {
			name = catalogue[titleID]
		}
		if name == "" {
			name = nameByDeployment[deploymentID]
		}
		if name == "" {
			continue
		}
		body.SetAttributeValue("app_title_name", cty.StringVal(name))
		filled++
	}
	if filled == 0 {
		return 0, nil
	}

	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return filled, nil
}

// appInstallerTitlesFromEvents reads the app_title_id and name each App
// Installer deployment reported, keyed by deployment id.
func appInstallerTitlesFromEvents(eventsFile string) (titleIDs, names map[string]string, err error) {
	titleIDs, names = map[string]string{}, map[string]string{}
	f, err := os.Open(eventsFile)
	if err != nil {
		// No event log (a cached or resumed run): the caller falls back to
		// whatever the catalogue and the config already carry.
		return titleIDs, names, nil
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
		// If the provider ever starts returning the name, prefer it.
		if obj.AppTitleName != "" {
			names[id] = obj.AppTitleName
		} else if obj.Name != "" {
			names[id] = obj.Name
		}
		if obj.AppTitleID != "" {
			titleIDs[id] = obj.AppTitleID
		}
	}
	return titleIDs, names, scanner.Err()
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
