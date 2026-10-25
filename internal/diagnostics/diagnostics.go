package diagnostics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

// Severity describes how much attention a finding needs.
type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Finding is one diagnostic result.
type Finding struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Summary     string   `json:"summary"`
	Detail      string   `json:"detail,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	AutoFixable bool     `json:"autoFixable"`
}

// Summary aggregates finding severities.
type Summary struct {
	OK       int `json:"ok"`
	Info     int `json:"info"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
}

// Report is the complete doctor result.
type Report struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	HiltVersion string        `json:"hiltVersion"`
	Platform    platform.Info `json:"platform"`
	Findings    []Finding     `json:"findings"`
	Summary     Summary       `json:"summary"`
}

// Deps are the inputs to the doctor engine.
type Deps struct {
	Version        string
	Platform       platform.Info
	Paths          platform.Paths
	Config         config.Config
	ConfigWarnings []string
	LookPath       func(string) (string, error)
	Stat           func(string) (os.FileInfo, error)
	Inventory      func(context.Context) (dotnet.Inventory, error)
	Cwd            string
	PackageManager *platform.PackageManager
}

// Run executes all Phase 1 checks.
func Run(ctx context.Context, deps Deps) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.Stat == nil {
		deps.Stat = os.Stat
	}
	if deps.Cwd == "" {
		deps.Cwd, _ = os.Getwd()
	}

	report := Report{
		GeneratedAt: time.Now().UTC(),
		HiltVersion: deps.Version,
		Platform:    deps.Platform,
	}
	add := func(finding Finding) {
		report.Findings = append(report.Findings, finding)
	}

	add(platformFinding(deps))
	add(configFinding(deps))
	add(dotnetPathFinding(deps))

	inventory, inventoryErr := loadInventory(ctx, deps)
	add(inventoryFinding(deps, inventory, inventoryErr))
	add(globalJSONFinding(deps, inventory, inventoryErr))
	add(packageManagerFinding(deps))
	add(gitFinding(deps))
	add(managedRootFinding(deps))

	for _, finding := range report.Findings {
		switch finding.Severity {
		case SeverityOK:
			report.Summary.OK++
		case SeverityInfo:
			report.Summary.Info++
		case SeverityWarning:
			report.Summary.Warnings++
		case SeverityError:
			report.Summary.Errors++
		}
	}
	return report
}

func platformFinding(deps Deps) Finding {
	distro := deps.Platform.DisplayName()
	return Finding{
		ID:       "platform",
		Title:    "Platform",
		Severity: SeverityOK,
		Summary:  fmt.Sprintf("%s · %s", deps.Platform.PlatformLabel(), distro),
		Detail:   fmt.Sprintf("kernel %s", valueOr(deps.Platform.Kernel, "unknown")),
	}
}

func configFinding(deps Deps) Finding {
	if len(deps.ConfigWarnings) > 0 {
		return Finding{
			ID:          "config",
			Title:       "Configuration",
			Severity:    SeverityWarning,
			Summary:     "Configuration needed attention",
			Detail:      strings.Join(deps.ConfigWarnings, "\n"),
			Remediation: "Review the configuration file and restart Hilt. Invalid files are preserved with a .corrupt-* suffix.",
		}
	}
	return Finding{
		ID:       "config",
		Title:    "Configuration",
		Severity: SeverityOK,
		Summary:  "Configuration loaded",
		Detail:   fmt.Sprintf("config: %s\nmanaged root: %s", deps.Paths.ConfigFile(), deps.Config.ManagedRoot),
	}
}

func dotnetPathFinding(deps Deps) Finding {
	path, err := deps.LookPath("dotnet")
	if err != nil {
		return Finding{
			ID:          "dotnet-path",
			Title:       "dotnet on PATH",
			Severity:    SeverityWarning,
			Summary:     "dotnet is not on PATH",
			Detail:      "Hilt can still detect installations in known roots and DOTNET_ROOT.",
			Remediation: "Install the .NET SDK. Managed installation is planned for Phase 2; system installation is available through the distribution package manager.",
		}
	}
	return Finding{
		ID:       "dotnet-path",
		Title:    "dotnet on PATH",
		Severity: SeverityInfo,
		Summary:  "Resolved " + path,
	}
}

