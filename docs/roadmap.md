# Roadmap

Phase 1 is complete. The phases below are planned, not implemented. Commands for
them are not registered and screens state that they are placeholders.

## Phase 2 — SDK and runtime installation

- Microsoft release metadata with caching and offline fallback.
- `dotnet-install.sh` downloaded to a temporary file, inspected, and executed
  without piping to a shell.
- Explicit version resolution and a user-owned installation directory.
- Verification through the installed `dotnet --info`.
- Managed-install manifest; removal only for Hilt-managed roots.
- Arch/CachyOS pacman adapter with availability checks and explicit approval.
- Never silent `sudo`; no automatic `PATH` changes.

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
