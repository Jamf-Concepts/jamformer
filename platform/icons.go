// Copyright 2026, Jamf Software LLC

package platform

import (
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
	"github.com/zclconf/go-cty/cty"
)

// iconSelfServiceIconPath is the nested object path to a policy's self-service
// icon object (general → self_service → self_service_icon is two object hops
// from the resource body since self_service is itself a nested attribute).
var iconSelfServiceIconPath = []string{"self_service", "self_service_icon"}

// GenerateIcons synthesises jamfplatform_pro_icon resources from the
// self_service_icon references hydrated into policies. jamfplatform_pro_icon has
// no list resource, so icons are not query-discoverable; instead each policy's
// self_service_icon carries the icon's id, CDN uri, and filename. For every
// unique referenced icon this emits one jamfplatform_pro_icon resource whose
// icon_file_source is the CDN uri (the provider downloads + re-uploads it) plus
// an import block keyed by the icon id, registers the icon id so the
// self_service_icon.id reference can be rewritten to the new resource, and drops
// the server-computed uri/filename echoes from each policy's self_service_icon
// (they would otherwise drift on a fresh tenant). Returns the icon count.
func GenerateIcons(generatedFile string, reg *registry.Registry) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	type iconInfo struct{ uri, filename string }
	icons := make(map[string]iconInfo)
	var order []string

	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "jamfplatform_pro_policy" {
			continue
		}
		id := postprocess.ReadObjectAttrString(block.Body(), iconSelfServiceIconPath, "id")
		if id == "" {
			continue
		}
		if _, seen := icons[id]; !seen {
			icons[id] = iconInfo{
				uri:      postprocess.ReadObjectAttrString(block.Body(), iconSelfServiceIconPath, "uri"),
				filename: postprocess.ReadObjectAttrString(block.Body(), iconSelfServiceIconPath, "filename"),
			}
			order = append(order, id)
		}
		postprocess.RemoveNestedAttrs(block.Body(), iconSelfServiceIconPath, "uri", "filename")
	}

	if len(order) == 0 {
		return 0, nil
	}

	tracker := naming.NewTracker()
	for _, id := range order {
		info := icons[id]
		label := tracker.Label(tIcon, iconLabelBase(info.filename, id))
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
	}

	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return len(order), nil
}

// iconLabelBase derives a friendly label seed for an icon from its filename
// (extension stripped), falling back to icon_<id> when the filename is absent.
func iconLabelBase(filename, id string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base != "" {
		if s := naming.Sanitize(base); s != "" {
			return s
		}
	}
	return "icon_" + id
}
