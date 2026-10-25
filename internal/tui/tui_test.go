package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

type fakeBackend struct {
	inventory      dotnet.Inventory
	inventoryErr   error
	selection      dotnet.Selection
	selectionErr   error
	report         diagnostics.Report
	config         config.Config
	paths          platform.Paths
	configWarnings []string
}

func (f fakeBackend) Version() string          { return "test" }
func (f fakeBackend) Config() config.Config    { return f.config }
func (f fakeBackend) Paths() platform.Paths    { return f.paths }
func (f fakeBackend) ConfigWarnings() []string { return f.configWarnings }
func (f fakeBackend) Inventory(context.Context) (dotnet.Inventory, error) {
	return f.inventory, f.inventoryErr
}
func (f fakeBackend) Resolve(context.Context, dotnet.Inventory) (dotnet.Selection, error) {
	return f.selection, f.selectionErr
}
func (f fakeBackend) Diagnose(context.Context) diagnostics.Report { return f.report }
func (f fakeBackend) Platform() platform.Info {
	return platform.Info{OS: "linux", Arch: "amd64", Distro: "cachyos", DistroName: "CachyOS", IsLinux: true}
}
func (f fakeBackend) PackageManager() *platform.PackageManager {
	return &platform.PackageManager{Name: "pacman", Path: "/usr/bin/pacman", Privilege: "sudo"}
}
func (f fakeBackend) Tool(name string) (string, bool) {
	if name == "git" {
		return "/usr/bin/git", true
	}
	return "", false
}

func newFakeBackend() fakeBackend {
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
				{Framework: "Microsoft.NETCore.App", Version: "10.0.12", Path: "/usr/share/dotnet/shared/Microsoft.NETCore.App/10.0.12", Root: "/usr/share/dotnet", Provenance: dotnet.ProvenanceSystem},
				{Framework: "Microsoft.AspNetCore.App", Version: "10.0.12", Path: "/usr/share/dotnet/shared/Microsoft.AspNetCore.App/10.0.12", Root: "/usr/share/dotnet", Provenance: dotnet.ProvenanceSystem},
			},
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
			Findings: []diagnostics.Finding{
				{ID: "platform", Title: "Platform", Severity: diagnostics.SeverityOK, Summary: "linux/amd64 · CachyOS"},
				{ID: "dotnet-path", Title: "dotnet on PATH", Severity: diagnostics.SeverityWarning, Summary: "dotnet is not on PATH"},
			},
			Summary: diagnostics.Summary{OK: 1, Warnings: 1},
		},
		config: config.Config{
			ManagedRoot:    "/home/u/.local/share/hilt/dotnet",
			ScanExclusions: []string{"bin", "obj"},
			UI:             config.UIPreferences{Theme: "auto", VimKeys: true},
		},
		paths: platform.Paths{
			ConfigDir:   "/home/u/.config/hilt",
			StateDir:    "/home/u/.local/state/hilt",
			CacheDir:    "/home/u/.cache/hilt",
			DataDir:     "/home/u/.local/share/hilt",
			ManagedRoot: "/home/u/.local/share/hilt/dotnet",
		},
	}
}

func loadedModel(t *testing.T, backend fakeBackend, width, height int) Model {
	t.Helper()
	model := Now(backend)
	model = modelWithSize(t, model, width, height)
	model = press(t, model, key('r', "r"))
	message := inventoryMsg{inventory: backend.inventory, selectionErr: backend.selectionErr}
	if backend.selection.Root != "" {
		selection := backend.selection
		message.selection = &selection
	}
	model = apply(t, model, message)
	model = apply(t, model, doctorMsg{report: backend.report})
	return model
}

func modelWithSize(t *testing.T, model Model, width, height int) Model {
	t.Helper()
	return apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
}

func apply(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	updated, _ := model.Update(message)
	result, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	return result
}

func press(t *testing.T, model Model, message tea.KeyPressMsg) Model {
	t.Helper()
	return apply(t, model, message)
}

func key(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func TestViewFitsLayouts(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 12}, {80, 24}, {160, 48}} {
		backend := newFakeBackend()
		model := loadedModel(t, backend, size.width, size.height)
		view := model.View()
		lines := strings.Split(view.Content, "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d: got %d lines, want %d", size.width, size.height, len(lines), size.height)
		}
		for index, line := range lines {
			if width := ansi.StringWidth(line); width > size.width {
				t.Errorf("%dx%d: line %d width %d: %q", size.width, size.height, index, width, line)
			}
		}
		if !strings.Contains(view.Content, "HILT") || !strings.Contains(view.Content, "test") {
			t.Errorf("%dx%d: missing header: %q", size.width, size.height, view.Content)
		}
		if !view.AltScreen {
			t.Error("View must use the alternate screen")
		}
	}
}

