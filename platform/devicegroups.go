// Copyright 2026, Jamf Software LLC

package platform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// DeviceGroupInfo carries the data needed to bridge a jamfplatform_device_group
// to classic-API scope references. A device group is a single TF type covering
// both computer and mobile groups (distinguished by DeviceType), each with its
// own Jamf Pro numeric ID space (JamfProID).
type DeviceGroupInfo struct {
	Address    string // e.g. "jamfplatform_device_group.engineering_computer"
	DeviceType string // "computer" | "mobile"
	JamfProID  string // classic numeric ID; empty when not recoverable
}

// CollectDeviceGroupInfo reads the generated HCL and returns DeviceGroupInfo for
// every jamfplatform_device_group, keyed by its Platform UUID (the import-block
// identity id). DeviceType is read from the resource block (it is a Required
// attribute, so it is always present). JamfProID is read from the block too when
// the provider emitted it (it is computed, so usually absent — recover it via
// MergeJamfProIDsFromEvents).
func CollectDeviceGroupInfo(generatedFile string) (map[string]DeviceGroupInfo, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return nil, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}

	// address -> {deviceType, jamfProID} from resource blocks.
	type attrs struct{ deviceType, jamfProID string }
	byAddr := make(map[string]attrs)
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "jamfplatform_device_group" {
			continue
		}
		addr := labels[0] + "." + labels[1]
		var a attrs
		if dt := block.Body().GetAttribute("device_type"); dt != nil {
			a.deviceType = postprocess.ExtractStringValue(dt)
		}
		if jp := block.Body().GetAttribute("jamf_pro_id"); jp != nil {
			a.jamfProID = postprocess.ExtractStringValue(jp)
		}
		byAddr[addr] = a
	}

	// UUID -> address from import blocks.
	out := make(map[string]DeviceGroupInfo)
	for _, block := range f.Body().Blocks() {
		if block.Type() != "import" {
			continue
		}
		toAttr := block.Body().GetAttribute("to")
		if toAttr == nil {
			continue
		}
		addr := strings.TrimSpace(string(toAttr.Expr().BuildTokens(nil).Bytes()))
		if !strings.HasPrefix(addr, "jamfplatform_device_group.") {
			continue
		}
		uuid := importIdentityID(block)
		if uuid == "" {
			continue
		}
		a := byAddr[addr]
		out[uuid] = DeviceGroupInfo{Address: addr, DeviceType: a.deviceType, JamfProID: a.jamfProID}
	}

	return out, nil
}

// importIdentityID extracts the identity id (Platform UUID) from an import block.
func importIdentityID(block *hclwrite.Block) string {
	if identityAttr := block.Body().GetAttribute("identity"); identityAttr != nil {
		for _, tok := range identityAttr.Expr().BuildTokens(nil) {
			if tok.Type == hclsyntax.TokenQuotedLit {
				return string(tok.Bytes)
			}
		}
	}
	if idAttr := block.Body().GetAttribute("id"); idAttr != nil {
		return postprocess.ExtractStringValue(idAttr)
	}
	return ""
}

// dgListEvent decodes the fields of a terraform query "list_resource_found"
// event that may carry a device group's computed jamf_pro_id / device_type in
// the per-row resource_object payload.
type dgListEvent struct {
	Type              string `json:"type"`
	ListResourceFound struct {
		ResourceType string `json:"resource_type"`
		Identity     struct {
			ID string `json:"id"`
		} `json:"identity"`
		ResourceObject struct {
			JamfProID  string `json:"jamf_pro_id"`
			DeviceType string `json:"device_type"`
		} `json:"resource_object"`
	} `json:"list_resource_found"`
}

// MergeJamfProIDsFromEvents fills in JamfProID (and DeviceType, if missing) on the
// device group info map from a terraform query -json event log. jamf_pro_id is a
// computed attribute that `-generate-config-out` drops, so it can only be
// recovered from the query event stream (when the provider surfaces it) or a
// direct API call. Missing data leaves JamfProID empty and is not an error.
func MergeJamfProIDsFromEvents(info map[string]DeviceGroupInfo, eventsFile string) error {
	f, err := os.Open(eventsFile)
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var ev dgListEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Type != "list_resource_found" || ev.ListResourceFound.ResourceType != "jamfplatform_device_group" {
			continue
		}
		uuid := ev.ListResourceFound.Identity.ID
		dgi, ok := info[uuid]
		if !ok {
			continue
		}
		if dgi.JamfProID == "" && ev.ListResourceFound.ResourceObject.JamfProID != "" {
			dgi.JamfProID = ev.ListResourceFound.ResourceObject.JamfProID
		}
		if dgi.DeviceType == "" && ev.ListResourceFound.ResourceObject.DeviceType != "" {
			dgi.DeviceType = ev.ListResourceFound.ResourceObject.DeviceType
		}
		info[uuid] = dgi
	}
	return scanner.Err()
}

// PopulateDeviceGroupSubtypes registers each device group under a synthetic
// registry type — jamfplatform_device_group#computer or
// jamfplatform_device_group#mobile — keyed by its Jamf Pro numeric ID and
// mapping to the real jamfplatform_device_group.<label> address. Classic scope
// rules resolve computer_group_ids / mobile_device_group_ids against these
// subtypes and reference .jamf_pro_id. Entries without a recoverable JamfProID
// are skipped (scope references to them stay as TODO).
//
// Returns the number of device groups that could not be bridged (no JamfProID),
// so the caller can warn the user.
func PopulateDeviceGroupSubtypes(reg *registry.Registry, info map[string]DeviceGroupInfo) int {
	unbridged := 0
	for _, dgi := range info {
		if dgi.DeviceType == "" {
			continue
		}
		if dgi.JamfProID == "" {
			unbridged++
			continue
		}
		reg.Register("jamfplatform_device_group#"+dgi.DeviceType, dgi.JamfProID, dgi.Address)
	}
	return unbridged
}
