// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

const tfResourceIcon = "jamfpro_icon"

// IconInfo holds metadata about a discovered icon for downloading.
type IconInfo struct {
	ID       int
	Name     string
	URL      string
	Label    string // Terraform resource label (includes type hint + referencing resource label)
	RefLabel string // Label of the resource that references this icon
}

// iconRef holds the referencing resource's label and a short type prefix for icon naming.
type iconRef struct {
	label    string // the referencing resource's Terraform label
	typeHint string // short prefix: "policy", "profile", "mac_app", "mobile_app"
}

// discoverIcons finds icons referenced by policies, macOS configuration profiles,
// mac applications, and mobile device applications.
// Since there is no "list all icons" API, we fetch each resource individually
// to extract self_service_icon IDs, then call GetIconByID for each unique one.
// Icon resource labels include the referencing resource type and label for clarity,
// e.g. jamfpro_icon.policy_install_chrome for an icon used by jamfpro_policy.install_chrome.
func discoverIcons(client *jamfpro.Client, reg *registry.Registry, policies, profiles, macApps, mobileDeviceApps []Resource) ([]Resource, map[string]IconInfo, error) {
	// Scan resources for icon IDs in parallel, tracking which resource references each icon
	var mu sync.Mutex
	iconRefs := make(map[int]iconRef) // icon ID → referencing resource info

	var wg sync.WaitGroup
	const workers = 5

	if !Quiet {
		fmt.Printf("  Scanning %d policies, %d profiles, %d mac applications, and %d mobile device applications for icons...\n",
			len(policies), len(profiles), len(macApps), len(mobileDeviceApps))
	}

	policyCh := make(chan Resource, len(policies))
	for _, p := range policies {
		policyCh <- p
	}
	close(policyCh)

	for range workers {
		wg.Go(func() {
			for p := range policyCh {
				policy, err := client.GetPolicyByID(p.JamfID)
				if err != nil {
					if !Quiet {
						fmt.Printf("  Warning: could not read policy %s for icon discovery: %v\n", p.JamfID, err)
					}
					continue
				}
				if policy.SelfService.SelfServiceIcon != nil && policy.SelfService.SelfServiceIcon.ID > 0 {
					mu.Lock()
					if _, exists := iconRefs[policy.SelfService.SelfServiceIcon.ID]; !exists {
						iconRefs[policy.SelfService.SelfServiceIcon.ID] = iconRef{label: p.Label, typeHint: "policy"}
					}
					mu.Unlock()
				}
			}
		})
	}

	// Scan profiles concurrently with policies
	profileCh := make(chan Resource, len(profiles))
	for _, p := range profiles {
		profileCh <- p
	}
	close(profileCh)

	for range workers {
		wg.Go(func() {
			for p := range profileCh {
				profile, err := client.GetMacOSConfigurationProfileByID(p.JamfID)
				if err != nil {
					if !Quiet {
						fmt.Printf("  Warning: could not read profile %s for icon discovery: %v\n", p.JamfID, err)
					}
					continue
				}
				if profile.SelfService.SelfServiceIcon.ID > 0 {
					mu.Lock()
					if _, exists := iconRefs[profile.SelfService.SelfServiceIcon.ID]; !exists {
						iconRefs[profile.SelfService.SelfServiceIcon.ID] = iconRef{label: p.Label, typeHint: "profile"}
					}
					mu.Unlock()
				}
			}
		})
	}

	// Scan mac applications concurrently
	macAppCh := make(chan Resource, len(macApps))
	for _, a := range macApps {
		macAppCh <- a
	}
	close(macAppCh)

	for range workers {
		wg.Go(func() {
			for a := range macAppCh {
				app, err := client.GetMacApplicationByID(a.JamfID)
				if err != nil {
					if !Quiet {
						fmt.Printf("  Warning: could not read mac application %s for icon discovery: %v\n", a.JamfID, err)
					}
					continue
				}
				if app.SelfService.SelfServiceIcon.ID > 0 {
					mu.Lock()
					if _, exists := iconRefs[app.SelfService.SelfServiceIcon.ID]; !exists {
						iconRefs[app.SelfService.SelfServiceIcon.ID] = iconRef{label: a.Label, typeHint: "mac_app"}
					}
					mu.Unlock()
				}
			}
		})
	}

	// Scan mobile device applications concurrently
	mobileAppCh := make(chan Resource, len(mobileDeviceApps))
	for _, a := range mobileDeviceApps {
		mobileAppCh <- a
	}
	close(mobileAppCh)

	for range workers {
		wg.Go(func() {
			for a := range mobileAppCh {
				app, err := client.GetMobileDeviceApplicationByID(a.JamfID)
				if err != nil {
					if !Quiet {
						fmt.Printf("  Warning: could not read mobile application %s for icon discovery: %v\n", a.JamfID, err)
					}
					continue
				}
				if app.SelfService.SelfServiceIcon.ID > 0 {
					mu.Lock()
					if _, exists := iconRefs[app.SelfService.SelfServiceIcon.ID]; !exists {
						iconRefs[app.SelfService.SelfServiceIcon.ID] = iconRef{label: a.Label, typeHint: "mobile_app"}
					}
					mu.Unlock()
				}
			}
		})
	}

	wg.Wait()

	// Fetch metadata for each unique icon.
	// The icon resource label includes the referencing resource's type and label,
	// e.g. "policy_install_chrome" for an icon used by jamfpro_policy.install_chrome.
	// This avoids collisions when different resource types share the same name.
	iconInfoMap := make(map[string]IconInfo)
	var resources []Resource
	usedLabels := make(map[string]bool)

	for iconID, ref := range iconRefs {
		icon, err := client.GetIconByID(iconID)
		if err != nil {
			if !Quiet {
				fmt.Printf("  Warning: could not fetch icon %d: %v\n", iconID, err)
			}
			continue
		}

		idStr := strconv.Itoa(icon.ID)

		// Build label: {typeHint}_{refLabel}, falling back to the icon's own name.
		label := ref.label
		if label == "" {
			label = naming.Sanitize(icon.Name)
		}
		if label == "" {
			label = fmt.Sprintf("icon_%d", icon.ID)
		}
		if ref.typeHint != "" {
			label = ref.typeHint + "_" + label
		}

		if usedLabels[label] {
			base := label
			counter := 2
			for usedLabels[fmt.Sprintf("%s_%d", base, counter)] {
				counter++
			}
			label = fmt.Sprintf("%s_%d", base, counter)
		}
		usedLabels[label] = true

		tfAddr := fmt.Sprintf("%s.%s", tfResourceIcon, label)
		reg.Register(tfResourceIcon, idStr, tfAddr)

		iconInfoMap[idStr] = IconInfo{
			ID:       icon.ID,
			Name:     icon.Name,
			URL:      icon.URL,
			Label:    label,
			RefLabel: ref.label,
		}

		resources = append(resources, Resource{
			JamfID: idStr,
			Name:   icon.Name,
			Label:  label,
		})
	}

	return resources, iconInfoMap, nil
}
