package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"io"
	"path/filepath"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
	"github.com/mohammedbakri123/hilt/internal/process"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
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
	available           []install.AvailableSDK
	availableErr        error
	availableCalls      *[]bool
	installStarted      chan struct{}
	projectResult       projects.Result
	projectErr          error
	projectCalls        *int
	state               *state.State
	stateErr            error
	buildResult         execution.Result
	buildErr            error
	buildLines          []string
	buildStarted        chan struct{}
	buildBlock          chan struct{}
	buildRequests       *[]execution.BuildRequest
	buildRecords        *[]state.BuildTarget
	buildRecordErr      error

	directories        map[string][]string
	directoryErr       map[string]error
	directoryCalls     *[]pickerDirectoryCall
	defaultProjectDir  *string
	setProjectDirCalls *[]string
	setProjectDirErr   error
}

type pickerDirectoryCall struct {
	path   string
	hidden bool
}

func (f fakeBackend) Version() string { return "test" }
func (f fakeBackend) Config() config.Config {
	current := f.config
	if f.defaultProjectDir != nil {
		current.DefaultProjectDir = *f.defaultProjectDir
	}
	return current
}
func (f fakeBackend) Paths() platform.Paths    { return f.paths }
func (f fakeBackend) ConfigWarnings() []string { return f.configWarnings }
func (f fakeBackend) State() state.State {
	if f.state == nil {
		return state.Defaults()
	}
	return *f.state
}
func (f fakeBackend) BuildProject(ctx context.Context, request execution.BuildRequest, stdout, stderr io.Writer) (execution.Result, error) {
	f.record("build")
	if f.buildRequests != nil {
		*f.buildRequests = append(*f.buildRequests, request)
	}
	if f.buildStarted != nil {
		close(f.buildStarted)
	}
	if len(f.buildLines) > 0 && stdout != nil {
		_, _ = io.WriteString(stdout, strings.Join(f.buildLines, "\n")+"\n")
	}
	if f.buildBlock != nil {
		select {
		case <-f.buildBlock:
		case <-ctx.Done():
			result := f.buildResult
			result.Started = true
			result.Cancelled = true
			return result, ctx.Err()
		}
	}
	if f.buildErr != nil {
		return f.buildResult, f.buildErr
	}
	return f.buildResult, nil
}

func (f fakeBackend) RecordBuildTarget(target state.BuildTarget) error {
	f.record("record-build")
	if f.buildRecords != nil {
		*f.buildRecords = append(*f.buildRecords, target)
	}
	if f.buildRecordErr != nil {
		return f.buildRecordErr
	}
	if f.state != nil {
		copy := target
		f.state.LastBuild = &copy
	}
	return nil
}

func (f fakeBackend) CreateProjects(_ context.Context, _ projects.CreatePlan, stdout, _ io.Writer) error {
	f.record("create-projects")
	if stdout != nil {
		_, _ = io.WriteString(stdout, "created project\n")
	}
	return nil
}

func (f fakeBackend) RecordProjectOpen(projectPath, workspacePath string) error {
	if f.stateErr != nil {
		return f.stateErr
	}
	if f.state == nil {
		return errors.New("state unavailable")
	}
	f.state.SchemaVersion = state.CurrentSchemaVersion
	f.state.ActiveProject = projectPath
	f.state.ActiveWorkspace = workspacePath
	recent := []state.RecentProject{{Path: projectPath, LastOpened: time.Now().UTC()}}
	for _, entry := range f.state.RecentProjects {
		if entry.Path == projectPath {
			continue
		}
		recent = append(recent, entry)
		if len(recent) == 10 {
			break
		}
	}
	f.state.RecentProjects = recent
	return nil
}

func (f fakeBackend) PinExternalProject(path string) error {
	if f.state == nil {
		return errors.New("state unavailable")
	}
	f.state.PinExternalProject(path)
	return nil
}

func (f fakeBackend) UnpinExternalProject(path string) error {
	if f.state == nil {
		return errors.New("state unavailable")
	}
	f.state.UnpinExternalProject(path)
	return nil
}
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

func (f fakeBackend) AvailableSDKs(_ context.Context, allowPrerelease bool) ([]install.AvailableSDK, error) {
	f.record("available-sdks")
	if f.availableCalls != nil {
		*f.availableCalls = append(*f.availableCalls, allowPrerelease)
	}
	return f.available, f.availableErr
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

func (f fakeBackend) ScanProjects(context.Context, string, bool) (projects.Result, error) {
	f.record("scan-projects")
	if f.projectCalls != nil {
		*f.projectCalls++
	}
	return f.projectResult, f.projectErr
}
func (f fakeBackend) ListDirectories(_ context.Context, path string, includeHidden bool) ([]string, error) {
	if f.directoryCalls != nil {
		*f.directoryCalls = append(*f.directoryCalls, pickerDirectoryCall{path: path, hidden: includeHidden})
	}
	if err := f.directoryErr[path]; err != nil {
		return nil, err
	}
	entries := f.directories[path]
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !includeHidden && strings.HasPrefix(filepath.Base(entry), ".") {
			continue
		}
		result = append(result, entry)
	}
	return result, nil
}

func (f fakeBackend) SetDefaultProjectDir(path string) error {
	if f.setProjectDirCalls != nil {
		*f.setProjectDirCalls = append(*f.setProjectDirCalls, path)
	}
	if f.setProjectDirErr != nil {
		return f.setProjectDirErr
	}
	if f.defaultProjectDir != nil {
		*f.defaultProjectDir = path
	}
	return nil
}

func (f fakeBackend) record(name string) {
	if f.calls != nil {
		f.calls.record(name)
	}
}

func newFakeBackend() fakeBackend {
	stateValue := state.Defaults()
	return fakeBackend{
		calls: &fakeCalls{},
		state: &stateValue,
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
	for _, size := range []struct{ width, height int }{{40, 12}, {60, 20}, {80, 24}, {160, 48}} {
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

func TestHomeShowsInventory(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	view := model.View().Content
	for _, want := range []string{"Home", "SYSTEM", ".NET TOOLCHAIN", "10.0.112", "9.0.300", "CachyOS", "pacman", "/usr/share/dotnet/dotnet", "Selected SDK", "system", "PROJECTS", "Press 2 to scan projects"} {
		if !strings.Contains(view, want) {
			t.Errorf("home missing %q", want)
		}
	}
}

func TestHomeShowsProjectScanSummary(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 120, 36)
	if model.screen != ScreenHome {
		model = press(t, model, key('1', "1"))
	}
	plain := ansi.Strip(model.View().Content)
	result := projectFixture()
	for _, want := range []string{
		"PROJECTS",
		"/work",
		fmt.Sprintf("%d projects  ·  %d workspaces", len(result.Projects), len(result.Workspaces)),
		"Selected project",
		"Shop",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("home project summary missing %q", want)
		}
	}
}

func TestNavigation(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('3', "3"))
	if model.screen != ScreenSDKs {
		t.Fatalf("screen = %v, want SDKs", model.screen)
	}
	view := model.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "SDKs & Runtimes") || !strings.Contains(plain, "HOSTS") || !strings.Contains(plain, "RUNTIMES") {
		t.Errorf("SDK screen missing content: %q", view)
	}
	if !strings.Contains(plain, "SELECTED TOOLCHAIN") || !strings.Contains(plain, "* 10.0.112") {
		t.Errorf("SDK screen missing selection: %q", view)
	}

	model = press(t, model, key('4', "4"))
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

func TestHelpFitsStandardTerminal(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 80, 24)
	model = press(t, model, key('?', "?"))
	view := model.View().Content
	for _, want := range []string{"Keyboard shortcuts", "NAVIGATION", "MANAGED .NET", "VIEW", "refresh inventory"} {
		if !strings.Contains(view, want) {
			t.Errorf("help missing %q", want)
		}
	}
	if lines := strings.Split(view, "\n"); len(lines) != 24 {
		t.Fatalf("help has %d lines, want 24", len(lines))
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
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('4', "4"))
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
	model = press(t, model, key('5', "5"))
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
	model = press(t, model, key('3', "3"))
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

func TestSDKTablesShowColumns(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('3', "3"))
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"SELECTED TOOLCHAIN", "VERSION", "SOURCE", "LOCATION", "FRAMEWORK", "managed"} {
		if !strings.Contains(plain, want) {
			t.Errorf("SDK table missing %q", want)
		}
	}
}

