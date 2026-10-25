# Phase 2 handoff — hybrid toolchain model

Status: approved, not implemented.
Date: 2026-10-25.
Baseline commit: `75c1853` (`fix(dotnet): preserve locator priority in host ordering`).

This document is the starting point for the next session. Phase 1 is complete and
committed. Phase 2 adds managed SDK/runtime installation under a hybrid model:
discover and use everything read-only, but write only Hilt-owned toolchains.

## Approved model

**Read-wide, write-private.**

- Discover and use system, distro, user, and Hilt-managed SDKs.
- Install only into the Hilt-owned, user-owned root.
- Never modify, upgrade, or remove system installations in Phase 2.
- Never modify `PATH` automatically and never invoke `sudo`.
- Record Hilt-installed toolchains in a manifest. Removal requires manifest
  ownership, not merely living under the managed root.

## Decisions already made

1. If the same version exists in managed and system roots, prefer managed.
2. System-wide installs are out of scope for Phase 2. Hilt reports system
   installations and leaves updates to the distribution.
3. Build environment injection now. Add an opt-in `hilt env` command later;
   never edit shell startup files.
4. Support exact versions first. Channels, aliases, and prerelease policies come
   after the core installer works.
5. `hilt sdk remove` removes manifest-owned versions only. System, external, and
   unowned-managed installations are refused.
6. Selection order: workspace `global.json`, then managed newest, then system
   newest.
7. Do not reimplement .NET roll-forward. Pick a candidate root, run that root's
   `dotnet --version` in the workspace, and report the actual result. If it
   fails, try the other root and show both outcomes.
8. Managed update checks are explicit. No automatic updates.
9. Always display which toolchain was selected and why.

## Terminology

| Term | Meaning | Writable by Hilt |
| --- | --- | --- |
| `system` | Distro or admin installation, including `/usr/share/dotnet`, `/usr/lib/dotnet`, `/usr/lib64/dotnet`, `/opt/dotnet`, or root-owned paths. | No |
| `external` | A user installation Hilt did not create, such as `~/.dotnet` or an unknown `DOTNET_ROOT`. | No |
| `managed` | Any root inside the configured managed root. | Yes, but only with manifest ownership |
| `owned` | A managed toolchain recorded in Hilt's install manifest. | Yes |

`Managed` stays path-based. `Owned` comes from the manifest. This prevents Hilt
from deleting an SDK a user copied into the managed root manually.

## Development environment baseline

- Go toolchain: `go1.27.1`; module directive: `go 1.26.0`.
- Host: CachyOS, linux/amd64, kernel 7.2.9-2-cachyos.
- System SDK: `10.0.112` at `/usr/share/dotnet/sdk/10.0.112`.
- System runtimes: `Microsoft.AspNetCore.App 10.0.12` and
  `Microsoft.NETCore.App 10.0.12` at `/usr/share/dotnet/shared`.
- `~/.dotnet` exists but has no usable `dotnet` executable.
- `hilt doctor` exits `0`; `hilt sdk list` reports the system SDK.
- Package manager: pacman with sudo detected. Hilt does not run either.

Verification commands used for Phase 1:

```bash
./scripts/verify.sh
HILT_TEST_REAL_DOTNET=1 go test ./internal/dotnet -run TestRealDotnetInventory -v
```

## Current implementation map

- `internal/dotnet/locator.go` — discovery order (`DOTNET_ROOT`, `PATH`, known
  roots) and deduplication. `Source` constants are `path`, `dotnet_root`,
  `known`, and `managed`.
- `internal/dotnet/inventory.go` — `Installation`, `Host`, `SDK`, `Runtime`,
  `Inventory`, `Discover`, filesystem fallback, version sorting, and
  `ResolvedDotnet`. Hosts preserve locator priority.
- `internal/dotnet/probe.go` — `CLIProber`, `--list-sdks`, `--list-runtimes`,
  `--info`, and parsers. The prober sets `DOTNET_ROOT`,
  `DOTNET_MULTILEVEL_LOOKUP=0`, and telemetry/first-run opt-outs.
- `internal/dotnet/globaljson.go` — upward search and parsing.
- `internal/dotnet/version.go` — version parsing and comparison.
- `internal/platform/paths.go` — `ManagedRoot` is `<data>/hilt/dotnet`.
- `internal/app/app.go` — composition root. The locator is built once from
  config, so a changed `managedRoot` requires a restart.
