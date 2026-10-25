package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

type fakeBackend struct {
	inventory    dotnet.Inventory
	inventoryErr error
	selection    dotnet.Selection
	selectionErr error
	report       diagnostics.Report
}

func (f fakeBackend) Version() string       { return "test" }
func (f fakeBackend) Config() config.Config { return config.Config{} }
func (f fakeBackend) Paths() platform.Paths { return platform.Paths{} }
func (f fakeBackend) ConfigWarnings() []string {
	return nil
}
func (f fakeBackend) PackageManager() *platform.PackageManager { return nil }
func (f fakeBackend) Inventory(context.Context) (dotnet.Inventory, error) {
	return f.inventory, f.inventoryErr
}
func (f fakeBackend) Resolve(context.Context, dotnet.Inventory) (dotnet.Selection, error) {
	return f.selection, f.selectionErr
}
func (f fakeBackend) Diagnose(context.Context) diagnostics.Report { return f.report }
func (f fakeBackend) Tool(string) (string, bool)                  { return "", false }
func (f fakeBackend) Platform() platform.Info {
	return platform.Info{OS: "linux", Arch: "amd64", Distro: "cachyos", DistroName: "CachyOS"}
}

func populatedBackend() fakeBackend {
	return fakeBackend{
		inventory: dotnet.Inventory{
			Hosts: []dotnet.Host{{
				Root:       "/usr/share/dotnet",
				DotnetPath: "/usr/share/dotnet/dotnet",
				Source:     dotnet.SourcePath,
				Provenance: dotnet.ProvenanceSystem,
				Usable:     true,
			}},
			SDKs: []dotnet.SDK{
				{Version: "10.0.112", Path: "/usr/share/dotnet/sdk/10.0.112", Root: "/usr/share/dotnet", Source: dotnet.SourcePath, Provenance: dotnet.ProvenanceSystem, Usable: true},
				{Version: "9.0.300", Path: "/home/u/.local/share/hilt/dotnet/sdk/9.0.300", Root: "/home/u/.local/share/hilt/dotnet", Source: dotnet.SourceManaged, Provenance: dotnet.ProvenanceManaged, Managed: true, Usable: true},
			},
			Runtimes: []dotnet.Runtime{
				{Framework: "Microsoft.NETCore.App", Version: "10.0.12", Path: "/usr/share/dotnet/shared/Microsoft.NETCore.App/10.0.12", Root: "/usr/share/dotnet", Source: dotnet.SourcePath, Provenance: dotnet.ProvenanceSystem},
				{Framework: "Microsoft.AspNetCore.App", Version: "10.0.12", Path: "/usr/share/dotnet/shared/Microsoft.AspNetCore.App/10.0.12", Root: "/home/u/.local/share/hilt/dotnet", Source: dotnet.SourceManaged, Provenance: dotnet.ProvenanceManaged, Managed: true},
			},
			Warnings: []string{"No .NET SDKs were detected."},
		},
		selection: dotnet.Selection{
			Root:       "/usr/share/dotnet",
			DotnetPath: "/usr/share/dotnet/dotnet",
			SDKVersion: "10.0.112",
			Source:     dotnet.SourcePath,
			Provenance: dotnet.ProvenanceSystem,
			Rationale:  "system SDK 10.0.112 selected as the newest system installation; no managed SDK is available",
		},
		report: diagnostics.Report{
			HiltVersion: "test",
			Platform:    platform.Info{OS: "linux", Arch: "amd64", DistroName: "CachyOS"},
			Findings: []diagnostics.Finding{
				{ID: "platform", Title: "Platform", Severity: diagnostics.SeverityOK, Summary: "linux/amd64 · CachyOS"},
			},
			Summary: diagnostics.Summary{OK: 1},
		},
	}
}

type result struct {
	code    int
	stdout  string
	stderr  string
	tuiRuns int
}

