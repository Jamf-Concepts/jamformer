// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	sdkpro "github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/zclconf/go-cty/cty"
)

// iconGetter is the slice of the federated pro client GenerateIcons needs,
// kept narrow so tests can supply a fake.
type iconGetter interface {
	GetIconV1(ctx context.Context, id string) (*sdkpro.IconResponse, error)
}

// iconSource describes where one resource type carries a Self Service icon id.
// Policies, Mac App Store apps and mobile device apps nest an
// id/uri/filename object; ebooks and patch policies carry a flat id attribute.
// echoes are the server-computed siblings dropped once the id is rewritten.
type iconSource struct {
	resourceType string
	hint         string // label prefix, naming the kind of referencing resource
	path         []string
	idAttr       string
	echoes       []string
}

var iconObjectPath = []string{"self_service", "self_service_icon"}

// iconSources lists every jamfplatform resource type that references an icon.
// Configuration profiles are absent: the provider's profile schemas carry no
// Self Service icon.
var iconSources = []iconSource{
	{"jamfplatform_pro_policy", "policy", iconObjectPath, "id", []string{"uri", "filename"}},
	{"jamfplatform_pro_mac_app_store_app", "mac_app", iconObjectPath, "id", []string{"uri", "filename"}},
	{"jamfplatform_pro_mobile_device_app", "mobile_app", iconObjectPath, "id", []string{"uri", "filename"}},
	{"jamfplatform_pro_ebook", "ebook", []string{"self_service"}, "icon_id", []string{"icon_uri"}},
	{"jamfplatform_pro_patch_policy", "patch_policy", []string{"user_interaction"}, "self_service_icon_id", nil},
}

// GenerateIcons synthesises jamfplatform_pro_icon resources from the
// self_service_icon references hydrated into policies. jamfplatform_pro_icon has
// no list resource, so icons are not query-discoverable; instead each policy's
// self_service_icon carries the icon's id (uri and filename are Computed-only,
// so `-generate-config-out` omits them; the uri is recovered via ig when the
// generated config lacks it). For every
// unique referenced icon this emits one jamfplatform_pro_icon resource whose
// icon_file_source is the CDN uri (the provider downloads + re-uploads it) plus
// an import block keyed by the icon id, registers the icon id so the
// self_service_icon.id reference can be rewritten to the new resource, and drops
// the server-computed uri/filename echoes from each policy's self_service_icon
// (they would otherwise drift on a fresh tenant). Returns the icon count.
//
// An icon whose uri cannot be resolved (ig is nil, or the lookup fails) is not
// emitted: icon_file_source is Required and an empty string would be turned
// into an unsettable variable. Its policy references stay as the raw icon id.
func GenerateIcons(ctx context.Context, ig iconGetter, generatedFile string, reg *registry.Registry) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	type iconInfo struct{ uri, filename, refHint, refLabel string }
	icons := make(map[string]iconInfo)
	var order []string

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		var src *iconSource
		for i := range iconSources {
			if iconSources[i].resourceType == labels[0] {
				src = &iconSources[i]
				break
			}
		}
		if src == nil {
			continue
		}
		id := postprocess.ReadObjectAttrString(block.Body(), src.path, src.idAttr)
		if id == "" {
			continue
		}
		info, seen := icons[id]
		if !seen {
			order = append(order, id)
			info.refHint, info.refLabel = src.hint, labels[1]
		}
		// uri and filename are Computed-only, so generated config rarely has them.
		if info.uri == "" && len(src.echoes) > 0 {
			info.uri = postprocess.ReadObjectAttrString(block.Body(), src.path, src.echoes[0])
		}
		if info.filename == "" && len(src.echoes) > 1 {
			info.filename = postprocess.ReadObjectAttrString(block.Body(), src.path, src.echoes[1])
		}
		icons[id] = info
		if len(src.echoes) > 0 {
			postprocess.RemoveNestedAttrs(block.Body(), src.path, src.echoes...)
		}
	}

	if len(order) == 0 {
		return 0, nil
	}

	tracker := naming.NewTracker()
	emitted := 0
	for _, id := range order {
		info := icons[id]
		if info.uri == "" && ig != nil {
			if resp, gerr := ig.GetIconV1(ctx, id); gerr == nil && resp != nil {
				info.uri = resp.URL
			}
		}
		if info.uri == "" {
			fmt.Fprintf(os.Stderr, "  Warning: could not resolve a URL for Self Service icon %s; leaving the reference as a raw id\n", id)
			continue
		}
		label := tracker.Label(tIcon, iconLabelBase(info.refHint, info.refLabel, info.filename, id))
		addr := fmt.Sprintf("%s.%s", tIcon, label)
		reg.Register(tIcon, id, addr)

		f.Body().AppendNewline()
		imp := f.Body().AppendNewBlock("import", nil)
		imp.Body().SetAttributeRaw("to", hclwrite.Tokens{{Type: hclsyntax.TokenIdent, Bytes: []byte(addr)}})
		imp.Body().SetAttributeValue("id", cty.StringVal(id))

		f.Body().AppendNewline()
		res := f.Body().AppendNewBlock("resource", []string{tIcon, label})
		res.Body().SetAttributeValue("icon_file_source", cty.StringVal(info.uri))
		lc := res.Body().AppendNewBlock("lifecycle", nil)
		lc.Body().SetAttributeRaw("ignore_changes", hclwrite.Tokens{
			{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			{Type: hclsyntax.TokenIdent, Bytes: []byte("icon_file_source")},
			{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
		})
		emitted++
	}

	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return emitted, nil
}

// iconLabelBase derives a label seed for an icon: <hint>_<label> of the first
// resource that references it (e.g. policy_install_chrome, matching the Jamf
// Pro provider), else the filename (extension stripped), else icon_<id>.
func iconLabelBase(hint, refLabel, filename, id string) string {
	if refLabel != "" {
		if hint != "" {
			return hint + "_" + refLabel
		}
		return refLabel
	}
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base != "" {
		if s := naming.Sanitize(base); s != "" {
			return s
		}
	}
	return "icon_" + id
}
