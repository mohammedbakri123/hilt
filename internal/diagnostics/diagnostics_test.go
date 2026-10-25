package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

func baseDeps(t *testing.T) Deps {
	t.Helper()
	return Deps{
		Version: "test",
		Platform: platform.Info{
			OS: "linux", Arch: "amd64", Distro: "cachyos", IsLinux: true,
		},
		Paths: platform.Paths{
			ConfigDir:   "/tmp/config",
			ManagedRoot: "/tmp/managed",
		},
		Config: config.Defaults("/tmp/managed"),
		LookPath: func(name string) (string, error) {
			switch name {
			case "dotnet":
				return "/usr/bin/dotnet", nil
			case "git":
				return "/usr/bin/git", nil
			}
			return "", errors.New("not found")
		},
		Stat: os.Stat,
		Inventory: func(context.Context) (dotnet.Inventory, error) {
			return dotnet.Inventory{
				Hosts: []dotnet.Host{{Root: "/usr/share/dotnet", DotnetPath: "/usr/share/dotnet/dotnet", Source: dotnet.SourcePath}},
				SDKs:  []dotnet.SDK{{Version: "10.0.112", Root: "/usr/share/dotnet"}},
			}, nil
		},
		Cwd:            t.TempDir(),
		PackageManager: &platform.PackageManager{Name: "pacman", Path: "/usr/bin/pacman", Privilege: "sudo"},
	}
}

func TestRunHealthy(t *testing.T) {
	report := Run(context.Background(), baseDeps(t))
	if !report.Healthy() {
		t.Fatalf("expected healthy, got %+v", report.Summary)
	}
	if report.ExitCode() != 0 {
		t.Errorf("exit code = %d", report.ExitCode())
	}
	if finding := report.FindingByID("dotnet-inventory"); finding == nil || finding.Severity != SeverityOK {
		t.Errorf("inventory finding = %+v", finding)
	}
	if report.Summary.OK == 0 {
		t.Error("expected ok findings")
	}
}

func TestRunWithoutDotnet(t *testing.T) {
	deps := baseDeps(t)
	deps.LookPath = func(name string) (string, error) {
		if name == "git" {
			return "/usr/bin/git", nil
		}
		return "", errors.New("not found")
	}
	deps.Inventory = func(context.Context) (dotnet.Inventory, error) {
		return dotnet.Inventory{Warnings: []string{"No .NET installation roots found"}}, nil
	}

	report := Run(context.Background(), deps)
	if !report.Healthy() {
		t.Fatalf("missing .NET must not be an error: %+v", report.Summary)
	}
	if report.Summary.Warnings == 0 {
		t.Fatal("expected warnings")
	}
	if report.ExitCode() != 0 {
		t.Errorf("exit code = %d", report.ExitCode())
	}
}