func loadInventory(ctx context.Context, deps Deps) (dotnet.Inventory, error) {
	if deps.Inventory == nil {
		return dotnet.Inventory{}, fmt.Errorf("inventory service is not configured")
	}
	return deps.Inventory(ctx)
}

func inventoryFinding(deps Deps, inventory dotnet.Inventory, inventoryErr error) Finding {
	if inventoryErr != nil {
		return Finding{
			ID:          "dotnet-inventory",
			Title:       "Installed .NET",
			Severity:    SeverityError,
			Summary:     "Inventory could not be collected",
			Detail:      inventoryErr.Error(),
			Remediation: "Check filesystem permissions and re-run hilt doctor.",
		}
	}

	sdkCount := len(inventory.SDKs)
	runtimeCount := len(inventory.Runtimes)
	hostCount := len(inventory.Hosts)
	summary := fmt.Sprintf("%d host(s), %d SDK(s), %d runtime(s)", hostCount, sdkCount, runtimeCount)

	details := []string{}
	for _, host := range inventory.Hosts {
		line := fmt.Sprintf("%s (%s, managed=%t)", host.Root, host.Source, host.Managed)
		if host.Architecture != "" {
			line += fmt.Sprintf(", arch=%s", host.Architecture)
		}
		if host.ProbeError != "" {
			line += fmt.Sprintf(", probe error: %s", host.ProbeError)
		}
		details = append(details, line)
	}
	if len(inventory.Warnings) > 0 {
		details = append(details, inventory.Warnings...)
	}
	detail := strings.Join(details, "\n")

	switch {
	case sdkCount > 0:
		return Finding{
			ID:       "dotnet-inventory",
			Title:    "Installed .NET",
			Severity: SeverityOK,
			Summary:  summary,
			Detail:   detail,
		}
	case hostCount > 0:
		return Finding{
			ID:          "dotnet-inventory",
			Title:       "Installed .NET",
			Severity:    SeverityWarning,
			Summary:     "No SDK found in detected installations",
			Detail:      detail,
			Remediation: "Install an SDK for build, project, and package features. Managed installation is planned for Phase 2.",
		}
	default:
		return Finding{
			ID:          "dotnet-inventory",
			Title:       "Installed .NET",
			Severity:    SeverityWarning,
			Summary:     "No .NET installation detected",
			Detail:      detail,
			Remediation: "Install the .NET SDK. Hilt starts without .NET; managed installation is planned for Phase 2.",
		}
	}
}

func globalJSONFinding(deps Deps, inventory dotnet.Inventory, inventoryErr error) Finding {
	globalJSON, err := dotnet.FindGlobalJSON(deps.Cwd, deps.Stat)
	if err != nil {
		return Finding{
			ID:          "global-json",
			Title:       "global.json",
			Severity:    SeverityWarning,
			Summary:     "global.json could not be read",
			Detail:      err.Error(),
			Remediation: "Fix or remove the malformed global.json.",
		}
	}
	if globalJSON == nil {
		return Finding{
			ID:       "global-json",
			Title:    "global.json",
			Severity: SeverityInfo,
			Summary:  "No global.json in the current directory ancestry",
		}
	}

	detail := fmt.Sprintf("path: %s", globalJSON.Path)
	if globalJSON.RollForward != "" {
		detail += fmt.Sprintf("\nrollForward: %s", globalJSON.RollForward)
	}
	if globalJSON.AllowPrerelease != nil {
		detail += fmt.Sprintf("\nallowPrerelease: %t", *globalJSON.AllowPrerelease)
	}

	if globalJSON.Version == "" {
		return Finding{
			ID:          "global-json",
			Title:       "global.json",
			Severity:    SeverityWarning,
			Summary:     "global.json does not select an SDK version",
			Detail:      detail,
			Remediation: "Add sdk.version to global.json, or remove the file to use the default SDK.",
		}
	}

	if inventoryErr != nil {
		return Finding{
			ID:       "global-json",
			Title:    "global.json",
			Severity: SeverityWarning,
			Summary:  "global.json requires SDK " + globalJSON.Version,
			Detail:   detail + "\nSDK availability could not be checked.",
		}
	}

	if inventory.FindSDK(globalJSON.Version) != nil {
		return Finding{
			ID:       "global-json",
			Title:    "global.json",
			Severity: SeverityOK,
			Summary:  "Required SDK " + globalJSON.Version + " is installed",
			Detail:   detail,
		}
	}

	remediation := fmt.Sprintf("Install .NET SDK %s. Managed installation is planned for Phase 2.", globalJSON.Version)
	if globalJSON.RollForward != "" {
		remediation += " The configured rollForward policy may allow another installed SDK; verify with dotnet --version in the project directory."
	}
	return Finding{
		ID:          "global-json",
		Title:       "global.json",
		Severity:    SeverityError,
		Summary:     "Required SDK " + globalJSON.Version + " is not installed",
		Detail:      detail,
		Remediation: remediation,
	}
}