func TestHomeFooterShowsInstallHint(t *testing.T) {
	for _, width := range []int{80, 90} {
		model := loadedModel(t, newFakeBackend(), width, 24)
		lines := strings.Split(ansi.Strip(model.View().Content), "\n")
		footer := lines[len(lines)-1]
		if !strings.Contains(footer, "i install SDK") || !strings.Contains(footer, "q quit") {
			t.Fatalf("%d: home footer missing actions: %q", width, footer)
		}
	}
}

func TestSDKFooterKeepsQuitHint(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 80, 24)
	model = press(t, model, key('3', "3"))
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "? help") || !strings.Contains(footer, "q quit") {
		t.Fatalf("footer lost essential hints: %q", footer)
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
	model = press(t, model, key('3', "3"))
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

func TestSDKAvailablePickerHidesInstalledAndSelects(t *testing.T) {
	backend := newFakeBackend()
	calls := []bool{}
	backend.availableCalls = &calls
	backend.available = []install.AvailableSDK{
		{Version: "10.0.112", Channel: "10.0", SupportPhase: "active", ReleaseType: "lts", Latest: true},
		{Version: "9.0.404", Channel: "9.0", SupportPhase: "active", ReleaseType: "sts", Latest: true},
		{Version: "9.0.300", Channel: "9.0", SupportPhase: "active", ReleaseType: "sts"},
		{Version: "8.0.400", Channel: "8.0", SupportPhase: "active", ReleaseType: "lts", Latest: true},
	}
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('3', "3"))
	updated, command := model.Update(key('i', "i"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("SDK prompt should start an available-SDK request")
	}
	model = applyMessages(t, model, runCmd(t, command))
	if len(calls) != 1 || calls[0] {
		t.Fatalf("available calls = %v, want one stable-only request", calls)
	}

	view := ansi.Strip(model.View().Content)
	start := strings.Index(view, "AVAILABLE  2")
	end := strings.Index(view, "tab type")
	if start < 0 || end < start {
		t.Fatalf("picker section missing: %q", view)
	}
	picker := view[start:end]
	if strings.Contains(picker, "10.0.112") || strings.Contains(picker, "9.0.300") {
		t.Fatalf("installed SDKs were shown in the picker: %q", picker)
	}
	for _, want := range []string{"AVAILABLE  2", "9.0.404", "8.0.400", "installed hidden"} {
		if !strings.Contains(picker, want) {
			t.Errorf("picker missing %q: %q", want, picker)
		}
	}

	model = press(t, model, key('j', "j"))
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("selecting an SDK should return a plan command")
	}
	model = applyMessages(t, model, runCmd(t, command))
	if model.confirm != confirmInstallSDK {
		t.Fatalf("confirm = %v, want SDK install", model.confirm)
	}
	if len(backend.calls.sdkPlans) != 1 || backend.calls.sdkPlans[0] != "8.0.400" {
		t.Fatalf("plans = %v, want 8.0.400", backend.calls.sdkPlans)
	}
}

func TestSDKAvailablePickerKeepsTypedInput(t *testing.T) {
	backend := newFakeBackend()
	backend.available = []install.AvailableSDK{
		{Version: "10.0.200", Channel: "10.0", Latest: true},
	}
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('3', "3"))
	updated, command := model.Update(key('i', "i"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	if !model.promptFocusInput {
		t.Fatal("tab should focus the text input")
	}
	model = typeText(t, model, "latest")
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if len(backend.calls.sdkPlans) != 1 || backend.calls.sdkPlans[0] != "latest" {
		t.Fatalf("plans = %v, want typed latest", backend.calls.sdkPlans)
	}
}

func TestSDKAvailablePickerAllInstalled(t *testing.T) {
	backend := newFakeBackend()
	backend.available = []install.AvailableSDK{
		{Version: "10.0.112", Channel: "10.0", Latest: true},
		{Version: "9.0.300", Channel: "9.0", Latest: true},
	}
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('3', "3"))
	updated, command := model.Update(key('i', "i"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if !strings.Contains(ansi.Strip(model.View().Content), "already installed") {
		t.Fatalf("all-installed message missing: %q", model.View().Content)
	}

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command != nil {
		t.Fatal("enter with no picker rows should not submit")
	}
	if !model.statusIsError || !strings.Contains(model.statusMessage, "Press tab") {
		t.Fatalf("missing typed-input hint: %q", model.statusMessage)
	}
}

func TestSDKAvailablePickerError(t *testing.T) {
	backend := newFakeBackend()
	backend.availableErr = errors.New("release metadata unavailable")
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('3', "3"))
	updated, command := model.Update(key('i', "i"))
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Could not load available SDKs") || !strings.Contains(view, "release metadata unavailable") {
		t.Fatalf("available error missing: %q", view)
	}
	if model.availableLoading {
		t.Fatal("picker should stop loading after an error")
	}
}

func TestOperationDownloadProgressBar(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 120, 36)
	model = press(t, model, key('3', "3"))
	model.startOperation("Installing managed SDK 10.0.100", func() {})
	model = apply(t, model, progressMsg{downloaded: 5 << 20, total: 10 << 20, hasProgress: true})
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"50%", "5.0 MiB / 10.0 MiB", "█████", "░░░░░"} {
		if !strings.Contains(view, want) {
			t.Errorf("progress bar missing %q: %q", want, view)
		}
	}
}

func TestManagementPanelsAndStatusBadges(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('3', "3"))

	prompt := model
	prompt.startSDKPrompt()
	plain := ansi.Strip(prompt.View().Content)
	if !strings.Contains(plain, "INSTALL SDK") || !strings.Contains(plain, "version") {
		t.Fatalf("install prompt panel missing: %q", plain)
	}

	confirm := model
	confirm.confirm = confirmInstallSDK
	confirm.confirmSummary = []string{"Install managed SDK 10.0.112?", "Only Hilt-managed files are changed."}
	plain = ansi.Strip(confirm.View().Content)
	if !strings.Contains(plain, "CONFIRM INSTALL SDK") || !strings.Contains(plain, "• Install managed SDK") {
		t.Fatalf("confirmation panel missing: %q", plain)
	}

	operation := model
	operation.operationRunning = true
	operation.operationLabel = "Installing SDK 10.0.112..."
	operation.progressLines = []string{"Downloading archive", "Verifying SHA-512"}
	plain = ansi.Strip(operation.View().Content)
	if !strings.Contains(plain, "IN PROGRESS") || !strings.Contains(plain, "Verifying SHA-512") {
		t.Fatalf("operation panel missing: %q", plain)
	}

	status := model
	status.setStatus("Installed managed SDK 10.0.112.", false)
	if !strings.Contains(ansi.Strip(status.View().Content), "[ok] Installed managed SDK") {
		t.Fatalf("success status missing: %q", status.View().Content)
	}
	status.setStatus("Install failed.", true)
	if !strings.Contains(ansi.Strip(status.View().Content), "[err] Install failed") {
		t.Fatalf("error status missing: %q", status.View().Content)
	}
}

func TestRuntimeInstallPromptIncludesFramework(t *testing.T) {
	backend := newFakeBackend()
	model := loadedModel(t, backend, 120, 36)
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('3', "3"))
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
	model = press(t, model, key('3', "3"))
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

func TestHomeInstallShortcut(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 120, 36)
	model = press(t, model, key('i', "i"))
	if model.screen != ScreenSDKs || model.sdkFocus != focusSDKs || model.prompt != promptInstallSDK {
		t.Fatalf("home i did not open the SDK install prompt: screen=%v focus=%v prompt=%v", model.screen, model.sdkFocus, model.prompt)
	}
}

