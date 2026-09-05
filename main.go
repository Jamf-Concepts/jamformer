// Copyright 2026, Jamf Software LLC

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/Jamf-Concepts/jamformer/compact"
	"github.com/Jamf-Concepts/jamformer/jsc"
	"github.com/Jamf-Concepts/jamformer/multienv"
	"github.com/Jamf-Concepts/jamformer/platform"
	platformclient "github.com/Jamf-Concepts/jamformer/platform/client"
	"github.com/Jamf-Concepts/jamformer/postprocess"
	"github.com/Jamf-Concepts/jamformer/pro"
	proclient "github.com/Jamf-Concepts/jamformer/pro/client"
	"github.com/Jamf-Concepts/jamformer/protect"
	protectclient "github.com/Jamf-Concepts/jamformer/protect/client"
	"github.com/Jamf-Concepts/jamformer/secrets"
	"github.com/Jamf-Concepts/jamformer/terraform"
	"github.com/Jamf-Concepts/jamformer/update"
)

// version, commit, and date are set at build time via -ldflags
// "-X main.version=... -X main.commit=... -X main.date=...".
// Defaults are used for local `go build` / `go run` without ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Terminal formatting sequences for interactive output.
// Empty by default (safe for non-interactive / CI); populated by enableColors().
var (
	uBold   string
	uDim    string
	uReset  string
	uGreen  string
	uYellow string
	uBlue   string
	uPurple string
	uCyan   string
)

func enableColors() {
	uBold = "\033[1m"
	uDim = "\033[2m"
	uReset = "\033[0m"
	uGreen = "\033[32m"
	uYellow = "\033[33m"
	uBlue = "\033[34m"
	uPurple = "\033[35m"
	uCyan = "\033[36m"
}

