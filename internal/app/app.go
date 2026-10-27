package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
	"github.com/mohammedbakri123/hilt/internal/process"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

// App is the composition root for Hilt services. It is safe to construct
// without .NET installed and performs no network access.
type App struct {
	version        string
	platform       platform.Info
	paths          platform.Paths
	config         config.Config
	configStore    config.Store
	configMu       sync.RWMutex
	configWarnings []string
	runner         process.Runner
	locator        dotnet.Locator
	prober         dotnet.Prober
	lookPath       func(string) (string, error)
	packageManager *platform.PackageManager
	cwd            string
	home           string
	state          state.State
	stateStore     state.Store
	stateMu        sync.Mutex
}

// New detects the host and loads configuration.
func New(version string) *App {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	paths := platform.NewPaths(os.Getenv, home)
	info := platform.Detect()

	configPath := paths.ConfigFile()
	store := config.Store{Path: configPath}
	cfg, warnings, err := store.Load()
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	if cfg.ManagedRoot == "" {
		cfg.ManagedRoot = paths.ManagedRoot
	}
	if cfg.DefaultProjectDir == "" {
		cfg.DefaultProjectDir = paths.ProjectsRoot
	}
	stateStore := state.Store{Path: paths.StateFile()}
	currentState, stateWarnings, stateErr := stateStore.Load()
	if stateErr != nil {
		warnings = append(warnings, "state: "+stateErr.Error())
	}
	warnings = append(warnings, stateWarnings...)
	if err := paths.EnsureLayout(); err != nil {
		warnings = append(warnings, "create Hilt directories: "+err.Error())
	}
	warnings = append(warnings, legacyDataWarnings(paths, os.Getenv, home)...)

	lookPath := exec.LookPath
	cwd, _ := os.Getwd()

	manifest, manifestErr := install.Load(paths.InstallManifestFile(), cfg.ManagedRoot)
	if manifestErr != nil {
		warnings = append(warnings, "install manifest: "+manifestErr.Error())
		manifest = install.NewManifest(cfg.ManagedRoot)
	}

	return &App{
		version:        version,
		platform:       info,
		paths:          paths,
		config:         cfg,
		configStore:    store,
		configWarnings: warnings,
		runner:         process.ExecRunner{},
		locator: dotnet.Locator{
			LookPath:     lookPath,
			EvalSymlinks: filepath.EvalSymlinks,
			Stat:         os.Stat,
			Getenv:       os.Getenv,
			ManagedRoot:  cfg.ManagedRoot,
			Home:         home,
			Ownership:    manifest,
		},
		prober:         dotnet.CLIProber{Runner: process.ExecRunner{}},
		lookPath:       lookPath,
		packageManager: platform.FindPackageManager(lookPath),
		cwd:            cwd,
		home:           home,
		state:          currentState,
		stateStore:     stateStore,
	}
}

// Version returns the build version.
func (a *App) Version() string { return a.version }

// Platform returns detected host information.
func (a *App) Platform() platform.Info { return a.platform }

// Paths returns the resolved Hilt home layout.
func (a *App) Paths() platform.Paths { return a.paths }

// Config returns a snapshot of the loaded configuration.
func (a *App) Config() config.Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	current := a.config
	current.RecentProjects = append([]config.RecentProject(nil), current.RecentProjects...)
	current.ScanExclusions = append([]string(nil), current.ScanExclusions...)
	return current
}

// ConfigWarnings returns non-fatal configuration problems.
func (a *App) ConfigWarnings() []string {
	return append([]string(nil), a.configWarnings...)
}

// State returns a copy of the loaded project state.
func (a *App) State() state.State {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	current := a.state
	current.RecentProjects = append([]state.RecentProject(nil), current.RecentProjects...)
	current.PinnedExternalProjects = append([]string(nil), current.PinnedExternalProjects...)
	if current.LastBuild != nil {
		lastBuild := *current.LastBuild
		current.LastBuild = &lastBuild
	}
	return current
}