func projectFixture() projects.Result {
	return projects.Result{
		Root: "/work",
		Workspaces: []projects.Workspace{{
			Path:         "/work/Shop.sln",
			Name:         "Shop",
			Directory:    "/work",
			Kind:         projects.KindSolution,
			ProjectPaths: []string{"/work/src/Shop/Shop.csproj", "/work/tests/Shop.Tests/Shop.Tests.csproj"},
		}},
		Projects: []projects.Project{
			{
				Path:              "/work/src/Shop/Shop.csproj",
				Name:              "Shop",
				Directory:         "/work/src/Shop",
				Kind:              projects.KindProject,
				TargetFrameworks:  []string{"net10.0", "net9.0"},
				SDK:               "Microsoft.NET.Sdk",
				OutputType:        "Exe",
				ProjectReferences: []string{"/work/src/Library/Library.csproj"},
			},
			{
				Path:             "/work/tests/Shop.Tests/Shop.Tests.csproj",
				Name:             "Shop.Tests",
				Directory:        "/work/tests/Shop.Tests",
				Kind:             projects.KindProject,
				TargetFrameworks: []string{"net10.0"},
				SDK:              "Microsoft.NET.Sdk",
				TestProject:      true,
			},
		},
		Warnings: []string{"broken.csproj: unexpected EOF"},
	}
}

func openProjects(t *testing.T, backend fakeBackend, width, height int) Model {
	t.Helper()
	model := loadedModel(t, backend, width, height)
	updated, command := model.Update(key('2', "2"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("opening projects returned no scan command")
	}
	return apply(t, model, command())
}

func TestProjectsViewFitsLayouts(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 12}, {60, 20}, {80, 24}, {160, 48}} {
		backend := newFakeBackend()
		backend.projectResult = projectFixture()
		model := openProjects(t, backend, size.width, size.height)
		lines := strings.Split(model.View().Content, "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d: got %d lines, want %d", size.width, size.height, len(lines), size.height)
		}
		for index, line := range lines {
			if width := ansi.StringWidth(line); width > size.width {
				t.Errorf("%dx%d: line %d width %d: %q", size.width, size.height, index, width, line)
			}
		}
	}
}

func TestProjectsScreenScansAndShowsMetadata(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)
	if model.screen != ScreenProjects {
		t.Fatalf("screen = %v, want projects", model.screen)
	}
	if !model.projectsLoaded || model.projectsLoading {
		t.Errorf("loaded = %v loading = %v", model.projectsLoaded, model.projectsLoading)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"Projects & Solutions", "WORKSPACES", "Shop.sln", "PROJECTS", "Shop.Tests", "net10.0, net9.0", "[test]", "SELECTED PROJECT", "Microsoft.NET.Sdk", "WARNINGS", "broken.csproj"} {
		if !strings.Contains(plain, want) {
			t.Errorf("projects screen missing %q:\n%s", want, plain)
		}
	}
}

func TestProjectsCanImportAndPinExternalSource(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)

	model = press(t, model, key('a', "a"))
	if model.prompt != promptImportPath {
		t.Fatalf("prompt = %v, want import path", model.prompt)
	}
	for _, character := range "/home/me/src/Orders.sln" {
		model = press(t, model, key(character, string(character)))
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("import did not return a command")
	}
	model = apply(t, model, command())
	if model.prompt != promptNone {
		t.Fatal("import prompt remained open")
	}
	if got := model.backend.State().PinnedExternalProjects; len(got) != 1 || got[0] != "/home/me/src/Orders.sln" {
		t.Fatalf("pinned sources = %v", got)
	}
	if !strings.Contains(model.statusMessage, "Imported project source") {
		t.Fatalf("status = %q", model.statusMessage)
	}
}

func TestProjectsTreeCanCollapseAndExpandWorkspace(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 160, 30)

	model = press(t, model, key('e', "e"))
	collapsed := ansi.Strip(model.View().Content)
	if strings.Contains(collapsed, "Shop.Tests") {
		t.Fatalf("collapsed workspace still shows child project:\n%s", collapsed)
	}
	if !strings.Contains(model.statusMessage, "collapsed") {
		t.Fatalf("status = %q", model.statusMessage)
	}

	model = press(t, model, key('e', "e"))
	expanded := ansi.Strip(model.View().Content)
	if !strings.Contains(expanded, "Shop.Tests") {
		t.Fatalf("expanded workspace is missing child project:\n%s", expanded)
	}
}

func TestProjectsCreateWizardBuildsNewSolutionPlan(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)

	model = press(t, model, key('n', "n"))
	if !model.createWizard.open {
		t.Fatal("create wizard did not open")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	for _, character := range "Orders" {
		model = press(t, model, key(character, string(character)))
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.confirm != confirmCreateProjects || model.pendingCreatePlan == nil {
		t.Fatalf("confirm = %v plan = %+v", model.confirm, model.pendingCreatePlan)
	}
	if model.pendingCreatePlan.Solution != "/work/Orders.sln" {
		t.Fatalf("solution = %q", model.pendingCreatePlan.Solution)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"CONFIRM CREATE PROJECT", "Orders.sln"} {
		if !strings.Contains(plain, want) {
			t.Errorf("creation preview missing %q:\n%s", want, plain)
		}
	}
}

func TestProjectsSelectionSurvivesRefresh(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)
	model = press(t, model, key('j', "j"))
	if selected, ok := model.selectedProject(); !ok || selected.Name != "Shop.Tests" {
		t.Fatalf("selected = %+v", selected)
	}

	command := model.startProjectsScan()
	if command == nil {
		t.Fatal("refresh returned no command")
	}
	model = apply(t, model, command())
	if model.projectCursor != 1 {
		t.Errorf("cursor = %d, want 1", model.projectCursor)
	}
	selected, ok := model.selectedProject()
	if !ok || selected.Name != "Shop.Tests" {
		t.Errorf("selection after refresh = %+v", selected)
	}
}

func TestProjectsCancelAndErrors(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)

	command := model.startProjectsScan()
	if command == nil || !model.projectsLoading {
		t.Fatal("scan did not start")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.projectsLoading {
		t.Error("escape did not stop the scan")
	}
	if !strings.Contains(model.statusMessage, "cancelled") {
		t.Errorf("status = %q", model.statusMessage)
	}

	model = apply(t, model, projectsMsg{request: model.projectRequest, err: context.Canceled})
	if model.projectsErr != nil {
		t.Errorf("cancel produced an error: %v", model.projectsErr)
	}

	model = apply(t, model, projectsMsg{request: model.projectRequest, err: errors.New("permission denied")})
	if model.projectsErr == nil {
		t.Fatal("expected a scan error")
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "Project scan failed") || !strings.Contains(plain, "permission denied") {
		t.Errorf("error not rendered:\n%s", plain)
	}
}

func TestProjectsSelectionClampsOnEmptyRefresh(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)
	model = press(t, model, key('j', "j"))
	model = apply(t, model, projectsMsg{request: model.projectRequest, result: projects.Result{Root: "/work"}})
	if model.projectCursor != 0 {
		t.Errorf("cursor = %d, want 0", model.projectCursor)
	}
	if _, ok := model.selectedProject(); ok {
		t.Error("empty result still reports a selection")
	}
}

func TestProjectsFooterKeepsQuitHint(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 80, 24)
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "r rescan") || !strings.Contains(footer, "q quit") {
		t.Fatalf("projects footer missing actions: %q", footer)
	}
}

