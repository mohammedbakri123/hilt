# Roadmap

Phase 1 is complete. The managed-SDK portion of Phase 2 is implemented; the
remaining Phase 2 items and later phases are planned, not implemented. Commands
that do not work are not registered, and screens state that they are
placeholders.

## Phase 2 — Managed SDKs (partially implemented)

Implemented:

- Official Microsoft release metadata with explicit network access.
- Direct `.tar.gz` downloads for the host RID, restricted to Microsoft HTTPS
  hosts, with size limits and SHA-512 verification.
- Exact stable version resolution and per-version, user-owned installation
  directories under `<data>/hilt/dotnet`.
- Staged installation verified through the installed `dotnet --list-sdks` and
  `dotnet --info` with an explicit environment.
- Atomic publish, idempotent installs, install locking, and cleanup on failure.
- Managed-install manifest; removal only for manifest-owned roots.
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

- TUI install/remove controls.
- Arch/CachyOS pacman adapter with availability checks and explicit approval.

## Phase 3 — Projects and execution

- Solution and project scanning with exclusions and cancellation.
- `.sln`, `.slnx`, `.slnf`, and `.csproj` metadata.
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