// RecordProjectOpen makes a project active and moves it to the front of the
// recent list. Project files are referenced, never copied.
func (a *App) RecordProjectOpen(projectPath, workspacePath string) error {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	current := a.state
	current.SchemaVersion = state.CurrentSchemaVersion
	current.ActiveProject = projectPath
	current.ActiveWorkspace = workspacePath
	recent := []state.RecentProject{{Path: projectPath, LastOpened: time.Now().UTC()}}
	for _, entry := range current.RecentProjects {
		if entry.Path == projectPath {
			continue
		}
		recent = append(recent, entry)
		if len(recent) == 10 {
			break
		}
	}
	current.RecentProjects = recent
	if current.PinnedExternalProjects == nil {
		current.PinnedExternalProjects = []string{}
	}
	if err := a.stateStore.Save(current); err != nil {
		return fmt.Errorf("save project state: %w", err)
	}
	a.state = current
	return nil
}

// PinExternalProject validates and remembers a project source outside the
// configured default root. The source may be a directory, project, or solution
// file and is never copied into Hilt's home.
func (a *App) PinExternalProject(path string) error {
	resolved, err := canonicalProjectPath(path)
	if err != nil {
		return fmt.Errorf("pin project source: %w", err)
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	current := a.state
	current.SchemaVersion = state.CurrentSchemaVersion
	current.PinExternalProject(resolved)
	if err := a.stateStore.Save(current); err != nil {
		return fmt.Errorf("save project state: %w", err)
	}
	a.state = current
	return nil
}

// UnpinExternalProject removes a remembered external project source.
func (a *App) UnpinExternalProject(path string) error {
	resolved, err := canonicalProjectPath(path)
	if err != nil {
		return fmt.Errorf("unpin project source: %w", err)
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	current := a.state
	current.SchemaVersion = state.CurrentSchemaVersion
	current.UnpinExternalProject(resolved)
	if err := a.stateStore.Save(current); err != nil {
		return fmt.Errorf("save project state: %w", err)
	}
	a.state = current
	return nil
}

// RecordBuildTarget remembers the most recent project Hilt built. Callers must
// only record a target after the build command was actually started.
func (a *App) RecordBuildTarget(target state.BuildTarget) error {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	current := a.state
	current.SchemaVersion = state.CurrentSchemaVersion
	if target.LastBuilt.IsZero() {
		target.LastBuilt = time.Now().UTC()
	}
	current.LastBuild = &target
	if current.PinnedExternalProjects == nil {
		current.PinnedExternalProjects = []string{}
	}
	if err := a.stateStore.Save(current); err != nil {
		return fmt.Errorf("save build state: %w", err)
	}
	a.state = current
	return nil
}

// BuildProject runs dotnet build for one project. When no dotnet path is
// supplied it resolves and verifies the toolchain for the current workspace.
func (a *App) BuildProject(ctx context.Context, request execution.BuildRequest, stdout, stderr io.Writer) (execution.Result, error) {
	if strings.TrimSpace(request.DotnetPath) == "" {
		inventory, err := a.Inventory(ctx)
		if err != nil {
			return execution.Result{}, fmt.Errorf("discover .NET: %w", err)
		}
		selection, err := a.Resolve(ctx, inventory)
		if err != nil {
			return execution.Result{}, err
		}
		if strings.TrimSpace(selection.DotnetPath) == "" {
			return execution.Result{}, fmt.Errorf("no usable .NET host was found")
		}
		request.DotnetPath = selection.DotnetPath
		if request.Environment == nil {
			request.Environment = selection.Environment(nil)
		}
	}
	return execution.Builder{Runner: a.runner}.Build(ctx, request, stdout, stderr)
}

// CreateProjects executes a previously validated project creation plan without
// invoking a shell. Each step is streamed in order so callers can show the
// exact generator output and stop safely through context cancellation.
func (a *App) CreateProjects(ctx context.Context, plan projects.CreatePlan, stdout, stderr io.Writer) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("project creation plan has no steps")
	}
	for _, step := range plan.Steps {
		if len(step.Command) == 0 || strings.TrimSpace(step.Command[0]) == "" {
			return fmt.Errorf("project creation plan contains an empty command")
		}
		_, err := a.runner.Stream(ctx, process.Spec{
			Name: step.Command[0],
			Args: step.Command[1:],
			Dir:  step.Dir,
		}, stdout, stderr)
		if err != nil {
			return fmt.Errorf("create step %q: %w", strings.Join(step.Command, " "), err)
		}
	}
	return nil
}