func TestStatusLineAppearsOnEveryScreen(t *testing.T) {
	projectsBackend := newFakeBackend()
	projectsBackend.projectResult = projectFixture()
	tests := []struct {
		name  string
		model Model
	}{
		{name: "home", model: loadedModel(t, newFakeBackend(), 100, 30)},
		{name: "projects", model: openProjects(t, projectsBackend, 100, 30)},
		{name: "diagnostics", model: press(t, loadedModel(t, newFakeBackend(), 100, 30), key('4', "4"))},
		{name: "settings", model: press(t, loadedModel(t, newFakeBackend(), 100, 30), key('5', "5"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := test.model
			model.setStatus("Status is visible here.", false)
			plain := ansi.Strip(model.View().Content)
			if !strings.Contains(plain, "[ok] Status is visible here.") {
				t.Fatalf("status line missing:\n%s", plain)
			}
			lines := strings.Split(plain, "\n")
			if len(lines) < 2 || !strings.Contains(lines[len(lines)-2], "Status is visible here.") {
				t.Fatalf("status line is not above the footer:\n%s", plain)
			}

			model.setStatus("Status failed here.", true)
			plain = ansi.Strip(model.View().Content)
			if !strings.Contains(plain, "[err] Status failed here.") {
				t.Fatalf("error status line missing:\n%s", plain)
			}
		})
	}
}

func TestQuitDuringOperationRequiresConfirmation(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model.startOperation("Installing managed SDK 9.0.300", func() {})

	updated, command := model.Update(key('q', "q"))
	model = updated.(Model)
	if command != nil {
		t.Fatal("q during an operation must not quit immediately")
	}
	if !model.quitConfirm {
		t.Fatal("q during an operation should open a quit confirmation")
	}
	if !model.operationRunning || model.operationCancelling {
		t.Fatalf("operation state changed: running=%v cancelling=%v", model.operationRunning, model.operationCancelling)
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "An operation is still running") || !strings.Contains(plain, "quit anyway") {
		t.Fatalf("quit confirmation missing: %q", plain)
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.quitConfirm {
		t.Fatal("esc should stay in the operation")
	}
	if !model.operationRunning || model.operationCancelling {
		t.Fatalf("esc should not cancel from the quit dialog: running=%v cancelling=%v", model.operationRunning, model.operationCancelling)
	}

	model = press(t, model, key('q', "q"))
	updated, command = model.Update(key('y', "y"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("confirming quit should return a command")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("quit command returned %T, want tea.QuitMsg", command())
	}
}

func TestEscapeCancelsDuringOperation(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	cancelled := false
	model.startOperation("Installing managed SDK 9.0.300", func() { cancelled = true })

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !cancelled {
		t.Fatal("esc should cancel the running operation")
	}
	if !model.operationCancelling {
		t.Fatal("operation should be marked as cancelling")
	}
	if model.quitConfirm {
		t.Fatal("esc must not open the quit confirmation")
	}
}

func TestKeymapHasNoConflicts(t *testing.T) {
	if conflicts := keymapConflicts(keymap); len(conflicts) > 0 {
		t.Fatalf("keymap conflicts: %v", conflicts)
	}
}

func TestKeymapDetectsDuplicateBindings(t *testing.T) {
	conflicted := map[keyContext]keyContextSpec{
		ctxHome: {Bindings: []keyBinding{
			{Key: "q", Action: actionQuit, Label: "quit"},
			{Key: "q", Action: actionHelp, Label: "help"},
		}},
	}
	conflicts := keymapConflicts(conflicted)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want one duplicate", conflicts)
	}
	if !strings.Contains(conflicts[0], `duplicate key "q"`) {
		t.Fatalf("unexpected conflict: %q", conflicts[0])
	}
}

func TestKeymapActions(t *testing.T) {
	tests := []struct {
		context keyContext
		key     string
		want    keyAction
	}{
		{context: ctxQuitConfirm, key: "y/enter", want: actionQuit},
		{context: ctxOperation, key: "esc", want: actionCancelOperation},
		{context: ctxSDKs, key: "u", want: actionUpdates},
		{context: ctxProjectsLoading, key: "esc", want: actionCancelScan},
		{context: ctxProjectFilter, key: "type", want: actionFilter},
	}
	for _, test := range tests {
		got, ok := actionFor(test.context, test.key)
		if !ok || got != test.want {
			t.Errorf("%s/%s = %q, %v; want %q", test.context, test.key, got, ok, test.want)
		}
	}
	if _, ok := actionFor(ctxHome, "z"); ok {
		t.Error("unknown key should not resolve")
	}
}

func TestFooterMatchesKeymap(t *testing.T) {
	operation := loadedModel(t, newFakeBackend(), 160, 40)
	operation.startOperation("Installing managed SDK 9.0.300", func() {})

	sdkInput := loadedModel(t, newFakeBackend(), 160, 40)
	sdkInput.prompt = promptInstallSDK
	sdkInput.promptFocusInput = true

	sdkList := loadedModel(t, newFakeBackend(), 160, 40)
	sdkList.prompt = promptInstallSDK
	sdkList.promptFocusInput = false

	runtimePrompt := loadedModel(t, newFakeBackend(), 160, 40)
	runtimePrompt.prompt = promptInstallRuntime

	confirmation := loadedModel(t, newFakeBackend(), 160, 40)
	confirmation.confirm = confirmInstallSDK

	quitConfirm := loadedModel(t, newFakeBackend(), 160, 40)
	quitConfirm.quitConfirm = true

	updates := loadedModel(t, newFakeBackend(), 160, 40)
	updates.loadingUpdates = true

	projectsLoading := loadedModel(t, newFakeBackend(), 160, 40)
	projectsLoading.screen = ScreenProjects
	projectsLoading.projectsLoading = true

	projectFiltering := loadedModel(t, newFakeBackend(), 160, 40)
	projectFiltering.screen = ScreenProjects
	projectFiltering.projectFiltering = true

	settings := loadedModel(t, newFakeBackend(), 160, 40)
	settings.screen = ScreenSettings

	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	projects := openProjects(t, backend, 160, 40)

	drawer := loadedModel(t, newFakeBackend(), 160, 40)
	drawer.drawer.open = true

	building := loadedModel(t, newFakeBackend(), 160, 40)
	building.buildRunning = true
	building.buildTarget = projectFixture().Projects[0]
	building.drawer.status = "building · Shop"

	buildingOutput := building
	buildingOutput.drawer.open = true

	tests := []struct {
		name    string
		model   Model
		context keyContext
	}{
		{name: "quit confirm", model: quitConfirm, context: ctxQuitConfirm},
		{name: "operation", model: operation, context: ctxOperation},
		{name: "updates", model: updates, context: ctxUpdates},
		{name: "sdk input", model: sdkInput, context: ctxPromptSDKInput},
		{name: "sdk list", model: sdkList, context: ctxPromptSDKList},
		{name: "runtime prompt", model: runtimePrompt, context: ctxPromptRuntime},
		{name: "confirmation", model: confirmation, context: ctxConfirm},
		{name: "home", model: loadedModel(t, newFakeBackend(), 160, 40), context: ctxHome},
		{name: "sdks", model: press(t, loadedModel(t, newFakeBackend(), 160, 40), key('3', "3")), context: ctxSDKs},
		{name: "diagnostics", model: press(t, loadedModel(t, newFakeBackend(), 160, 40), key('4', "4")), context: ctxDiagnostics},
		{name: "projects loading", model: projectsLoading, context: ctxProjectsLoading},
		{name: "projects", model: openProjects(t, backend, 110, 30), context: ctxProjects},
		{name: "project split", model: projects, context: ctxProjectSplit},
		{name: "project filter", model: projectFiltering, context: ctxProjectFilter},
		{name: "output", model: drawer, context: ctxOutput},
		{name: "building", model: building, context: ctxBuild},
		{name: "building output", model: buildingOutput, context: ctxBuildOutput},
		{name: "settings", model: settings, context: ctxSettings},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if context := test.model.footerContext(); context != test.context {
				t.Fatalf("footer context = %s, want %s", context, test.context)
			}
			plain := ansi.Strip(test.model.View().Content)
			lines := strings.Split(plain, "\n")
			footer := lines[len(lines)-1]
			for _, binding := range bindingsFor(test.context) {
				if !strings.Contains(footer, binding.Key+" "+binding.Label) {
					t.Errorf("footer missing %q: %q", binding.Key+" "+binding.Label, footer)
				}
			}
		})
	}
}

func TestHelpMatchesKeymap(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, key('?', "?"))
	plain := ansi.Strip(model.View().Content)
	for _, context := range []keyContext{ctxHelpNavigation, ctxHelpManaged, ctxHelpView} {
		for _, binding := range bindingsFor(context) {
			if !strings.Contains(plain, binding.Key) {
				t.Errorf("help missing key %q", binding.Key)
			}
			if binding.Help != "" && !strings.Contains(plain, binding.Help) {
				t.Errorf("help missing description %q", binding.Help)
			}
		}
	}
}