func run(t *testing.T, args []string, backend Backend, terminal bool, runTUI func(Backend) error) result {
	t.Helper()
	var out, errOut bytes.Buffer
	runs := 0
	wrapped := runTUI
	if wrapped != nil {
		wrapped = func(b Backend) error {
			runs++
			return runTUI(b)
		}
	}
	code := Run(context.Background(), args, Options{
		Backend:    backend,
		Out:        &out,
		Err:        &errOut,
		IsTerminal: func() bool { return terminal },
		RunTUI:     wrapped,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String(), tuiRuns: runs}
}

func TestVersion(t *testing.T) {
	backend := populatedBackend()

	human := run(t, []string{"version"}, backend, false, nil)
	if human.code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", human.code, human.stderr)
	}
	if !strings.Contains(human.stdout, "hilt test") || !strings.Contains(human.stdout, "linux/amd64") {
		t.Errorf("unexpected version output: %q", human.stdout)
	}

	machine := run(t, []string{"version", "--json"}, backend, false, nil)
	var payload struct {
		SchemaVersion int           `json:"schemaVersion"`
		Version       string        `json:"version"`
		Platform      platform.Info `json:"platform"`
	}
	if err := json.Unmarshal([]byte(machine.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, machine.stdout)
	}
	if payload.SchemaVersion != 1 || payload.Version != "test" || payload.Platform.OS != "linux" {
		t.Errorf("unexpected payload: %+v", payload)
	}

	flag := run(t, []string{"--version"}, backend, false, nil)
	if flag.code != 0 || !strings.Contains(flag.stdout, "hilt test") {
		t.Errorf("--version = %d %q", flag.code, flag.stdout)
	}
}

func TestVersionIncludesBuildMetadata(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"version"}, Options{
		Backend:    populatedBackend(),
		Commit:     "abc1234",
		Date:       "2026-10-25T00:00:00Z",
		Out:        &out,
		Err:        &errOut,
		IsTerminal: func() bool { return false },
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", code, errOut.String())
	}
	for _, want := range []string{"hilt test (abc1234)", "commit abc1234", "built 2026-10-25T00:00:00Z"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %q", want, out.String())
		}
	}
}

func TestHelp(t *testing.T) {
	help := run(t, []string{"--help"}, populatedBackend(), false, nil)
	if help.code != 0 {
		t.Fatalf("exit = %d, want 0", help.code)
	}
	if !strings.Contains(help.stdout, "Available Commands") || !strings.Contains(help.stdout, "doctor") {
		t.Errorf("help missing commands: %q", help.stdout)
	}
}

func TestRootWithoutTTYDoesNotStartTUI(t *testing.T) {
	tui := func(Backend) error { return nil }
	result := run(t, nil, populatedBackend(), false, tui)
	if result.code != 2 {
		t.Fatalf("exit = %d, want 2", result.code)
	}
	if result.tuiRuns != 0 {
		t.Error("TUI should not run without a terminal")
	}
	if !strings.Contains(result.stderr, "requires a terminal") {
		t.Errorf("missing usage hint: %q", result.stderr)
	}
}

func TestRootWithTTYStartsTUI(t *testing.T) {
	result := run(t, nil, populatedBackend(), true, func(Backend) error { return nil })
	if result.code != 0 || result.tuiRuns != 1 {
		t.Fatalf("exit = %d, tui runs = %d, want 0 and 1", result.code, result.tuiRuns)
	}

	failed := run(t, nil, populatedBackend(), true, func(Backend) error { return errors.New("terminal broke") })
	if failed.code != 1 {
		t.Fatalf("exit = %d, want 1", failed.code)
	}
	if !strings.Contains(failed.stderr, "terminal broke") {
		t.Errorf("missing TUI error: %q", failed.stderr)
	}
}

