package app

import (
	"context"
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