func TestQuitDuringUpdateCheckRequiresConfirmation(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model.loadingUpdates = true

	updated, command := model.Update(key('q', "q"))
	model = updated.(Model)
	if command != nil || !model.quitConfirm {
		t.Fatalf("q during update check should open confirmation: confirm=%v command=%v", model.quitConfirm, command)
	}
	if !model.loadingUpdates || model.operationCancelling {
		t.Fatalf("update check state changed: loading=%v cancelling=%v", model.loadingUpdates, model.operationCancelling)
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "Checking for managed updates") {
		t.Fatalf("quit confirmation should name the update check: %q", plain)
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.quitConfirm || !model.loadingUpdates {
		t.Fatalf("esc should stay in the update check: confirm=%v loading=%v", model.quitConfirm, model.loadingUpdates)
	}
}

func TestTopLevelNavigationHasFiveDestinations(t *testing.T) {
	want := []string{"Home", "Projects", "SDKs", "Diagnostics", "Settings"}
	if len(screens) != len(want) {
		t.Fatalf("screens = %d, want %d", len(screens), len(want))
	}
	for index, short := range want {
		if screens[index].short != short {
			t.Errorf("screen %d = %q, want %q", index+1, screens[index].short, short)
		}
	}

	model := loadedModel(t, newFakeBackend(), 100, 30)
	for index := range want {
		model = press(t, model, key(rune('1'+index), string(rune('1'+index))))
		if model.screen != Screen(index) {
			t.Errorf("key %d selected screen %v, want %v", index+1, model.screen, Screen(index))
		}
	}
}

func TestProjectsSplitFocusAndScrolling(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	result := projectFixture()
	for index := 0; index < 30; index++ {
		result.Projects = append(result.Projects, projects.Project{
			Path:             fmt.Sprintf("/work/src/Extra%02d/Extra%02d.csproj", index, index),
			Name:             fmt.Sprintf("Extra%02d", index),
			Directory:        fmt.Sprintf("/work/src/Extra%02d", index),
			Kind:             projects.KindProject,
			TargetFrameworks: []string{"net10.0"},
			SDK:              "Microsoft.NET.Sdk",
		})
	}
	for index := 0; index < 30; index++ {
		result.Projects[0].ProjectReferences = append(result.Projects[0].ProjectReferences, fmt.Sprintf("/work/src/Ref%02d/Ref%02d.csproj", index, index))
	}
	backend.projectResult = result
	model := openProjects(t, backend, 160, 30)
	if !model.projectSplit() {
		t.Fatal("projects should use split panes at 160 columns")
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"OVERVIEW", "PROJECTS [focus]", "focus: project list", "Shop.csproj"} {
		if !strings.Contains(plain, want) {
			t.Errorf("split projects missing %q:\n%s", want, plain)
		}
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	if model.projectFocus != projectFocusDetail {
		t.Fatalf("tab focus = %v, want detail", model.projectFocus)
	}
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "OVERVIEW [focus]") || !strings.Contains(plain, "focus: project detail") {
		t.Fatalf("detail focus not visible:\n%s", plain)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.projectDetailScroll <= 0 {
		t.Fatal("pgdown did not scroll the focused detail pane")
	}
	if model.projectListScroll != 0 {
		t.Fatalf("detail scrolling changed the list scroll to %d", model.projectListScroll)
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.projectListScroll <= 0 {
		t.Fatal("pgdown did not scroll the focused list pane")
	}
	model = press(t, model, key('j', "j"))
	if model.projectCursor == 0 {
		t.Fatal("selection did not move")
	}
	if model.projectDetailScroll != 0 {
		t.Fatalf("list scrolling changed the detail scroll to %d", model.projectDetailScroll)
	}
}

func TestSectionNavigationUsesBrackets(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = press(t, model, key('j', "j"))
	if model.screen != ScreenHome {
		t.Fatalf("tab or j changed section to %v", model.screen)
	}
	model = press(t, model, key(']', "]"))
	if model.screen != ScreenProjects {
		t.Fatalf("] selected %v, want Projects", model.screen)
	}
	model = press(t, model, key('[', "["))
	if model.screen != ScreenHome {
		t.Fatalf("[ selected %v, want Home", model.screen)
	}
	model = press(t, model, key('[', "["))
	if model.screen != ScreenSettings {
		t.Fatalf("[ did not wrap to Settings: %v", model.screen)
	}
}

func TestProjectsNarrowLayoutKeepsSectionNavigation(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 80, 24)
	if model.projectSplit() {
		t.Fatal("projects should use the stacked layout at 80 columns")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	if model.screen != ScreenProjects {
		t.Fatalf("tab navigated to %v, want Projects", model.screen)
	}
	model = press(t, model, key(']', "]"))
	if model.screen != ScreenSDKs {
		t.Fatalf("] navigated to %v, want SDKs", model.screen)
	}
	model = press(t, model, key('[', "["))
	if model.screen != ScreenProjects {
		t.Fatalf("[ navigated to %v, want Projects", model.screen)
	}
}

func TestProjectsFilterSelectionAndEmptyState(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)
	model = press(t, model, key('j', "j"))
	if selected, ok := model.selectedProject(); !ok || selected.Name != "Shop.Tests" {
		t.Fatalf("selected = %+v", selected)
	}

	model = press(t, model, key('/', "/"))
	if !model.projectFiltering {
		t.Fatal("/ should start filtering")
	}
	for _, character := range "shop" {
		model = press(t, model, key(character, string(character)))
	}
	if model.projectFilter != "shop" {
		t.Fatalf("filter = %q, want shop", model.projectFilter)
	}
	if selected, ok := model.selectedProject(); !ok || selected.Name != "Shop.Tests" {
		t.Fatalf("selection after filter = %+v", selected)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"filter: /shop", "2/2 projects", "Shop.Tests"} {
		if !strings.Contains(plain, want) {
			t.Errorf("filtered list missing %q:\n%s", want, plain)
		}
	}

	model = press(t, model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	for _, character := range "zzz" {
		model = press(t, model, key(character, string(character)))
	}
	plain = ansi.Strip(model.View().Content)
	for _, want := range []string{`No projects match "zzz".`, "Press esc to clear"} {
		if !strings.Contains(plain, want) {
			t.Errorf("no-results state missing %q:\n%s", want, plain)
		}
	}
	if _, ok := model.selectedProject(); ok {
		t.Error("empty filter still reports a selection")
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.projectFilter != "" || model.projectFiltering {
		t.Fatalf("esc did not clear the filter: %q filtering=%v", model.projectFilter, model.projectFiltering)
	}
	if _, ok := model.selectedProject(); !ok {
		t.Fatal("clearing the filter should restore the project list")
	}

	model = press(t, model, key('/', "/"))
	for _, character := range "tests" {
		model = press(t, model, key(character, string(character)))
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.projectFiltering {
		t.Fatal("enter should leave filter input")
	}
	if model.projectFilter != "tests" {
		t.Fatalf("enter cleared the filter: %q", model.projectFilter)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.projectFilter != "" {
		t.Fatalf("esc did not clear the applied filter: %q", model.projectFilter)
	}
}

func TestProjectsEnterMarksActiveAndShowsRecent(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 40)
	model = press(t, model, key('j', "j"))
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))

	value := backend.State()
	if value.ActiveProject != "/work/tests/Shop.Tests/Shop.Tests.csproj" {
		t.Fatalf("active project = %q", value.ActiveProject)
	}
	if value.ActiveWorkspace != "/work/Shop.sln" {
		t.Fatalf("active workspace = %q", value.ActiveWorkspace)
	}
	if len(value.RecentProjects) != 1 || value.RecentProjects[0].Path != value.ActiveProject {
		t.Fatalf("recent projects = %+v", value.RecentProjects)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "Active project: Shop.Tests") {
		t.Fatalf("status missing active project:\n%s", ansi.Strip(model.View().Content))
	}

	model = press(t, model, key('1', "1"))
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"Active project", value.ActiveProject, "Recent projects", "Shop.Tests.csproj"} {
		if !strings.Contains(plain, want) {
			t.Errorf("home state missing %q:\n%s", want, plain)
		}
	}
}

func TestProjectsEnterReportsStateErrors(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	backend.stateErr = errors.New("disk is read-only")
	model := openProjects(t, backend, 100, 30)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	model = applyMessages(t, model, runCmd(t, command))
	if !strings.Contains(ansi.Strip(model.View().Content), "Could not remember project: disk is read-only") {
		t.Fatalf("state error missing:\n%s", ansi.Strip(model.View().Content))
	}
}

func TestProjectsMarksExternalProjects(t *testing.T) {
	backend := newFakeBackend()
	backend.paths.ProjectsRoot = "/work"
	result := projectFixture()
	result.Projects = append(result.Projects, projects.Project{
		Path:             "/elsewhere/Other/Other.csproj",
		Name:             "Other",
		Directory:        "/elsewhere/Other",
		Kind:             projects.KindProject,
		TargetFrameworks: []string{"net10.0"},
		SDK:              "Microsoft.NET.Sdk",
	})
	backend.projectResult = result
	model := openProjects(t, backend, 100, 30)
	model = press(t, model, key('j', "j"))
	model = press(t, model, key('j', "j"))
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"[external]", "Location", "external", "/elsewhere/Other/Other.csproj"} {
		if !strings.Contains(plain, want) {
			t.Errorf("external project missing %q:\n%s", want, plain)
		}
	}
}

func TestProjectTabs(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	model := openProjects(t, backend, 100, 30)
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "[Overview]") || !strings.Contains(plain, "Microsoft.NET.Sdk") {
		t.Fatalf("overview tab missing:\n%s", plain)
	}

	model = press(t, model, key('.', "."))
	if model.projectTab != projectTabPackages {
		t.Fatalf("tab = %v, want packages", model.projectTab)
	}
	plain = ansi.Strip(model.View().Content)
	for _, want := range []string{"[Packages]", "No package actions yet"} {
		if !strings.Contains(plain, want) {
			t.Errorf("packages tab missing %q:\n%s", want, plain)
		}
	}

	model = press(t, model, key('.', "."))
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "[Data]") || !strings.Contains(plain, "No migrations loaded") {
		t.Errorf("data tab missing:\n%s", plain)
	}

	model = press(t, model, key('.', "."))
	plain = ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "[Output]") || !strings.Contains(plain, "No build or run output yet") {
		t.Errorf("output tab missing:\n%s", plain)
	}

	model = press(t, model, key('.', "."))
	if model.projectTab != projectTabOverview {
		t.Fatalf("tab did not wrap to overview: %v", model.projectTab)
	}
	model = press(t, model, key(',', ","))
	if model.projectTab != projectTabOutput {
		t.Fatalf("tab did not wrap backwards to output: %v", model.projectTab)
	}
}

