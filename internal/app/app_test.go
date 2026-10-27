package app

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

func TestNewUsesHiltHomeAndDefaults(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "custom-hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	app := New("test-version")
	if app.Version() != "test-version" {
		t.Errorf("Version = %q", app.Version())
	}
	paths := app.Paths()
	if paths.HomeDir != hiltHome {
		t.Errorf("HomeDir = %q", paths.HomeDir)
	}
	if paths.ProjectsRoot != filepath.Join(hiltHome, "apps") {
		t.Errorf("ProjectsRoot = %q", paths.ProjectsRoot)
	}
	if paths.ConfigDir != filepath.Join(hiltHome, ".hilt") {
		t.Errorf("ConfigDir = %q", paths.ConfigDir)
	}
	if app.Config().ManagedRoot != paths.ManagedRoot {
		t.Errorf("ManagedRoot = %q, want %q", app.Config().ManagedRoot, paths.ManagedRoot)
	}
	if app.Config().SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d", app.Config().SchemaVersion)
	}
	if app.Platform().OS == "" || app.Platform().Arch == "" {
		t.Errorf("Platform = %+v", app.Platform())
	}
}

func TestNewSurfacesCorruptConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "hilt", ".hilt")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))

	app := New("test")
	if len(app.ConfigWarnings()) == 0 {
		t.Fatal("expected a configuration warning")
	}
}

func TestNewDiscoversManifestManagedRoot(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	dataDir := filepath.Join(hiltHome, ".hilt", "data")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	managedRoot := filepath.Join(dataDir, "dotnet")
	versionRoot := filepath.Join(managedRoot, "10.0.100")
	if err := os.MkdirAll(filepath.Join(versionRoot, "sdk", "10.0.100"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dataDir, "installs.json")
	manifest, err := install.Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(install.Entry{
		Product:   install.ProductSDK,
		Version:   "10.0.100",
		RID:       "linux-x64",
		Root:      "10.0.100",
		SourceURL: "https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz",
		SHA512:    install.FormatSHA512(hash[:]),
	}); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}

	app := New("test")
	inventory, err := app.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sdk := inventory.FindSDK("10.0.100")
	if sdk == nil {
		t.Fatalf("SDK not discovered: %+v", inventory)
	}
	if !sdk.Managed || !sdk.Owned || sdk.Provenance != dotnet.ProvenanceOwned {
		t.Errorf("SDK ownership = %+v", sdk)
	}
	if sdk.Source != dotnet.SourceManaged {
		t.Errorf("SDK source = %q", sdk.Source)
	}
}

func TestNewSurfacesCorruptInstallManifest(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	manifestDir := filepath.Join(hiltHome, ".hilt", "data")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "installs.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := New("test")
	found := false
	for _, warning := range app.ConfigWarnings() {
		if strings.Contains(warning, "install manifest") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected install manifest warning: %v", app.ConfigWarnings())
	}
}

func TestToolResolution(t *testing.T) {
	app := New("test")
	if path, ok := app.Tool("sh"); !ok || path == "" {
		t.Errorf("sh should resolve: %q %v", path, ok)
	}
	if _, ok := app.Tool("hilt-definitely-missing-tool"); ok {
		t.Error("unexpected tool resolution")
	}
}

func testAppEnv(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)
	return home, filepath.Join(hiltHome, ".hilt", "data", "dotnet")
}

func writeSDKRoot(t *testing.T, root, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "sdk", version), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeManifestEntry(t *testing.T, manifestPath, managedRoot, version string) {
	t.Helper()
	manifest, err := install.Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(install.Entry{
		Product:   install.ProductSDK,
		Version:   version,
		RID:       "linux-x64",
		Root:      version,
		SourceURL: "https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz",
		SHA512:    install.FormatSHA512(hash[:]),
	}); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveSDKRemovesOwnedInstall(t *testing.T) {
	_, managedRoot := testAppEnv(t)
	version := "10.0.100"
	root := filepath.Join(managedRoot, version)
	writeSDKRoot(t, root, version)
	writeManifestEntry(t, filepath.Join(filepath.Dir(managedRoot), "installs.json"), managedRoot, version)

	app := New("test")
	removal, err := app.RemoveSDK(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if removal.Stale || removal.Root != root {
		t.Errorf("removal = %+v", removal)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Errorf("root still exists: %v", statErr)
	}
	inventory, err := app.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if inventory.FindSDK(version) != nil {
		t.Errorf("removed SDK still discovered: %+v", inventory)
	}
}

func TestRemoveSDKRefusesUnownedManagedRoot(t *testing.T) {
	_, managedRoot := testAppEnv(t)
	version := "10.0.100"
	root := filepath.Join(managedRoot, version)
	writeSDKRoot(t, root, version)

	app := New("test")
	_, err := app.RemoveSDK(context.Background(), version)
	if err == nil || !strings.Contains(err.Error(), "not recorded") {
		t.Fatalf("error = %v, want unowned managed refusal", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Errorf("unowned root removed: %v", statErr)
	}
}

func TestRemoveSDKRefusesSystemAndExternalRoots(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*App, string)
		want  string
	}{
		{
			name: "system",
			setup: func(app *App, root string) {
				app.locator.KnownRoots = []string{root}
				app.locator.SystemRoots = []string{root}
			},
			want: "provided by the system",
		},
		{
			name: "external",
			setup: func(app *App, root string) {
				app.locator.KnownRoots = []string{root}
				app.locator.SystemRoots = []string{}
			},
			want: "external installation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, _ := testAppEnv(t)
			version := "10.0.100"
			root := filepath.Join(home, "root")
			writeSDKRoot(t, root, version)
			app := New("test")
			test.setup(app, root)

			_, err := app.RemoveSDK(context.Background(), version)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			if _, statErr := os.Stat(root); statErr != nil {
				t.Errorf("root removed: %v", statErr)
			}
		})
	}
}

