package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CurrentSchemaVersion is the configuration schema Hilt writes.
const CurrentSchemaVersion = 1

// RecentProject records a project Hilt has opened.
type RecentProject struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// UIPreferences controls presentation and key handling.
type UIPreferences struct {
	Theme   string `json:"theme"`
	VimKeys bool   `json:"vimKeys"`
}

// Config is Hilt's versioned user configuration.
type Config struct {
	SchemaVersion     int             `json:"schemaVersion"`
	ManagedRoot       string          `json:"managedRoot"`
	DefaultProjectDir string          `json:"defaultProjectDir"`
	RecentProjects    []RecentProject `json:"recentProjects"`
	ScanExclusions    []string        `json:"scanExclusions"`
	Editor            string          `json:"editor"`
	UI                UIPreferences   `json:"ui"`
	extra             map[string]json.RawMessage
}

// Defaults returns a complete configuration.
func Defaults(managedRoot string) Config {
	return Config{
		SchemaVersion:  CurrentSchemaVersion,
		ManagedRoot:    managedRoot,
		RecentProjects: []RecentProject{},
		ScanExclusions: []string{"bin", "obj", ".git", "node_modules"},
		UI: UIPreferences{
			Theme:   "auto",
			VimKeys: true,
		},
	}
}

// Store loads and saves a configuration file.
type Store struct {
	Path string
}

// Load reads the configuration. Missing files return defaults. Malformed files
// are moved aside and replaced by defaults in memory; they are not overwritten
// on disk until an explicit save.
func (s Store) Load() (Config, []string, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(""), nil, nil
		}
		return Defaults(""), nil, fmt.Errorf("read config: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		backup := backupPath(s.Path)
		if renameErr := os.Rename(s.Path, backup); renameErr != nil {
			return Defaults(""), []string{
				fmt.Sprintf("config is invalid (%v) and could not be moved aside: %v", err, renameErr),
			}, nil
		}
		return Defaults(""), []string{
			fmt.Sprintf("config was invalid (%v); it was moved to %s and defaults were loaded", err, backup),
		}, nil
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Defaults(""), []string{fmt.Sprintf("config has unexpected values: %v", err)}, nil
	}

	var warnings []string
	switch {
	case config.SchemaVersion == 0:
		config.SchemaVersion = CurrentSchemaVersion
		warnings = append(warnings, "config schemaVersion was missing; treated as version 1")
	case config.SchemaVersion > CurrentSchemaVersion:
		warnings = append(warnings, fmt.Sprintf(
			"config schemaVersion %d is newer than supported version %d; defaults were loaded and the file was not modified",
			config.SchemaVersion, CurrentSchemaVersion,
		))
		return Defaults(""), warnings, nil
	}

	config.extra = make(map[string]json.RawMessage)
	known := knownFields()
	for key, value := range raw {
		if !known[key] {
			config.extra[key] = value
		}
	}

	return config, warnings, nil
}

// Save atomically writes the configuration with mode 0600.
func (s Store) Save(config Config) error {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = CurrentSchemaVersion
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if len(config.extra) > 0 {
		var merged map[string]json.RawMessage
		if err := json.Unmarshal(data, &merged); err != nil {
			return fmt.Errorf("re-encode config: %w", err)
		}
		for key, value := range config.extra {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
		data, err = json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return fmt.Errorf("encode merged config: %w", err)
		}
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tempPath, s.Path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func knownFields() map[string]bool {
	return map[string]bool{
		"schemaVersion":     true,
		"managedRoot":       true,
		"defaultProjectDir": true,
		"recentProjects":    true,
		"scanExclusions":    true,
		"editor":            true,
		"ui":                true,
	}
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