func TestProjectTabsResetDetailScroll(t *testing.T) {
	backend := newFakeBackend()
	result := projectFixture()
	for index := 0; index < 30; index++ {
		result.Projects[0].ProjectReferences = append(result.Projects[0].ProjectReferences, fmt.Sprintf("/work/src/Ref%02d/Ref%02d.csproj", index, index))
	}
	backend.projectResult = result
	model := openProjects(t, backend, 160, 30)
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.projectDetailScroll == 0 {
		t.Fatal("detail did not scroll")
	}
	model = press(t, model, key('.', "."))
	if model.projectDetailScroll != 0 {
		t.Fatalf("tab change kept detail scroll %d", model.projectDetailScroll)
	}
}

func TestOutputDrawerStartsClosedAndToggles(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 100, 30)
	if model.drawer.open {
		t.Fatal("output drawer should start closed")
	}
	if strings.Contains(ansi.Strip(model.View().Content), "No build output yet") {
		t.Fatal("closed drawer should not render its body")
	}

	model = press(t, model, key('o', "o"))
	if !model.drawer.open {
		t.Fatal("o should open the output drawer")
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"OUTPUT", "[follow]", "No build output yet", "o close"} {
		if !strings.Contains(plain, want) {
			t.Errorf("open drawer missing %q", want)
		}
	}

	model = press(t, model, key('o', "o"))
	if model.drawer.open {
		t.Fatal("o should close the output drawer")
	}
	model = press(t, model, key('o', "o"))
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.drawer.open {
		t.Fatal("esc should close the output drawer")
	}
}

func TestOutputDrawerFitsLayouts(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 20}, {80, 24}, {160, 48}} {
		model := loadedModel(t, newFakeBackend(), size.width, size.height)
		model = press(t, model, key('o', "o"))
		if model.drawerHeight() == 0 {
			t.Fatalf("%dx%d: drawer has no height", size.width, size.height)
		}
		if model.bodyHeight() < 3 {
			t.Fatalf("%dx%d: body height = %d", size.width, size.height, model.bodyHeight())
		}
		lines := strings.Split(model.View().Content, "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d: got %d lines, want %d", size.width, size.height, len(lines), size.height)
		}
		for index, line := range lines {
			if width := ansi.StringWidth(line); width > size.width {
				t.Errorf("%dx%d: line %d width %d: %q", size.width, size.height, index, width, line)
			}
		}
		titleLine := 3 + model.bodyHeight()
		if titleLine >= len(lines) || !strings.Contains(ansi.Strip(lines[titleLine]), "OUTPUT") {
			t.Fatalf("%dx%d: drawer title not at line %d:\n%s", size.width, size.height, titleLine, ansi.Strip(model.View().Content))
		}
		footer := ansi.Strip(lines[len(lines)-1])
		if !strings.Contains(footer, "o close") || !strings.Contains(footer, "q quit") {
			t.Errorf("%dx%d: drawer footer = %q", size.width, size.height, footer)
		}
	}
}

func TestOutputDrawerFollowAndScroll(t *testing.T) {
	model := loadedModel(t, newFakeBackend(), 80, 24)
	model.drawer.open = true
	for index := 0; index < 40; index++ {
		model.appendDrawerLine(fmt.Sprintf("line %02d", index))
	}
	if !model.drawer.follow {
		t.Fatal("appending while following should keep follow enabled")
	}
	if offset := model.drawerOffset(); offset != model.drawerMaxScroll() {
		t.Fatalf("follow offset = %d, want %d", offset, model.drawerMaxScroll())
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if model.drawer.follow {
		t.Fatal("pgup should pause follow")
	}
	model = press(t, model, key('f', "f"))
	if !model.drawer.follow {
		t.Fatal("f should resume follow")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyHome})
	if model.drawer.follow || model.drawerOffset() != 0 {
		t.Fatal("home should pause follow at the top")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if !model.drawer.follow {
		t.Fatal("end should resume follow")
	}
}

func startBuildAndDrain(t *testing.T, model *Model, command tea.Cmd) []tea.Msg {
	t.Helper()
	if command == nil {
		t.Fatal("build returned no command")
	}
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("build command returned %T, want a batch", command())
	}
	done := make(chan tea.Msg, 1)
	go func() {
		done <- batch[0]()
	}()
	messages := make([]tea.Msg, 0, 8)
	for message := range model.buildCh {
		messages = append(messages, message)
	}
	messages = append(messages, <-done)
	return messages
}

func TestBuildStreamsOutputAndPersistsTarget(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	requests := []execution.BuildRequest{}
	records := []state.BuildTarget{}
	backend.buildRequests = &requests
	backend.buildRecords = &records
	backend.buildResult = execution.Result{ExitCode: 0, Duration: 1250 * time.Millisecond, Started: true}
	backend.buildLines = []string{"Build succeeded.", "warning: NU1900"}

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("b should return a build command")
	}
	if !model.buildRunning || !model.drawer.open || !model.drawer.follow {
		t.Fatalf("build state: running=%v open=%v follow=%v", model.buildRunning, model.drawer.open, model.drawer.follow)
	}
	if backend.calls.count("build") != 0 {
		t.Fatal("build must not run inside Update")
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "OUTPUT") || !strings.Contains(ansi.Strip(model.View().Content), "building") {
		t.Fatalf("building drawer missing: %q", ansi.Strip(model.View().Content))
	}

	messages := startBuildAndDrain(t, &model, command)
	model = applyMessages(t, model, messages)
	if backend.calls.count("build") != 1 || backend.calls.count("record-build") != 1 {
		t.Fatalf("calls = %v", backend.calls.snapshot())
	}
	if model.buildRunning || model.buildCancelling {
		t.Fatalf("build did not finish: running=%v cancelling=%v", model.buildRunning, model.buildCancelling)
	}
	if model.buildResult == nil || model.buildResult.ExitCode != 0 {
		t.Fatalf("result = %+v", model.buildResult)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"Build succeeded.", "warning: NU1900", "exit 0", "Built Shop in 1.25s."} {
		if !strings.Contains(plain, want) {
			t.Errorf("completed build missing %q:\n%s", want, plain)
		}
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %+v", requests)
	}
	request := requests[0]
	if request.DotnetPath != "/usr/share/dotnet/dotnet" ||
		request.ProjectPath != "/work/src/Shop/Shop.csproj" ||
		request.WorkingDirectory != "/work/src/Shop" ||
		request.Configuration != "Debug" {
		t.Errorf("request = %+v", request)
	}
	if len(request.Environment) == 0 {
		t.Error("build must use the selected toolchain environment")
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record.ProjectPath != "/work/src/Shop/Shop.csproj" ||
		record.WorkspacePath != "/work/Shop.sln" ||
		record.SDKVersion != "10.0.112" ||
		record.Configuration != "Debug" {
		t.Errorf("record = %+v", record)
	}
	if model.backend.State().LastBuild == nil || model.backend.State().LastBuild.ProjectPath != record.ProjectPath {
		t.Errorf("last build not persisted: %+v", model.backend.State().LastBuild)
	}
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Shop", "SDK 10.0.112", "/work/src/Shop", "dotnet build"} {
		if !strings.Contains(view, want) {
			t.Errorf("drawer metadata missing %q:\n%s", want, view)
		}
	}
}

