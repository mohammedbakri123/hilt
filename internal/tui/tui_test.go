package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"io"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

type fakeCalls struct {
	mu              sync.Mutex
	names           []string
	sdkPlans        []string
	sdkInstalls     []install.Plan
	sdkRemoves      []string
	runtimePlans    []install.RuntimePlan
	runtimeInstalls []install.RuntimePlan
	runtimeRemoves  []string
	sdkUpdates      []string
}

func (c *fakeCalls) record(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = append(c.names, name)
}

func (c *fakeCalls) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.names...)
}

func (c *fakeCalls) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, value := range c.names {
		if value == name {
			count++
		}
	}
	return count
}

func (c *fakeCalls) installPlans() []install.Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]install.Plan(nil), c.sdkInstalls...)
}

func (c *fakeCalls) runtimeInstallPlans() []install.RuntimePlan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]install.RuntimePlan(nil), c.runtimeInstalls...)
}

type fakeBackend struct {
	inventory      dotnet.Inventory
	inventoryErr   error
	selection      dotnet.Selection
	selectionErr   error
	report         diagnostics.Report
	config         config.Config
	paths          platform.Paths
	configWarnings []string

	calls *fakeCalls

	sdkPlan             install.Plan
	sdkPlanErr          error
	installResult       install.Result
	installErr          error
	removeSDKResult     install.Removal
	removeSDKErr        error
	updateResult        install.UpdateResult
	updateErr           error
	runtimePlan         install.RuntimePlan
	runtimePlanErr      error
	runtimeResult       install.RuntimeResult
	runtimeInstallErr   error
	removeRuntimeResult install.Removal
	removeRuntimeErr    error
	sdkUpdates          []install.Update
	runtimeUpdates      []install.Update
	updatesErr          error
	installStarted      chan struct{}
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

func (f fakeBackend) PlanSDKInstall(_ context.Context, version string, _ bool) (install.Plan, error) {
	f.record("plan-sdk")
	f.calls.sdkPlans = append(f.calls.sdkPlans, version)
	if f.sdkPlanErr != nil {
		return install.Plan{}, f.sdkPlanErr
	}
	if f.sdkPlan.Version != "" {
		return f.sdkPlan, nil
	}
	return install.Plan{
		Version:    version,
		Requested:  version,
		RID:        "linux-x64",
		Root:       f.config.ManagedRoot + "/" + version,
		DotnetPath: f.config.ManagedRoot + "/" + version + "/dotnet",
	}, nil
}

func (f fakeBackend) InstallSDK(ctx context.Context, plan install.Plan, progress io.Writer) (install.Result, error) {
	f.record("install-sdk")
	f.calls.mu.Lock()
	f.calls.sdkInstalls = append(f.calls.sdkInstalls, plan)
	f.calls.mu.Unlock()
	if f.installStarted != nil {
		close(f.installStarted)
	}
	if f.installErr != nil {
		return install.Result{}, f.installErr
	}
	if f.installStarted != nil {
		<-ctx.Done()
		return install.Result{}, ctx.Err()
	}
	if progress != nil {
		_, _ = progress.Write([]byte("Downloading staged SDK\nVerified SHA-512\n"))
	}
	if f.installResult.Version != "" {
		return f.installResult, nil
	}
	return install.Result{
		Version:    plan.Version,
		RID:        plan.RID,
		Root:       plan.Root,
		DotnetPath: plan.DotnetPath,
	}, nil
}

func (f fakeBackend) RemoveSDK(_ context.Context, version string) (install.Removal, error) {
	f.record("remove-sdk")
	f.calls.mu.Lock()
	f.calls.sdkRemoves = append(f.calls.sdkRemoves, version)
	f.calls.mu.Unlock()
	if f.removeSDKErr != nil {
		return install.Removal{}, f.removeSDKErr
	}
	if f.removeSDKResult.Version != "" {
		return f.removeSDKResult, nil
	}
	return install.Removal{Product: install.ProductSDK, Version: version, Root: f.config.ManagedRoot + "/" + version}, nil
}

func (f fakeBackend) UpdateSDK(_ context.Context, version string, _ bool, progress io.Writer) (install.UpdateResult, error) {
	f.record("update-sdk")
	f.calls.mu.Lock()
	f.calls.sdkUpdates = append(f.calls.sdkUpdates, version)
	f.calls.mu.Unlock()
	if f.updateErr != nil {
		return install.UpdateResult{}, f.updateErr
	}
	if progress != nil {
		_, _ = progress.Write([]byte("Downloading update\n"))
	}
	if f.updateResult.From != "" {
		return f.updateResult, nil
	}
	return install.UpdateResult{From: version, To: version, UpdateAvailable: false, OldRetained: true}, nil
}

func (f fakeBackend) CheckSDKUpdates(context.Context) ([]install.Update, error) {
	f.record("check-sdk-updates")
	return f.sdkUpdates, f.updatesErr
}

func (f fakeBackend) PlanRuntimeInstall(_ context.Context, version string, framework install.FrameworkFlavor, _ bool) (install.RuntimePlan, error) {
	f.record("plan-runtime")
	f.calls.mu.Lock()
	f.calls.runtimePlans = append(f.calls.runtimePlans, install.RuntimePlan{Version: version, Framework: string(framework)})
	f.calls.mu.Unlock()
	if f.runtimePlanErr != nil {
		return install.RuntimePlan{}, f.runtimePlanErr
	}
	if f.runtimePlan.Version != "" {
		return f.runtimePlan, nil
	}
	return install.RuntimePlan{
		Version:    version,
		Requested:  version,
		RID:        "linux-x64",
		Framework:  string(framework),
		Root:       f.config.ManagedRoot + "/runtimes/" + version,
		Frameworks: []string{install.RuntimeFrameworkASPNETCore, install.RuntimeFrameworkNetCore},
	}, nil
}

func (f fakeBackend) InstallRuntime(_ context.Context, plan install.RuntimePlan, progress io.Writer) (install.RuntimeResult, error) {
	f.record("install-runtime")
	f.calls.mu.Lock()
	f.calls.runtimeInstalls = append(f.calls.runtimeInstalls, plan)
	f.calls.mu.Unlock()
	if f.runtimeInstallErr != nil {
		return install.RuntimeResult{}, f.runtimeInstallErr
	}
	if progress != nil {
		_, _ = progress.Write([]byte("Downloading runtime\n"))
	}
	if f.runtimeResult.Version != "" {
		return f.runtimeResult, nil
	}
	return install.RuntimeResult{
		Version:    plan.Version,
		RID:        plan.RID,
		Framework:  plan.Framework,
		Frameworks: plan.Frameworks,
		Root:       plan.Root,
	}, nil
}

func (f fakeBackend) RemoveRuntime(_ context.Context, version string) (install.Removal, error) {
	f.record("remove-runtime")
	f.calls.mu.Lock()
	f.calls.runtimeRemoves = append(f.calls.runtimeRemoves, version)
	f.calls.mu.Unlock()
	if f.removeRuntimeErr != nil {
		return install.Removal{}, f.removeRuntimeErr
	}
	if f.removeRuntimeResult.Version != "" {
		return f.removeRuntimeResult, nil
	}
	return install.Removal{Product: install.ProductRuntime, Version: version, Root: f.config.ManagedRoot + "/runtimes/" + version}, nil
}

func (f fakeBackend) CheckRuntimeUpdates(context.Context) ([]install.Update, error) {
	f.record("check-runtime-updates")
	return f.runtimeUpdates, nil
}

func (f fakeBackend) record(name string) {
	if f.calls != nil {
		f.calls.record(name)
	}
}

func newFakeBackend() fakeBackend {
	return fakeBackend{
		calls: &fakeCalls{},
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
	if !strings.Contains(view, "not detected") || !strings.Contains(view, "hilt sdk install") {
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

func collectCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	message := cmd()
	if message == nil {
		return nil
	}
	if batch, ok := message.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, command := range batch {
			messages = append(messages, collectCmd(command)...)
		}
		return messages
	}
	return []tea.Msg{message}
}

func runCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	return collectCmd(cmd)
}

func applyMessages(t *testing.T, model Model, messages []tea.Msg) Model {
	t.Helper()
	for _, message := range messages {
		if message == nil {
			continue
		}
		model = apply(t, model, message)
	}
	return model
}

func typeText(t *testing.T, model Model, text string) Model {
	t.Helper()
	for _, character := range text {
		model = press(t, model, key(character, string(character)))
	}
	return model
}

func findMessage[T any](t *testing.T, messages []tea.Msg) T {
	t.Helper()
	for _, message := range messages {
		if value, ok := message.(T); ok {
			return value
		}
	}
	var zero T
	t.Fatalf("message %T not found in %v", zero, messages)
	return zero
}

func TestSDKListFocusAndSelection(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 120, 36)
	model = press(t, model, key('2', "2"))
	if model.sdkFocus != focusSDKs {
		t.Fatalf("focus = %v, want SDKs", model.sdkFocus)
	}
	model = press(t, model, key('j', "j"))
	if model.sdkCursor != 1 {
		t.Fatalf("sdk cursor = %d, want 1", model.sdkCursor)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	if model.sdkFocus != focusRuntimes {
		t.Fatalf("focus = %v, want runtimes", model.sdkFocus)
	}
	model = press(t, model, key('j', "j"))
	if model.runtimeCursor != 1 {
		t.Fatalf("runtime cursor = %d, want 1", model.runtimeCursor)
	}
	view := model.View().Content
	if !strings.Contains(view, "RUNTIMES") || !strings.Contains(view, "(focused)") || !strings.Contains(view, ">") {
		t.Fatalf("runtime focus not rendered: %q", view)
	}
}

func TestSDKInstallFlow(t *testing.T) {
	backend := newFakeBackend()
	backend.sdkPlan = install.Plan{
		Version:    "10.0.112",
		Requested:  "latest",
		RID:        "linux-x64",
		Root:       "/home/u/.local/share/hilt/dotnet/10.0.112",
		DotnetPath: "/home/u/.local/share/hilt/dotnet/10.0.112/dotnet",
		SystemCopy: true,
	}
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('2', "2"))
	model = press(t, model, key('i', "i"))
	if model.prompt != promptInstallSDK {
		t.Fatalf("prompt = %v, want SDK install", model.prompt)
	}
	model = typeText(t, model, "latest")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("submit should return a plan command")
	}
	if backend.calls.count("plan-sdk") != 0 {
		t.Fatal("planning must not run inside Update")
	}
	model = applyMessages(t, model, runCmd(t, command))
	if model.confirm != confirmInstallSDK {
		t.Fatalf("confirm = %v, want install SDK", model.confirm)
	}
	view := model.View().Content
	if !strings.Contains(view, "CONFIRM") || !strings.Contains(view, "system copy") {
		t.Fatalf("confirmation summary missing: %q", view)
	}

	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("confirm should return an install command")
	}
	model = applyMessages(t, model, runCmd(t, command))
	if backend.calls.count("install-sdk") != 1 {
		t.Fatalf("install calls = %d, want 1", backend.calls.count("install-sdk"))
	}
	if model.operationRunning {
		t.Fatal("operation should be finished")
	}
	if !strings.Contains(model.View().Content, "Installed and verified managed SDK 10.0.112") {
		t.Fatalf("install result missing: %q", model.View().Content)
	}
	plans := backend.calls.installPlans()
	if len(plans) != 1 || plans[0].Version != "10.0.112" {
		t.Fatalf("install plans = %+v", plans)
	}
}

