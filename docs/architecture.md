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
internal/platform        OS/distro detection, XDG paths, package managers
internal/process         external command execution with contexts and groups
```

## Dependency direction

```text
cmd/hilt
  ├── internal/app
  │     ├── internal/config
  │     ├── internal/diagnostics
  │     ├── internal/dotnet
  │     │     └── internal/process
  │     └── internal/platform
  ├── internal/cli ──► internal/{config,diagnostics,dotnet,platform}
  └── internal/tui ──► internal/{config,diagnostics,dotnet,platform}
```

- `internal/tui` and `internal/cli` define their own small `Backend`
  interfaces. Both are satisfied by `*internal/app.App`.
- `internal/dotnet` depends on `internal/process` for probing, but parsers and
  filesystem scanning are independent of the process layer.
- `internal/platform` and `internal/config` do not import higher-level packages.
- No package imports `internal/app`, which prevents composition cycles.

## Runtime flow

1. `main` creates a signal-aware context and constructs `app.New`.
2. `app.New` detects the platform, resolves XDG paths, loads configuration, and
   wires the process runner, locator, prober, and package-manager lookup.
3. The CLI either runs a non-interactive command or hands the backend to the
   TUI.
4. Discovery runs in `tea.Cmd` / CLI command context, never in the TUI update
   loop. Missing .NET is represented as an empty inventory plus warnings.
5. Diagnostics reuses the inventory and adds configuration, `global.json`,
   package-manager, git, and managed-root findings.

## Contracts

- External commands receive argument slices; no shell interpolation.
- Every command takes a `context.Context`; cancellation kills the process group.
- Parsers are exported so captured .NET output can be tested without .NET.
- Configuration saves are atomic, mode `0600`, and preserve unknown fields.
- JSON payloads carry `schemaVersion: 1`.

## Testing seams

`Locator`, `Prober`, `process.Runner`, `LookPath`, `Stat`, environment lookup,
and clock-free parsers are injectable. Unit tests use temporary directory
trees, fake probers, and captured fixtures under `tests/testdata`. A real .NET
integration test is opt-in through `HILT_TEST_REAL_DOTNET=1`.