func TestSDKListJSON(t *testing.T) {
	result := run(t, []string{"sdk", "list", "--json"}, populatedBackend(), false, nil)
	if result.code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", result.code, result.stderr)
	}
	if result.stderr != "" {
		t.Errorf("JSON mode wrote to stderr: %q", result.stderr)
	}

	var payload struct {
		SchemaVersion int    `json:"schemaVersion"`
		GeneratedAt   string `json:"generatedAt"`
		SDKs          []struct {
			Version string `json:"version"`
			Managed bool   `json:"managed"`
		} `json:"sdks"`
		Runtimes []struct {
			Framework string `json:"framework"`
		} `json:"runtimes"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, result.stdout)
	}
	if payload.SchemaVersion != 1 {
		t.Errorf("schemaVersion = %d", payload.SchemaVersion)
	}
	if len(payload.SDKs) != 2 || payload.SDKs[0].Version != "10.0.112" || !payload.SDKs[1].Managed {
		t.Errorf("unexpected SDKs: %+v", payload.SDKs)
	}
	if len(payload.Runtimes) != 2 || payload.Runtimes[0].Framework != "Microsoft.NETCore.App" {
		t.Errorf("unexpected runtimes: %+v", payload.Runtimes)
	}
	if len(payload.Warnings) != 1 {
		t.Errorf("warnings = %v, want 1", payload.Warnings)
	}
}

func TestRuntimeListHuman(t *testing.T) {
	result := run(t, []string{"runtime", "list"}, populatedBackend(), false, nil)
	if result.code != 0 {
		t.Fatalf("exit = %d, want 0", result.code)
	}
	for _, want := range []string{"FRAMEWORK", "Microsoft.NETCore.App", "Microsoft.AspNetCore.App", "managed", "system"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, result.stdout)
		}
	}
	if !strings.Contains(result.stderr, "warning:") {
		t.Errorf("discovery warnings should go to stderr: %q", result.stderr)
	}
}

func TestEmptyInventorySucceeds(t *testing.T) {
	backend := fakeBackend{inventory: dotnet.Inventory{}}
	human := run(t, []string{"sdk", "list"}, backend, false, nil)
	if human.code != 0 || !strings.Contains(human.stdout, "No .NET SDKs detected") {
		t.Errorf("empty inventory = %d %q", human.code, human.stdout)
	}

	machine := run(t, []string{"sdk", "list", "--json"}, backend, false, nil)
	var payload struct {
		SDKs     []json.RawMessage `json:"sdks"`
		Hosts    []json.RawMessage `json:"hosts"`
		Runtimes []json.RawMessage `json:"runtimes"`
		Warnings []json.RawMessage `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(machine.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if machine.code != 0 || payload.SDKs == nil || payload.Hosts == nil || payload.Runtimes == nil || payload.Warnings == nil {
		t.Errorf("empty inventory JSON should use empty arrays: %s", machine.stdout)
	}
}

func TestDoctorExitCodes(t *testing.T) {
	warnings := populatedBackend()
	warnings.report.Summary = diagnostics.Summary{OK: 4, Warnings: 2}
	warnings.report.Findings = append(warnings.report.Findings, diagnostics.Finding{
		ID: "config", Title: "Configuration", Severity: diagnostics.SeverityWarning,
		Summary: "Configuration needed attention", Remediation: "Review the config file.",
	})
	okResult := run(t, []string{"doctor"}, warnings, false, nil)
	if okResult.code != 0 {
		t.Fatalf("warnings should exit 0, got %d (%s)", okResult.code, okResult.stderr)
	}
	for _, want := range []string{"[WARN]", "Configuration", "fix: Review the config file."} {
		if !strings.Contains(okResult.stdout, want) {
			t.Errorf("doctor output missing %q:\n%s", want, okResult.stdout)
		}
	}

	failing := populatedBackend()
	failing.report.Summary = diagnostics.Summary{OK: 1, Errors: 1}
	failing.report.Findings = append(failing.report.Findings, diagnostics.Finding{
		ID: "dotnet", Title: "dotnet", Severity: diagnostics.SeverityError, Summary: "broken",
	})
	failResult := run(t, []string{"doctor"}, failing, false, nil)
	if failResult.code != 1 {
		t.Fatalf("errors should exit 1, got %d", failResult.code)
	}
	if !strings.Contains(failResult.stdout, "[ERROR]") {
		t.Errorf("missing error badge: %q", failResult.stdout)
	}

	machine := run(t, []string{"doctor", "--json"}, warnings, false, nil)
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Summary       diagnostics.Summary
		Findings      []diagnostics.Finding
	}
	if err := json.Unmarshal([]byte(machine.stdout), &payload); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, machine.stdout)
	}
	if payload.SchemaVersion != 1 || payload.Summary.Warnings != 2 || len(payload.Findings) != 2 {
		t.Errorf("unexpected doctor payload: %+v", payload)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nope"},
		{"doctor", "--nope"},
		{"doctor", "extra"},
		{"sdk"},
		{"sdk", "list", "extra"},
		{"runtime", "list", "--bad"},
	} {
		result := run(t, args, populatedBackend(), false, func(Backend) error { return nil })
		if result.code != 2 {
			t.Errorf("%v: exit = %d, want 2", args, result.code)
		}
		if result.tuiRuns != 0 {
			t.Errorf("%v: TUI ran for a usage error", args)
		}
	}
}