// PackageManager returns the detected package manager, if any.
func (a *App) PackageManager() *platform.PackageManager { return a.packageManager }

// Cwd returns the directory Hilt was started from.
func (a *App) Cwd() string { return a.cwd }

// Home returns the resolved home directory.
func (a *App) Home() string { return a.home }

// Tool resolves an executable name.
func (a *App) Tool(name string) (string, bool) {
	path, err := a.lookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// Inventory discovers .NET hosts, SDKs, and runtimes.
func (a *App) Inventory(ctx context.Context) (dotnet.Inventory, error) {
	return dotnet.Discover(ctx, a.locator, a.prober, os.Stat), nil
}

// PlanSDKInstall resolves a managed SDK install without downloading it. The
// version request may be exact or a channel/alias request.
func (a *App) PlanSDKInstall(ctx context.Context, version string, allowPrerelease bool) (install.Plan, error) {
	plan, err := a.installer().Plan(ctx, version, allowPrerelease)
	if err != nil {
		return install.Plan{}, err
	}
	inventory, inventoryErr := a.Inventory(ctx)
	if inventoryErr == nil {
		for _, sdk := range inventory.SDKs {
			if sdk.Version == plan.Version && !sdk.Managed {
				plan.SystemCopy = true
				break
			}
		}
	}
	return plan, nil
}

// InstallSDK downloads, verifies, and publishes a managed SDK.
func (a *App) InstallSDK(ctx context.Context, plan install.Plan, progress io.Writer) (install.Result, error) {
	result, err := a.installer().Install(ctx, plan, progress)
	if err != nil {
		return install.Result{}, err
	}
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.Config().ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return result, nil
}

// RemoveSDK removes a manifest-owned managed SDK.
func (a *App) RemoveSDK(ctx context.Context, version string) (install.Removal, error) {
	removal, err := a.installer().Remove(ctx, version)
	if err != nil {
		return install.Removal{}, a.removalError(ctx, install.ProductSDK, version, err)
	}
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.Config().ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return removal, nil
}

// CheckSDKUpdates checks manifest-owned SDKs for newer stable releases.
func (a *App) CheckSDKUpdates(ctx context.Context) ([]install.Update, error) {
	return a.installer().CheckUpdates(ctx)
}

// AvailableSDKs lists downloadable SDK versions for this host. It performs no
// downloads or filesystem changes.
func (a *App) AvailableSDKs(ctx context.Context, allowPrerelease bool) ([]install.AvailableSDK, error) {
	return a.installer().AvailableSDKs(ctx, allowPrerelease)
}

// ScanProjects discovers projects and solutions from root. An empty root uses
// the configured default project directory, then the Hilt projects root. The
// current directory is only used when no other root is available.
func (a *App) ScanProjects(ctx context.Context, root string, includeHidden bool) (projects.Result, error) {
	current := a.Config()
	if root == "" {
		root = current.DefaultProjectDir
	}
	if root == "" {
		root = a.paths.ProjectsRoot
	}
	if root == "" {
		root = a.cwd
	}
	return projects.Scan(ctx, projects.Options{
		Root:          root,
		Exclusions:    current.ScanExclusions,
		IncludeHidden: includeHidden,
	})
}

// ListDirectories returns the absolute paths of directories directly under
// path. Symlinked directories are omitted so callers cannot walk outside the
// selected tree accidentally.
func (a *App) ListDirectories(ctx context.Context, path string, includeHidden bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := canonicalDirectory(path)
	if err != nil {
		return nil, fmt.Errorf("list directories: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list directories in %s: %w", root, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directories := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		if !includeHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		directories = append(directories, filepath.Join(root, entry.Name()))
	}
	sort.Strings(directories)
	return directories, nil
}

// SetDefaultProjectDir validates and persists the directory the Projects
// screen scans by default.
func (a *App) SetDefaultProjectDir(path string) error {
	root, err := canonicalDirectory(path)
	if err != nil {
		return fmt.Errorf("set default project directory: %w", err)
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()
	next := a.config
	next.SchemaVersion = config.CurrentSchemaVersion
	next.DefaultProjectDir = root
	if err := a.configStore.Save(next); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	a.config = next
	return nil
}

func canonicalDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s is not an absolute path", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", resolved)
	}
	return resolved, nil
}

func canonicalProjectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") || path == "~" {
		path = filepath.Join(aHome(path), strings.TrimPrefix(path, "~/"))
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s is not an absolute path", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", resolved, err)
	}
	if info.IsDir() {
		return filepath.Clean(resolved), nil
	}
	if _, ok := projects.KindForPath(resolved); !ok {
		return "", fmt.Errorf("%s is not a supported project or solution path", resolved)
	}
	return filepath.Clean(resolved), nil
}

func aHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Dir(path)
	}
	return home
}

// InspectProject parses one project or solution file.
func (a *App) InspectProject(ctx context.Context, path string) (projects.Inspection, error) {
	return projects.Inspect(ctx, path)
}

// UpdateSDK applies a managed SDK update.
func (a *App) UpdateSDK(ctx context.Context, version string, removeOld bool, progress io.Writer) (install.UpdateResult, error) {
	result, err := a.installer().UpdateSDK(ctx, version, removeOld, progress)
	if err != nil {
		// Translate removal/ownership errors for the old version where relevant.
		var notInstalled *install.NotInstalledError
		var notOwned *install.NotOwnedError
		if errors.As(err, &notInstalled) || errors.As(err, &notOwned) {
			return result, a.removalError(ctx, install.ProductSDK, version, err)
		}
		return result, err
	}
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.Config().ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return result, nil
}

// PlanRuntimeInstall resolves a managed runtime install without downloading it.
// The version request may be exact or a channel/alias request.
func (a *App) PlanRuntimeInstall(ctx context.Context, version string, framework install.FrameworkFlavor, allowPrerelease bool) (install.RuntimePlan, error) {
	plan, err := a.installer().PlanRuntime(ctx, version, framework, allowPrerelease)
	if err != nil {
		return install.RuntimePlan{}, err
	}
	inventory, inventoryErr := a.Inventory(ctx)
	if inventoryErr == nil {
		for _, runtime := range inventory.Runtimes {
			if runtime.Version == plan.Version && !runtime.Managed {
				plan.SystemCopy = true
				break
			}
		}
	}
	return plan, nil
}

// InstallRuntime downloads, verifies, and publishes a managed runtime.
func (a *App) InstallRuntime(ctx context.Context, plan install.RuntimePlan, progress io.Writer) (install.RuntimeResult, error) {
	result, err := a.installer().InstallRuntime(ctx, plan, progress)
	if err != nil {
		return install.RuntimeResult{}, err
	}
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.Config().ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return result, nil
}

// RemoveRuntime removes a manifest-owned managed runtime.
func (a *App) RemoveRuntime(ctx context.Context, version string) (install.Removal, error) {
	removal, err := a.installer().RemoveRuntime(ctx, version)
	if err != nil {
		return install.Removal{}, a.removalError(ctx, install.ProductRuntime, version, err)
	}
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.Config().ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return removal, nil
}

// CheckRuntimeUpdates checks manifest-owned runtimes for newer stable releases.
func (a *App) CheckRuntimeUpdates(ctx context.Context) ([]install.Update, error) {
	return a.installer().CheckRuntimeUpdates(ctx)
}