func TestBuildNonZeroExitShowsFailureAndRecordsTarget(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	records := []state.BuildTarget{}
	backend.buildRecords = &records
	backend.buildResult = execution.Result{ExitCode: 2, Duration: 80 * time.Millisecond, Started: true}
	backend.buildErr = &process.ExitError{Code: 2}
	backend.buildLines = []string{"error CS0001: broken"}

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	model = applyMessages(t, model, startBuildAndDrain(t, &model, command))

	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"Build failed: Shop exited with code 2.", "error CS0001", "exit 2"} {
		if !strings.Contains(plain, want) {
			t.Errorf("failure missing %q:\n%s", want, plain)
		}
	}
	if !model.statusIsError {
		t.Error("failed build should set an error status")
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v, want the exercised target", records)
	}
}

func TestBuildCancellation(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	records := []state.BuildTarget{}
	backend.buildRecords = &records
	backend.buildResult = execution.Result{ExitCode: -1, Started: true}
	backend.buildBlock = make(chan struct{})
	backend.buildStarted = make(chan struct{})

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	batch := command().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() {
		done <- batch[0]()
	}()
	<-backend.buildStarted

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !model.buildCancelling || !model.buildRunning {
		t.Fatalf("escape should request cancellation: running=%v cancelling=%v", model.buildRunning, model.buildCancelling)
	}
	if !strings.Contains(model.statusMessage, "Cancelling build") {
		t.Fatalf("status = %q", model.statusMessage)
	}
	model = apply(t, model, <-done)
	if model.buildRunning || model.buildCancelling {
		t.Fatal("cancelled build should finish")
	}
	if !strings.Contains(model.statusMessage, "Build cancelled.") {
		t.Fatalf("status = %q", model.statusMessage)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "cancelled") {
		t.Fatalf("drawer status missing cancellation: %q", ansi.Strip(model.View().Content))
	}
	if len(records) != 1 {
		t.Fatalf("cancelled but started builds should be recorded: %+v", records)
	}
}

func TestBuildStartFailureIsNotRecorded(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	records := []state.BuildTarget{}
	backend.buildRecords = &records
	backend.buildResult = execution.Result{ExitCode: -1}
	backend.buildErr = errors.New("start failed")

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	model = applyMessages(t, model, startBuildAndDrain(t, &model, command))

	if len(records) != 0 {
		t.Fatalf("start failure must not record a build target: %+v", records)
	}
	if model.backend.State().LastBuild != nil {
		t.Fatalf("start failure persisted state: %+v", model.backend.State().LastBuild)
	}
	if !strings.Contains(model.statusMessage, "Build failed") {
		t.Fatalf("status = %q", model.statusMessage)
	}
}

func TestBuildRequiresProjectAndSDK(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projects.Result{Root: "/work"}
	model := openProjects(t, backend, 100, 30)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	if command != nil {
		t.Fatal("build without a project must not return a command")
	}
	if !strings.Contains(model.statusMessage, "Select a project") {
		t.Fatalf("status = %q", model.statusMessage)
	}

	backend = newFakeBackend()
	backend.projectResult = projectFixture()
	backend.selection = dotnet.Selection{}
	model = openProjects(t, backend, 100, 30)
	updated, command = model.Update(key('b', "b"))
	model = updated.(Model)
	if command != nil {
		t.Fatal("build without an SDK must not return a command")
	}
	if !strings.Contains(model.statusMessage, "No usable .NET SDK") {
		t.Fatalf("status = %q", model.statusMessage)
	}
}

