package app

import (
	"context"
	"crypto/sha512"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
)

func TestNewUsesXDGPathsAndDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	app := New("test-version")
	if app.Version() != "test-version" {
		t.Errorf("Version = %q", app.Version())
	}
	paths := app.Paths()
	if paths.ConfigDir != filepath.Join(home, "cfg", "hilt") {
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
	configDir := filepath.Join(home, "hilt")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))

	app := New("test")
	if len(app.ConfigWarnings()) == 0 {
		t.Fatal("expected a configuration warning")
	}
}

func TestNewDiscoversManifestManagedRoot(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))

	managedRoot := filepath.Join(dataDir, "hilt", "dotnet")
	versionRoot := filepath.Join(managedRoot, "10.0.100")
	if err := os.MkdirAll(filepath.Join(versionRoot, "sdk", "10.0.100"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dataDir, "hilt", "installs.json")
	manifest, err := install.Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(install.Entry{
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
	dataDir := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))

	manifestDir := filepath.Join(dataDir, "hilt")
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
	dataDir := filepath.Join(home, "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	return home, filepath.Join(dataDir, "hilt", "dotnet")
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