func (a *App) removalError(ctx context.Context, product install.ProductKind, version string, cause error) error {
	var notInstalled *install.NotInstalledError
	var notOwned *install.NotOwnedError
	if !errors.As(cause, &notInstalled) && !errors.As(cause, &notOwned) {
		return cause
	}
	parsed, err := install.ValidateVersion(version)
	if err != nil {
		return cause
	}
	canonical := parsed.String()
	inventory, inventoryErr := a.Inventory(ctx)
	if inventoryErr != nil {
		return cause
	}
	if product == install.ProductRuntime {
		for _, runtime := range inventory.Runtimes {
			if runtime.Version == canonical && runtime.EffectiveProvenance() == dotnet.ProvenanceManaged {
				return fmt.Errorf("runtime %s at %s is under the managed root but is not recorded in Hilt's install manifest; refusing to remove it", canonical, runtime.Root)
			}
		}
		for _, runtime := range inventory.Runtimes {
			if runtime.Version != canonical {
				continue
			}
			switch runtime.EffectiveProvenance() {
			case dotnet.ProvenanceSystem:
				return fmt.Errorf("runtime %s is provided by the system at %s; Hilt only removes managed runtimes", canonical, runtime.Root)
			case dotnet.ProvenanceExternal:
				return fmt.Errorf("runtime %s is provided by an external installation at %s; Hilt only removes managed runtimes", canonical, runtime.Root)
			}
		}
		return cause
	}
	for _, sdk := range inventory.SDKs {
		if sdk.Version == canonical && sdk.EffectiveProvenance() == dotnet.ProvenanceManaged {
			return fmt.Errorf("SDK %s at %s is under the managed root but is not recorded in Hilt's install manifest; refusing to remove it", canonical, sdk.Root)
		}
	}
	for _, sdk := range inventory.SDKs {
		if sdk.Version != canonical {
			continue
		}
		switch sdk.EffectiveProvenance() {
		case dotnet.ProvenanceSystem:
			return fmt.Errorf("SDK %s is provided by the system at %s; Hilt only removes managed SDKs", canonical, sdk.Root)
		case dotnet.ProvenanceExternal:
			return fmt.Errorf("SDK %s is provided by an external installation at %s; Hilt only removes managed SDKs", canonical, sdk.Root)
		}
	}
	return cause
}

func productLabel(product install.ProductKind) string {
	if product == install.ProductRuntime {
		return "runtime"
	}
	return "SDK"
}

func (a *App) installer() install.Installer {
	return install.Installer{
		ManagedRoot:  a.Config().ManagedRoot,
		ManifestPath: a.paths.InstallManifestFile(),
		LockPath:     a.paths.InstallLockFile(),
		Runner:       a.runner,
		FindHost: func() (string, bool) {
			return a.Tool("dotnet")
		},
	}
}

// Resolve selects the toolchain Hilt will use for the current workspace.
func (a *App) Resolve(ctx context.Context, inventory dotnet.Inventory) (dotnet.Selection, error) {
	return dotnet.Resolver{
		Inventory: inventory,
		Dir:       a.cwd,
		Runner:    a.runner,
		Stat:      os.Stat,
		BaseEnv:   os.Environ(),
	}.Resolve(ctx)
}

// Diagnose runs the full doctor engine.
func (a *App) Diagnose(ctx context.Context) diagnostics.Report {
	return diagnostics.Run(ctx, diagnostics.Deps{
		Version:        a.version,
		Platform:       a.platform,
		Paths:          a.paths,
		Config:         a.Config(),
		ConfigWarnings: a.ConfigWarnings(),
		LookPath:       a.lookPath,
		Stat:           os.Stat,
		Inventory:      a.Inventory,
		Resolve:        a.Resolve,
		Cwd:            a.cwd,
		PackageManager: a.packageManager,
	})
}

// Compile-time assertions: *App must satisfy both consumer interfaces so they
// cannot silently diverge from the implementation.
var _ interface {
	Version() string
	Platform() platform.Info
	Paths() platform.Paths
	Config() config.Config
	ConfigWarnings() []string
	Inventory(ctx context.Context) (dotnet.Inventory, error)
	Resolve(ctx context.Context, inventory dotnet.Inventory) (dotnet.Selection, error)
	Diagnose(ctx context.Context) diagnostics.Report
} = (*App)(nil)

func legacyDataWarnings(paths platform.Paths, getenv func(string) string, home string) []string {
	legacyRoot := legacyXDGDir(getenv("XDG_DATA_HOME"), home, ".local/share")
	legacyManifest := filepath.Join(legacyRoot, "hilt", "installs.json")
	if _, err := os.Stat(legacyManifest); err != nil {
		return nil
	}
	if _, err := os.Stat(paths.InstallManifestFile()); err == nil {
		return nil
	}
	return []string{
		"legacy managed installs were found at " + filepath.Dir(legacyManifest) +
			"; Hilt now uses " + paths.DataDir + " and does not migrate them automatically",
	}
}

func legacyXDGDir(value, home, fallback string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(home, filepath.FromSlash(fallback))
}