func TestQuitDuringBuildRequiresConfirmation(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	backend.buildResult = execution.Result{ExitCode: -1, Started: true}
	backend.buildBlock = make(chan struct{})
	backend.buildStarted = make(chan struct{})

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	batch := command().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() {
		done <- batch[0]()
	}()
	<-backend.buildStarted

	updated, command = model.Update(key('q', "q"))
	model = updated.(Model)
	if command != nil || !model.quitConfirm {
		t.Fatalf("q during build should confirm: confirm=%v command=%v", model.quitConfirm, command)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"An operation is still running", "Building Shop"} {
		if !strings.Contains(plain, want) {
			t.Errorf("quit dialog missing %q", want)
		}
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !model.buildRunning {
		t.Fatal("escaping the quit dialog must not cancel the build")
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	model = apply(t, model, <-done)
	if model.buildRunning {
		t.Fatal("build should be cancelled")
	}
}

func TestBuildOutputDrawerCanCloseWhileBuilding(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	backend.buildResult = execution.Result{ExitCode: 0, Started: true}
	backend.buildBlock = make(chan struct{})
	backend.buildStarted = make(chan struct{})

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	batch := command().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() {
		done <- batch[0]()
	}()
	<-backend.buildStarted

	model = press(t, model, key('o', "o"))
	if model.drawer.open {
		t.Fatal("o should close the drawer while building")
	}
	if !model.buildRunning {
		t.Fatal("closing the drawer must not cancel the build")
	}
	model = press(t, model, key('o', "o"))
	if !model.drawer.open {
		t.Fatal("o should reopen the drawer while building")
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	model = apply(t, model, <-done)
}

func TestBuildStreamWriterSerializesAndFlushes(t *testing.T) {
	ch := make(chan tea.Msg, 8)
	stdout := &buildStreamWriter{ch: ch, run: 3}
	stderr := &buildStreamWriter{ch: ch, run: 3}
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, _ = stdout.Write([]byte("stdout one\nstdout two\n"))
	}()
	go func() {
		defer wait.Done()
		_, _ = stderr.Write([]byte("stderr one\npartial"))
	}()
	wait.Wait()
	stdout.Flush()
	stderr.Flush()
	close(ch)

	lines := map[string]bool{}
	for message := range ch {
		line, ok := message.(buildLineMsg)
		if !ok {
			t.Fatalf("message = %T", message)
		}
		if line.run != 3 {
			t.Errorf("run = %d, want 3", line.run)
		}
		lines[line.text] = true
	}
	for _, want := range []string{"stdout one", "stdout two", "stderr one", "partial"} {
		if !lines[want] {
			t.Errorf("missing line %q: %v", want, lines)
		}
	}
}

func TestBuildStreamsIntoDrawerWhileRunning(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	backend.buildResult = execution.Result{ExitCode: -1, Started: true}
	backend.buildLines = []string{"first line", "second line"}
	backend.buildBlock = make(chan struct{})
	backend.buildStarted = make(chan struct{})

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	batch := command().(tea.BatchMsg)
	done := make(chan tea.Msg, 1)
	go func() {
		done <- batch[0]()
	}()
	<-backend.buildStarted

	for index := 0; index < 2; index++ {
		select {
		case message := <-model.buildCh:
			model = apply(t, model, message)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for build output")
		}
	}
	if !model.buildRunning {
		t.Fatal("build should still be running while output streams")
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"first line", "second line", "building"} {
		if !strings.Contains(plain, want) {
			t.Errorf("streaming drawer missing %q:\n%s", want, plain)
		}
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	model = apply(t, model, <-done)
}

func TestBuildRecordFailureIsVisible(t *testing.T) {
	backend := newFakeBackend()
	backend.projectResult = projectFixture()
	backend.buildResult = execution.Result{ExitCode: 0, Duration: time.Second, Started: true}
	backend.buildRecordErr = errors.New("disk full")

	model := openProjects(t, backend, 120, 40)
	updated, command := model.Update(key('b', "b"))
	model = updated.(Model)
	model = applyMessages(t, model, startBuildAndDrain(t, &model, command))

	if !strings.Contains(model.statusMessage, "Could not save the last build target") {
		t.Fatalf("status = %q", model.statusMessage)
	}
	if !model.statusIsError {
		t.Fatal("record failure should be reported as an error")
	}
}

func withProjectRoot(backend fakeBackend, root string) fakeBackend {
	backend.config.DefaultProjectDir = root
	dir := root
	backend.defaultProjectDir = &dir
	return backend
}

func openRootPicker(t *testing.T, backend fakeBackend, width, height int) Model {
	t.Helper()
	model := openProjects(t, backend, width, height)
	updated, command := model.Update(key('p', "p"))
	model = updated.(Model)
	if !model.rootPicker.open {
		t.Fatal("p did not open the project root picker")
	}
	if command == nil {
		t.Fatal("opening the project root picker returned no directory command")
	}
	return apply(t, model, command())
}

func TestProjectRootPickerOpensAtConfiguredRoot(t *testing.T) {
	backend := withProjectRoot(newFakeBackend(), "/work")
	backend.directories = map[string][]string{
		"/work": {"/work/apps", "/work/tools"},
	}
	model := openProjects(t, backend, 100, 30)
	updated, command := model.Update(key('p', "p"))
	model = updated.(Model)
	if !model.rootPicker.open {
		t.Fatal("p did not open the project root picker")
	}
	if command == nil {
		t.Fatal("opening the project root picker returned no directory command")
	}
	model = apply(t, model, command())
	if model.rootPicker.loading {
		t.Fatal("picker should finish loading directories")
	}
	if model.rootPicker.path != "/work" {
		t.Fatalf("picker path = %q, want /work", model.rootPicker.path)
	}
	if model.rootPicker.cursor != 1 {
		t.Fatalf("picker cursor = %d, want first child at 1", model.rootPicker.cursor)
	}
	plain := ansi.Strip(model.View().Content)
	for _, want := range []string{"CHOOSE PROJECT ROOT", "/work", "apps/", "tools/", ".."} {
		if !strings.Contains(plain, want) {
			t.Errorf("picker missing %q: %q", want, plain)
		}
	}
	if model.footerContext() != ctxProjectRoot {
		t.Fatalf("footer context = %q, want %q", model.footerContext(), ctxProjectRoot)
	}
}

func TestProjectRootPickerNavigatesAndSavesCurrentRoot(t *testing.T) {
	saved := []string(nil)
	backend := withProjectRoot(newFakeBackend(), "/work")
	backend.directories = map[string][]string{
		"/work":               {"/work/apps", "/work/tools"},
		"/work/apps":          {"/work/apps/Shop", "/work/apps/Library"},
		"/work/apps/Shop":     {},
		"/work/apps/Library":  {},
		"/work/apps/Other":    {},
		"/work/apps/Third":    {},
		"/work/apps/Archived": {},
	}
	backend.setProjectDirCalls = &saved
	model := openRootPicker(t, backend, 100, 30)

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("enter on apps/ returned no directory command")
	}
	model = apply(t, model, command())
	if model.rootPicker.path != "/work/apps" {
		t.Fatalf("picker path = %q, want /work/apps", model.rootPicker.path)
	}

	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if model.rootPicker.cursor != 2 {
		t.Fatalf("picker cursor = %d, want 2", model.rootPicker.cursor)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("enter on Library/ returned no directory command")
	}
	model = apply(t, model, command())
	if model.rootPicker.path != "/work/apps/Library" {
		t.Fatalf("picker path = %q, want /work/apps/Library", model.rootPicker.path)
	}

	updated, command = model.Update(key('s', "s"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("s returned no save command")
	}
	if !model.rootPicker.saving {
		t.Fatal("picker should be saving")
	}
	model = apply(t, model, command())
	if model.rootPicker.open {
		t.Fatal("picker should close after saving")
	}
	if len(saved) != 1 || saved[0] != "/work/apps/Library" {
		t.Fatalf("saved roots = %v, want [/work/apps/Library]", saved)
	}
	if got := model.projectScanRoot(); got != "/work/apps/Library" {
		t.Fatalf("project scan root = %q, want /work/apps/Library", got)
	}
	if !strings.Contains(model.statusMessage, "Project root: /work/apps/Library") {
		t.Fatalf("status = %q, want saved root", model.statusMessage)
	}
	if !model.projectsLoading {
		t.Fatal("saving the root should start a project rescan")
	}
}

func TestProjectRootPickerTogglesHiddenDirectories(t *testing.T) {
	calls := []pickerDirectoryCall(nil)
	backend := withProjectRoot(newFakeBackend(), "/work")
	backend.directories = map[string][]string{
		"/work": {"/work/apps", "/work/.git", "/work/tools"},
	}
	backend.directoryCalls = &calls
	model := openRootPicker(t, backend, 100, 30)
	if strings.Contains(ansi.Strip(model.View().Content), ".git/") {
		t.Fatal("hidden directories should be hidden initially")
	}

	updated, command := model.Update(key('.', "."))
	model = updated.(Model)
	if command == nil {
		t.Fatal(". returned no directory command")
	}
	model = apply(t, model, command())
	if !model.rootPicker.hidden {
		t.Fatal("picker should show hidden directories")
	}
	if !strings.Contains(ansi.Strip(model.View().Content), ".git/") {
		t.Fatal("hidden directory was not shown")
	}
	if len(calls) < 2 || calls[len(calls)-1].hidden != true {
		t.Fatalf("directory calls = %#v, want final hidden=true", calls)
	}
}

func TestProjectRootPickerReadErrorKeepsModalOpen(t *testing.T) {
	backend := withProjectRoot(newFakeBackend(), "/work")
	backend.directoryErr = map[string]error{"/work": errors.New("permission denied")}
	model := openRootPicker(t, backend, 100, 30)
	if model.rootPicker.loadErr == nil {
		t.Fatal("expected directory read error")
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "Cannot read this folder") || !strings.Contains(plain, "permission denied") {
		t.Fatalf("picker missing read error: %q", plain)
	}
	updated, command := model.Update(key('s', "s"))
	model = updated.(Model)
	if command != nil || !model.rootPicker.open {
		t.Fatalf("choose should be blocked on read error: open=%v command=%v", model.rootPicker.open, command)
	}
	model = press(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.rootPicker.open {
		t.Fatal("esc should close the picker")
	}
}

func TestProjectRootPickerSaveFailureKeepsModalOpen(t *testing.T) {
	saved := []string(nil)
	backend := withProjectRoot(newFakeBackend(), "/work")
	backend.setProjectDirCalls = &saved
	backend.setProjectDirErr = errors.New("disk full")
	model := openRootPicker(t, backend, 100, 30)

	updated, command := model.Update(key('s', "s"))
	model = updated.(Model)
	if command == nil {
		t.Fatal("s returned no save command")
	}
	model = apply(t, model, command())
	if !model.rootPicker.open {
		t.Fatal("picker should stay open after a save failure")
	}
	if model.rootPicker.saveErr == nil {
		t.Fatal("expected save error")
	}
	plain := ansi.Strip(model.View().Content)
	if !strings.Contains(plain, "Could not save") || !strings.Contains(plain, "disk full") {
		t.Fatalf("picker missing save error: %q", plain)
	}
	if len(saved) != 1 || saved[0] != "/work" {
		t.Fatalf("save calls = %v, want [/work]", saved)
	}
}

func TestProjectRootPickerFitsLayouts(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 12}, {60, 20}, {80, 24}, {160, 48}} {
		backend := withProjectRoot(newFakeBackend(), "/work")
		backend.directories = map[string][]string{
			"/work": {
				"/work/Apps", "/work/Infrastructure", "/work/Libraries",
				"/work/Services", "/work/Tests", "/work/Tools",
				"/work/Web", "/work/Workers",
			},
		}
		model := openRootPicker(t, backend, size.width, size.height)
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
		if !strings.Contains(ansi.Strip(view.Content), "CHOOSE PROJECT ROOT") {
			t.Errorf("%dx%d: picker title missing", size.width, size.height)
		}
	}
}