func TestRemoveSDKNotInstalled(t *testing.T) {
	testAppEnv(t)
	app := New("test")
	_, err := app.RemoveSDK(context.Background(), "10.0.100")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %v, want not installed", err)
	}
}

func TestScanProjectsUsesRootAndConfiguration(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	projectDir := filepath.Join(hiltHome, "apps", "Shop")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(projectDir, "App.csproj")
	if err := os.WriteFile(projectPath, []byte(`<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup></Project>`), 0o600); err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Join(home, "work")
	if err := os.MkdirAll(externalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(externalDir, "External.csproj")
	if err := os.WriteFile(externalPath, []byte(`<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup></Project>`), 0o600); err != nil {
		t.Fatal(err)
	}

	application := New("test")
	if application.Config().DefaultProjectDir != application.Paths().ProjectsRoot {
		t.Errorf("DefaultProjectDir = %q, want %q", application.Config().DefaultProjectDir, application.Paths().ProjectsRoot)
	}
	result, err := application.ScanProjects(context.Background(), externalDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 1 || result.Projects[0].Path != externalPath {
		t.Fatalf("external projects = %+v", result.Projects)
	}

	inspection, err := application.InspectProject(context.Background(), projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Kind != projects.KindProject || len(inspection.Projects) != 1 {
		t.Errorf("inspection = %+v", inspection)
	}

	t.Chdir(externalDir)
	application = New("test")
	if err := application.SetDefaultProjectDir(externalDir); err != nil {
		t.Fatalf("set custom default project directory: %v", err)
	}
	result, err = application.ScanProjects(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 1 || result.Projects[0].Path != projectPath {
		t.Errorf("default scan = %+v, want project under HILT_HOME/apps", result.Projects)
	}
}

func TestNewWarnsAboutLegacyManagedInstalls(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	legacyData := filepath.Join(home, "legacy-data")
	legacyDir := filepath.Join(legacyData, "hilt")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "installs.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)
	t.Setenv("XDG_DATA_HOME", legacyData)

	application := New("test")
	found := false
	for _, warning := range application.ConfigWarnings() {
		if strings.Contains(warning, "legacy managed installs") && strings.Contains(warning, legacyDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected legacy install warning: %v", application.ConfigWarnings())
	}
}

func TestNewCreatesHiltLayout(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	application := New("test")
	for _, directory := range []string{
		hiltHome,
		filepath.Join(hiltHome, "apps"),
		filepath.Join(hiltHome, ".hilt"),
		filepath.Join(hiltHome, ".hilt", "cache"),
		filepath.Join(hiltHome, ".hilt", "data"),
		filepath.Join(hiltHome, ".hilt", "logs"),
		filepath.Join(hiltHome, ".hilt", "data", "dotnet"),
	} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Errorf("%s: %v", directory, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", directory)
		}
	}

	result, err := application.ScanProjects(context.Background(), "", false)
	if err != nil {
		t.Fatalf("scan empty Hilt home: %v", err)
	}
	if len(result.Projects) != 0 {
		t.Errorf("projects = %+v, want none", result.Projects)
	}
}

func TestRecordProjectOpenPersistsState(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	application := New("test")
	first := filepath.Join(hiltHome, "apps", "Shop", "Shop.csproj")
	firstWorkspace := filepath.Join(hiltHome, "apps", "Shop", "Shop.sln")
	if err := application.RecordProjectOpen(first, firstWorkspace); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(hiltHome, "apps", "Library", "Library.csproj")
	if err := application.RecordProjectOpen(second, ""); err != nil {
		t.Fatal(err)
	}
	if err := application.RecordProjectOpen(first, firstWorkspace); err != nil {
		t.Fatal(err)
	}

	value := application.State()
	if value.ActiveProject != first || value.ActiveWorkspace != firstWorkspace {
		t.Fatalf("active state = %+v", value)
	}
	if len(value.RecentProjects) != 2 || value.RecentProjects[0].Path != first || value.RecentProjects[1].Path != second {
		t.Fatalf("recent projects = %+v", value.RecentProjects)
	}

	store := state.Store{Path: application.Paths().StateFile()}
	loaded, warnings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if loaded.ActiveProject != first || len(loaded.RecentProjects) != 2 {
		t.Fatalf("loaded state = %+v", loaded)
	}
	info, err := os.Stat(application.Paths().StateFile())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
}

func TestNewSurfacesCorruptState(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	stateDir := filepath.Join(hiltHome, ".hilt")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	application := New("test")
	found := false
	for _, warning := range application.ConfigWarnings() {
		if strings.Contains(warning, "state") {
			found = true
		}
	}
	if !found {
		t.Fatalf("state warning missing: %v", application.ConfigWarnings())
	}
}

func TestListDirectories(t *testing.T) {
	home, _ := testAppEnv(t)
	root := filepath.Join(home, "projects")
	for _, name := range []string{"alpha", "beta", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "alpha"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	application := New("test")
	directories, err := application.ListDirectories(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
	if strings.Join(directories, "\n") != strings.Join(want, "\n") {
		t.Fatalf("directories = %v, want %v", directories, want)
	}

	directories, err = application.ListDirectories(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{filepath.Join(root, ".hidden"), filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
	if strings.Join(directories, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hidden directories = %v, want %v", directories, want)
	}

	link := filepath.Join(home, "projects-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	directories, err = application.ListDirectories(context.Background(), link, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(directories) != 2 || directories[0] != want[1] || directories[1] != want[2] {
		t.Fatalf("canonical directories = %v", directories)
	}

	if _, err := application.ListDirectories(context.Background(), filepath.Join(root, "notes.txt"), false); err == nil {
		t.Fatal("expected a file listing error")
	}
	if _, err := application.ListDirectories(context.Background(), filepath.Join(home, "missing"), false); err == nil {
		t.Fatal("expected a missing directory error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := application.ListDirectories(ctx, root, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestSetDefaultProjectDirPersists(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	configDir := filepath.Join(hiltHome, ".hilt")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.json")
	content := `{"schemaVersion":1,"defaultProjectDir":"/old","futureSetting":{"enabled":true}}`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)

	target := filepath.Join(home, "work")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "work-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	application := New("test")
	if err := application.SetDefaultProjectDir(link); err != nil {
		t.Fatalf("SetDefaultProjectDir: %v", err)
	}
	if got := application.Config().DefaultProjectDir; got != target {
		t.Fatalf("DefaultProjectDir = %q, want %q", got, target)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config permissions = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := json.Unmarshal(raw["defaultProjectDir"], &saved); err != nil {
		t.Fatal(err)
	}
	if saved != target {
		t.Errorf("saved defaultProjectDir = %q, want %q", saved, target)
	}
	if _, ok := raw["futureSetting"]; !ok {
		t.Fatalf("futureSetting was dropped: %s", data)
	}

	reloaded := New("test")
	if got := reloaded.Config().DefaultProjectDir; got != target {
		t.Errorf("reloaded DefaultProjectDir = %q, want %q", got, target)
	}
}

func TestSetDefaultProjectDirRejectsInvalidPaths(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)
	application := New("test")
	original := application.Config().DefaultProjectDir

	file := filepath.Join(home, "file.txt")
	if err := os.WriteFile(file, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "relative/path", file, filepath.Join(home, "missing")} {
		if err := application.SetDefaultProjectDir(path); err == nil {
			t.Errorf("SetDefaultProjectDir(%q) succeeded", path)
		}
	}
	if got := application.Config().DefaultProjectDir; got != original {
		t.Fatalf("invalid path changed config to %q", got)
	}
}

func TestSetDefaultProjectDirSaveFailureLeavesConfig(t *testing.T) {
	home := t.TempDir()
	hiltHome := filepath.Join(home, "hilt")
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", hiltHome)
	application := New("test")
	original := application.Config().DefaultProjectDir

	target := filepath.Join(home, "work")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	application.configStore = config.Store{Path: filepath.Join(blocker, "config.json")}

	if err := application.SetDefaultProjectDir(target); err == nil {
		t.Fatal("expected a save error")
	}
	if got := application.Config().DefaultProjectDir; got != original {
		t.Fatalf("failed save changed config to %q", got)
	}
}
