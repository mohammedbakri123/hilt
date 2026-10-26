package platform

import (
	"os"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	data := []byte(`NAME="CachyOS Linux"
ID=cachyos
ID_LIKE="arch"
VERSION_ID=20261025
PRETTY_NAME="CachyOS"
# comment
QUOTED='single'
EMPTY=
ESCAPED="a \"quoted\" \\ \$HOME"
EMPTY_QUOTED=""
`)
	values, err := ParseOSRelease(data)
	if err != nil {
		t.Fatalf("ParseOSRelease: %v", err)
	}
	if got := values["ID"]; got != "cachyos" {
		t.Errorf("ID = %q, want cachyos", got)
	}
	if got := values["NAME"]; got != "CachyOS Linux" {
		t.Errorf("NAME = %q", got)
	}
	if got := values["ID_LIKE"]; got != "arch" {
		t.Errorf("ID_LIKE = %q", got)
	}
	if got := values["QUOTED"]; got != "single" {
		t.Errorf("QUOTED = %q", got)
	}
	if got, ok := values["EMPTY"]; !ok || got != "" {
		t.Errorf("EMPTY = %q, ok=%v", got, ok)
	}
	if got := values["ESCAPED"]; got != `a "quoted" \ $HOME` {
		t.Errorf("ESCAPED = %q", got)
	}
	if got, ok := values["EMPTY_QUOTED"]; !ok || got != "" {
		t.Errorf("EMPTY_QUOTED = %q, ok=%v", got, ok)
	}
}

func TestDetectFrom(t *testing.T) {
	dir := t.TempDir()
	osRelease := dir + "/os-release"
	kernel := dir + "/kernel"
	if err := os.WriteFile(osRelease, []byte("ID=cachyos\nID_LIKE=\"arch\"\nVERSION_ID=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kernel, []byte("7.2.9-cachyos\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	info := DetectFrom(osRelease, kernel)
	if info.Distro != "cachyos" {
		t.Errorf("Distro = %q", info.Distro)
	}
	if info.Kernel != "7.2.9-cachyos" {
		t.Errorf("Kernel = %q", info.Kernel)
	}
	if len(info.DistroLike) != 1 || info.DistroLike[0] != "arch" {
		t.Errorf("DistroLike = %v", info.DistroLike)
	}
	if info.DisplayName() != "Cachyos 1" {
		t.Errorf("DisplayName = %q", info.DisplayName())
	}
	if info.DistroName != "" {
		t.Errorf("DistroName = %q, want empty", info.DistroName)
	}
}

func TestDetectFromMissingFiles(t *testing.T) {
	info := DetectFrom("/nonexistent/os-release", "/nonexistent/kernel")
	if info.OS == "" || info.Arch == "" {
		t.Fatalf("expected runtime platform, got %+v", info)
	}
	if info.Distro != "" {
		t.Errorf("Distro = %q, want empty", info.Distro)
	}
}

func TestNewPathsHiltHomeOverride(t *testing.T) {
	env := map[string]string{"HILT_HOME": "/srv/hilt"}
	paths := NewPaths(func(k string) string { return env[k] }, "/home/user")
	if paths.HomeDir != "/srv/hilt" {
		t.Errorf("HomeDir = %q", paths.HomeDir)
	}
	if paths.ProjectsRoot != "/srv/hilt/apps" {
		t.Errorf("ProjectsRoot = %q", paths.ProjectsRoot)
	}
	if paths.ConfigDir != "/srv/hilt/.hilt" {
		t.Errorf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.StateDir != "/srv/hilt/.hilt" {
		t.Errorf("StateDir = %q", paths.StateDir)
	}
	if paths.CacheDir != "/srv/hilt/.hilt/cache" {
		t.Errorf("CacheDir = %q", paths.CacheDir)
	}
	if paths.DataDir != "/srv/hilt/.hilt/data" {
		t.Errorf("DataDir = %q", paths.DataDir)
	}
	if paths.LogsDir != "/srv/hilt/.hilt/logs" {
		t.Errorf("LogsDir = %q", paths.LogsDir)
	}
	if paths.ManagedRoot != "/srv/hilt/.hilt/data/dotnet" {
		t.Errorf("ManagedRoot = %q", paths.ManagedRoot)
	}
	if paths.ConfigFile() != "/srv/hilt/.hilt/config.json" {
		t.Errorf("ConfigFile = %q", paths.ConfigFile())
	}
	if paths.StateFile() != "/srv/hilt/.hilt/state.json" {
		t.Errorf("StateFile = %q", paths.StateFile())
	}
	if paths.InstallManifestFile() != "/srv/hilt/.hilt/data/installs.json" {
		t.Errorf("InstallManifestFile = %q", paths.InstallManifestFile())
	}
	if paths.InstallLockFile() != "/srv/hilt/.hilt/data/installs.lock" {
		t.Errorf("InstallLockFile = %q", paths.InstallLockFile())
	}
}

func TestNewPathsDefaultHome(t *testing.T) {
	paths := NewPaths(nil, "/home/user")
	if paths.HomeDir != "/home/user/hilt" {
		t.Errorf("HomeDir = %q", paths.HomeDir)
	}
	if paths.ProjectsRoot != "/home/user/hilt/apps" {
		t.Errorf("ProjectsRoot = %q", paths.ProjectsRoot)
	}
	if paths.ConfigDir != "/home/user/hilt/.hilt" {
		t.Errorf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.ManagedRoot != "/home/user/hilt/.hilt/data/dotnet" {
		t.Errorf("ManagedRoot = %q", paths.ManagedRoot)
	}
}

func TestNewPathsIgnoresRelativeOverride(t *testing.T) {
	for _, value := range []string{"relative", "./relative", ""} {
		paths := NewPaths(func(string) string { return value }, "/home/user")
		if paths.HomeDir != "/home/user/hilt" {
			t.Errorf("HILT_HOME=%q: HomeDir = %q", value, paths.HomeDir)
		}
	}
}

func TestNewPathsHomeFallback(t *testing.T) {
	paths := NewPaths(nil, "")
	if paths.HomeDir == "" || paths.ConfigDir == "" || paths.ManagedRoot == "" {
		t.Fatalf("paths should not be empty: %+v", paths)
	}
}

func TestFindPackageManager(t *testing.T) {
	lookPath := func(name string) (string, error) {
		switch name {
		case "pacman", "sudo":
			return "/usr/bin/" + name, nil
		}
		return "", errNotFound
	}
	manager := FindPackageManager(lookPath)
	if manager == nil {
		t.Fatal("expected pacman")
	}
	if manager.Name != "pacman" || manager.Privilege != "sudo" {
		t.Errorf("manager = %+v", manager)
	}
	if FindPackageManager(func(string) (string, error) { return "", errNotFound }) != nil {
		t.Error("expected nil manager")
	}
}

type notFound struct{}

func (notFound) Error() string { return "not found" }

var errNotFound = notFound{}
