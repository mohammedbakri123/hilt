# Step 9 — Testing, verification, and repository hygiene

## Objective

Prove Phase 1 works on a machine with .NET and remains functional on a machine
without it. No fabricated results.

## Test layers

1. **Unit tests** — parsers, paths, version comparison, config, diagnostics.
2. **Process integration tests** — temporary shell scripts, cancellation,
   environment handling.
3. **Discovery integration** — temporary .NET-like directory trees and fake
   probers; optional real-SDK test guarded by `HILT_TEST_REAL_DOTNET=1`.
4. **TUI tests** — direct `Update`/`View` assertions at fixed sizes.
5. **CLI tests** — fake backend, JSON decoding, exit codes.

## Required commands

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build -trimpath -o bin/hilt ./cmd/hilt
```

## Manual verification

1. `hilt doctor` on the development machine; confirm the real SDK/runtime.
2. `hilt doctor --json | jq`; confirm schema and severities.
3. `hilt sdk list`; confirm `10.0.112` and a system-managed classification.
4. `hilt runtime list`; confirm `Microsoft.NETCore.App` and
   `Microsoft.AspNetCore.App`.
5. Simulate no .NET:
   ```bash
   env -i HOME="$HOME" PATH=/usr/bin:/bin DOTNET_ROOT= ./bin/hilt doctor
   ```
   Expect a warning, exit 0. Run the TUI from a TTY with a PATH that excludes
   `dotnet`; expect a functional dashboard.
6. Corrupt the config and confirm the backup plus warning.
7. Resize the terminal and navigate every Phase 1 screen.
8. `printf '' | ./bin/hilt` returns 2 and does not hang.

## Artifacts and fixtures

- `tests/testdata/` holds captured .NET and pacman output.
- Fixtures are sanitized; no secrets or user-specific paths beyond `/usr/share`.
- Fixtures are static; tests never call the network.

## Documentation

- README: purpose, requirements, build, run, commands, phase status, safety.
- `docs/safety.md`: version management and installation safety policy.
- `docs/architecture.md`: package boundaries and dependency direction.
- `docs/roadmap.md`: phases 2–6, explicitly unimplemented.
- Plan documents updated if behavior diverges.

## Repository hygiene

- `git init` with `origin` at `github.com/mohammedbakri123/hilt`.
- No generated binaries committed.
- No secrets.
- Commit only when the user asks.

## Acceptance

- All required commands pass.
- Manual checks are recorded in the final report with actual output.
- Phase 2 is identified as the next step, with no Phase 2 features claimed.
