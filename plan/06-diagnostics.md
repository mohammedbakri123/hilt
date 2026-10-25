# Step 6 — Diagnostics (`hilt doctor` engine)

## Objective

Produce honest, structured findings about the environment and .NET health.
Distinguish facts from assumptions and offer remediation without claiming
fixes that were not performed.

## Package

`internal/diagnostics`

## API

```go
type Severity string // ok, info, warning, error

type Finding struct {
    ID, Title string
    Severity  Severity
    Summary   string
    Detail    string
    Remediation string
    AutoFixable bool
}

type Summary struct{ OK, Info, Warnings, Errors int }

type Report struct {
    GeneratedAt time.Time
    HiltVersion string
    Platform    platform.Info
    Findings    []Finding
    Summary     Summary
}

func Run(context.Context, Deps) Report
```

## Checks

| ID | Produces |
| --- | --- |
| `platform` | OS, architecture, distro |
| `config` | Load warnings, config path, managed root |
| `dotnet-path` | Resolved `dotnet`, or warning |
| `dotnet-inventory` | Host/SDK/runtime counts, probe failures |
| `global-json` | Nearest file, required version, availability |
| `package-manager` | Detected manager and privilege tool |
| `git` | Git availability |
| `managed-root` | Path and whether it exists |

## Rules

- Findings are ordered deterministically.
- Missing .NET is a warning, not an error: Hilt's purpose is to work before
  .NET exists.
- A `global.json` that selects an unavailable SDK is an error.
- Warnings never make `doctor` exit non-zero; errors do. `--json` carries all
  severities so scripts can choose stricter behavior.
- Remediation text must not reference unimplemented commands. In Phase 1 the
  install hint says installation is planned for Phase 2.
- `AutoFixable` is false for every Phase 1 finding.

## Tests

- Healthy inventory → no errors.
- Empty inventory → warning, exit 0.
- Missing SDK required by `global.json` → error, exit 1.
- Probe failure is surfaced as a finding.
- Config warnings are included.
- Summary counts match findings.

## Acceptance

`hilt doctor` on this machine reports the SDK and runtimes and exits 0. With a
`global.json` requesting an unavailable SDK, it exits 1 and names the exact
version.