func printSplash() {
	const (
		gray     = "\033[38;5;245m"
		reset    = "\033[0m"
		bright   = "\033[1;97m"    // bold bright white for shimmer
		clearScr = "\033[2J\033[H" // clear visible screen, cursor to top-left (scrollback preserved)
		hideCur  = "\033[?25l"
		showCur  = "\033[?25h"
	)

	type rgb struct{ r, g, b int }
	blueC := rgb{29, 108, 232}
	purpleC := rgb{99, 56, 228}

	colorStr := func(c rgb) string {
		return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.r, c.g, c.b)
	}
	lerpColor := func(a, b rgb, t float64) string {
		if t <= 0 {
			return colorStr(a)
		}
		if t >= 1 {
			return colorStr(b)
		}
		return fmt.Sprintf("\033[38;2;%d;%d;%dm",
			a.r+int(float64(b.r-a.r)*t),
			a.g+int(float64(b.g-a.g)*t),
			a.b+int(float64(b.b-a.b)*t))
	}

	jamfBlue := colorStr(blueC)
	tfPurple := colorStr(purpleC)

	// ── Pixel maps (16 rows × 32 cols) ─────────────────────────────
	// Jamf logo: blue rounded square with white swoosh.
	jamfData := [16]string{
		"████████████████                ",
		"█████████████████               ",
		"█████████████████               ",
		"████████████████     ███████████",
		"████████████████   █████████████",
		"███████████████  ███████████████",
		"███████████████  ███████████████",
		"██████████████  ████████████████",
		"█████████████   ████████████████",
		" █████████     █████████████████",
		"               █████████████████",
		"              ██████████████████",
		"            ████████████████████",
		"         ███████████████████████",
		"████████████████████████████████",
		"████████████████████████████████",
	}

	// .tf file icon: rectangle with dog-ear, "tf" carved out.
	tfData := [16]string{
		"     ******************         ",
		"     ********************       ",
		"     **********************     ",
		"     **********************     ",
		"     **********************     ",
		"     ***       **       ***     ",
		"     *****  *****  ********     ",
		"     *****  *****      ****     ",
		"     *****  *****  ********     ",
		"     *****  *****  ********     ",
		"     **********************     ",
		"     **********************     ",
		"     **********************     ",
		"     **********************     ",
		"     **********************     ",
		"     **********************     ",
	}

	const (
		gridRows = 16
		gridCols = 32
	)

	// Parse into boolean grids
	var jamf, tf [gridRows][gridCols]bool
	for r := range gridRows {
		runes := []rune(jamfData[r])
		for c := range gridCols {
			if c < len(runes) {
				jamf[r][c] = runes[c] != ' '
			}
			if c < len(tfData[r]) {
				tf[r][c] = tfData[r][c] == '*'
			}
		}
	}

	const logoPad = "                       " // center 32-char art within ~78 cols

	// ── Rendering helpers ───────────────────────────────────────────

	// renderScaled draws a grid at a scaled size (tR rows × tC cols), centered
	// in the gridRows × gridCols display area, using nearest-neighbor sampling.
	renderScaled := func(grid *[gridRows][gridCols]bool, tR, tC int, color string) {
		var buf strings.Builder
		topPad := (gridRows - tR) / 2
		leftPad := (gridCols - tC) / 2
		for dr := range gridRows {
			buf.WriteString(logoPad)
			if dr < topPad || dr >= topPad+tR {
				buf.WriteString(strings.Repeat(" ", gridCols))
			} else {
				sr := (dr - topPad) * gridRows / tR
				buf.WriteString(strings.Repeat(" ", leftPad))
				buf.WriteString(color)
				for dc := range tC {
					sc := dc * gridCols / tC
					if grid[sr][sc] {
						buf.WriteRune('█')
					} else {
						buf.WriteRune(' ')
					}
				}
				buf.WriteString(reset)
				buf.WriteString(strings.Repeat(" ", gridCols-leftPad-tC))
			}
			buf.WriteByte('\n')
		}
		fmt.Print(buf.String())
	}

	fmt.Print(clearScr)
	fmt.Print(hideCur)

	// Version left, copyright right
	versionStr := "v" + version
	copyrightStr := "© Jamf Software LLC 2026"
	headerPad := max(78-len(versionStr)-len(copyrightStr), 1)
	fmt.Println(gray + versionStr + strings.Repeat(" ", headerPad) + copyrightStr + reset)

	// Print initial blank rows (reserve display area)
	for range gridRows {
		fmt.Println(logoPad + strings.Repeat(" ", gridCols))
	}

	// ── Zoom scale levels (rows, cols) ──────────────────────────────
	type zoomLevel struct{ rows, cols int }
	zoomIn := []zoomLevel{
		{2, 4}, {4, 8}, {6, 12}, {8, 16}, {10, 20}, {12, 24}, {14, 28}, {16, 32},
	}

	// ── Phase 1: Jamf logo reveal (full size, hold) ────────────────
	fmt.Printf("\033[%dA", gridRows)
	renderScaled(&jamf, gridRows, gridCols, jamfBlue)
	time.Sleep(800 * time.Millisecond)

	// ── Phase 2: Zoom out → spin → zoom in ─────────────────────────
	// 2a: Jamf logo zooms out (full → small)
	for i := len(zoomIn) - 1; i >= 0; i-- {
		fmt.Printf("\033[%dA", gridRows)
		renderScaled(&jamf, zoomIn[i].rows, zoomIn[i].cols, jamfBlue)
		time.Sleep(40 * time.Millisecond)
	}

	// 2b: Spin moment — horizontal line flash with sparkle
	sparkleChars := []rune{'✦', '✧', '·', '+', '∗'}
	for flash := range 3 {
		fmt.Printf("\033[%dA", gridRows)
		midColor := lerpColor(blueC, purpleC, float64(flash)/2.0)
		var buf strings.Builder
		for r := range gridRows {
			buf.WriteString(logoPad)
			if r == gridRows/2-1 || r == gridRows/2 {
				buf.WriteString(midColor)
				for range gridCols {
					buf.WriteRune('═')
				}
				buf.WriteString(reset)
			} else if flash >= 1 {
				// Sparkles radiate outward from center
				buf.WriteString(bright)
				for c := range gridCols {
					dist := (r-gridRows/2)*(r-gridRows/2) + (c-gridCols/2)*(c-gridCols/2)
					sparkleRadius := flash * 40
					if dist > sparkleRadius-15 && dist < sparkleRadius+15 {
						buf.WriteRune(sparkleChars[(r+c+flash)%len(sparkleChars)])
					} else {
						buf.WriteRune(' ')
					}
				}
				buf.WriteString(reset)
			} else {
				buf.WriteString(strings.Repeat(" ", gridCols))
			}
			buf.WriteByte('\n')
		}
		fmt.Print(buf.String())
		time.Sleep(60 * time.Millisecond)
	}

	// 2c: .tf icon zooms in (small → full)
	for _, z := range zoomIn {
		fmt.Printf("\033[%dA", gridRows)
		renderScaled(&tf, z.rows, z.cols, tfPurple)
		time.Sleep(45 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	// ── Phase 3: Fade .tf icon and overlay JAMFORMER text ───────────
	dimPurple := rgb{35, 20, 80}

	// Fade .tf icon to dim
	fadeSteps := 5
	for step := 1; step <= fadeSteps; step++ {
		fmt.Printf("\033[%dA", gridRows)
		t := float64(step) / float64(fadeSteps)
		fadeColor := lerpColor(purpleC, dimPurple, t)
		renderScaled(&tf, gridRows, gridCols, fadeColor)
		time.Sleep(60 * time.Millisecond)
	}

	dimColor := colorStr(dimPurple)

	// JAMFORMER art + footer, all composited over the dimmed icon
	const shimmerLines = 6 // only the art lines get the shimmer sweep
	rowColors := []string{
		"\033[38;2;180;200;245m", // silver-blue highlight
		"\033[38;2;29;108;232m",  // Jamf blue
		"\033[38;2;50;92;231m",   //
		"\033[38;2;71;76;230m",   //   ↓ gradient
		"\033[38;2;85;66;229m",   //
		"\033[38;2;99;56;228m",   // Terraform purple
		gray,                     // footer lines
		gray,
		gray,
	}
	artLines := []string{
		"     ██╗ █████╗ ███╗   ███╗███████╗ ██████╗ ██████╗ ███╗   ███╗███████╗██████╗",
		"     ██║██╔══██╗████╗ ████║██╔════╝██╔═══██╗██╔══██╗████╗ ████║██╔════╝██╔══██╗",
		"     ██║███████║██╔████╔██║█████╗  ██║   ██║██████╔╝██╔████╔██║█████╗  ██████╔╝",
		"██   ██║██╔══██║██║╚██╔╝██║██╔══╝  ██║   ██║██╔══██╗██║╚██╔╝██║██╔══╝  ██╔══██╗",
		"╚█████╔╝██║  ██║██║ ╚═╝ ██║██║     ╚██████╔╝██║  ██║██║ ╚═╝ ██║███████╗██║  ██║",
		" ╚════╝ ╚═╝  ╚═╝╚═╝     ╚═╝╚═╝      ╚═════╝ ╚═╝  ╚═╝╚═╝     ╚═╝╚══════╝╚═╝  ╚═╝",
		"                               Jamf → Terraform",
		"                  Export your Jamf config as Terraform HCL.",
		"                               by Jamf Concepts",
	}

	artRunes := make([][]rune, len(artLines))
	maxWidth := 0
	for i, line := range artLines {
		artRunes[i] = []rune(line)
		if len(artRunes[i]) > maxWidth {
			maxWidth = len(artRunes[i])
		}
	}

	numRows := len(artLines)               // 9 (6 art + 3 footer)
	textStartRow := gridRows - numRows - 3 // row 4 — less empty space at top
	_ = numRows                            // used only for textStartRow calc
	const compositeRows = gridRows         // render all 16 rows

	// renderComposite draws the dimmed .tf icon with text overlaid.
	// shimStart/shimEnd define the bright shimmer band (art lines only); use -1,-1 for none.
	renderComposite := func(shimStart, shimEnd int) {
		var buf strings.Builder
		for r := range compositeRows {
			textIdx := r - textStartRow
			if textIdx >= 0 && textIdx < numRows {
				// Composite row: text over dimmed icon
				runes := artRunes[textIdx]
				textColor := rowColors[textIdx]
				applyShimmer := textIdx < shimmerLines
				for c := 0; c < maxWidth; c++ {
					if c < len(runes) && runes[c] != ' ' {
						if applyShimmer && c >= shimStart && c < shimEnd {
							buf.WriteString(bright)
						} else {
							buf.WriteString(textColor)
						}
						buf.WriteRune(runes[c])
					} else {
						iconC := c - 23
						if iconC >= 0 && iconC < gridCols && tf[r][iconC] {
							buf.WriteString(dimColor)
							buf.WriteRune('█')
						} else {
							buf.WriteRune(' ')
						}
					}
				}
				buf.WriteString(reset)
			} else {
				// Non-text row: dimmed icon only
				buf.WriteString(logoPad)
				buf.WriteString(dimColor)
				for c := range gridCols {
					if tf[r][c] {
						buf.WriteRune('█')
					} else {
						buf.WriteRune(' ')
					}
				}
				buf.WriteString(reset)
			}
			buf.WriteByte('\n')
		}
		fmt.Print(buf.String())
	}

	// Initial composite (no shimmer)
	fmt.Printf("\033[%dA", compositeRows)
	renderComposite(-1, -1)

	// Shimmer sweep
	const (
		shimmerWidth = 8
		shimFrames   = 20
		shimDelay    = 20 * time.Millisecond
	)
	for frame := range shimFrames {
		pos := int(float64(frame) / float64(shimFrames-1) * float64(maxWidth+shimmerWidth))
		fmt.Printf("\033[%dA", compositeRows)
		renderComposite(pos-shimmerWidth, pos)
		time.Sleep(shimDelay)
	}

	// Final settled frame
	fmt.Printf("\033[%dA", compositeRows)
	renderComposite(-1, -1)

	fmt.Print(showCur)
	fmt.Println()
}

func printResourceList(provider string) {
	showPro := provider == "" || provider == "jamfpro"
	showProtect := provider == "" || provider == "jamfprotect"
	showPlatform := provider == "" || provider == "jamfplatform"
	showJSC := provider == "" || provider == "jsc"

	if !showPro && !showProtect && !showPlatform && !showJSC {
		fmt.Fprintf(os.Stderr, "Unknown provider %q. Valid providers: jamfplatform, jamfprotect, jsc, jamfpro\n", provider)
		os.Exit(1)
	}

	if showPlatform {
		fmt.Println("Jamf Platform (jamfplatform) [default]:")
		sorted := slices.Clone(platform.Resources)
		slices.SortFunc(sorted, func(a, b platform.ResourceDef) int {
			return strings.Compare(a.FilterKey, b.FilterKey)
		})
		for _, r := range sorted {
			fmt.Printf("  %-50s %s\n", r.FilterKey, r.TFType)
		}
		if showProtect || showJSC || showPro {
			fmt.Println()
		}
	}
	if showProtect {
		fmt.Println("Jamf Protect (jamfprotect):")
		sorted := slices.Clone(protect.Resources)
		slices.SortFunc(sorted, func(a, b protect.ResourceDef) int {
			return strings.Compare(a.FilterKey, b.FilterKey)
		})
		for _, r := range sorted {
			fmt.Printf("  %-50s %s\n", r.FilterKey, r.TFType)
		}
		if showJSC || showPro {
			fmt.Println()
		}
	}
	if showJSC {
		fmt.Println("JSC - Jamf Security Cloud (jsc):")
		sorted := slices.Clone(jsc.Resources)
		slices.SortFunc(sorted, func(a, b jsc.ResourceDef) int {
			return strings.Compare(a.FilterKey, b.FilterKey)
		})
		for _, r := range sorted {
			fmt.Printf("  %-50s %s\n", r.FilterKey, r.TFType)
		}
		if showPro {
			fmt.Println()
		}
	}
	if showPro {
		fmt.Println("Jamf Pro (jamfpro) - community provider by Deployment Theory:")
		sorted := slices.Clone(pro.Resources)
		slices.SortFunc(sorted, func(a, b pro.ResourceDef) int {
			return strings.Compare(a.FilterKey, b.FilterKey)
		})
		for _, r := range sorted {
			fmt.Printf("  %-50s %s\n", r.FilterKey, r.TFType)
		}
	}
}

func main() {
	// Intercept -help <topic> before flag.Parse (which treats -help specially)
	if checkHelpArgs() {
		os.Exit(0)
	}

	// Override default flag.Usage to include help topics hint
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jamformer [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nFor detailed help on a topic:\n  jamformer -help <topic>\n\nAvailable topics:\n")
		printHelpTopicList()
	}

	url := flag.String("url", "", "Jamf instance URL (e.g. https://yourinstance.jamfcloud.com) [env: JAMF_URL]")

	// Credentials are sourced from environment variables or interactive prompts only
	// (not CLI flags) to avoid leaking secrets in shell history and process listings.
	username := new(string)
	password := new(string)
	clientID := new(string)
	clientSecret := new(string)
	// Jamf Platform scope. An API integration targets exactly one of an
	// environment or a tenant, or neither for organization scope; the two IDs
	// are mutually exclusive and ResolveScope enforces that.
	environmentID := new(string) // JAMF_ENVIRONMENT_ID / JAMFPLATFORM_ENVIRONMENT_ID
	tenantID := new(string)      // JAMF_TENANT_ID / JAMFPLATFORM_TENANT_ID (legacy)
	platformScope := platform.Scope{}
	terraformPath := flag.String("terraform-path", "", "Path to terraform binary (skip auto-download) [env: JAMFORMER_TERRAFORM_PATH]")
	outputDir := flag.String("output", "generated", "Output directory for generated Terraform project [env: JAMFORMER_OUTPUT]")
	verbose := flag.Bool("verbose", false, "Show terraform command output [env: JAMFORMER_VERBOSE]")
	skipPackageDownloads := flag.Bool("skip-package-downloads", false, "Skip downloading package files from the Cloud Distribution Point [env: JAMFORMER_SKIP_PACKAGE_DOWNLOADS]")
	resourcesFlag := flag.String("include-resources", "", "Space-separated list of resource types to include (see -help filtering) [env: JAMFORMER_RESOURCES]")
	excludeFlag := flag.String("exclude-resources", "", "Space-separated list of resource types to exclude (see -help filtering) [env: JAMFORMER_EXCLUDE]")
	skipReferences := flag.Bool("skip-references", false, "Skip cross-resource reference resolution (leave raw ID values) [env: JAMFORMER_SKIP_REFERENCES]")
	skipImportBlocks := flag.Bool("skip-import-blocks", false, "Exclude import blocks from output (for applying to a new instance) [env: JAMFORMER_SKIP_IMPORT_BLOCKS]")
	providerFlag := flag.String("provider", "", "Terraform provider: jamfplatform (default), jamfprotect, jsc, or jamfpro [env: JAMFORMER_PROVIDER]")
	providerVersionFlag := flag.String("provider-version", "", "Pin a specific provider version (see -help provider-version) [env: JAMFORMER_PROVIDER_VERSION]")
	allowDevOverrides := flag.Bool("allow-dev-overrides", false, "Allow Terraform provider dev_overrides from CLI config (see -help dev-overrides) [env: JAMFORMER_ALLOW_DEV_OVERRIDES]")
	compactMode := flag.Bool("compact", false, "Consolidate simple resource types into for_each patterns (see -help compact) [env: JAMFORMER_COMPACT]")
	compactIncludeFlag := flag.String("compact-include", "", "Space-separated list of resource types to compact (default: all eligible) [env: JAMFORMER_COMPACT_INCLUDE]")
	compactExcludeFlag := flag.String("compact-exclude", "", "Space-separated list of resource types to exclude from compaction [env: JAMFORMER_COMPACT_EXCLUDE]")
	splitByCategory := flag.Bool("split-by-category", false, "Split categorised resource types into per-category output files [env: JAMFORMER_SPLIT_BY_CATEGORY]")
	skipSecretScan := flag.Bool("skip-secret-scan", false, "Skip secret scanning of generated output (see -help secrets) [env: JAMFORMER_SKIP_SECRET_SCAN]")
	parallelismFlag := flag.Int("parallelism", 1, "Number of concurrent Terraform provider reads during config generation [env: JAMFORMER_PARALLELISM]")
	multiEnvFlag := flag.String("multi-env", "", "Multi-env export with module + branch structure (see -help multi-env) [env: JAMFORMER_MULTI_ENV]")
	sourceEnvFlag := flag.String("source-env", "", "Source-of-truth environment (default: first in -multi-env list) [env: JAMFORMER_SOURCE_ENV]")
	listResources := flag.Bool("list-resources", false, "List supported resource types and exit. Filter by -provider to show a specific provider.")
	showCredits := flag.Bool("credits", false, "Show credits and acknowledgements")
	showVersion := flag.Bool("version", false, "Print version and exit")
	showVersionShort := flag.Bool("v", false, "Print version and exit")
	flag.Parse()

	// Handle -version / -v early exit
	if *showVersion || *showVersionShort {
		fmt.Printf("jamformer %s\n  commit: %s\n  built:  %s\n  install: %s\n",
			version, commit, date, update.DetectInstallMethod())
		os.Exit(0)
	}

	// Handle -credits early exit
	if *showCredits {
		printCredits()
		os.Exit(0)
	}

	// Handle -list-resources early exit
	if *listResources {
		printResourceList(*providerFlag)
		os.Exit(0)
	}

	// Start the update check now so its latency overlaps the prompting and
	// authentication that follow. It is advisory and never blocks a run: the
	// result is collected with a short grace period later, and a check that has
	// not finished by then is dropped.
	updateCheck := startUpdateCheck()

	// Apply environment variable defaults for unset flags
	if *url == "" {
		*url = os.Getenv("JAMF_URL")
	}

	// Credentials from environment variables (or interactive prompts below)
	*username = os.Getenv("JAMF_USERNAME")
	*password = os.Getenv("JAMF_PASSWORD")
	*clientID = os.Getenv("JAMF_CLIENT_ID")
	*clientSecret = os.Getenv("JAMF_CLIENT_SECRET")
	*environmentID = os.Getenv("JAMF_ENVIRONMENT_ID")
	*tenantID = os.Getenv("JAMF_TENANT_ID")
	// The Jamf Platform provider reads its own JAMFPLATFORM_* variables. Accept
	// them as fallbacks so a shell already set up to run `terraform` against a
	// tenant needs no second set of exports; the JAMF_* name wins where both
	// are present.
	if isPlatformProvider(*providerFlag) {
		if *clientID == "" {
			*clientID = os.Getenv("JAMFPLATFORM_CLIENT_ID")
		}
		if *clientSecret == "" {
			*clientSecret = os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
		}
		if *url == "" {
			*url = os.Getenv("JAMFPLATFORM_BASE_URL")
		}
	}
	if !*verbose && os.Getenv("JAMFORMER_VERBOSE") == "true" {
		*verbose = true
	}
	if !*skipPackageDownloads && os.Getenv("JAMFORMER_SKIP_PACKAGE_DOWNLOADS") == "true" {
		*skipPackageDownloads = true
	}
	if *terraformPath == "" {
		*terraformPath = os.Getenv("JAMFORMER_TERRAFORM_PATH")
	}
	if *resourcesFlag == "" {
		*resourcesFlag = os.Getenv("JAMFORMER_RESOURCES")
	}
	if *excludeFlag == "" {
		*excludeFlag = os.Getenv("JAMFORMER_EXCLUDE")
	}
	if !*skipReferences && os.Getenv("JAMFORMER_SKIP_REFERENCES") == "true" {
		*skipReferences = true
	}
	if !*skipImportBlocks && os.Getenv("JAMFORMER_SKIP_IMPORT_BLOCKS") == "true" {
		*skipImportBlocks = true
	}
	if *providerFlag == "" {
		*providerFlag = os.Getenv("JAMFORMER_PROVIDER")
	}
	if *providerVersionFlag == "" {
		*providerVersionFlag = os.Getenv("JAMFORMER_PROVIDER_VERSION")
	}
	if !*allowDevOverrides && os.Getenv("JAMFORMER_ALLOW_DEV_OVERRIDES") == "true" {
		*allowDevOverrides = true
	}
	if !*skipSecretScan && os.Getenv("JAMFORMER_SKIP_SECRET_SCAN") == "true" {
		*skipSecretScan = true
	}
	if !*splitByCategory && os.Getenv("JAMFORMER_SPLIT_BY_CATEGORY") == "true" {
		*splitByCategory = true
	}
	if !*compactMode && os.Getenv("JAMFORMER_COMPACT") == "true" {
		*compactMode = true
	}
	if *compactIncludeFlag == "" {
		*compactIncludeFlag = os.Getenv("JAMFORMER_COMPACT_INCLUDE")
	}
	if *compactExcludeFlag == "" {
		*compactExcludeFlag = os.Getenv("JAMFORMER_COMPACT_EXCLUDE")
	}
	if *multiEnvFlag == "" {
		*multiEnvFlag = os.Getenv("JAMFORMER_MULTI_ENV")
	}
	if *sourceEnvFlag == "" {
		*sourceEnvFlag = os.Getenv("JAMFORMER_SOURCE_ENV")
	}
	if *parallelismFlag == 1 {
		if v := os.Getenv("JAMFORMER_PARALLELISM"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*parallelismFlag = n
			}
		}
	}

	// Validate provider flag if explicitly set
	if *providerFlag != "" && *providerFlag != "jamfpro" && *providerFlag != "jamfprotect" && *providerFlag != "jamfplatform" && *providerFlag != "jsc" {
		log.Fatalf("Invalid provider %q. Valid options: jamfplatform, jamfprotect, jsc, jamfpro", *providerFlag)
	}

	// Parse and validate multi-env flag
	var multiEnvNames []string
	if *multiEnvFlag != "" {
		raw := strings.ReplaceAll(*multiEnvFlag, ",", " ")
		for name := range strings.FieldsSeq(raw) {
			multiEnvNames = append(multiEnvNames, strings.ToLower(strings.TrimSpace(name)))
		}
		if len(multiEnvNames) < 1 {
			log.Fatal("-multi-env requires at least one environment name")
		}
		// Check for duplicates
		seen := make(map[string]bool)
		for _, name := range multiEnvNames {
			if seen[name] {
				log.Fatalf("Duplicate environment name %q in -multi-env", name)
			}
			seen[name] = true
		}
		// Multi-env supports the jamfplatform and jamfpro providers (those with a
		// discovery/generate split). Protect and JSC are not supported. An unset
		// provider follows the global default (jamfplatform).
		switch *providerFlag {
		case "":
			*providerFlag = "jamfplatform"
		case "jamfplatform", "jamfpro":
			// supported
		default:
			log.Fatalf("-multi-env supports only the jamfplatform and jamfpro providers (got %q)", *providerFlag)
		}
		if *compactMode {
			log.Fatal("-multi-env and -compact cannot be used together")
		}
	}
	isMultiEnv := len(multiEnvNames) > 0

	// Detect whether we're running interactively (needed early for provider prompt)
	interactive := term.IsTerminal(int(syscall.Stdin))
	if interactive {
		enableColors()
	}

	// Prompt for provider if not set via flag or env
	splashShown := false
	if *providerFlag == "" {
		if interactive {
			printSplash()
			splashShown = true
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("  %s[L]%s Jamf Platform %s(default)%s\n", uCyan, uReset, uDim, uReset)
			fmt.Printf("  %s[T]%s Jamf Protect\n", uPurple, uReset)
			fmt.Printf("  %s[S]%s JSC (Jamf Security Cloud)\n", uYellow, uReset)
			fmt.Printf("  %s[P]%s Jamf Pro %s(community provider by Deployment Theory)%s\n", uBlue, uReset, uDim, uReset)
			fmt.Println()
			choice := promptLine(reader, fmt.Sprintf("%sChoose provider%s %s(L/t/s/p)%s: ", uBold, uReset, uDim, uReset))
			choice = strings.ToLower(strings.TrimSpace(choice))
			switch choice {
			case "t":
				*providerFlag = "jamfprotect"
			case "s":
				*providerFlag = "jsc"
			case "p":
				*providerFlag = "jamfpro"
			default:
				*providerFlag = "jamfplatform"
			}
		} else {
			*providerFlag = "jamfplatform"
		}
	}

	isProtect := *providerFlag == "jamfprotect"
	isPlatform := *providerFlag == "jamfplatform"
	isJSC := *providerFlag == "jsc"

	// Validate that -include-resources and -exclude-resources are not used together
	if *resourcesFlag != "" && *excludeFlag != "" {
		log.Fatal("Cannot use both -include-resources and -exclude-resources at the same time")
	}

	// Select the right resource name map based on provider
	nameMap := pro.ValidFilterNames()
	if isProtect {
		nameMap = protect.ValidFilterNames()
	} else if isPlatform {
		nameMap = platform.ValidFilterNames()
	} else if isJSC {
		nameMap = jsc.ValidFilterNames()
	}

	// Parse selected resource types
	selectedResources := parseResourceFilter(*resourcesFlag, nameMap)
	if selectedResources == nil && *excludeFlag != "" {
		excluded := parseResourceFilter(*excludeFlag, nameMap)
		if excluded != nil {
			selectedResources = make(map[string]bool)
			for _, canonical := range nameMap {
				if !excluded[canonical] {
					selectedResources[canonical] = true
				}
			}
		}
	}
	if *outputDir == "generated" {
		if v := os.Getenv("JAMFORMER_OUTPUT"); v != "" {
			*outputDir = v
		}
	}

	// Determine auth method based on provided credentials
	// (skipped in multi-env mode — credentials are resolved per-env)
	authMethod := ""
	if isMultiEnv {
		authMethod = "multienv" // placeholder to skip single-env credential flow
	} else if *clientID != "" || *clientSecret != "" {
		if isJSC {
			log.Fatal("JSC requires username and password authentication (JAMF_USERNAME and JAMF_PASSWORD)")
		}
		authMethod = "oauth2"
	}
	if *username != "" || *password != "" {
		if isProtect {
			log.Fatal("Jamf Protect only supports OAuth2 authentication (JAMF_CLIENT_ID and JAMF_CLIENT_SECRET)")
		}
		if isPlatform {
			log.Fatal("Jamf Platform only supports OAuth2 authentication (JAMF_CLIENT_ID and JAMF_CLIENT_SECRET)")
		}
		if authMethod == "oauth2" {
			log.Fatal("Cannot use both basic auth (JAMF_USERNAME/JAMF_PASSWORD) and OAuth2 (JAMF_CLIENT_ID/JAMF_CLIENT_SECRET) at the same time")
		}
		authMethod = "basic"
	}

	quiet := interactive && !*verbose

	if interactive && !splashShown {
		printSplash()
	}

	// Advise on a newer release before the export starts, so the user can stop
	// and upgrade rather than discover it afterwards.
	reportUpdateNotice(updateCheck)

	// Interactive prompts if required fields are missing
	// (skipped in multi-env mode — credentials are resolved per-env)
	if !isMultiEnv && (*url == "" || authMethod == "" || (isPlatform && needsScopePrompt(*environmentID, *tenantID))) {
		if !interactive {
			var missing []string
			if *url == "" {
				missing = append(missing, "-url / JAMF_URL")
			}
			if authMethod == "" {
				if isProtect || isPlatform {
					missing = append(missing, "JAMF_CLIENT_ID + JAMF_CLIENT_SECRET")
				} else if isJSC {
					missing = append(missing, "JAMF_USERNAME + JAMF_PASSWORD")
				} else {
					missing = append(missing, "JAMF_USERNAME + JAMF_PASSWORD (or JAMF_CLIENT_ID + JAMF_CLIENT_SECRET)")
				}
			}
			log.Fatalf("Missing required credentials in non-interactive mode: %s\nSet via environment variables (or -url flag for URL).", strings.Join(missing, ", "))
		}

		reader := bufio.NewReader(os.Stdin)

		if *url == "" {
			switch {
			case isPlatform:
				*url = promptLine(reader, fmt.Sprintf("Jamf Platform API gateway host %s(e.g. https://us.api.jamfcloud.com, or eu. / apac.)%s: ", uDim, uReset))
			case isProtect:
				*url = promptLine(reader, fmt.Sprintf("Jamf Protect URL %s(e.g. your-tenant.protect.jamfcloud.com)%s: ", uDim, uReset))
			case isJSC:
				*url = promptLine(reader, fmt.Sprintf("JSC URL %s(default: radar.wandera.com)%s: ", uDim, uReset))
				if *url == "" {
					*url = "radar.wandera.com"
				}
			default:
				*url = promptLine(reader, fmt.Sprintf("Jamf Pro URL %s(e.g. yourinstance.jamfcloud.com)%s: ", uDim, uReset))
			}
			if *url == "" && !isJSC {
				log.Fatal("URL is required")
			}
		}

		if authMethod == "" {
			if isProtect || isPlatform {
				// Protect and Platform only support OAuth2
				authMethod = "oauth2"
				if *clientID == "" {
					*clientID = promptLine(reader, "API Client ID: ")
				}
				if *clientSecret == "" {
					*clientSecret = promptPassword("API Client Secret: ")
				}
			} else {
				authMethod = "basic"
				if *username == "" {
					*username = promptLine(reader, "Username: ")
				}
				if *password == "" {
					*password = promptPassword("Password: ")
				}
			}
		}

		// Scope. An integration is registered at exactly one of three scopes and
		// cannot be re-scoped afterwards, so this asks which one rather than
		// assuming, and accepts an empty answer as organization scope.
		if isPlatform && *environmentID == "" && *tenantID == "" &&
			os.Getenv("JAMFPLATFORM_ORGANIZATION_ID") == "" && os.Getenv("JAMF_ORGANIZATION_ID") == "" {
			fmt.Printf("\n%sAPI integration scope%s — chosen when the integration was created in Jamf Account.\n", uBold, uReset)
			fmt.Printf("  %sEnvironment%s (preferred) reaches everything except Jamf Account.\n", uBold, uReset)
			fmt.Printf("  %sTenant%s (legacy) reaches Jamf Pro and Security Cloud, but not Blueprints,\n", uBold, uReset)
			fmt.Printf("  Compliance Benchmarks or AI Governance.\n")
			fmt.Printf("  %sOrganization%s reaches only the Jamf Account SSO resources. Leave both blank for it.\n\n", uBold, uReset)
			*environmentID = promptLine(reader, fmt.Sprintf("Environment ID %s(blank if not environment-scoped)%s: ", uDim, uReset))
			if *environmentID == "" {
				*tenantID = promptLine(reader, fmt.Sprintf("Tenant ID %s(blank for organization scope)%s: ", uDim, uReset))
			}
		}
	}

	// Normalize URL — expand shorthand instance names and ensure https:// prefix
	// (skipped in multi-env mode — URLs are normalized per-env in resolveMultiEnvCredentials)
	if !isMultiEnv {
		*url = strings.TrimRight(*url, "/")
		if !isPlatform && !isJSC && !strings.Contains(*url, ".") && !strings.Contains(*url, "://") {
			if isProtect {
				*url = *url + ".protect.jamfcloud.com"
			} else {
				*url = *url + ".jamfcloud.com"
			}
		}
		if !strings.HasPrefix(*url, "https://") && !strings.HasPrefix(*url, "http://") {
			*url = "https://" + *url
		}
	}

	// Validate required fields
	switch authMethod {
	case "basic":
		if *username == "" || *password == "" {
			log.Fatal("Basic auth requires username and password")
		}
	case "oauth2":
		if *clientID == "" || *clientSecret == "" {
			log.Fatal("OAuth2 auth requires client ID and client secret")
		}
	case "multienv":
		// Credentials are resolved per-env later
	default:
		log.Fatal("No authentication credentials provided")
	}
	if !isMultiEnv && isPlatform {
		var scopeErr error
		platformScope, scopeErr = platform.ResolveScope(*environmentID, *tenantID)
		if scopeErr != nil {
			log.Fatalf("%v", scopeErr)
		}
		reportPlatformScope(platformScope, selectedResources, quiet)
	}

	// Verify authentication before proceeding with interactive prompts / terraform download
	// (skipped in multi-env mode — each env is verified during its pipeline run)
	var skipDataForwarding bool
	if isMultiEnv {
		// Auth verification happens per-env in the multienv pipeline
	} else if isProtect {
		if interactive && !*verbose {
			fmt.Print("Verifying authentication... ")
		} else if !interactive || *verbose {
			fmt.Println("Verifying authentication...")
		}
		protectClient, err := protectclient.VerifyAuth(*url, *clientID, *clientSecret)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
		configured, err := protectclient.IsDataForwardingConfigured(context.Background(), protectClient)
		if err != nil && (*verbose || !interactive) {
			fmt.Printf("Warning: could not check data forwarding status: %v\n", err)
		}
		skipDataForwarding = !configured
	} else if isPlatform {
		if interactive && !*verbose {
			fmt.Print("Verifying authentication... ")
		} else if !interactive || *verbose {
			fmt.Println("Verifying authentication...")
		}
		if err := platformclient.VerifyAuth(*url, *clientID, *clientSecret,
			platformclient.Scope{EnvironmentID: platformScope.EnvironmentID(), TenantID: platformScope.TenantID()}); err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
	} else if isJSC {
		// JSC auth is verified during terraform init/apply
	} else {
		if interactive && !*verbose {
			fmt.Print("Verifying authentication... ")
		} else if !interactive || *verbose {
			fmt.Println("Verifying authentication...")
		}
		if _, err := proclient.VerifyAuth(&proclient.AuthConfig{
			URL:          *url,
			AuthMethod:   authMethod,
			Username:     *username,
			Password:     *password,
			ClientID:     *clientID,
			ClientSecret: *clientSecret,
		}); err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
	}
	if !isMultiEnv && interactive && !*verbose {
		fmt.Printf("%s✓%s\n", uGreen, uReset)
	}

	absOutput, err := filepath.Abs(*outputDir)
	if err != nil {
		log.Fatalf("Failed to resolve output path: %v", err)
	}

	// 0. Ensure terraform is available
	// Use a signal-aware context so Ctrl+C cancels in-flight terraform commands,
	// which causes tfexec to kill the terraform process and its provider subprocesses.
	sigCtx, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer sigCancel()
	terraform.Ctx = sigCtx

	terraform.Quiet = quiet
	terraform.AllowDevOverrides = *allowDevOverrides
	terraform.Parallelism = *parallelismFlag
	var tfPath string
	if *terraformPath != "" {
		tfPath = *terraformPath
	} else {
		var err error
		tfPath, err = terraform.EnsureTerraform()
		if err != nil {
			log.Fatalf("Failed to ensure terraform: %v", err)
		}
	}
	terraform.SetTerraformPath(tfPath)

	// Interactive resource selection when no -include-resources or -exclude-resources flag was provided
	if interactive && selectedResources == nil && *resourcesFlag == "" && *excludeFlag == "" {
		reader := bufio.NewReader(os.Stdin)
		choice := promptLine(reader, fmt.Sprintf("\nGenerate config for all resources? %s[Y/n]%s: ", uDim, uReset))
		choice = strings.ToLower(strings.TrimSpace(choice))

		if choice == "n" || choice == "no" {
			selectedResources = make(map[string]bool)
			fmt.Println("\nSelect resources to include:")
			switch {
			case isPlatform:
				for _, rt := range platform.Resources {
					answer := promptLine(reader, fmt.Sprintf("  %s? %s[y/N]%s: ", rt.DisplayName, uDim, uReset))
					answer = strings.ToLower(strings.TrimSpace(answer))
					if answer == "y" || answer == "yes" {
						selectedResources[rt.FilterKey] = true
					}
				}
			case isProtect:
				for _, rt := range protect.Resources {
					answer := promptLine(reader, fmt.Sprintf("  %s? %s[y/N]%s: ", rt.DisplayName, uDim, uReset))
					answer = strings.ToLower(strings.TrimSpace(answer))
					if answer == "y" || answer == "yes" {
						selectedResources[rt.FilterKey] = true
					}
				}
			case isJSC:
				for _, rt := range jsc.Resources {
					answer := promptLine(reader, fmt.Sprintf("  %s? [y/N]: ", rt.DisplayName))
					answer = strings.ToLower(strings.TrimSpace(answer))
					if answer == "y" || answer == "yes" {
						selectedResources[rt.FilterKey] = true
					}
				}
			default:
				for _, rt := range pro.Resources {
					answer := promptLine(reader, fmt.Sprintf("  %s? %s[y/N]%s: ", rt.DisplayName, uDim, uReset))
					answer = strings.ToLower(strings.TrimSpace(answer))
					if answer == "y" || answer == "yes" {
						selectedResources[rt.FilterKey] = true
					}
				}
			}

			if len(selectedResources) == 0 {
				log.Fatal("No resources selected")
			}

			fmt.Println()
		}
	}

	// Prompt to skip package downloads if packages are included (Jamf Pro and
	// Jamf Platform). Pro filters on the "packages" key; Platform on "package".
	pkgFilterKey := "packages"
	if isPlatform {
		pkgFilterKey = "package"
	}
	if !isProtect && !isJSC && interactive && !*skipPackageDownloads && (selectedResources == nil || selectedResources[pkgFilterKey]) {
		reader := bufio.NewReader(os.Stdin)
		prompt := "Download package files from the Cloud Distribution Point?"
		if isPlatform {
			prompt = "Download package files from the Jamf Cloud Distribution Service?"
		}
		answer := promptLine(reader, fmt.Sprintf("%s %s(can be slow/large) [y/N]%s: ", prompt, uDim, uReset))
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			*skipPackageDownloads = true
		}
	}

	// Prompt to split output files by category (Jamf Pro and Jamf Platform —
	// both have categories; Protect and JSC do not)
	if !isProtect && !isJSC && interactive && !*splitByCategory {
		reader := bufio.NewReader(os.Stdin)
		answer := promptLine(reader, fmt.Sprintf("Split output files by category? %s(e.g. policies_production.tf) [y/N]%s: ", uDim, uReset))
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "y" || answer == "yes" {
			*splitByCategory = true
		}
	}

	// Check for existing output directory
	if info, err := os.Stat(absOutput); err == nil && info.IsDir() {
		if interactive {
			reader := bufio.NewReader(os.Stdin)
			answer := promptLine(reader, fmt.Sprintf("Output directory %s%s%s already exists. Delete and replace? %s[y/N]%s: ", uBold, absOutput, uReset, uDim, uReset))
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				log.Fatal("Aborted — output directory already exists")
			}
		}
		if err := os.RemoveAll(absOutput); err != nil {
			log.Fatalf("Failed to remove existing output directory: %v", err)
		}
	}

	// Start spinner in quiet mode
	var spin *spinner
	if quiet {
		spin = startSpinner()
	}

	// Run the provider-specific pipeline
	var pipelineErr error
	var fixResult *postprocess.FixResult
	if isMultiEnv {
		// Multi-env mode: resolve per-env credentials and run the merge pipeline
		envConfigs, err := resolveMultiEnvCredentials(*providerFlag, multiEnvNames, interactive)
		if err != nil {
			log.Fatalf("Multi-env credential resolution failed: %v", err)
		}
		multienvOpts := &multienv.Options{
			Provider:             *providerFlag,
			Envs:                 envConfigs,
			SourceEnv:            *sourceEnvFlag,
			OutputDir:            absOutput,
			SelectedResources:    selectedResources,
			SkipReferences:       *skipReferences,
			SkipImportBlocks:     *skipImportBlocks,
			SkipPackageDownloads: *skipPackageDownloads,
			ProviderVersion:      *providerVersionFlag,
			Quiet:                quiet,
			Verbose:              *verbose,
			ResourcesFlag:        *resourcesFlag,
			ExcludeFlag:          *excludeFlag,
		}
		if spin != nil {
			multienvOpts.StatusFunc = spin.Update
		}
		multienv.Quiet = quiet
		pipelineErr = multienv.RunPipeline(multienvOpts)
	} else if isPlatform {
		platformOpts := &platform.PipelineOptions{
			OutputDir:            absOutput,
			BaseURL:              *url,
			ClientID:             *clientID,
			ClientSecret:         *clientSecret,
			Scope:                platformScope,
			SelectedResources:    selectedResources,
			SkipReferences:       *skipReferences,
			SkipPackageDownloads: *skipPackageDownloads,
			SplitByCategory:      *splitByCategory,
			ProviderVersion:      *providerVersionFlag,
			Quiet:                quiet,
			Verbose:              *verbose,
		}
		if spin != nil {
			platformOpts.StatusFunc = spin.Update
		}
		fixResult, pipelineErr = platform.RunPipeline(platformOpts)
	} else if isProtect {
		protectOpts := &protect.PipelineOptions{
			OutputDir:          absOutput,
			URL:                *url,
			ClientID:           *clientID,
			ClientSecret:       *clientSecret,
			SelectedResources:  selectedResources,
			SkipReferences:     *skipReferences,
			ProviderVersion:    *providerVersionFlag,
			Quiet:              quiet,
			Verbose:            *verbose,
			SkipDataForwarding: skipDataForwarding,
		}
		if spin != nil {
			protectOpts.StatusFunc = spin.Update
		}
		fixResult, pipelineErr = protect.RunPipeline(protectOpts)
	} else if isJSC {
		jscOpts := &jsc.PipelineOptions{
			OutputDir:         absOutput,
			URL:               *url,
			Username:          *username,
			Password:          *password,
			SelectedResources: selectedResources,
			SkipReferences:    *skipReferences,
			ProviderVersion:   *providerVersionFlag,
			Quiet:             quiet,
			Verbose:           *verbose,
		}
		if spin != nil {
			jscOpts.StatusFunc = spin.Update
		}
		fixResult, pipelineErr = jsc.RunPipeline(jscOpts)
	} else {
		proOpts := &pro.PipelineOptions{
			OutputDir:            absOutput,
			URL:                  *url,
			AuthMethod:           authMethod,
			Username:             *username,
			Password:             *password,
			ClientID:             *clientID,
			ClientSecret:         *clientSecret,
			SelectedResources:    selectedResources,
			SkipReferences:       *skipReferences,
			SplitByCategory:      *splitByCategory,
			SkipPackageDownloads: *skipPackageDownloads,
			ProviderVersion:      *providerVersionFlag,
			Quiet:                quiet,
			Verbose:              *verbose,
			ResourcesFlag:        *resourcesFlag,
			ExcludeFlag:          *excludeFlag,
		}
		if spin != nil {
			proOpts.StatusFunc = spin.Update
		}
		fixResult, pipelineErr = pro.RunPipeline(proOpts)
	}

	if spin != nil {
		spin.Stop()
	}
	if pipelineErr != nil {
		if sigCtx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Interrupted.")
			os.Exit(130)
		}
		log.Fatalf("Pipeline failed: %v", pipelineErr)
	}

	// Remove import blocks if requested (multi-env handles this internally)
	if *skipImportBlocks && !isMultiEnv {
		importFiles, _ := filepath.Glob(filepath.Join(absOutput, "*_import.tf"))
		for _, f := range importFiles {
			_ = os.Remove(f)
		}
		if !quiet {
			fmt.Println("  Removed import blocks (-skip-import-blocks)")
		}
	}

	// Compact simple resource types into for_each patterns
	if *compactMode {
		compact.Quiet = quiet
		if !quiet {
			fmt.Println("Compacting simple resources into for_each patterns...")
		}
		compactOpts := &compact.Options{
			Include: parseStringSet(*compactIncludeFlag),
			Exclude: parseStringSet(*compactExcludeFlag),
		}
		if err := compact.Run(absOutput, compactOpts); err != nil {
			log.Fatalf("Compaction failed: %v", err)
		}
	}

	// Scan for secrets in generated output
	if !*skipSecretScan {
		findings, scanErr := secrets.Scan(absOutput, quiet)
		if scanErr != nil {
			fmt.Printf("\n%s!%s  Secret scan failed: %v\n", uYellow, uReset, scanErr)
		} else if len(findings) > 0 {
			reportOpts := secrets.ReportOptions{
				Bold:   uBold,
				Dim:    uDim,
				Yellow: uYellow,
				Reset:  uReset,
			}
			secrets.PrintReport(findings, reportOpts)

			// Offer to move secrets to sensitive variables
			if interactive {
				reader := bufio.NewReader(os.Stdin)
				answer := promptLine(reader, fmt.Sprintf("  Remediate secrets? %s[a]%sll / %s[s]%select individually / %s[N]%sone: ",
					uBold, uReset, uBold, uReset, uBold, uReset))
				answer = strings.ToLower(strings.TrimSpace(answer))

				var selected []secrets.Finding
				switch answer {
				case "a", "all":
					selected = findings
				case "s", "select":
					for i, f := range findings {
						desc := findingDescription(f)
						choice := promptLine(reader, fmt.Sprintf("    %s[%d/%d]%s %s %s[y/N]%s: ",
							uDim, i+1, len(findings), uReset, desc, uDim, uReset))
						choice = strings.ToLower(strings.TrimSpace(choice))
						if choice == "y" || choice == "yes" {
							selected = append(selected, f)
						}
					}
				}

				if len(selected) > 0 {
					result, remErr := secrets.Remediate(absOutput, selected)
					if remErr != nil {
						fmt.Printf("\n  %s!%s  Remediation failed: %v\n", uYellow, uReset, remErr)
					} else {
						terraform.FormatDir(absOutput)
						fmt.Printf("\n  %s✓%s  Moved %s%d%s secret(s) to sensitive variables\n",
							uGreen, uReset, uBold, result.VariablesAdded, uReset)
						if result.SupportFiles > 0 {
							fmt.Printf("    %s%d support file(s) converted to templatefile()%s\n",
								uDim, result.SupportFiles, uReset)
						}
						if result.Skipped > 0 {
							fmt.Printf("    %s%s!%s %d secret(s) skipped (embedded in larger strings — remediate manually)%s\n",
								uYellow, uBold, uReset, result.Skipped, uReset)
						}
						fmt.Printf("    Updated: %svariables.tf%s, %sterraform.tfvars%s\n",
							uBold, uReset, uBold, uReset)
					}
				}
			}
		} else if !quiet {
			fmt.Printf("\n%s✓%s  No secrets detected in generated output\n", uGreen, uReset)
		}
	}

	// Print summary
	resources, types, files := summarizeOutput(absOutput)
	fmt.Println()
	if resources > 0 {
		fmt.Printf("%s✓%s %s%d%s resources across %s%d%s types in %s%d%s files\n",
			uGreen, uReset, uBold, resources, uReset, uBold, types, uReset, uBold, files, uReset)
	} else {
		fmt.Printf("%s✓%s Terraform project generated\n", uGreen, uReset)
	}
	fmt.Printf("  %sOutput:%s %s\n", uDim, uReset, absOutput)

	// Show validation fix summary
	if fixResult != nil && (fixResult.Fixed > 0 || len(fixResult.RequiredVars) > 0) {
		if fixResult.Fixed > 0 {
			fmt.Printf("  %sAuto-fixed %d invalid attribute(s) removed by validation%s\n", uDim, fixResult.Fixed, uReset)
		}
		if len(fixResult.RequiredVars) > 0 {
			noun := "variables"
			if len(fixResult.RequiredVars) == 1 {
				noun = "variable"
			}
			fmt.Printf("\n  %s⚠%s  %s%d sensitive %s%s require values at plan/apply time:\n",
				uYellow, uReset, uBold, len(fixResult.RequiredVars), noun, uReset)

			// Group by leaf attribute name for a scannable summary
			type varEntry struct {
				varName  string
				resource string
			}
			type attrGroup struct {
				entries []varEntry
			}
			groups := make(map[string]*attrGroup)
			var order []string
			for _, v := range fixResult.RequiredVars {
				leaf := v.AttrPath
				if idx := strings.LastIndex(leaf, "."); idx >= 0 {
					leaf = leaf[idx+1:]
				}
				if g, ok := groups[leaf]; ok {
					g.entries = append(g.entries, varEntry{v.VarName, v.Resource})
				} else {
					groups[leaf] = &attrGroup{entries: []varEntry{{v.VarName, v.Resource}}}
					order = append(order, leaf)
				}
			}
			for _, key := range order {
				g := groups[key]
				// Extract resource type display name from first entry
				resType := g.entries[0].resource
				if dot := strings.Index(resType, "."); dot >= 0 {
					resType = resType[:dot]
				}
				resNoun := strings.ReplaceAll(strings.TrimPrefix(resType, "jamfpro_"), "_", " ")
				if len(g.entries) > 1 {
					resNoun += "s"
				}
				fmt.Printf("    %s•%s %s%s%s  %s× %d %s%s\n",
					uYellow, uReset, uBold, key, uReset, uDim, len(g.entries), resNoun, uReset)
				for _, e := range g.entries {
					fmt.Printf("      %svar.%s%s\n", uDim, e.varName, uReset)
				}
			}
		}
	}

	fmt.Printf("\n%sNext steps:%s\n", uBold, uReset)
	fmt.Printf("  %scd%s %s\n", uCyan, uReset, absOutput)
	if *skipImportBlocks {
		fmt.Printf("  %sterraform plan%s    %s# Review the configuration%s\n", uCyan, uReset, uDim, uReset)
		fmt.Printf("  %sterraform apply%s   %s# Create resources%s\n", uCyan, uReset, uDim, uReset)
	} else {
		fmt.Printf("  %sterraform plan%s    %s# Review the import plan%s\n", uCyan, uReset, uDim, uReset)
		fmt.Printf("  %sterraform apply%s   %s# Import resources into state%s\n", uCyan, uReset, uDim, uReset)
		fmt.Printf("  %srm *_import.tf%s   %s# Remove import blocks after apply%s\n", uCyan, uReset, uDim, uReset)
	}
	fmt.Println()
	fmt.Printf("%s⚠%s  Review carefully before running %sterraform apply%s. The generated\n", uYellow, uReset, uBold, uReset)
	fmt.Println("   configuration may need manual fixes for provider validation errors.")
}