func TestInventoryFailureExitsOne(t *testing.T) {
	backend := populatedBackend()
	backend.inventoryErr = errors.New("probe failed")
	result := run(t, []string{"sdk", "list"}, backend, false, nil)
	if result.code != 1 {
		t.Fatalf("exit = %d, want 1", result.code)
	}
	if !strings.Contains(result.stderr, "probe failed") {
		t.Errorf("missing failure: %q", result.stderr)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	code := Run(ctx, []string{"doctor"}, Options{
		Backend:    populatedBackend(),
		Out:        &out,
		Err:        &errOut,
		IsTerminal: func() bool { return false },
	})
	if code != 130 {
		t.Fatalf("exit = %d, want 130", code)
	}
	if !strings.Contains(errOut.String(), "cancelled") {
		t.Errorf("missing cancellation message: %q", errOut.String())
	}
}

func TestSDKListMarksSelectedToolchain(t *testing.T) {
	result := run(t, []string{"sdk", "list"}, populatedBackend(), false, nil)
	if result.code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", result.code, result.stderr)
	}
	for _, want := range []string{"VERSION", "SOURCE", "OWNERSHIP", "SELECTED", "USABLE", "10.0.112", "9.0.300", "system", "managed", "*"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, result.stdout)
		}
	}
}

func TestSDKWhichHumanAndJSON(t *testing.T) {
	human := run(t, []string{"sdk", "which"}, populatedBackend(), false, nil)
	if human.code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", human.code, human.stderr)
	}
	for _, want := range []string{"Selected .NET toolchain", "10.0.112", "/usr/share/dotnet", "system", "newest system installation"} {
		if !strings.Contains(human.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, human.stdout)
		}
	}

	machine := run(t, []string{"sdk", "which", "--json"}, populatedBackend(), false, nil)
	if machine.code != 0 {
		t.Fatalf("exit = %d, want 0 (%s)", machine.code, machine.stderr)
	}
	if machine.stderr != "" {
		t.Errorf("JSON mode wrote to stderr: %q", machine.stderr)
	}
	var payload struct {
		SchemaVersion int    `json:"schemaVersion"`
		Root          string `json:"root"`
		DotnetPath    string `json:"dotnetPath"`
		SDKVersion    string `json:"sdkVersion"`
		Source        string `json:"source"`
		Provenance    string `json:"provenance"`
	}
	if err := json.Unmarshal([]byte(machine.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, machine.stdout)
	}
	if payload.SchemaVersion != 1 || payload.Root != "/usr/share/dotnet" || payload.SDKVersion != "10.0.112" {
		t.Errorf("unexpected payload: %+v", payload)
	}
	if payload.Provenance != "system" || payload.Source != "path" {
		t.Errorf("unexpected provenance/source: %+v", payload)
	}
}

func TestSDKListSelectionFailureIsAWarning(t *testing.T) {
	backend := populatedBackend()
	backend.selectionErr = &dotnet.MissingSDKError{
		RequestedVersion: "8.0.100",
		GlobalJSONPath:   "/workspace/global.json",
		Attempts: []dotnet.Attempt{
			{Root: "/usr/share/dotnet", Source: dotnet.SourcePath, Provenance: dotnet.ProvenanceSystem, Error: "not found"},
		},
	}
	human := run(t, []string{"sdk", "list"}, backend, false, nil)
	if human.code != 0 {
		t.Fatalf("exit = %d, want 0", human.code)
	}
	if !strings.Contains(human.stdout, "10.0.112") {
		t.Errorf("SDK list should still render: %q", human.stdout)
	}
	if !strings.Contains(human.stderr, "8.0.100") {
		t.Errorf("selection warning missing: %q", human.stderr)
	}

	machine := run(t, []string{"sdk", "list", "--json"}, backend, false, nil)
	if machine.stderr != "" {
		t.Errorf("JSON mode wrote to stderr: %q", machine.stderr)
	}
	var payload struct {
		Selection      *dotnet.Selection `json:"selection"`
		SelectionError string            `json:"selectionError"`
	}
	if err := json.Unmarshal([]byte(machine.stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, machine.stdout)
	}
	if payload.Selection != nil || !strings.Contains(payload.SelectionError, "8.0.100") {
		t.Errorf("unexpected selection payload: %+v", payload)
	}
}

func TestSDKWhichSelectionFailureExitsOne(t *testing.T) {
	backend := populatedBackend()
	backend.selectionErr = &dotnet.MissingSDKError{RequestedVersion: "8.0.100"}
	result := run(t, []string{"sdk", "which"}, backend, false, nil)
	if result.code != 1 {
		t.Fatalf("exit = %d, want 1", result.code)
	}
	if !strings.Contains(result.stderr, "8.0.100") {
		t.Errorf("missing failure: %q", result.stderr)
	}
}