func TestDashboardShowsInventory(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	view := model.View().Content
	for _, want := range []string{"Dashboard", "10.0.112", "9.0.300", "CachyOS", "pacman", "/usr/share/dotnet/dotnet", "Selected SDK", "system"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestNavigation(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('2', "2"))
	if model.screen != ScreenSDKs {
		t.Fatalf("screen = %v, want SDKs", model.screen)
	}
	view := model.View().Content
	if !strings.Contains(view, "SDKs & Runtimes") || !strings.Contains(view, "HOSTS") || !strings.Contains(view, "RUNTIMES") {
		t.Errorf("SDK screen missing content: %q", view)
	}
	if !strings.Contains(view, "SELECTED TOOLCHAIN") || !strings.Contains(view, "* 10.0.112") {
		t.Errorf("SDK screen missing selection: %q", view)
	}

	model = press(t, model, key('7', "7"))
	if model.screen != ScreenDiagnostics {
		t.Fatalf("screen = %v, want Diagnostics", model.screen)
	}
	view = model.View().Content
	if !strings.Contains(view, "dotnet is not on PATH") || !strings.Contains(view, "WARN") {
		t.Errorf("diagnostics missing content: %q", view)
	}

	model = press(t, model, key('d', "d"))
	if model.screen != ScreenDiagnostics {
		t.Fatalf("d key did not open diagnostics")
	}
}

func TestDiagnosticsRunsWhenOpened(t *testing.T) {
	model := Now(newFakeBackend())
	model = modelWithSize(t, model, 100, 30)
	updated, command := model.Update(key('d', "d"))
	model = updated.(Model)
	if command == nil || !model.loadingDoctor {
		t.Fatal("opening diagnostics should start a doctor run")
	}
	model = apply(t, model, doctorMsg{report: newFakeBackend().report})
	if model.loadingDoctor {
		t.Error("doctor should stop loading")
	}
	if !strings.Contains(model.View().Content, "Platform") {
		t.Error("diagnostics report not rendered")
	}
}

func TestRefreshAndSpinner(t *testing.T) {
	model := Now(newFakeBackend())
	model = modelWithSize(t, model, 100, 30)
	updated, command := model.Update(key('r', "r"))
	model = updated.(Model)
	if command == nil || !model.loadingInventory {
		t.Fatal("refresh should start an inventory load")
	}
	model = apply(t, model, inventoryMsg{err: errors.New("boom")})
	if !strings.Contains(model.View().Content, "Inventory failed: boom") {
		t.Error("inventory error not rendered")
	}
}

func TestNoDotnetIsNotFatal(t *testing.T) {
	backend := newFakeBackend()
	backend.inventory = dotnet.Inventory{Warnings: []string{"No .NET installation roots found; install the .NET SDK to use build and project features."}}
	backend.selection = dotnet.Selection{}
	backend.selectionErr = &dotnet.MissingSDKError{}
	model := loadedModel(t, backend, 120, 36)
	view := model.View().Content
	if !strings.Contains(view, "not detected") || !strings.Contains(view, "Phase 2") {
		t.Errorf("missing graceful no-.NET state: %q", view)
	}
}

func TestHelpOverlay(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('?', "?"))
	view := model.View().Content
	if !strings.Contains(view, "Keyboard shortcuts") || !strings.Contains(view, "refresh inventory") {
		t.Errorf("help overlay missing: %q", view)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.showHelp {
		t.Error("esc should close help")
	}
}

func TestQuit(t *testing.T) {
	model := Now(newFakeBackend())
	updated, command := model.Update(key('q', "q"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("quit should return a command")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("quit command returned %T, want tea.QuitMsg", command())
	}
}

func TestScrollingClamps(t *testing.T) {
	backend := newFakeBackend()
	backend.inventory.SDKs = nil
	for index := 0; index < 60; index++ {
		backend.inventory.SDKs = append(backend.inventory.SDKs, dotnet.SDK{
			Version: "1.0." + strings.Repeat("0", 1) + string(rune('a'+index%26)),
			Path:    "/tmp/sdk",
			Usable:  true,
		})
	}
	model := loadedModel(t, backend, 80, 24)
	model = press(t, model, key('2', "2"))
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.sdkScroll <= 0 {
		t.Fatal("pgdown did not scroll")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.sdkScroll != model.maxScroll() {
		t.Fatalf("end scroll = %d, want %d", model.sdkScroll, model.maxScroll())
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyHome})
	if model.sdkScroll != 0 {
		t.Fatalf("home scroll = %d, want 0", model.sdkScroll)
	}
	view := model.View().Content
	if lines := strings.Split(view, "\n"); len(lines) != 24 {
		t.Fatalf("scrolled view has %d lines, want 24", len(lines))
	}
}

func TestDiagnosticsScrollAndClamp(t *testing.T) {
	backend := newFakeBackend()
	backend.report.Findings = nil
	for index := 0; index < 40; index++ {
		backend.report.Findings = append(backend.report.Findings, diagnostics.Finding{
			ID: "check", Title: "Check", Severity: diagnostics.SeverityOK, Summary: "detail",
		})
	}
	model := loadedModel(t, backend, 80, 24)
	model = press(t, model, key('7', "7"))
	if model.diagScroll != 0 {
		t.Fatalf("initial scroll = %d", model.diagScroll)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.diagScroll <= 0 {
		t.Fatal("pgdown did not scroll diagnostics")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.diagScroll != model.maxScroll() {
		t.Fatalf("end scroll = %d, want %d", model.diagScroll, model.maxScroll())
	}
	if lines := strings.Split(model.View().Content, "\n"); len(lines) != 24 {
		t.Fatalf("scrolled diagnostics has %d lines, want 24", len(lines))
	}
}

func TestTooSmall(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 30, 6)
	view := model.View().Content
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("missing too-small message: %q", view)
	}
	if lines := strings.Split(view, "\n"); len(lines) != 6 {
		t.Errorf("too-small view has %d lines, want 6", len(lines))
	}
}

func TestSettingsShowsPaths(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('8', "8"))
	view := model.View().Content
	if !strings.Contains(view, "/home/u/.config/hilt/config.json") || !strings.Contains(view, "read-only") {
		t.Errorf("settings missing paths: %q", view)
	}
}
