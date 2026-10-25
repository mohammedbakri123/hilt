# Step 7 — TUI

## Objective

Deliver a polished, keyboard-first full-screen interface that starts without
.NET installed. Phase 1 implements Dashboard, SDKs, Diagnostics, and Settings;
the remaining sections are honest placeholders.

## Package

`internal/tui`

## Construction

- `Now(Backend) Model` creates the root model.
- `Run(Backend) error` starts `tea.NewProgram` and returns the run error.
- The `Backend` interface is defined in the TUI and implemented by
  `internal/app.App`. It exposes only shared domain types, never `*app.App`.

## Screens

| Key | Screen | Phase 1 content |
| --- | --- | --- |
| 1 | Dashboard | host, .NET status, counts, resolved tool, git, package manager |
| 2 | SDKs & Runtimes | hosts, SDKs, runtimes, styles, provenance |
| 3 | Projects | placeholder, Phase 3 |
| 4 | NuGet | placeholder, Phase 4 |
| 5 | Create | placeholder, Phase 5 |
| 6 | Templates | placeholder, Phase 5 |
| 7 | Diagnostics | findings, severity, remediation |
| 8 | Settings | resolved paths and configuration |

## Interaction

- `↑/↓` or `j/k`: move section.
- `1`–`8`: jump.
- `tab` / `shift+tab`: cycle.
- `r`: refresh inventory; re-run diagnostics on the diagnostics screen.
- `d`: open diagnostics.
- `pgup`/`pgdown` or `ctrl+u`/`ctrl+d`: scroll long content.
- `?`: help overlay; `esc` closes it.
- `q` or `ctrl+c`: quit.
- Terminal resize is honored; no mouse required.

## Async model

- Reads happen in `tea.Cmd` with a timeout context.
- Messages: `inventoryMsg{Inv, Err}`, `doctorMsg{Report, Err}`.
- The update loop never runs a process or blocks on I/O.
- A spinner is shown while discovery runs.
- Errors are displayed in the active screen, not swallowed.

## Rendering

- Bubble Tea v2: `View() tea.View`, `tea.NewView`, `v.AltScreen = true`.
- Lip Gloss styles are package-level immutable values.
- Layout: header, sidebar, main pane, footer.
- Content is clipped to the available height without ANSI corruption.
- No color assumptions about the terminal background.
- If the terminal is too small, show a compact layout rather than panicking.

## Tests

- Fake backend with injectable inventory and report.
- Initial view includes “Dashboard”, version, and platform.
- No-.NET inventory shows “not detected” and does not crash.
- Key navigation changes screens; digits jump; `q` quits; `?` opens help.
- Inventory and doctor messages populate views.
- Long diagnostic output scrolls and clamps at bounds.
- View is renderable at several widths/heights (40×12, 80×24, 160×48).
