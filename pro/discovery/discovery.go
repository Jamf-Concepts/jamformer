// Copyright 2026, Jamf Software LLC

package discovery

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/Jamf-Concepts/jamformer/naming"
	"github.com/Jamf-Concepts/jamformer/registry"
	"github.com/deploymenttheory/go-api-sdk-jamfpro/sdk/jamfpro"
)

// Quiet suppresses progress messages.
var Quiet bool

// Resource represents a discovered Jamf Pro resource with its ID, name, and derived TF label.
type Resource struct {
	JamfID string
	Name   string
	Label  string // Sanitized Terraform resource label
}

// Results holds all discovered resources grouped by type.
type Results struct {
	Sites                             []Resource
	Buildings                         []Resource
	Categories                        []Resource
	Departments                       []Resource
	Scripts                           []Resource
	ComputerExtensionAttributes       []Resource
	Packages                          []Resource
	DockItems                         []Resource
	Printers                          []Resource
	NetworkSegments                   []Resource
	SmartComputerGroups               []Resource
	StaticComputerGroups              []Resource
	MacOSConfigurationProfiles        []Resource
	Policies                          []Resource
	Icons                             []Resource
	IconInfo                          map[string]IconInfo // icon ID -> metadata (for downloading)
	EnrollmentCustomizations          []Resource
	EnrollmentCustomizationInfo       map[string]EnrollmentCustomizationInfo // enrollment customization ID -> metadata (for image downloading)
	ComputerPrestages                 []Resource
	AdvancedComputerSearches          []Resource
	AppInstallers                     []Resource
	MacApplications                   []Resource
	DeviceEnrollments                 []Resource
	VolumePurchasingLocations         []Resource
	RestrictedSoftware                []Resource
	SmartMobileDeviceGroups           []Resource
	StaticMobileDeviceGroups          []Resource
	MobileDeviceConfigurationProfiles []Resource
	MobileDevicePrestages             []Resource
	MobileDeviceExtensionAttributes   []Resource
	AdvancedMobileDeviceSearches      []Resource
	APIIntegrations                   []Resource
	APIRoles                          []Resource
	Accounts                          []Resource
	Webhooks                          []Resource
	AccountGroups                     []Resource
	DiskEncryptionConfigurations      []Resource
	AllowedFileExtensions             []Resource
	LDAPServers                       []Resource
	MobileDeviceApplications          []Resource
	UserGroups                        []Resource
	SelfServiceBrandingMacOS          []Resource
	SelfServiceBrandingIOS            []Resource
	AdvancedUserSearches              []Resource

	// Singletons maps TF resource type -> single Resource for settings with no list API.
	Singletons map[string][]Resource
}

