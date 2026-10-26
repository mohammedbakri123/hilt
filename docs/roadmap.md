# Roadmap

Phase 1 is complete. Phase 2 managed SDKs and runtimes are implemented; the
package-manager adapter remains. Phase 3 project discovery is implemented;
MSBuild evaluation, builds, and execution remain planned. Hilt now uses a
single `~/hilt` home, configurable with `HILT_HOME`, with projects under
`apps/` and internals under `.hilt/`. Commands that do not work are not
registered, and unimplemented features show explicit empty states rather than
dead destinations.

## Phase 2 — Managed SDKs (partially implemented)

Implemented:

- Official Microsoft release metadata with explicit network access.
- Direct `.tar.gz` downloads for the host RID, restricted to Microsoft HTTPS
  hosts, with size limits and SHA-512 verification.
- Exact stable version resolution and per-version, user-owned installation
  directories under `<HILT_HOME>/.hilt/data/dotnet`.
- Staged installation verified through the installed `dotnet --list-sdks` and
  `dotnet --info` with an explicit environment.
- Atomic publish, idempotent installs, install locking, and cleanup on failure.
- Managed-install manifest; removal only for manifest-owned roots.
- `hilt sdk available` for browsing downloadable versions from 7.0 onward
  across active, maintenance, and end-of-life channels, with installed versions
  hidden by default in both the CLI and TUI. TUI downloads show a progress bar.
- `hilt sdk install`, `hilt sdk remove`, and `hilt sdk outdated` with JSON
  output.
- Explicit same-channel update checks; no automatic updates.
- Read-wide discovery with no `sudo`, package-manager writes, or `PATH` edits.

Implemented (continued):

- Managed runtime installation, removal, and update checks for netcore and
  aspnetcore frameworks.
- Applying SDK updates (`hilt sdk update`), plus channels, aliases, and
  prerelease policies.
- Version requests (`latest`, `lts`, `10.0`, exact versions, `--prerelease`).

Remaining:

- Arch/CachyOS pacman adapter with availability checks and explicit approval.

## Phase 3 — Projects and execution

Implemented:

- Solution and project discovery with configured exclusions, hidden-path
  handling, depth limits, symlink-loop guards, and cancellation.
- `.sln`, `.slnx`, `.slnf`, and `.csproj` declared metadata: target frameworks,
  SDK hint, output type, test-project hint, and project references.
- `hilt projects list` and `hilt projects inspect` with human and JSON output.
- TUI Projects & Solutions screen with background scanning, cancellation, and
  selection preserved across refreshes.
- Single Hilt home layout with `apps/` and `.hilt/`, plus an atomic,
  versioned state store package for active and recent projects.
- Five-section TUI navigation, a global status line, a keymap-backed footer and
  help view, and a project cockpit with list/detail panes, filtering, external
  badges, and Overview/Packages/Data/Output tabs.

Remaining:

- Wire the state store into the TUI for active and recent projects.
- Evaluated properties through MSBuild when available; declared-only values
  labeled as such.
- Build, clean, restore, run, stop, and test with streaming output.
- `global.json` inspection and write support.

## Phase 4 — NuGet

- Direct and transitive dependency inventory.
- Search, add, update, remove, and restore.
- SDK capability detection.
- Vulnerability and audit information where supported.

## Phase 5 — Project creation

- Dynamic `dotnet new` template discovery.
- Clean Architecture preset.
- Vertical Slice preset.
- Reference validation and post-generation verification.

## Phase 6 — Distribution

- Linux amd64 and arm64 releases with checksums.
- CI and release automation.
- Shell completions.
- Additional platforms after Linux stabilizes.
