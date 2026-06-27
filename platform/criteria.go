// Copyright 2026, Jamf Software LLC

package platform

import (
	"fmt"
	"os"

	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Synthetic registry types keyed by NAME (not ID) for the name-based criteria
// references. Smart-group criteria reference other objects by name: device-group
// "member of" criteria carry the target group's name, and an extension-attribute
// criterion carries the EA's name as the criterion field itself.
const (
	// DeviceGroup{Computer,Mobile}NameType resolve a device-group name (the value
	// of a "Computer Group" / "Mobile Device Group" member-of criterion) to a
	// jamfplatform_device_group address, scoped by device_type so a computer and a
	// mobile group sharing a name don't collide.
	DeviceGroupComputerNameType = "jamfplatform_device_group#name#computer"
	DeviceGroupMobileNameType   = "jamfplatform_device_group#name#mobile"

	tComputerEA = "jamfplatform_pro_computer_extension_attribute"
	tMobileEA   = "jamfplatform_pro_mobile_device_extension_attribute"

	// {Computer,Mobile}EANameType resolve an extension-attribute name (used as a
	// device-group criterion field) to its EA resource address.
	ComputerEANameType = tComputerEA + "#name"
	MobileEANameType   = tMobileEA + "#name"
)

// PopulateCriteriaNameIndexes registers name→address entries used by the
// name-based criteria reference rules: device-group names per device_type and
// computer/mobile extension-attribute names. Addresses come straight from the
// resource block labels in the generated HCL.
func PopulateCriteriaNameIndexes(generatedFile string, reg *registry.Registry) error {
	f, err := parseGeneratedHCL(generatedFile)
	if err != nil {
		return err
	}
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 {
			continue
		}
		name := postprocess.ExtractStringValue(block.Body().GetAttribute("name"))
		if name == "" {
			continue
		}
		addr := labels[0] + "." + labels[1]
		switch labels[0] {
		case "jamfplatform_device_group":
			switch postprocess.ExtractStringValue(block.Body().GetAttribute("device_type")) {
			case "computer":
				reg.Register(DeviceGroupComputerNameType, name, addr)
			case "mobile":
				reg.Register(DeviceGroupMobileNameType, name, addr)
			}
		case tComputerEA:
			reg.Register(ComputerEANameType, name, addr)
		case tMobileEA:
			reg.Register(MobileEANameType, name, addr)
		}
	}
	return nil
}

// ResolveCriteriaExtensionAttributes rewrites a device group's smart-group
// criterion field (the `criteria` attribute of each criteria element) to a
// jamfplatform_pro_*_extension_attribute.<x>.name reference when it names a
// known EA for the group's device_type. Built-in criteria (e.g. "Application
// Bundle ID") don't match any EA name and are left untouched. Returns the number
// of device-group resources whose criteria were modified.
//
// Device-group member-of values and user-group member-of values are handled by
// the DefaultRules discriminator engine; only the EA-name-as-criterion-field
// case needs the owning resource's device_type, so it lives here.
func ResolveCriteriaExtensionAttributes(generatedFile string, reg *registry.Registry) (int, error) {
	f, err := parseGeneratedHCL(generatedFile)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, block := range f.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) < 2 || labels[0] != "jamfplatform_device_group" {
			continue
		}
		var eaType string
		switch postprocess.ExtractStringValue(block.Body().GetAttribute("device_type")) {
		case "computer":
			eaType = ComputerEANameType
		case "mobile":
			eaType = MobileEANameType
		default:
			continue
		}
		if postprocess.RewriteListElementField(block.Body(), "criteria", "criteria", func(name string) (string, bool) {
			return reg.AttrReference(eaType, name, "name")
		}) {
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}
	if err := os.WriteFile(generatedFile, f.Bytes(), 0644); err != nil {
		return 0, fmt.Errorf("writing generated file: %w", err)
	}
	return count, nil
}

// parseGeneratedHCL reads and parses the generated HCL file.
func parseGeneratedHCL(generatedFile string) (*hclwrite.File, error) {
	src, err := os.ReadFile(generatedFile)
	if err != nil {
		return nil, fmt.Errorf("reading generated file: %w", err)
	}
	f, diags := hclwrite.ParseConfig(src, generatedFile, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing generated HCL: %s", diags.Error())
	}
	return f, nil
}
