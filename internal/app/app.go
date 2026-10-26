package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
	"github.com/mohammedbakri123/hilt/internal/process"
)

// App is the composition root for Hilt services. It is safe to construct
// without .NET installed and performs no network access.
type App struct {
	version        string
	platform       platform.Info
	paths          platform.Paths
	config         config.Config
	configWarnings []string
	runner         process.Runner
	locator        dotnet.Locator
	prober         dotnet.Prober
	lookPath       func(string) (string, error)
	packageManager *platform.PackageManager
	cwd            string
	home           string
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
	}
}

// Version returns the build version.
func (a *App) Version() string { return a.version }

// Platform returns detected host information.
func (a *App) Platform() platform.Info { return a.platform }

// Paths returns resolved XDG directories.
func (a *App) Paths() platform.Paths { return a.paths }

// Config returns the loaded configuration.
func (a *App) Config() config.Config { return a.config }

// ConfigWarnings returns non-fatal configuration problems.
func (a *App) ConfigWarnings() []string {
	return append([]string(nil), a.configWarnings...)
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
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.config.ManagedRoot); loadErr == nil {
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
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.config.ManagedRoot); loadErr == nil {
		a.locator.Ownership = manifest
	}
	return removal, nil
}

// CheckSDKUpdates checks manifest-owned SDKs for newer stable releases.
func (a *App) CheckSDKUpdates(ctx context.Context) ([]install.Update, error) {
	return a.installer().CheckUpdates(ctx)
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
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.config.ManagedRoot); loadErr == nil {
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
	if manifest, loadErr := install.Load(a.paths.InstallManifestFile(), a.config.ManagedRoot); loadErr == nil {
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
		ManagedRoot:  a.config.ManagedRoot,
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
		Config:         a.config,
		ConfigWarnings: a.ConfigWarnings(),
		LookPath:       a.lookPath,
		Stat:           os.Stat,
		Inventory:      a.Inventory,
		Resolve:        a.Resolve,
		Cwd:            a.cwd,
		PackageManager: a.packageManager,
	})
}
