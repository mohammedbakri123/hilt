package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingReturnsDefaultsWithoutCreatingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	current, warnings, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if current.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", current.SchemaVersion)
	}
	if current.RecentProjects == nil || current.PinnedExternalProjects == nil {
		t.Errorf("defaults should use empty slices: %+v", current)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("load created state file: %v", statErr)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".hilt", "state.json")
	opened := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	want := State{
		SchemaVersion:          CurrentSchemaVersion,
		ActiveWorkspace:        "/work/Shop.sln",
		ActiveProject:          "/work/src/Shop/Shop.csproj",
		RecentProjects:         []RecentProject{{Path: "/work/src/Shop/Shop.csproj", LastOpened: opened}},
		PinnedExternalProjects: []string{"/elsewhere/External.csproj"},
	}
	store := Store{Path: path}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("state mode = %o, want 600", mode)
	}

	got, warnings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got.ActiveWorkspace != want.ActiveWorkspace || got.ActiveProject != want.ActiveProject {
		t.Errorf("active state = %+v", got)
	}
	if len(got.RecentProjects) != 1 || got.RecentProjects[0] != want.RecentProjects[0] {
		t.Errorf("recent projects = %+v", got.RecentProjects)
	}
	if len(got.PinnedExternalProjects) != 1 || got.PinnedExternalProjects[0] != want.PinnedExternalProjects[0] {
		t.Errorf("pinned projects = %+v", got.PinnedExternalProjects)
	}
}

func TestSaveDefaultsSchemaVersionAndEmptySlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := Store{Path: path}
	if err := store.Save(State{ActiveProject: "/work/App.csproj"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SchemaVersion          int               `json:"schemaVersion"`
		RecentProjects         []json.RawMessage `json:"recentProjects"`
		PinnedExternalProjects []json.RawMessage `json:"pinnedExternalProjects"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", decoded.SchemaVersion)
	}
	if decoded.RecentProjects == nil || decoded.PinnedExternalProjects == nil {
		t.Errorf("empty slices should be encoded, got %s", data)
	}
}

func TestLoadCorruptMovesAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}

	current, warnings, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if current.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", current.SchemaVersion)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "moved to") {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("corrupt state still exists: %v", statErr)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
}

func TestLoadNewerSchemaDoesNotModifyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte(`{"schemaVersion":99,"activeProject":"/work/App.csproj"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	current, warnings, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "newer than supported") {
		t.Fatalf("warnings = %v", warnings)
	}
	if current.ActiveProject != "" {
		t.Errorf("active project = %q, want defaults", current.ActiveProject)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Errorf("state file was modified: %s", data)
	}
}

func TestLoadZeroSchemaWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"activeProject":"/work/App.csproj"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	current, warnings, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if current.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d", current.SchemaVersion)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "missing") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestSaveRejectsEmptyPath(t *testing.T) {
	if err := (Store{}).Save(Defaults()); err == nil {
		t.Fatal("expected an empty-path error")
	}
}