func TestRunGlobalJSONMissingSDK(t *testing.T) {
	deps := baseDeps(t)
	cwd := t.TempDir()
	content := `{"sdk":{"version":"8.0.100","rollForward":"latestFeature"}}`
	if err := os.WriteFile(filepath.Join(cwd, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Cwd = cwd

	report := Run(context.Background(), deps)
	finding := report.FindingByID("global-json")
	if finding == nil {
		t.Fatal("missing global-json finding")
	}
	if finding.Severity != SeverityError {
		t.Errorf("severity = %q", finding.Severity)
	}
	if report.ExitCode() != 1 {
		t.Errorf("exit code = %d", report.ExitCode())
	}
}

func TestRunGlobalJSONSatisfied(t *testing.T) {
	deps := baseDeps(t)
	cwd := t.TempDir()
	content := `{"sdk":{"version":"10.0.112"}}`
	if err := os.WriteFile(filepath.Join(cwd, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Cwd = cwd

	report := Run(context.Background(), deps)
	finding := report.FindingByID("global-json")
	if finding == nil || finding.Severity != SeverityOK {
		t.Errorf("finding = %+v", finding)
	}
}

func TestRunConfigWarning(t *testing.T) {
	deps := baseDeps(t)
	deps.ConfigWarnings = []string{"config was invalid"}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("config")
	if finding == nil || finding.Severity != SeverityWarning {
		t.Errorf("finding = %+v", finding)
	}
	if report.ExitCode() != 0 {
		t.Errorf("warnings should not fail doctor: %d", report.ExitCode())
	}
}

func TestRunInventoryError(t *testing.T) {
	deps := baseDeps(t)
	deps.Inventory = func(context.Context) (dotnet.Inventory, error) {
		return dotnet.Inventory{}, errors.New("permission denied")
	}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("dotnet-inventory")
	if finding == nil || finding.Severity != SeverityError {
		t.Errorf("finding = %+v", finding)
	}
	if report.ExitCode() != 1 {
		t.Errorf("exit code = %d", report.ExitCode())
	}
}

func TestRunProbeErrorIsVisible(t *testing.T) {
	deps := baseDeps(t)
	deps.Inventory = func(context.Context) (dotnet.Inventory, error) {
		return dotnet.Inventory{
			Hosts: []dotnet.Host{{Root: "/usr/share/dotnet", ProbeError: "exit status 1"}},
		}, nil
	}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("dotnet-inventory")
	if finding == nil {
		t.Fatal("missing finding")
	}
	if finding.Detail == "" {
		t.Error("expected probe detail")
	}
}

func TestSortedFindings(t *testing.T) {
	report := Report{Findings: []Finding{
		{ID: "b", Severity: SeverityInfo},
		{ID: "a", Severity: SeverityError},
		{ID: "c", Severity: SeverityWarning},
	}}
	sorted := report.SortedFindings()
	if sorted[0].ID != "a" || sorted[1].ID != "c" || sorted[2].ID != "b" {
		t.Errorf("sorted = %+v", sorted)
	}
}

func TestRunShowsSelectedToolchain(t *testing.T) {
	deps := baseDeps(t)
	deps.Resolve = func(context.Context, dotnet.Inventory) (dotnet.Selection, error) {
		return dotnet.Selection{
			Root:       "/home/u/.local/share/hilt/dotnet",
			DotnetPath: "/home/u/.local/share/hilt/dotnet/dotnet",
			SDKVersion: "9.0.300",
			Source:     dotnet.SourceManaged,
			Provenance: dotnet.ProvenanceManaged,
			Owned:      true,
			Rationale:  "managed SDK 9.0.300 selected as the newest managed installation",
		}, nil
	}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("dotnet-toolchain")
	if finding == nil || finding.Severity != SeverityOK {
		t.Fatalf("toolchain finding = %+v", finding)
	}
	for _, want := range []string{"9.0.300", "managed", "/home/u/.local/share/hilt/dotnet"} {
		if !strings.Contains(finding.Summary+finding.Detail, want) {
			t.Errorf("toolchain finding missing %q: %+v", want, finding)
		}
	}
}

func TestRunGlobalJSONUsesResolverRollForward(t *testing.T) {
	deps := baseDeps(t)
	cwd := t.TempDir()
	content := `{"sdk":{"version":"8.0.100","rollForward":"latestFeature"}}`
	if err := os.WriteFile(filepath.Join(cwd, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Cwd = cwd
	deps.Resolve = func(context.Context, dotnet.Inventory) (dotnet.Selection, error) {
		return dotnet.Selection{
			Root:             "/usr/share/dotnet",
			DotnetPath:       "/usr/share/dotnet/dotnet",
			SDKVersion:       "10.0.112",
			Source:           dotnet.SourcePath,
			Provenance:       dotnet.ProvenanceSystem,
			GlobalJSONPath:   filepath.Join(cwd, "global.json"),
			RequestedVersion: "8.0.100",
			Verified:         true,
		}, nil
	}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("global-json")
	if finding == nil || finding.Severity != SeverityOK {
		t.Fatalf("global-json finding = %+v", finding)
	}
	if !strings.Contains(finding.Summary, "10.0.112") {
		t.Errorf("summary should report the actual selected SDK: %+v", finding)
	}
}

func TestRunGlobalJSONResolverMissing(t *testing.T) {
	deps := baseDeps(t)
	cwd := t.TempDir()
	content := `{"sdk":{"version":"8.0.100"}}`
	if err := os.WriteFile(filepath.Join(cwd, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.Cwd = cwd
	deps.Resolve = func(context.Context, dotnet.Inventory) (dotnet.Selection, error) {
		return dotnet.Selection{}, &dotnet.MissingSDKError{
			RequestedVersion: "8.0.100",
			GlobalJSONPath:   filepath.Join(cwd, "global.json"),
			Attempts: []dotnet.Attempt{{
				Root: "/usr/share/dotnet", Source: dotnet.SourcePath,
				Provenance: dotnet.ProvenanceSystem, Error: "not found",
			}},
		}
	}
	report := Run(context.Background(), deps)
	globalJSON := report.FindingByID("global-json")
	if globalJSON == nil || globalJSON.Severity != SeverityError {
		t.Fatalf("global-json finding = %+v", globalJSON)
	}
	if !strings.Contains(globalJSON.Detail, "not found") {
		t.Errorf("detail should include attempts: %+v", globalJSON)
	}
	toolchain := report.FindingByID("dotnet-toolchain")
	if toolchain == nil || toolchain.Severity != SeverityWarning {
		t.Errorf("toolchain finding = %+v", toolchain)
	}
	if report.ExitCode() != 1 {
		t.Errorf("exit code = %d", report.ExitCode())
	}
}

func TestRunCrossRootDuplicatesAreInfo(t *testing.T) {
	deps := baseDeps(t)
	deps.Inventory = func(context.Context) (dotnet.Inventory, error) {
		return dotnet.Inventory{
			Hosts: []dotnet.Host{
				{Root: "/usr/share/dotnet", DotnetPath: "/usr/share/dotnet/dotnet", Source: dotnet.SourcePath},
				{Root: "/home/u/.dotnet", DotnetPath: "/home/u/.dotnet/dotnet", Source: dotnet.SourcePath},
			},
			SDKs: []dotnet.SDK{
				{Version: "10.0.112", Root: "/usr/share/dotnet", Source: dotnet.SourcePath},
				{Version: "10.0.112", Root: "/home/u/.dotnet", Source: dotnet.SourcePath},
			},
		}, nil
	}
	report := Run(context.Background(), deps)
	finding := report.FindingByID("cross-root-sdks")
	if finding == nil || finding.Severity != SeverityInfo {
		t.Fatalf("cross-root finding = %+v", finding)
	}
	if !strings.Contains(finding.Detail, "/usr/share/dotnet") || !strings.Contains(finding.Detail, "/home/u/.dotnet") {
		t.Errorf("detail = %q", finding.Detail)
	}
	if report.ExitCode() != 0 {
		t.Errorf("duplicates must not fail doctor: %d", report.ExitCode())
	}
}
