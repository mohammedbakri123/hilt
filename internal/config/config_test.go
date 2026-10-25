package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "config.json")}
	config, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if config.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", config.SchemaVersion)
	}
	if !config.UI.VimKeys {
		t.Error("expected vim keys default")
	}
	if len(config.ScanExclusions) != 4 {
		t.Errorf("ScanExclusions = %v", config.ScanExclusions)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store := Store{Path: path}
	config := Defaults("/tmp/managed")
	config.Editor = "nvim"
	config.RecentProjects = []RecentProject{{Path: "/tmp/project"}}

	if err := store.Save(config); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions = %o, want 600", info.Mode().Perm())
	}

	loaded, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if loaded.ManagedRoot != "/tmp/managed" || loaded.Editor != "nvim" {
		t.Errorf("loaded = %+v", loaded)
	}
	if len(loaded.RecentProjects) != 1 || loaded.RecentProjects[0].Path != "/tmp/project" {
		t.Errorf("RecentProjects = %+v", loaded.RecentProjects)
	}
}

func TestSavePreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{"schemaVersion":1,"editor":"vim","futureSetting":{"enabled":true}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	store := Store{Path: path}
	config, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	config.Editor = "nvim"
	if err := store.Save(config); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["futureSetting"]; !ok {
		t.Fatalf("futureSetting was dropped: %s", data)
	}
}

func TestLoadCorruptFileMovesItAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	config, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning")
	}
	if config.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("expected defaults, got %+v", config)
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup files = %v", matches)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original config still exists")
	}
}

func TestLoadNewerSchemaIsNotModified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{"schemaVersion":99,"editor":"future"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	config, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning")
	}
	if config.Editor != "" {
		t.Errorf("expected defaults, got %+v", config)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("file was modified: %s", data)
	}
}

func TestLoadMissingSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"editor":"vim"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	config, warnings, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning")
	}
	if config.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", config.SchemaVersion)
	}
	if config.Editor != "vim" {
		t.Errorf("Editor = %q", config.Editor)
	}
}