func TestRuntimeInstallPromptIncludesFramework(t *testing.T) {
	backend := newFakeBackend()
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('2', "2"))
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = press(t, model, key('i', "i"))
	if model.prompt != promptInstallRuntime {
		t.Fatalf("prompt = %v, want runtime install", model.prompt)
	}
	if !strings.Contains(model.View().Content, "framework all") {
		t.Fatalf("framework prompt missing: %q", model.View().Content)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	if model.promptFramework != install.FrameworkNetCore {
		t.Fatalf("framework = %s, want netcore", model.promptFramework)
	}
	model = typeText(t, model, "10.0.12")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if model.confirm != confirmInstallRuntime {
		t.Fatalf("confirm = %v, want runtime install", model.confirm)
	}
	if !strings.Contains(model.View().Content, "framework netcore") {
		t.Fatalf("runtime confirmation missing framework: %q", model.View().Content)
	}
	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	plans := backend.calls.runtimeInstallPlans()
	if len(plans) != 1 || plans[0].Framework != string(install.FrameworkNetCore) {
		t.Fatalf("runtime install plans = %+v", plans)
	}
}

func TestSystemAndUnownedSDKRemovalRefused(t *testing.T) {
	backend := newFakeBackend()
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('2', "2"))
	model = press(t, model, key('x', "x"))
	if model.confirm != confirmNone {
		t.Fatal("system SDK should not open a removal confirmation")
	}
	if !strings.Contains(model.View().Content, "provided by the system") {
		t.Fatalf("system refusal missing: %q", model.View().Content)
	}
	if backend.calls.count("remove-sdk") != 0 {
		t.Fatal("system SDK removal must not call the backend")
	}

	model = press(t, model, key('j', "j"))
	model = press(t, model, key('x', "x"))
	if !strings.Contains(model.View().Content, "not recorded in Hilt's install manifest") {
		t.Fatalf("unowned refusal missing: %q", model.View().Content)
	}
	if backend.calls.count("remove-sdk") != 0 {
		t.Fatal("unowned managed SDK removal must not call the backend")
	}
}

