package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