// findingDescription returns a short description of a secret finding for interactive selection.
func findingDescription(f secrets.Finding) string {
	if f.ResourceAddress != "" && f.AttrName != "" {
		return fmt.Sprintf("%s → %s", f.ResourceAddress, f.AttrName)
	}
	if f.ResourceAddress != "" {
		return fmt.Sprintf("%s (line %d)", f.ResourceAddress, f.StartLine)
	}
	if f.SupportFileRef != "" {
		return fmt.Sprintf("%s (line %d)", f.SupportFileRef, f.StartLine)
	}
	return fmt.Sprintf("%s:%d", filepath.Base(f.File), f.StartLine)
}

// promptLine prints a prompt and reads a line from the reader.
func promptLine(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// promptPassword prints a prompt and reads a password from the terminal
// without echoing the input.
func promptPassword(prompt string) string {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after hidden input
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}
	return strings.TrimSpace(string(b))
}

// summarizeOutput counts resource blocks and files in the output directory.
func summarizeOutput(dir string) (resources, types, files int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	skip := map[string]bool{"provider.tf": true, "variables.tf": true, "terraform.tfvars": true}
	typeSet := make(map[string]bool)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".tf") || skip[name] || strings.HasSuffix(name, "_import.tf") {
			continue
		}
		files++
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			continue
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "resource \"") {
				resources++
				if parts := strings.Fields(trimmed); len(parts) >= 2 {
					typeSet[strings.Trim(parts[1], "\"")] = true
				}
			}
		}
	}
	types = len(typeSet)
	return
}

