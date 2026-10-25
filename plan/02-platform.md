# Step 2 — Platform and XDG paths

## Objective

Detect the operating system, architecture, distribution, package manager, and
privilege tool. Resolve configuration, state, cache, and data directories
according to XDG on Linux.

## Package

`internal/platform`

## API

```go
type Info struct {
    OS, Arch, Distro, DistroVersion, Kernel string
    DistroLike []string
    IsLinux    bool
}

type PackageManager struct {
    Name      string // pacman, apt, dnf, zypper, apk
    Path      string
    Privilege string // sudo, doas, pkexec, or empty
}

type Paths struct {
    ConfigDir, StateDir, CacheDir, DataDir, ManagedRoot string
}

func Detect() Info
func DetectFrom(osReleasePath, kernelPath string) Info
func ParseOSRelease([]byte) (map[string]string, error)
func NewPaths(getenv func(string) string, home string) Paths
func FindPackageManager(lookPath func(string) (string, error)) *PackageManager
```

## Behavior

- `Detect` reads `/etc/os-release` and `/proc/sys/kernel/osrelease`; missing
  files degrade gracefully. `ISI`/`PRETTY_NAME` are not used as identity.
- `ID` is the primary distribution identifier, `ID_LIKE` is split on spaces.
- XDG precedence:
  - config: `XDG_CONFIG_HOME` → `~/.config`
  - state: `XDG_STATE_HOME` → `~/.local/state`
  - cache: `XDG_CACHE_HOME` → `~/.cache`
  - data: `XDG_DATA_HOME` → `~/.local/share`
- All values get a `hilt` suffix. `ManagedRoot` is `<data>/dotnet`.
- Empty `home` falls back to the current directory only for path construction;
  callers surface a warning instead of failing to start.
- Package-manager detection prefers `pacman`, then `apt`, `dnf`, `zypper`,
  `apk`. Privilege detection prefers `sudo`, then `doas`, then `pkexec`.
  Detection does not run any package command.

## Tests

- OS-release parsing with quoted values, escaped quotes, and comments.
- `ID_LIKE` splitting.
- XDG overrides, relative/absolute values, and home fallback.
- Package-manager selection with a fake `lookPath`.
- `DetectFrom` using temporary fixture files.

## Acceptance

`hilt doctor` prints `CachyOS`, `linux`, `amd64`, and the detected package
manager on the development machine, and the same code produces a useful
platform finding in an empty environment.
