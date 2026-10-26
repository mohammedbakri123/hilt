# Architecture

Hilt is a Go module (`github.com/mohammedbakri123/hilt`) with a small CLI entry
point and internal packages. Dependencies point inward, and the TUI never owns
system behavior.

## Packages

```text
cmd/hilt                 process entry point, linker-version variables
internal/app             composition root; implements CLI and TUI backends
internal/cli             cobra command tree, JSON and human output
internal/tui             Bubble Tea model, screens, rendering
internal/config          versioned JSON configuration with atomic saves
internal/diagnostics     doctor engine and findings
internal/dotnet          discovery, probing, inventory, global.json, versions
internal/install         release metadata, archives, manifest, install/remove
internal/platform        OS/distro detection, XDG paths, package managers
internal/process         external command execution with contexts and groups
internal/projects        project/solution discovery and declared metadata
```

## Dependency direction

```text
cmd/hilt
  ├── internal/app
  │     ├── internal/config
  │     ├── internal/diagnostics
  │     ├── internal/dotnet
  │     │     └── internal/process
  │     ├── internal/install
  │     │     ├── internal/dotnet
  │     │     └── internal/process
  │     ├── internal/platform
  │     └── internal/projects
  ├── internal/cli ──► internal/{config,diagnostics,dotnet,install,platform,projects}
  └── internal/tui ──► internal/{config,diagnostics,dotnet,platform,projects}
```

- `internal/tui` and `internal/cli` define their own small `Backend`
  interfaces. Both are satisfied by `*internal/app.App`.
- `internal/dotnet` depends on `internal/process` for probing, but parsers and
  filesystem scanning are independent of the process layer.
- `internal/install` owns all network, archive, manifest, and managed-root
  mutation logic. It depends on `internal/dotnet` for versions, RIDs, parsing,
  and the explicit toolchain environment.
- `internal/projects` owns filesystem discovery and declared project metadata.
  It parses `.csproj`, `.sln`, `.slnx`, and `.slnf` with standard-library
  decoders and never invokes MSBuild or `dotnet`.
- `internal/platform` and `internal/config` do not import higher-level packages.
- No package imports `internal/app`, which prevents composition cycles.

## Runtime flow

1. `main` creates a signal-aware context and constructs `app.New`.
2. `app.New` detects the platform, resolves XDG paths, loads configuration, and
   wires the process runner, locator, prober, package-manager lookup, and
   install manifest. The manifest supplies ownership and managed roots to the
   locator.
3. The CLI either runs a non-interactive command or hands the backend to the
   TUI.
4. Discovery runs in `tea.Cmd` / CLI command context, never in the TUI update
   loop. Missing .NET is represented as an empty inventory plus warnings.
5. Diagnostics reuses the inventory and adds configuration, `global.json`,
   package-manager, git, and managed-root findings.
6. `hilt projects list` walks the requested root with `filepath.WalkDir`,
   applies configured directory exclusions, and parses project/solution files
   without MSBuild. The TUI Projects screen runs the same scan in a `tea.Cmd`
   and preserves the selected project across refreshes.
7. `hilt sdk available` reads the release index and channel metadata to list
   downloadable versions from 7.0 onward for the host RID, including
   maintenance and end-of-life channels. `hilt sdk install` resolves
   metadata, downloads and verifies an archive, stages and verifies the
   toolchain, publishes it atomically, then records it in the manifest. `hilt sdk remove` reverses that only for manifest-owned
   roots. `hilt sdk outdated` reads metadata and reports newer same-channel
   stable versions. `hilt sdk update` and `hilt runtime install/remove/outdated`
   extend the same install flow. The TUI runs these same backend operations from
   `tea.Cmd` values with context cancellation; it never performs I/O in Update.

## Contracts

- External commands receive argument slices; no shell interpolation.
- Every command takes a `context.Context`; cancellation kills the process group.
- Parsers are exported so captured .NET output can be tested without .NET.
- Configuration saves are atomic, mode `0600`, and preserve unknown fields.
- The install manifest uses `schemaVersion: 2` (runtime-aware), stores roots relative to the
  managed root, and is written atomically with mode `0600`.
- Managed SDKs live at `<managedRoot>/<version>`. Ownership comes from the
  manifest, not from the path alone.
- Install and removal operations are serialized by `installs.lock`.
- Network access is confined to `internal/install`; ordinary tests use local
  HTTP servers.
- Project discovery is read-only and confined to the requested root (plus the
  paths a workspace file declares). It never writes projects or invokes MSBuild.
- JSON payloads carry `schemaVersion: 1`.

## Testing seams

`Locator`, `Prober`, `process.Runner`, `LookPath`, `Stat`, environment lookup,
clock-free parsers, `install.ReleaseClient`, `HTTPClient`, `ValidateURL`,
`RemoveAll`, and `Now` are injectable. Unit tests use temporary directory trees,
fake probers and runners, captured fixtures under `tests/testdata`, project
fixtures under `internal/projects/testdata`, and local metadata/archive
servers. A real .NET integration test is opt-in through
`HILT_TEST_REAL_DOTNET=1`.