// progressMsg carries a status update for the spinner, optionally with progress.
type progressMsg struct {
	text    string
	current int
	total   int
}

// spinner displays an animated status line on the terminal.
type spinner struct {
	stop     chan struct{}
	done     chan struct{}
	progress chan progressMsg
}

// startSpinner begins the animated spinner and returns it.
// Call Stop() to clear the line and halt the animation.
func startSpinner() *spinner {
	s := &spinner{
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		progress: make(chan progressMsg, 1),
	}
	go func() {
		defer close(s.done)
		const clearEOL = "\033[K" // ANSI: erase from cursor to end of line
		chars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		dots := [4]string{"   ", ".  ", ".. ", "..."}
		p := progressMsg{text: "jamforming"}
		i := 0
		fmt.Printf("\r%s %s%s", chars[0], p.text, dots[0])
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				fmt.Print("\r" + clearEOL)
				return
			case m := <-s.progress:
				p = m
			case <-ticker.C:
				i++
				if p.total > 0 {
					bar := renderBar(p.current, p.total, 20)
					fmt.Printf("\r%s %s %s %d/%d%s", chars[i%len(chars)], p.text, bar, p.current, p.total, clearEOL)
				} else {
					fmt.Printf("\r%s %s%s%s", chars[i%len(chars)], p.text, dots[(i/6)%4], clearEOL)
				}
			}
		}
	}()
	return s
}

