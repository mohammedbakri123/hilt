# Hilt

Hilt is a terminal UI and CLI for managing the .NET ecosystem on Linux. It is a
standalone Go executable: it starts and reports useful information even when no
.NET runtime or SDK is installed.

## Status

Phase 1, the managed SDK/runtime portions of Phase 2, and Phase 3 project
discovery are implemented.

Phase 1 provides:

- Full-screen TUI with Home, Projects, SDKs & Runtimes, Diagnostics, and Settings.
- .NET host discovery from `DOTNET_ROOT`, `PATH`, known roots, and the managed
  root.
- SDK and shared-runtime inventory with provenance, ownership, and selection.
- `hilt doctor` with human and JSON output.
- `hilt sdk list`, `hilt sdk which`, `hilt runtime list`, and `hilt version`
  with JSON output.
- A single Hilt home at `~/hilt` with corrupt-file preservation.

Phase 2 managed SDKs provide:

- `hilt sdk available` lists downloadable stable SDKs for the host platform
  from 7.0 onward, including maintenance and end-of-life channels; installed
  versions are hidden unless `--all` is passed.
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

Phase 3 project discovery provides:

- `hilt projects list [--path <dir>] [--all] [--json]` discovers `.sln`,
  `.slnx`, `.slnf`, and `.csproj` files without invoking MSBuild. The default
  root is `~/hilt/apps`; explicit `--path` values may point anywhere.
- `hilt projects inspect <path> [--json]` reports declared target frameworks,
  SDK hints, output type, test-project hints, and project references.
- Recursive scanning skips `bin`, `obj`, `.git`, and `node_modules` by default,
  ignores hidden paths unless `--all` is passed, guards symlink loops, and
  honors cancellation.
- The TUI Projects & Solutions screen scans in the background, keeps the
  selected project across refreshes, supports `/` filtering, marks external
  projects, and shows declared metadata in a sticky detail pane at wide widths.
  Project tabs cover Overview, Packages, Data, and Output; tabs without a
  backend yet show explicit empty states.

Not implemented: MSBuild evaluation, builds, run/test execution, `global.json`
writes, NuGet operations, and templates. Empty project tabs state each gap
explicitly once tabs exist. See [docs/roadmap.md](docs/roadmap.md).

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
bin/hilt sdk available
bin/hilt sdk available --all --prerelease
bin/hilt sdk available --json
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
bin/hilt projects list
bin/hilt projects list --path ~/src --all
bin/hilt projects list --json
bin/hilt projects inspect path/to/App.csproj
bin/hilt projects inspect path/to/Solution.sln --json
bin/hilt version
```

`hilt` with no arguments and no attached terminal exits with code `2` instead of
hanging. Commands for later phases are not registered until they work.

Managed SDKs are installed at `<HILT_HOME>/.hilt/data/dotnet/<version>`. The
install manifest and lock live beside that directory at
`<HILT_HOME>/.hilt/data/installs.json` and
`<HILT_HOME>/.hilt/data/installs.lock`. Removal requires both a managed path and
manifest ownership. An SDK copied into the managed root by hand is reported but
cannot be removed by Hilt.

`hilt sdk available` reads official Microsoft release metadata and lists every
stable patch version for the host RID from .NET 7.0 onward, newest first.
Maintenance and end-of-life channels are included. Versions already present in
the inventory are hidden by default; `--all` includes them and `--prerelease`
adds prerelease versions. It does not download or install anything. In the TUI
picker, downloads show a progress bar.

`hilt sdk outdated` checks the newest stable SDK in the same `major.minor`
release channel as each managed installation. It reports results only; it does
not download or update anything.

`hilt projects list` scans `~/hilt/apps` by default, a `--path` directory, or a
single project/solution file. Projects outside the Hilt home can be scanned or
inspected explicitly; Hilt references them and never copies or moves them.
Directory names matching the configured
`scanExclusions` (default `bin`, `obj`, `.git`, `node_modules`) are skipped at
any depth. `--all` also includes hidden files and directories. Declared values
are shown as written; Hilt does not evaluate MSBuild conditions or properties.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, already installed, stale entry reconciled, up to date, or `doctor` found no errors. |
| `1` | Operation failed, including a refused removal or unavailable metadata, or `doctor` found errors. |
| `2` | Usage error, unknown command, or missing TTY for the TUI. |
| `130` | Interrupted or cancelled. |

Warnings do not change the exit code. JSON output includes every finding so
scripts can apply stricter rules.

## Hilt home and configuration

Hilt uses one user-owned home, `~/hilt` by default. Set `HILT_HOME` to an
absolute path to move it. Relative overrides are ignored.

```text
~/hilt/
  apps/                 projects created or adopted under the Hilt home
  .hilt/
    config.json         user configuration
    state.json          active project, recents, and pinned external projects
    cache/              disposable metadata
    data/
      installs.json     managed-install manifest
      installs.lock     install lock
      dotnet/           managed SDKs and runtimes
    logs/               future build and run logs
```

- `Paths.ProjectsRoot` and the default project scan root are `~/hilt/apps`.
- Hilt creates the home, `apps/`, and internal directories at startup. The
  configuration, state, and install manifest files are still created only when
  they are written.
- `HILT_HOME` applies to configuration, state, cache, data, logs, and the
  managed toolchain root. XDG variables no longer select these locations.
- Projects outside `~/hilt/apps` remain valid. Hilt records their paths but
  never copies, moves, or rewrites them.
- Existing projects are never migrated into the Hilt home automatically.
- If a legacy XDG install manifest is present and the new manifest is not,
  Hilt warns at startup. It does not move or delete the legacy data, so managed
  installs are not lost silently.

The configuration file is created only when a save is requested. Malformed
configuration and state files are moved to `.corrupt-<timestamp>` and defaults
are loaded. Unknown configuration fields are preserved on save. Configuration
and state writes are atomic with mode `0600`. See
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