func TestOwnedSDKRemovalConfirms(t *testing.T) {
	backend := newFakeBackend()
	backend.inventory.SDKs[1].Owned = true
	backend.inventory.SDKs[1].Provenance = dotnet.ProvenanceOwned
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('2', "2"))
	model = press(t, model, key('j', "j"))
	model = press(t, model, key('x', "x"))
	if model.confirm != confirmRemoveSDK {
		t.Fatalf("confirm = %v, want remove SDK", model.confirm)
	}
	if !strings.Contains(model.View().Content, "Remove managed SDK 9.0.300") {
		t.Fatalf("removal summary missing: %q", model.View().Content)
	}
	updated, command := model.Update(key('y', "y"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if backend.calls.count("remove-sdk") != 1 {
		t.Fatalf("remove calls = %d, want 1", backend.calls.count("remove-sdk"))
	}
	if !strings.Contains(model.View().Content, "Removed managed SDK 9.0.300") {
		t.Fatalf("removal result missing: %q", model.View().Content)
	}
}

func TestUpdateCheckAndApply(t *testing.T) {
	backend := newFakeBackend()
	backend.sdkUpdates = []install.Update{{Version: "9.0.300", LatestVersion: "9.0.404", UpdateAvailable: true}}
	backend.updateResult = install.UpdateResult{From: "9.0.300", To: "9.0.404", UpdateAvailable: true, Installed: true, OldRetained: true}
	model := loadedModel(t, backend, 140, 40)
	model = press(t, model, key('2', "2"))
	model = press(t, model, key('j', "j"))
	updated, command := model.Update(key('u', "u"))
	model = updated.(Model)
	if command == nil || !model.loadingUpdates {
		t.Fatal("update check should start asynchronously")
	}
	if backend.calls.count("check-sdk-updates") != 0 {
		t.Fatal("update check must not run inside Update")
	}
	model = applyMessages(t, model, runCmd(t, command))
	if !model.updatesLoaded || model.loadingUpdates {
		t.Fatalf("updates not loaded: %+v", model)
	}
	if !strings.Contains(model.View().Content, "9.0.404 available") {
		t.Fatalf("update marker missing: %q", model.View().Content)
	}

	updated, command = model.Update(key('u', "u"))
	model = updated.(Model)
	if model.confirm != confirmUpdateSDK {
		t.Fatalf("confirm = %v, want update SDK", model.confirm)
	}
	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if backend.calls.count("update-sdk") != 1 {
		t.Fatalf("update calls = %d, want 1", backend.calls.count("update-sdk"))
	}
	if !strings.Contains(model.View().Content, "Updated SDK 9.0.300 to 9.0.404") {
		t.Fatalf("update result missing: %q", model.View().Content)
	}
}

func TestCancelInstallOperation(t *testing.T) {
	backend := newFakeBackend()
	backend.installStarted = make(chan struct{})
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('2', "2"))
	model = press(t, model, key('i', "i"))
	model = typeText(t, model, "10.0.112")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)

	results := make(chan []tea.Msg, 1)
	go func() {
		results <- collectCmd(command)
	}()
	<-backend.installStarted
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !model.operationCancelling {
		t.Fatal("esc should request cancellation")
	}
	messages := <-results
	model = applyMessages(t, model, messages)
	if model.operationRunning {
		t.Fatal("cancelled operation should finish")
	}
	if !strings.Contains(model.View().Content, "Operation cancelled") {
		t.Fatalf("cancellation status missing: %q", model.View().Content)
	}
}