- `internal/cli/` — cobra commands and JSON output. Inventory JSON already
  includes hosts, SDKs, runtimes, warnings, and `generatedAt`.
- `internal/diagnostics/` — doctor engine. The current `global-json` check uses
  exact version matching only and does not evaluate roll-forward.
- `internal/tui/` — dashboard, SDK/runtime screen, diagnostics, settings.
- `internal/config/` — schema version 1, atomic saves, unknown top-level fields
  preserved.

Known gaps relevant to Phase 2:

- No ownership/manifest model.
- No resolver or explicit selected toolchain.
- No environment builder shared by future build/run commands.
- `global-json` does not consider roll-forward or cross-root availability.
- `hilt sdk list` shows system/managed but not owned or selected.
- No installer, download, checksum, manifest, or removal code.
- JSON schema version is `1`; additive fields are currently acceptable.

## Implementation stages

Use small commits. Keep each stage buildable and tested.

### Stage A — provenance and ownership

Files: `internal/dotnet/locator.go`, `inventory.go`, `internal/cli/output.go`,
`internal/tui/view.go`, tests.

Tasks:

- Add a manifest-derived ownership signal to `Installation`, `Host`, `SDK`, and
  `Runtime`. Keep `Managed` path-based.
- Define provenance labels for JSON and human output: `system`, `external`,
  `managed`, `owned`.
- Add an ownership lookup abstraction so tests can inject a fake manifest.
- Treat `DOTNET_ROOT` as discovery input, never as an install destination.
- Do not bump `schemaVersion` for additive fields unless a breaking change is
  introduced.

Acceptance:

- An SDK in the managed root without manifest ownership is shown as `managed`,
  not `owned`.
- Removing ownership metadata never changes discovery.
- Existing JSON consumers still parse the payload.

### Stage B — resolver and environment

New files: suggested `internal/dotnet/resolver.go` and
`internal/dotnet/environment.go` (or a small `internal/toolchain` package if the
dependency direction stays clean).

Tasks:

- Define `Selection` with root, dotnet path, SDK version, source, ownership, and
  a short rationale.
- Resolve in this order:
  1. Workspace `global.json` requirement.
  2. Managed newest.
  3. System newest.
  4. No match: return a typed "missing SDK" result, not a generic error.
- When `global.json` is present, choose a candidate root and run
  `<root>/dotnet --version` with the workspace as `Dir`. Use the actual
  selection. If it fails, try the other root and preserve both errors.
- Prefer managed when both roots can satisfy the same request.
- Build an environment helper that returns an explicit environment:
  `DOTNET_ROOT=<selected root>`, `DOTNET_MULTILEVEL_LOOKUP=0`,
  `DOTNET_CLI_TELEMETRY_OPTOUT=1`, `DOTNET_NOLOGO=1`,
  `DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1`.
- Never consult ambient `dotnet` resolution after a selection is made.

Acceptance:

- System-only SDK is selected and usable without installing anything.
- Managed SDK wins when both could satisfy a request.
- A missing `global.json` SDK reports the exact version and source attempts.
- Cancellation is propagated.

### Stage C — surface selection

Files: `internal/cli/`, `internal/tui/`, `internal/diagnostics/`, tests.

Tasks:

- `hilt sdk list` shows source and ownership; mark the selected SDK.
- Add `hilt sdk which` (or `current`) reporting selected root, SDK, source, and
  rationale. Add `--json`.
- `hilt doctor` checks both roots for `global.json` requirements, shows the
  selected toolchain, and reports cross-root duplicates as information rather
  than an error.
- TUI SDK screen shows ownership and selection.
- Keep warnings on stderr in human mode; JSON remains clean.

Acceptance:

- `hilt sdk which` is deterministic with both roots present.
- Doctor exits `0` when a system SDK satisfies `global.json`.
- Doctor exits `1` only when no root satisfies the requirement.
- No process runs in the TUI update loop.

Checkpoint: after Stage C, demonstrate selection against the real system SDK and
fake managed roots. The installer has not started yet.

### Stage D — managed install

New package: suggested `internal/install`. Add manifest storage and network
metadata handling.

Tasks:

- Choose and document the manifest location. Recommendation: a sibling of the
  toolchain root, under the Hilt data directory, so the .NET root stays clean.
