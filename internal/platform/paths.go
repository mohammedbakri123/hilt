package platform

import (
	"path/filepath"
)

// Paths contains Hilt's user-owned directory layout.
type Paths struct {
	HomeDir      string `json:"homeDir"`
	ProjectsRoot string `json:"projectsRoot"`
	ConfigDir    string `json:"configDir"`
	StateDir     string `json:"stateDir"`
	CacheDir     string `json:"cacheDir"`
	DataDir      string `json:"dataDir"`
	LogsDir      string `json:"logsDir"`
	ManagedRoot  string `json:"managedRoot"`
}

// NewPaths resolves the single Hilt home. HILT_HOME wins when it is an absolute
// path; otherwise the home directory is <home>/hilt. Relative overrides are
// ignored so Hilt never scatters data relative to the working directory.
func NewPaths(getenv func(string) string, home string) Paths {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if home == "" {
		home = "."
	}

	root := getenv("HILT_HOME")
	if !filepath.IsAbs(root) {
		root = filepath.Join(home, "hilt")
	}
	root = filepath.Clean(root)

	hilt := filepath.Join(root, ".hilt")
	data := filepath.Join(hilt, "data")

	return Paths{
		HomeDir:      root,
		ProjectsRoot: filepath.Join(root, "apps"),
		ConfigDir:    hilt,
		StateDir:     hilt,
		CacheDir:     filepath.Join(hilt, "cache"),
		DataDir:      data,
		LogsDir:      filepath.Join(hilt, "logs"),
		ManagedRoot:  filepath.Join(data, "dotnet"),
	}
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
