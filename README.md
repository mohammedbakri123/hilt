# Hilt

Hilt is a terminal UI and CLI for managing the .NET ecosystem on Linux. It is a
standalone Go executable: it starts and reports useful information even when no
.NET runtime or SDK is installed.

## Phase 1 status

Phase 1 is implemented:

- Full-screen TUI with Dashboard, SDKs & Runtimes, Diagnostics, and Settings.
- .NET host discovery from `DOTNET_ROOT`, `PATH`, known roots, and the managed
  root.
- SDK and shared-runtime inventory with provenance and managed classification.
- `hilt doctor` with human and JSON output.
- `hilt sdk list`, `hilt runtime list`, and `hilt version` with JSON output.
- XDG configuration loading with corrupt-file preservation.

Not implemented in Phase 1: installation, removal, project scanning, builds,
NuGet operations, and templates. The corresponding screens say so explicitly.
See [docs/roadmap.md](docs/roadmap.md).

## Requirements

- Linux (amd64 and arm64 are the intended targets).
- Go 1.26 or newer to build.
- A terminal for the interactive TUI. The CLI works without one.
- .NET is optional.

## Build

```bash
make build
```

The binary is written to `bin/hilt`. Version, commit, and build date are
injected through linker flags. For a plain build without those flags:

```bash
go build ./...
```

## Run

```bash
bin/hilt          # TUI, requires a TTY
bin/hilt doctor
bin/hilt doctor --json
bin/hilt sdk list
bin/hilt sdk list --json
bin/hilt runtime list
bin/hilt version
```

`hilt` with no arguments and no attached terminal exits with code `2` instead of
hanging. Commands for later phases are not registered until they work.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, or `doctor` found no errors. |
| `1` | Operation failed, or `doctor` found errors. |
| `2` | Usage error, unknown command, or missing TTY for the TUI. |
| `130` | Interrupted or cancelled. |

Warnings do not change the exit code. JSON output includes every finding so
scripts can apply stricter rules.

## Configuration

Configuration lives under XDG directories, with a `hilt` suffix:

- `$XDG_CONFIG_HOME/hilt/config.json` (default `~/.config/hilt`)
- `$XDG_STATE_HOME/hilt` (default `~/.local/state/hilt`)
- `$XDG_CACHE_HOME/hilt` (default `~/.cache/hilt`)
- `$XDG_DATA_HOME/hilt` (default `~/.local/share/hilt`)

The file is created only when a save is requested. Malformed files are moved to
`config.json.corrupt-<timestamp>` and defaults are loaded. Unknown JSON fields
are preserved on save. See [docs/safety.md](docs/safety.md).

## Verification

```bash
./scripts/verify.sh
```

The script checks formatting, runs `go vet`, the test suite, the race-enabled
test suite, and builds the binary. Unit tests require neither network access nor
administrator privileges. A real-SDK integration test is opt-in:

```bash
HILT_TEST_REAL_DOTNET=1 go test ./internal/dotnet -run TestRealDotnetInventory -v
```

## Documentation

- [docs/architecture.md](docs/architecture.md) — package boundaries and data flow.
- [docs/safety.md](docs/safety.md) — installation and version-management policy.
- [docs/roadmap.md](docs/roadmap.md) — phases 2–6.

## License

MIT. See [LICENSE](LICENSE).
