package app

import (
	"os"
	"path/filepath"
	"testing"
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

func TestToolResolution(t *testing.T) {
	app := New("test")
	if path, ok := app.Tool("sh"); !ok || path == "" {
		t.Errorf("sh should resolve: %q %v", path, ok)
	}
	if _, ok := app.Tool("hilt-definitely-missing-tool"); ok {
		t.Error("unexpected tool resolution")
	}
}