func packageManagerFinding(deps Deps) Finding {
	if deps.PackageManager == nil {
		if deps.Platform.IsLinux {
			return Finding{
				ID:          "package-manager",
				Title:       "Package manager",
				Severity:    SeverityWarning,
				Summary:     "No supported package manager detected",
				Detail:      "System SDK installation is unavailable on this host.",
				Remediation: "Use a supported distribution or install .NET from Microsoft when available.",
			}
		}
		return Finding{
			ID:       "package-manager",
			Title:    "Package manager",
			Severity: SeverityInfo,
			Summary:  "No supported package manager (Linux-only integration)",
		}
	}
	detail := deps.PackageManager.Path
	if deps.PackageManager.Privilege != "" {
		detail += "\nprivilege tool: " + deps.PackageManager.Privilege
	}
	return Finding{
		ID:       "package-manager",
		Title:    "Package manager",
		Severity: SeverityInfo,
		Summary:  deps.PackageManager.Name,
		Detail:   detail,
	}
}

func gitFinding(deps Deps) Finding {
	path, err := deps.LookPath("git")
	if err != nil {
		return Finding{
			ID:          "git",
			Title:       "Git",
			Severity:    SeverityWarning,
			Summary:     "git is not available",
			Remediation: "Install git if you want project creation to initialize repositories (Phase 5).",
		}
	}
	return Finding{
		ID:       "git",
		Title:    "Git",
		Severity: SeverityInfo,
		Summary:  "Resolved " + path,
	}
}

func managedRootFinding(deps Deps) Finding {
	root := deps.Config.ManagedRoot
	if root == "" {
		root = deps.Paths.ManagedRoot
	}
	info, err := deps.Stat(root)
	exists := err == nil && info.IsDir()
	detail := "exists: false (created on first managed installation)"
	if exists {
		detail = "exists: true"
	}
	return Finding{
		ID:       "managed-root",
		Title:    "Hilt-managed installation root",
		Severity: SeverityInfo,
		Summary:  root,
		Detail:   detail,
	}
}

// Healthy reports whether the report contains no errors.
func (r Report) Healthy() bool {
	return r.Summary.Errors == 0
}

// Clean reports whether the report contains no warnings or errors.
func (r Report) Clean() bool {
	return r.Summary.Errors == 0 && r.Summary.Warnings == 0
}

// ExitCode maps the report to the documented process exit code.
func (r Report) ExitCode() int {
	if r.Healthy() {
		return 0
	}
	return 1
}

// SortedFindings returns findings ordered by severity and then ID. Errors and
// warnings appear first for human output.
func (r Report) SortedFindings() []Finding {
	findings := append([]Finding(nil), r.Findings...)
	sort.SliceStable(findings, func(i, j int) bool {
		left := severityRank(findings[i].Severity)
		right := severityRank(findings[j].Severity)
		if left != right {
			return left > right
		}
		return findings[i].ID < findings[j].ID
	})
	return findings
}

func severityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// FindingByID returns a finding, if present.
func (r Report) FindingByID(id string) *Finding {
	for index := range r.Findings {
		if r.Findings[index].ID == id {
			return &r.Findings[index]
		}
	}
	return nil
}

// ConfigPathForDisplay helps CLI/TUI render the config location.
func ConfigPathForDisplay(paths platform.Paths) string {
	return filepath.Clean(paths.ConfigFile())
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
