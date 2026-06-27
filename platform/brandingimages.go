// Copyright 2026, Jamf Software LLC

package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	sdkpro "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// brandingImageRefs are the Self Service branding singleton attributes that hold
// a branding-image id (top-level Int64). The branding-image store has no list
// resource, so image ids are recovered from these singletons.
var brandingImageRefs = []struct{ resourceType, attr string }{
	{"jamfplatform_pro_self_service_branding_macos", "icon_id"},
	{"jamfplatform_pro_self_service_branding_macos", "banner_image_id"},
	{"jamfplatform_pro_self_service_branding_ios", "icon_id"},
}

// GenerateBrandingImages synthesises jamfplatform_pro_self_service_branding_image
// resources from the branding-image ids referenced by the Self Service branding
// singletons. The branding-image store has no list resource (not
// query-discoverable), and the ids live on the already-materialised branding
// singletons. For every unique referenced id this downloads the image bytes via
// the federated pro SDK (`DownloadBrandingImageV1`), writes them to
// support_files/branding_images/, emits one resource (image_file_source = the
// local path, plus lifecycle ignore_changes since source_hash is ForceNew) and
// an import block keyed by the id, and registers the id so the singleton
// icon_id/banner_image_id references rewrite to the new resource (via the
// Numeric DefaultRules entries, since those attributes are number-typed).
// Returns the image count. Requires a tenant ID (pro endpoints are tenant-scoped).
func GenerateBrandingImages(ctx context.Context, pc *sdkpro.Client, generatedFile, outputDir string, reg *registry.Registry) (int, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return 0, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return 0, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	// Collect unique branding-image ids referenced by the branding singletons.
	var order []string
	seen := make(map[string]bool)
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		for _, ref := range brandingImageRefs {
			if labels[0] != ref.resourceType {
				continue
			}
			attr := block.Body().GetAttribute(ref.attr)
			if attr == nil {
				continue
			}
			id := postprocess.ExtractStringValue(attr)
			// 0 / -1 are the server's "no image" sentinels.
			if id == "" || id == "0" || id == "-1" {
				continue
			}
			if !seen[id] {
				seen[id] = true
				order = append(order, id)
			}
		}
	}
	if len(order) == 0 {
		return 0, nil
	}

	imgDir := filepath.Join(outputDir, "support_files", "branding_images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return 0, fmt.Errorf("creating branding_images directory: %w", err)
	}

	tracker := naming.NewTracker()
	count := 0
	for _, id := range order {
		data, err := pc.DownloadBrandingImageV1(ctx, id)
		if err != nil {
			if !Quiet {
				fmt.Printf("  Warning: could not download branding image %s: %v\n", id, err)
			}
			continue
		}
		fileName := "branding_image_" + id + brandingImageExt(data)
		if err := os.WriteFile(filepath.Join(imgDir, fileName), data, 0644); err != nil {
			return count, fmt.Errorf("writing branding image %s: %w", fileName, err)
		}

		label := tracker.Label(tBrandingImage, "branding_image_"+id)
		addr := fmt.Sprintf("%s.%s", tBrandingImage, label)
		reg.Register(tBrandingImage, id, addr)

		f.Body().AppendNewline()
		imp := f.Body().AppendNewBlock("import", nil)
		imp.Body().SetAttributeRaw("to", hclwrite.Tokens{{Type: hclsyntax.TokenIdent, Bytes: []byte(addr)}})
		imp.Body().SetAttributeValue("id", cty.StringVal(id))

		f.Body().AppendNewline()
		res := f.Body().AppendNewBlock("resource", []string{tBrandingImage, label})
		relPath := filepath.ToSlash(filepath.Join("support_files", "branding_images", fileName))
		res.Body().SetAttributeRaw("image_file_source", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: fmt.Appendf(nil, `"${path.module}/%s"`, relPath)},
		})
		lc := res.Body().AppendNewBlock("lifecycle", nil)
		lc.Body().SetAttributeRaw("ignore_changes", hclwrite.Tokens{
			{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			{Type: hclsyntax.TokenIdent, Bytes: []byte("image_file_source")},
			{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
		})
		count++
	}

	if count == 0 {
		return 0, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return count, nil
}

// brandingImageExt sniffs the image content type and returns a file extension,
// defaulting to .png when the type is not a recognised image.
func brandingImageExt(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