func TestRuntimeUpdateApplyUsesSelectedFramework(t *testing.T) {
	backend := newFakeBackend()
	backend.runtimeUpdates = []install.Update{{Version: "10.0.12", LatestVersion: "10.0.13", UpdateAvailable: true}}
	model := loadedModel(t, backend, 140, 40)
	model = press(t, model, key('2', "2"))
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = press(t, model, key('j', "j"))
	selected, ok := model.selectedRuntime()
	if !ok || selected.Framework != install.RuntimeFrameworkNetCore {
		t.Fatalf("selected runtime = %+v", selected)
	}
	updated, command := model.Update(key('u', "u"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	updated, _ = model.Update(key('u', "u"))
	model = updated.(Model)
	if model.confirm != confirmUpdateRuntime {
		t.Fatalf("confirm = %v, want runtime update", model.confirm)
	}
	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	plans := backend.calls.runtimeInstallPlans()
	if len(plans) != 1 || plans[0].Version != "10.0.13" || plans[0].Framework != string(install.FrameworkNetCore) {
		t.Fatalf("runtime update plans = %+v", plans)
	}
	if !strings.Contains(model.View().Content, "Installed newer managed runtime 10.0.13") {
		t.Fatalf("runtime update result missing: %q", model.View().Content)
	}
}

func TestDashboardInstallShortcut(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 120, 36)
	model = press(t, model, key('i', "i"))
	if model.screen != ScreenSDKs || model.sdkFocus != focusSDKs || model.prompt != promptInstallSDK {
		t.Fatalf("dashboard i did not open the SDK install prompt: screen=%v focus=%v prompt=%v", model.screen, model.sdkFocus, model.prompt)
	}
}
