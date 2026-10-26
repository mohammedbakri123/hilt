// Package state persists small, non-secret user state such as the active
// project and recent project paths. It never stores project contents, secrets,
// environment variables, or package data.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CurrentSchemaVersion is the state schema Hilt writes.
const CurrentSchemaVersion = 1

// RecentProject records a project Hilt has opened.
type RecentProject struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// BuildTarget records the most recent project Hilt actually built.
type BuildTarget struct {
	ProjectPath   string    `json:"projectPath"`
	WorkspacePath string    `json:"workspacePath,omitempty"`
	SDKVersion    string    `json:"sdkVersion,omitempty"`
	Configuration string    `json:"configuration,omitempty"`
	LastBuilt     time.Time `json:"lastBuilt"`
}

// State is Hilt's versioned, user-local session state.
type State struct {
	SchemaVersion          int             `json:"schemaVersion"`
	ActiveWorkspace        string          `json:"activeWorkspace,omitempty"`
	ActiveProject          string          `json:"activeProject,omitempty"`
	RecentProjects         []RecentProject `json:"recentProjects"`
	PinnedExternalProjects []string        `json:"pinnedExternalProjects"`
	LastBuild              *BuildTarget    `json:"lastBuild,omitempty"`
}

// Defaults returns empty state for the current schema.
func Defaults() State {
	return State{
		SchemaVersion:          CurrentSchemaVersion,
		RecentProjects:         []RecentProject{},
		PinnedExternalProjects: []string{},
	}
}

// Store loads and saves a state file.
type Store struct {
	Path string
}

// Load reads state. Missing files return defaults without being created.
// Malformed files are moved aside so a later explicit save can start clean.
func (s Store) Load() (State, []string, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil, nil
		}
		return Defaults(), nil, fmt.Errorf("read state: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		backup := backupPath(s.Path)
		if renameErr := os.Rename(s.Path, backup); renameErr != nil {
			return Defaults(), []string{
				fmt.Sprintf("state is invalid (%v) and could not be moved aside: %v", err, renameErr),
			}, nil
		}
		return Defaults(), []string{
			fmt.Sprintf("state was invalid (%v); it was moved to %s and defaults were loaded", err, backup),
		}, nil
	}

	var current State
	if err := json.Unmarshal(data, &current); err != nil {
		return Defaults(), []string{fmt.Sprintf("state has unexpected values: %v", err)}, nil
	}

	var warnings []string
	switch {
	case current.SchemaVersion == 0:
		current.SchemaVersion = CurrentSchemaVersion
		warnings = append(warnings, "state schemaVersion was missing; treated as version 1")
	case current.SchemaVersion > CurrentSchemaVersion:
		warnings = append(warnings, fmt.Sprintf(
			"state schemaVersion %d is newer than supported version %d; defaults were loaded and the file was not modified",
			current.SchemaVersion, CurrentSchemaVersion,
		))
		return Defaults(), warnings, nil
	}
	if current.RecentProjects == nil {
		current.RecentProjects = []RecentProject{}
	}
	if current.PinnedExternalProjects == nil {
		current.PinnedExternalProjects = []string{}
	}

	return current, warnings, nil
}

// Save atomically writes state with mode 0600.
func (s Store) Save(current State) error {
	if s.Path == "" {
		return fmt.Errorf("state path is empty")
	}
	if current.SchemaVersion == 0 {
		current.SchemaVersion = CurrentSchemaVersion
	}
	if current.RecentProjects == nil {
		current.RecentProjects = []RecentProject{}
	}
	if current.PinnedExternalProjects == nil {
		current.PinnedExternalProjects = []string{}
	}

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set state permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func backupPath(path string) string {
	base := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
	candidate := base
	for counter := 1; ; counter++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, counter)
	}
}