// renderBar draws a Unicode progress bar of the given width.
func renderBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	filled := min(current*width/total, width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// Update sends a status message with optional progress (current/total).
// When total is 0, the spinner shows the animated dots; when > 0, it shows a progress bar.
func (s *spinner) Update(msg string, current, total int) {
	// Non-blocking send; drop if channel is full (message will be picked up next tick)
	select {
	case s.progress <- progressMsg{text: msg, current: current, total: total}:
	default:
	}
}

// Stop halts the spinner and clears its line.
func (s *spinner) Stop() {
	close(s.stop)
	<-s.done
}

// parseStringSet parses a space/comma-separated string into a map[string]bool.
// Returns nil if the input is empty.
func parseStringSet(input string) map[string]bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	input = strings.ReplaceAll(input, ",", " ")
	m := make(map[string]bool)
	for name := range strings.FieldsSeq(input) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			m[name] = true
		}
	}
	return m
}

// parseResourceFilter parses a space-separated resource list into a filter map.
// Also accepts commas for backwards compatibility.
// Returns nil if the input is empty (meaning all resources).
func parseResourceFilter(input string, nameMap map[string]string) map[string]bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Accept both spaces and commas as separators
	input = strings.ReplaceAll(input, ",", " ")

	filter := make(map[string]bool)
	for name := range strings.FieldsSeq(input) {
		name = strings.ToLower(name)
		if canonical, ok := nameMap[name]; ok {
			filter[canonical] = true
		} else {
			var valid []string
			seen := make(map[string]bool)
			for _, v := range nameMap {
				if !seen[v] {
					valid = append(valid, v)
					seen[v] = true
				}
			}
			log.Fatalf("Unknown resource type %q. Valid types: %s", name, strings.Join(valid, " "))
		}
	}

	return filter
}

