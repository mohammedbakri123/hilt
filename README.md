# Hilt

Hilt is a terminal UI and CLI for managing the .NET ecosystem on Linux. It is a
standalone Go executable: it starts and reports useful information even when no
.NET runtime or SDK is installed.

## Status

Phase 1 and the managed SDK/runtime portions of Phase 2 are implemented.

Phase 1 provides:

- Full-screen TUI with Dashboard, SDKs & Runtimes, Diagnostics, and Settings.
- .NET host discovery from `DOTNET_ROOT`, `PATH`, known roots, and the managed
  root.
- SDK and shared-runtime inventory with provenance, ownership, and selection.
- `hilt doctor` with human and JSON output.
- `hilt sdk list`, `hilt sdk which`, `hilt runtime list`, and `hilt version`
  with JSON output.
- XDG configuration loading with corrupt-file preservation.

Phase 2 managed SDKs provide:

- `hilt sdk install <version>` from official Microsoft release metadata.
- SHA-512 verification, archive limits, staging, and verification through the
  installed `dotnet`.
- Per-version roots under the Hilt-managed root, recorded in an install
  manifest.
- `hilt sdk remove <version>` only for manifest-owned installations.
- `hilt sdk outdated` for explicit same-channel update checks. Hilt never
  updates automatically.
- `hilt sdk update <version> [--remove-old]` applies a managed SDK update.
- `hilt runtime install <version> --framework all|netcore|aspnetcore`.
- `hilt runtime remove <version>` and `hilt runtime outdated`.
- Managed install, removal, and update actions in the TUI's SDKs & Runtimes
  screen, with confirmation, cancellation, and streamed progress.
- Read-wide discovery with write-private mutations: system and external SDKs
  are never modified or removed, and Hilt never uses `sudo` or edits `PATH`.

Not implemented: package-manager writes, project scanning, builds, NuGet
operations, and templates. The corresponding screens say so explicitly. See
[docs/roadmap.md](docs/roadmap.md).

## Requirements

- Linux (amd64 and arm64 are the intended targets).
- Go 1.26 or newer to build.
- A terminal for the interactive TUI. The CLI works without one.
- .NET is optional.
- Managed installation and update checks require network access to Microsoft
  release metadata.

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
bin/hilt sdk which
bin/hilt sdk which --json
bin/hilt sdk install <version>
bin/hilt sdk install <version> --json
bin/hilt sdk remove <version>
bin/hilt sdk remove <version> --json
bin/hilt sdk outdated
bin/hilt sdk outdated --json
bin/hilt sdk update <version>
bin/hilt sdk update <version> --json
bin/hilt runtime list
bin/hilt runtime install <version> --framework all
bin/hilt runtime remove <version>
bin/hilt runtime outdated
bin/hilt runtime outdated --json
bin/hilt version
```

`hilt` with no arguments and no attached terminal exits with code `2` instead of
hanging. Commands for later phases are not registered until they work.

Managed SDKs are installed at `<data>/hilt/dotnet/<version>`. The install
manifest and lock live beside that directory at
`<data>/hilt/installs.json` and `<data>/hilt/installs.lock`. Removal requires
both a managed path and manifest ownership. An SDK copied into the managed root
by hand is reported but cannot be removed by Hilt.

`hilt sdk outdated` checks the newest stable SDK in the same `major.minor`
release channel as each managed installation. It reports results only; it does
not download or update anything.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, already installed, stale entry reconciled, up to date, or `doctor` found no errors. |
| `1` | Operation failed, including a refused removal or unavailable metadata, or `doctor` found errors. |
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

The configuration file is created only when a save is requested. Malformed
files are moved to `config.json.corrupt-<timestamp>` and defaults are loaded.
Unknown JSON fields are preserved on save. The data directory also holds the
managed toolchains, install manifest, and install lock. See
[docs/safety.md](docs/safety.md).

## Verification

```bash
./scripts/verify.sh
```

The script checks formatting, runs `go vet`, the test suite, the race-enabled
test suite, and builds the binary. Ordinary tests require neither network access
nor administrator privileges. Install, removal, and update checks use local
HTTP metadata and archive fixtures. A real-SDK integration test is opt-in:

```bash
HILT_TEST_REAL_DOTNET=1 go test ./internal/dotnet -run TestRealDotnetInventory -v
```

Real Microsoft downloads are not exercised by the regular suite.

## Documentation

- [docs/architecture.md](docs/architecture.md) — package boundaries and data flow.
- [docs/safety.md](docs/safety.md) — installation and version-management policy.
- [docs/roadmap.md](docs/roadmap.md) — phases 2–6.

## License

MIT. See [LICENSE](LICENSE).
