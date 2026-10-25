# Step 4 — .NET discovery and inventory

## Objective

Discover every plausible .NET host without assuming the first `dotnet` on PATH
is complete. Produce a structured inventory of SDKs, shared runtimes, and
ASP.NET Core runtimes, with provenance and management classification.

## Package

`internal/dotnet`

## API

```go
type Source string // path, dotnet_root, known, managed

type Installation struct {
    Root, DotnetPath string
    Source           Source
    Managed, Usable  bool
    Reason           string
}

type Locator struct {
    LookPath, EvalSymlinks, Stat, Getenv func(...)
    ManagedRoot, Home string
}
func (Locator) Locate() []Installation

type SDK struct {
    Version, Path, Root, Architecture string
    Managed, Usable                   bool
    DetectedBy                        string // probe, filesystem
}
type Runtime struct {
    Framework, Version, Path, Root string
    Managed                        bool
    DetectedBy                     string
}
type Host struct {
    Root, DotnetPath, Source, Architecture, RID string
    Managed                                     bool
    SDKs     []SDK
    Runtimes []Runtime
    ProbeError string
}

type Inventory struct { ... }
func Discover(ctx, Locator, Prober, StatFunc) Inventory

type Prober interface {
    ListSDKs(ctx, Installation) ([]SDK, error)
    ListRuntimes(ctx, Installation) ([]Runtime, error)
    Info(ctx, Installation) (HostInfo, error)
}
```

Parsers are exported for tests:

```go
func ParseSDKList([]byte, string) ([]SDK, error)
func ParseRuntimeList([]byte, string) ([]Runtime, error)
func ParseInfo([]byte) HostInfo
func ParseVersion(string) (Version, error)
```

## Discovery order

1. `DOTNET_ROOT`.
2. `dotnet` on PATH, resolving symlinks to the real root.
3. Known roots: `/usr/share/dotnet`, `/usr/lib/dotnet`, `/usr/lib64/dotnet`,
   `/opt/dotnet`, `~/.dotnet`, and the configured managed root.
4. Deduplicate by canonical root. Preserve first-seen priority.

## Inventory rules

- Probe usable hosts with `dotnet --list-sdks`, `dotnet --list-runtimes`, and
  `dotnet --info`, with:
  - `DOTNET_ROOT=<root>`
  - `DOTNET_MULTILEVEL_LOOKUP=0`
  - telemetry disabled and first-run experience skipped.
- If a probe fails, record `ProbeError`, add a warning, and fall back to
  scanning `<root>/sdk/*` and `<root>/shared/<framework>/*`.
- Merging filesystem and probe results is deduplicated by path/version.
- `Managed` is true only when the root is inside the configured managed root.
  System installations are never classified as managed by guesswork.
- A missing .NET installation is an empty inventory with warnings, not an
  error. Hilt must still start.
- Duplicate versions across roots produce a warning, not a failure.

## Version parsing

Support `major.minor.patch[.revision]` with optional `-prerelease`. Compare
numeric segments first; a release sorts above its prereleases.

## `global.json`

- Find the nearest `global.json` from a directory upward (bounded depth).
- Parse `sdk.version`, `sdk.rollForward`, and `sdk.allowPrerelease`.
- Phase 1 only reads; writing belongs to Phase 3.

## Tests

- Version parse/compare table including prerelease and malformed input.
- Parser fixtures captured from .NET 10 on this machine.
- Locator deduplication, symlink resolution, managed classification, and
  missing roots.
- Filesystem scanning with temporary directory trees.
- `Discover` with a fake prober, including probe failure fallback.
- `global.json` lookup from nested directories and malformed JSON.

## Acceptance

On this machine `hilt sdk list` reports `10.0.112` at
`/usr/share/dotnet/sdk/10.0.112`, runtime `10.0.12`, and marks both as
system-managed. With a stripped `PATH` and `DOTNET_ROOT`, discovery returns an
empty inventory without panicking.