// resolveMultiEnvCredentials builds EnvConfig for each environment from
// env-name-suffixed environment variables (e.g. JAMF_URL_DEV, JAMF_CLIENT_ID_DEV).
// If interactive and credentials are missing, prompts for each env. The shape of
// the credentials depends on the provider.
func resolveMultiEnvCredentials(provider string, envNames []string, interactive bool) ([]multienv.EnvConfig, error) {
	if provider == "jamfplatform" {
		return resolvePlatformMultiEnvCredentials(envNames, interactive)
	}
	return resolveProMultiEnvCredentials(envNames, interactive)
}

// resolvePlatformMultiEnvCredentials resolves per-env Jamf Platform credentials.
// Jamf Platform is OAuth2 only and uses a regional API gateway host (no
// .jamfcloud shorthand). Each environment carries its own scope: an environment
// ID (preferred), a tenant ID (legacy), or neither for organization scope.
func resolvePlatformMultiEnvCredentials(envNames []string, interactive bool) ([]multienv.EnvConfig, error) {
	var configs []multienv.EnvConfig
	for _, name := range envNames {
		upper := strings.ToUpper(name)
		env := multienv.EnvConfig{
			Name:         name,
			AuthMethod:   "oauth2",
			URL:          os.Getenv("JAMF_URL_" + upper),
			ClientID:     os.Getenv("JAMF_CLIENT_ID_" + upper),
			ClientSecret: os.Getenv("JAMF_CLIENT_SECRET_" + upper),
			// Scope per environment: the preferred environment ID, or the
			// legacy tenant ID. Neither is organization scope.
			EnvironmentID: firstNonEmpty(os.Getenv("JAMF_ENVIRONMENT_ID_"+upper), os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID_"+upper)),
			TenantID:      firstNonEmpty(os.Getenv("JAMF_TENANT_ID_"+upper), os.Getenv("JAMFPLATFORM_TENANT_ID_"+upper)),
		}
		if env.ClientID == "" {
			env.ClientID = os.Getenv("JAMFPLATFORM_CLIENT_ID_" + upper)
		}
		if env.ClientSecret == "" {
			env.ClientSecret = os.Getenv("JAMFPLATFORM_CLIENT_SECRET_" + upper)
		}
		if env.URL == "" {
			env.URL = os.Getenv("JAMFPLATFORM_BASE_URL_" + upper)
		}
		if env.EnvironmentID != "" && env.TenantID != "" {
			return nil, fmt.Errorf("conflicting API integration scope for environment %q: both "+
				"JAMF_ENVIRONMENT_ID_%s and JAMF_TENANT_ID_%s are set, but an integration targets one or the other", name, upper, upper)
		}

		if (env.URL == "" || env.ClientID == "" || env.ClientSecret == "") && interactive {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("\n%sEnvironment: %s%s\n", uBold, name, uReset)
			if env.URL == "" {
				env.URL = promptLine(reader, fmt.Sprintf("  Jamf Platform gateway host %s(e.g. https://us.api.jamfcloud.com)%s: ", uDim, uReset))
			}
			if env.ClientID == "" {
				env.ClientID = promptLine(reader, "  API Client ID: ")
			}
			if env.ClientSecret == "" {
				env.ClientSecret = promptPassword("  API Client Secret: ")
			}
			if env.EnvironmentID == "" && env.TenantID == "" {
				env.EnvironmentID = promptLine(reader, fmt.Sprintf("  Environment ID %s(preferred; blank to use a tenant ID)%s: ", uDim, uReset))
				if env.EnvironmentID == "" {
					env.TenantID = promptLine(reader, fmt.Sprintf("  Tenant ID %s(legacy; blank for organization scope)%s: ", uDim, uReset))
				}
			}
		}

		// Normalize URL — Platform uses regional gateway URLs directly (no
		// .jamfcloud shorthand expansion).
		env.URL = strings.TrimRight(env.URL, "/")
		if env.URL != "" && !strings.HasPrefix(env.URL, "https://") && !strings.HasPrefix(env.URL, "http://") {
			env.URL = "https://" + env.URL
		}

		if env.URL == "" {
			return nil, fmt.Errorf("missing base URL for environment %q (set JAMF_URL_%s)", name, upper)
		}
		if env.ClientID == "" || env.ClientSecret == "" {
			return nil, fmt.Errorf("missing OAuth2 credentials for environment %q (set JAMF_CLIENT_ID_%s and JAMF_CLIENT_SECRET_%s)", name, upper, upper)
		}

		configs = append(configs, env)
	}
	return configs, nil
}

// resolveProMultiEnvCredentials resolves per-env Jamf Pro credentials (basic or
// OAuth2), expanding bare instance names to <name>.jamfcloud.com.
func resolveProMultiEnvCredentials(envNames []string, interactive bool) ([]multienv.EnvConfig, error) {
	var configs []multienv.EnvConfig
	for _, name := range envNames {
		upper := strings.ToUpper(name)
		env := multienv.EnvConfig{
			Name:         name,
			URL:          os.Getenv("JAMF_URL_" + upper),
			ClientID:     os.Getenv("JAMF_CLIENT_ID_" + upper),
			ClientSecret: os.Getenv("JAMF_CLIENT_SECRET_" + upper),
			Username:     os.Getenv("JAMF_USERNAME_" + upper),
			Password:     os.Getenv("JAMF_PASSWORD_" + upper),
		}

		// Determine auth method
		hasOAuth := env.ClientID != "" || env.ClientSecret != ""
		hasBasic := env.Username != "" || env.Password != ""
		if hasOAuth && hasBasic {
			return nil, fmt.Errorf("cannot use both basic auth and OAuth2 for environment %q", name)
		}
		if hasOAuth {
			env.AuthMethod = "oauth2"
		} else if hasBasic {
			env.AuthMethod = "basic"
		}

		// Interactive prompts for missing credentials
		if (env.URL == "" || env.AuthMethod == "") && interactive {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("\n%sEnvironment: %s%s\n", uBold, name, uReset)
			if env.URL == "" {
				env.URL = promptLine(reader, fmt.Sprintf("  Jamf Pro URL %s(e.g. yourinstance.jamfcloud.com)%s: ", uDim, uReset))
			}
			if env.AuthMethod == "" {
				env.AuthMethod = "oauth2"
				if env.ClientID == "" {
					env.ClientID = promptLine(reader, "  API Client ID: ")
				}
				if env.ClientSecret == "" {
					env.ClientSecret = promptPassword("  API Client Secret: ")
				}
			}
		}

		// Normalize URL
		env.URL = strings.TrimRight(env.URL, "/")
		if !strings.Contains(env.URL, ".") && !strings.Contains(env.URL, "://") {
			env.URL = env.URL + ".jamfcloud.com"
		}
		if !strings.HasPrefix(env.URL, "https://") && !strings.HasPrefix(env.URL, "http://") {
			env.URL = "https://" + env.URL
		}

		// Validate
		if env.URL == "" {
			return nil, fmt.Errorf("missing URL for environment %q (set JAMF_URL_%s)", name, upper)
		}
		switch env.AuthMethod {
		case "oauth2":
			if env.ClientID == "" || env.ClientSecret == "" {
				return nil, fmt.Errorf("missing OAuth2 credentials for environment %q (set JAMF_CLIENT_ID_%s and JAMF_CLIENT_SECRET_%s)", name, upper, upper)
			}
		case "basic":
			if env.Username == "" || env.Password == "" {
				return nil, fmt.Errorf("missing basic auth credentials for environment %q (set JAMF_USERNAME_%s and JAMF_PASSWORD_%s)", name, upper, upper)
			}
		default:
			return nil, fmt.Errorf("no credentials provided for environment %q", name)
		}

		configs = append(configs, env)
	}
	return configs, nil
}