- Use official Microsoft release metadata and harvest/archive URLs.
- Resolve exact versions before download. Verify SHA-512 or published
  checksums/signatures where available.
- Download to a temporary file with cancellation, size limits, HTTPS, and clear
  errors. Never pipe a download into a shell.
- Extract into a staging directory inside the managed root. Verify with the new
  `dotnet --info`/`--list-sdks` using the explicit environment.
- Publish atomically, then write the manifest entry (version, RID, source URL,
  checksum, timestamp, ownership).
- Make installs idempotent and guard concurrent installs with a lock.
- Clean partial downloads and staging directories on failure.
- If the same version exists system-wide, still install a private copy as
  approved, but say so.

Acceptance:

- Test server only; no real network in the regular suite.
- A checksum mismatch leaves the managed root unchanged.
- Cancellation leaves no published toolchain.
- Installed `dotnet` resolves from the managed root with multilevel lookup off.

### Stage E — removal and updates

Tasks:

- `hilt sdk remove <version>` only for owned entries.
- Refuse system, external, and unowned-managed versions with specific messages.
- Remove the manifest entry only after successful removal; reconcile stale
  entries when the directory is already gone.
- Report managed update availability explicitly. Do not auto-update.
- Never run package-manager commands in Phase 2.

Acceptance:

- Removing a system-only or external SDK is impossible through Hilt.
- A manually copied SDK under the managed root is visible but not removable.
- A stale manifest entry is handled without deleting unrelated directories.

### Stage F — verification and documentation

Tasks:

- Add unit and integration tests for all stages.
- Keep the real-SDK test opt-in.
- Update `README.md`, `docs/safety.md`, `docs/architecture.md`, and
  `docs/roadmap.md`.
- Update this handoff or the phase plan if behavior diverges.
- Run the full verification set and record actual results.

## Required test scenarios

- System root only; no managed root.
- Managed root only.
- Both roots with the same version.
- Both roots with different versions.
- `global.json` exact match in system.
- `global.json` exact match in managed.
- `global.json` requiring roll-forward.
- Missing `global.json` SDK.
- System root made read-only; install still targets managed.
- External `~/.dotnet` root; never removable.
- Managed root with unowned contents; never removable.
- Install cancellation, checksum failure, malformed metadata, and partial
  staging cleanup.
- No write occurs outside the managed root.
- No `PATH` mutation.
- Environment always includes the explicit `DOTNET_ROOT` and disables
  multilevel lookup.

## Guardrails

- No `sudo`, no package-manager writes, no `PATH` changes in Phase 2.
- No shell interpolation; process arguments are slices.
- Every external command takes a `context.Context`.
- No network in ordinary tests.
- Never delete a system or external SDK.
- Never claim an install succeeded without verifying the installed host.
- Show the exact version and destination before a change.
- Keep JSON output stable and additive.

## Start here next session

1. Read this file, `plan/04-dotnet-discovery.md`,
   `plan/06-diagnostics.md`, `plan/08-cli.md`, and `docs/safety.md`.
2. Confirm the worktree is clean and `./scripts/verify.sh` passes.
3. Implement Stage A, then commit.
4. Implement Stage B, then commit.
5. Implement Stage C, then commit and demonstrate selection.
6. Continue with Stage D only after the selection behavior is verified.
7. Use small commits with conventional prefixes. Do not claim untested
   behavior.

Suggested commit sequence:

```text
feat(dotnet): track toolchain ownership separately from managed roots
feat(dotnet): resolve managed and system toolchains
feat(cli): show selected toolchain and source
feat(install): add manifest-backed managed installs
feat(cli): add managed sdk install and remove
docs: document hybrid toolchain behavior
```

## Deferred

- System-wide installation through pacman or other package managers.
- Per-project private toolchains.
- Automatic `PATH` shims or shell startup edits.
- Machine-wide or multi-user installs.
- Channels, aliases, and automatic updates.
- Project execution, NuGet, and templates (later phases).

## References

- `plan.md` — product specification, especially sections 5, 12, 13, 14, 17, 18.
- `plan/01-bootstrap.md` through `plan/09-testing-and-release.md` — Phase 1.
- `docs/safety.md` — installation and version-management policy.
- `docs/architecture.md` — package boundaries.
- `docs/roadmap.md` — phases 2–6.