// DiscoverAll lists all supported resources from Jamf Pro and registers them
// in the dependency registry for later reference resolution.
// packageInfo is populated with package_name -> filename mappings for post-processing.
// filter optionally restricts which resource types to discover. If nil, all types are discovered.
// Filter keys use friendly names matching the ResourceDef.FilterKey values in pro/resources.go.
// progressFn is called after each resource type finishes discovery (may be nil).
func DiscoverAll(client *jamfpro.Client, reg *registry.Registry, packageInfo map[string]string, filter map[string]bool, progressFn func()) (*Results, error) {
	results := &Results{}

	selected := func(name string) bool {
		return filter == nil || filter[name]
	}

	// Phase 1: Run discovery tasks sequentially via a single-slot semaphore.
	// The SDK serialises HTTP calls (MaxConcurrentRequests=1), so additional
	// goroutines would only pile up waiting — no parallelism is gained.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, 1)

	run := func(name string, fn func() error) {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					logPath := writePanicLog(name, r, stack)
					if logPath != "" {
						fmt.Fprintf(os.Stderr, "\n  WARNING: %s discovery crashed - details saved to %s\n", name, logPath)
					} else {
						fmt.Fprintf(os.Stderr, "\n  WARNING: %s discovery crashed: %v\n", name, r)
					}
					fmt.Fprintf(os.Stderr, "  This is likely a bug in an upstream dependency.\n")
					fmt.Fprintf(os.Stderr, "  Please report this to the jamformer maintainers with the log file attached.\n\n")
				}
				if progressFn != nil {
					progressFn()
				}
			}()
			if err := fn(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("discovering %s: %w", name, err)
				}
				mu.Unlock()
			}
		})
	}

	if selected("sites") {
		run("sites", func() error {
			var err error
			results.Sites, err = discoverSites(client, reg)
			return err
		})
	}

	if selected("buildings") {
		run("buildings", func() error {
			var err error
			results.Buildings, err = discoverBuildings(client, reg)
			return err
		})
	}

	if selected("categories") {
		run("categories", func() error {
			var err error
			results.Categories, err = discoverCategories(client, reg)
			return err
		})
	}

	if selected("departments") {
		run("departments", func() error {
			var err error
			results.Departments, err = discoverDepartments(client, reg)
			return err
		})
	}

	if selected("scripts") {
		run("scripts", func() error {
			var err error
			results.Scripts, err = discoverScripts(client, reg)
			return err
		})
	}

	if selected("extension_attributes") {
		run("extension_attributes", func() error {
			var err error
			results.ComputerExtensionAttributes, err = discoverComputerExtensionAttributes(client, reg)
			return err
		})
	}

	if selected("packages") {
		run("packages", func() error {
			var err error
			results.Packages, err = discoverPackages(client, reg, packageInfo)
			return err
		})
	}

	if selected("dock_items") {
		run("dock_items", func() error {
			var err error
			results.DockItems, err = discoverDockItems(client, reg)
			return err
		})
	}

	if selected("printers") {
		run("printers", func() error {
			var err error
			results.Printers, err = discoverPrinters(client, reg)
			return err
		})
	}

	if selected("network_segments") {
		run("network_segments", func() error {
			var err error
			results.NetworkSegments, err = discoverNetworkSegments(client, reg)
			return err
		})
	}

	if selected("smart_computer_groups") {
		run("smart_computer_groups", func() error {
			var err error
			results.SmartComputerGroups, err = discoverSmartComputerGroups(client, reg)
			return err
		})
	}

	if selected("static_computer_groups") {
		run("static_computer_groups", func() error {
			var err error
			results.StaticComputerGroups, err = discoverStaticComputerGroups(client, reg)
			return err
		})
	}

	if selected("macos_configuration_profiles") {
		run("macos_configuration_profiles", func() error {
			var err error
			results.MacOSConfigurationProfiles, err = discoverMacOSConfigurationProfiles(client, reg)
			return err
		})
	}

	if selected("policies") {
		run("policies", func() error {
			var err error
			results.Policies, err = discoverPolicies(client, reg)
			return err
		})
	}

	if selected("enrollment_customizations") {
		run("enrollment_customizations", func() error {
			var err error
			results.EnrollmentCustomizations, results.EnrollmentCustomizationInfo, err = discoverEnrollmentCustomizations(client, reg)
			return err
		})
	}

	if selected("computer_prestages") {
		run("computer_prestages", func() error {
			var err error
			results.ComputerPrestages, err = discoverComputerPrestages(client, reg)
			return err
		})
	}

	if selected("advanced_computer_searches") {
		run("advanced_computer_searches", func() error {
			var err error
			results.AdvancedComputerSearches, err = discoverAdvancedComputerSearches(client, reg)
			return err
		})
	}

	if selected("app_installers") {
		run("app_installers", func() error {
			var err error
			results.AppInstallers, err = discoverAppInstallers(client, reg)
			return err
		})
	}

	if selected("mac_applications") {
		run("mac_applications", func() error {
			var err error
			results.MacApplications, err = discoverMacApplications(client, reg)
			return err
		})
	}

	if selected("device_enrollments") {
		run("device_enrollments", func() error {
			var err error
			results.DeviceEnrollments, err = discoverDeviceEnrollments(client, reg)
			return err
		})
	}

	if selected("volume_purchasing_locations") {
		run("volume_purchasing_locations", func() error {
			var err error
			results.VolumePurchasingLocations, err = discoverVolumePurchasingLocations(client, reg)
			return err
		})
	}

	if selected("restricted_software") {
		run("restricted_software", func() error {
			var err error
			results.RestrictedSoftware, err = discoverRestrictedSoftware(client, reg)
			return err
		})
	}

	if selected("smart_mobile_device_groups") {
		run("smart_mobile_device_groups", func() error {
			var err error
			results.SmartMobileDeviceGroups, err = discoverSmartMobileDeviceGroups(client, reg)
			return err
		})
	}

	if selected("static_mobile_device_groups") {
		run("static_mobile_device_groups", func() error {
			var err error
			results.StaticMobileDeviceGroups, err = discoverStaticMobileDeviceGroups(client, reg)
			return err
		})
	}

	if selected("mobile_device_configuration_profiles") {
		run("mobile_device_configuration_profiles", func() error {
			var err error
			results.MobileDeviceConfigurationProfiles, err = discoverMobileDeviceConfigurationProfiles(client, reg)
			return err
		})
	}

	if selected("mobile_device_prestages") {
		run("mobile_device_prestages", func() error {
			var err error
			results.MobileDevicePrestages, err = discoverMobileDevicePrestages(client, reg)
			return err
		})
	}

	if selected("mobile_device_extension_attributes") {
		run("mobile_device_extension_attributes", func() error {
			var err error
			results.MobileDeviceExtensionAttributes, err = discoverMobileDeviceExtensionAttributes(client, reg)
			return err
		})
	}

	if selected("advanced_mobile_device_searches") {
		run("advanced_mobile_device_searches", func() error {
			var err error
			results.AdvancedMobileDeviceSearches, err = discoverAdvancedMobileDeviceSearches(client, reg)
			return err
		})
	}

	if selected("api_integrations") {
		run("api_integrations", func() error {
			var err error
			results.APIIntegrations, err = discoverAPIIntegrations(client, reg)
			return err
		})
	}

	if selected("api_roles") {
		run("api_roles", func() error {
			var err error
			results.APIRoles, err = discoverAPIRoles(client, reg)
			return err
		})
	}

	if selected("accounts") {
		run("accounts", func() error {
			var err error
			results.Accounts, err = discoverAccounts(client, reg)
			return err
		})
	}

	if selected("webhooks") {
		run("webhooks", func() error {
			var err error
			results.Webhooks, err = discoverWebhooks(client, reg)
			return err
		})
	}

	if selected("account_groups") {
		run("account_groups", func() error {
			var err error
			results.AccountGroups, err = discoverAccountGroups(client, reg)
			return err
		})
	}

	if selected("disk_encryption_configurations") {
		run("disk_encryption_configurations", func() error {
			var err error
			results.DiskEncryptionConfigurations, err = discoverDiskEncryptionConfigurations(client, reg)
			return err
		})
	}

	if selected("allowed_file_extensions") {
		run("allowed_file_extensions", func() error {
			var err error
			results.AllowedFileExtensions, err = discoverAllowedFileExtensions(client, reg)
			return err
		})
	}

	if selected("ldap_servers") {
		run("ldap_servers", func() error {
			var err error
			results.LDAPServers, err = discoverLDAPServers(client, reg)
			return err
		})
	}

	if selected("mobile_device_applications") {
		run("mobile_device_applications", func() error {
			var err error
			results.MobileDeviceApplications, err = discoverMobileDeviceApplications(client, reg)
			return err
		})
	}

	if selected("user_groups") {
		run("user_groups", func() error {
			var err error
			results.UserGroups, err = discoverUserGroups(client, reg)
			return err
		})
	}

	if selected("self_service_branding_macos") {
		run("self_service_branding_macos", func() error {
			var err error
			results.SelfServiceBrandingMacOS, err = discoverSelfServiceBrandingMacOS(client, reg)
			return err
		})
	}

	if selected("self_service_branding_ios") {
		run("self_service_branding_ios", func() error {
			var err error
			results.SelfServiceBrandingIOS, err = discoverSelfServiceBrandingIOS(client, reg)
			return err
		})
	}

	if selected("advanced_user_searches") {
		run("advanced_user_searches", func() error {
			var err error
			results.AdvancedUserSearches, err = discoverAdvancedUserSearches(client, reg)
			return err
		})
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	// Phase 2: Icons require scanning policies/profiles/mac apps/mobile device apps for self_service_icon IDs.
	// Must run after phase 1 since it depends on policy/profile/app results.
	if selected("icons") {
		policiesToScan := results.Policies
		profilesToScan := results.MacOSConfigurationProfiles
		macAppsToScan := results.MacApplications
		mobileDeviceAppsToScan := results.MobileDeviceApplications

		// If resources weren't selected, do a lightweight listing just for icon scanning
		if !selected("policies") && policiesToScan == nil {
			policiesToScan, _ = discoverPoliciesLightweight(client)
		}
		if !selected("macos_configuration_profiles") && profilesToScan == nil {
			profilesToScan, _ = discoverProfilesLightweight(client)
		}
		if !selected("mac_applications") && macAppsToScan == nil {
			macAppsToScan, _ = discoverMacApplicationsLightweight(client)
		}
		if !selected("mobile_device_applications") && mobileDeviceAppsToScan == nil {
			mobileDeviceAppsToScan, _ = discoverMobileDeviceApplicationsLightweight(client)
		}

		var err error
		results.Icons, results.IconInfo, err = discoverIcons(client, reg, policiesToScan, profilesToScan, macAppsToScan, mobileDeviceAppsToScan)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// discoverPoliciesLightweight lists policies (IDs and names only) without
// registering them in the registry. Used for icon scanning when policies
// aren't selected for import.
func discoverPoliciesLightweight(client *jamfpro.Client) ([]Resource, error) {
	resp, err := client.GetPolicies()
	if err != nil {
		return nil, err
	}
	tracker := naming.NewTracker()
	var resources []Resource
	for _, p := range resp.Policy {
		resources = append(resources, Resource{
			JamfID: strconv.Itoa(p.ID),
			Name:   p.Name,
			Label:  tracker.Label("jamfpro_policy", p.Name),
		})
	}
	return resources, nil
}

// discoverProfilesLightweight lists macOS configuration profiles (IDs and names only)
// without registering them in the registry.
func discoverProfilesLightweight(client *jamfpro.Client) ([]Resource, error) {
	resp, err := client.GetMacOSConfigurationProfiles()
	if err != nil {
		return nil, err
	}
	tracker := naming.NewTracker()
	var resources []Resource
	for _, p := range resp.Results {
		resources = append(resources, Resource{
			JamfID: strconv.Itoa(p.ID),
			Name:   p.Name,
			Label:  tracker.Label("jamfpro_macos_configuration_profile_plist", p.Name),
		})
	}
	return resources, nil
}

// discoverMacApplicationsLightweight lists mac applications (IDs and names only)
// without registering them in the registry. Used for icon scanning when mac
// applications aren't selected for import.
func discoverMacApplicationsLightweight(client *jamfpro.Client) ([]Resource, error) {
	resp, err := client.GetMacApplications()
	if err != nil {
		return nil, err
	}
	tracker := naming.NewTracker()
	var resources []Resource
	for _, a := range resp.MacApplications {
		resources = append(resources, Resource{
			JamfID: strconv.Itoa(a.ID),
			Name:   a.Name,
			Label:  tracker.Label("jamfpro_mac_application", a.Name),
		})
	}
	return resources, nil
}

// discoverMobileDeviceApplicationsLightweight lists mobile device applications
// (IDs and names only) without registering them in the registry. Used for icon
// scanning when mobile device applications aren't selected for import.
func discoverMobileDeviceApplicationsLightweight(client *jamfpro.Client) ([]Resource, error) {
	resp, err := client.GetMobileDeviceApplications()
	if err != nil {
		return nil, err
	}
	tracker := naming.NewTracker()
	var resources []Resource
	for _, a := range resp.MobileDeviceApplications {
		resources = append(resources, Resource{
			JamfID: strconv.Itoa(a.ID),
			Name:   a.Name,
			Label:  tracker.Label("jamfpro_mobile_device_application", a.Name),
		})
	}
	return resources, nil
}

// writePanicLog writes panic details and the full stack trace to a temporary
// log file and returns the file path. Returns "" if the file can't be created.
func writePanicLog(resourceType string, panicVal any, stack []byte) string {
	f, err := os.CreateTemp("", "jamformer-crash-*.log")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	_, _ = fmt.Fprintf(f, "jamformer panic during %s discovery\n", resourceType)
	_, _ = fmt.Fprintf(f, "time: %s\n", time.Now().Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "panic: %v\n\n", panicVal)
	_, _ = f.Write(stack)

	return f.Name()
}
