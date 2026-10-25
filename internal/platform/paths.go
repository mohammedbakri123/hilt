package platform

import (
	"path/filepath"
)

// Paths contains Hilt's XDG-aligned directories.
type Paths struct {
	ConfigDir   string `json:"configDir"`
	StateDir    string `json:"stateDir"`
	CacheDir    string `json:"cacheDir"`
	DataDir     string `json:"dataDir"`
	ManagedRoot string `json:"managedRoot"`
}

// NewPaths resolves XDG directories. Relative XDG values are ignored, matching
// the specification. An empty home falls back to the current directory only so
// that Hilt can still start and report the problem.
func NewPaths(getenv func(string) string, home string) Paths {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if home == "" {
		home = "."
	}

	config := xdgDir(getenv("XDG_CONFIG_HOME"), home, ".config")
	state := xdgDir(getenv("XDG_STATE_HOME"), home, ".local/state")
	cache := xdgDir(getenv("XDG_CACHE_HOME"), home, ".cache")
	data := xdgDir(getenv("XDG_DATA_HOME"), home, ".local/share")

	return Paths{
		ConfigDir:   filepath.Join(config, "hilt"),
		StateDir:    filepath.Join(state, "hilt"),
		CacheDir:    filepath.Join(cache, "hilt"),
		DataDir:     filepath.Join(data, "hilt"),
		ManagedRoot: filepath.Join(data, "hilt", "dotnet"),
	}
}

func xdgDir(value, home, fallback string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(home, filepath.FromSlash(fallback))
}

// ConfigFile is the main configuration path.
func (p Paths) ConfigFile() string {
	return filepath.Join(p.ConfigDir, "config.json")
}

// StateFile is the persistent state path.
func (p Paths) StateFile() string {
	return filepath.Join(p.StateDir, "state.json")
}

// InstallManifestFile is the managed SDK install manifest path.
func (p Paths) InstallManifestFile() string {
	return filepath.Join(p.DataDir, "installs.json")
}

// InstallLockFile is the managed SDK install lock path.
func (p Paths) InstallLockFile() string {
	return filepath.Join(p.DataDir, "installs.lock")
}
