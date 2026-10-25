# Step 8 — CLI

## Objective

Provide a reliable non-interactive command surface next to the TUI. Commands
must not depend on starting the full-screen UI, and must behave predictably
when no terminal is attached.

## Package

`internal/cli`

## Commands (Phase 1)

```text
hilt                      # TUI when attached to a TTY; usage error otherwise
hilt --help
hilt --version
hilt version [--json]
hilt doctor [--json]
hilt sdk list [--json]
hilt runtime list [--json]
```

Commands for later phases are not registered until they work.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success; doctor found no errors |
| 1 | operation failed; doctor found errors |
| 2 | usage error, unknown command, missing TTY for TUI |
| 130 | interrupted/cancelled |

Warnings do not change the exit code; `--json` exposes them.

## Output

- Human output is plain text with aligned columns, colored only when stdout is
  a TTY and `NO_COLOR` is unset.
- JSON output is stable and versioned: `{"schemaVersion":1,...}`.
- Inventory JSON includes hosts, SDKs, runtimes, warnings, and `generatedAt`.
- Doctor JSON includes findings, severity, remediation, and summary counts.
- Warnings from discovery are written to stderr in human mode so piping stdout
  remains clean.

## Rules

- No global mutable state; the backend is injected.
- No network access.
- No shell strings.
- Set `SilenceErrors` and `SilenceUsage`; the executor owns error rendering.
- `hilt` with no arguments and no TTY returns usage error 2 rather than
  hanging.

## Tests

- Version and help commands.
- `sdk list --json` against a fake backend produces valid JSON.
- Empty inventory is a successful command.
- Doctor exit code is 0 for warnings and 1 for errors.
- Unknown command and bad flags return 2.
- Non-interactive `hilt` returns 2 without launching the TUI.
