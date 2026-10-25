# Hilt — Phase 1 Execution Plan

This directory is the detailed, step-by-step implementation plan for Hilt.
`../plan.md` remains the product specification. This directory describes how
Phase 1 is built, tested, and verified.

## Phase 1 goal

Ship a standalone Go executable that:

- starts a full-screen TUI without a .NET runtime or SDK installed;
- discovers .NET hosts, SDKs, and runtimes from PATH, `DOTNET_ROOT`, known
  roots, and the filesystem;
- reports platform, configuration, and .NET health through `hilt doctor`;
- exposes non-interactive `hilt sdk list`, `hilt runtime list`, and
  `hilt version` commands with JSON output;
- stores configuration under XDG directories;
- keeps all system operations cancellable and testable.

## Non-goals for Phase 1

- No installation, removal, or modification of SDKs (Phase 2).
- No project scanning, build, run, or test execution (Phase 3).
- No NuGet operations (Phase 4).
- No project templates (Phase 5).
- No Windows or macOS support guarantees (abstractions only).
- No `sudo`, no network downloads, no PATH mutation.

## Step order and dependency graph

| Step | Document | Depends on |
| --- | --- | --- |
| 1 | [01-bootstrap.md](01-bootstrap.md) | — |
| 2 | [02-platform.md](02-platform.md) | 01 |
| 3 | [03-process.md](03-process.md) | 01 |
| 4 | [04-dotnet-discovery.md](04-dotnet-discovery.md) | 02, 03 |
| 5 | [05-config.md](05-config.md) | 02 |
| 6 | [06-diagnostics.md](06-diagnostics.md) | 02, 04, 05 |
| 7 | [07-tui.md](07-tui.md) | 02, 04, 05, 06 |
| 8 | [08-cli.md](08-cli.md) | 04, 06, 07 |
| 9 | [09-testing-and-release.md](09-testing-and-release.md) | all |

## Universal conventions

- Module path: `github.com/mohammedbakri123/hilt`.
- Go directive: `1.26.0`; development environment is Go 1.27.1.
- Charm v2 only:
  - `charm.land/bubbletea/v2 v2.1.0`
  - `charm.land/bubbles/v2 v2.2.1`
  - `charm.land/lipgloss/v2 v2.0.6`
- CLI: `github.com/spf13/cobra v1.10.2`.
- `View()` returns `tea.View`; alternate screen is declared on the view.
- Key handling uses `tea.KeyPressMsg`.
- Process execution never uses a shell; arguments are slices.
- Every external command takes a `context.Context`.
- Errors carry context (`fmt.Errorf("...: %w", err)`).
- No global mutable state. Styles and constants may be package-level values.
- Exit codes: `0` success, `1` operational failure or doctor errors, `2` usage
  error, `130` cancellation via interrupt.

## Definition of done

- `go build ./...` succeeds.
- `go vet ./...` succeeds.
- `go test -race ./...` succeeds without network or administrator access.
- `hilt` starts from a TTY and renders the dashboard.
- `hilt` renders correctly when `PATH` contains no `dotnet` executable.
- `hilt doctor`, `hilt doctor --json`, `hilt sdk list --json`, and
  `hilt runtime list` produce useful output and documented exit codes.
- README and this plan match actual behavior.
- No feature is reported as working unless it was exercised.
