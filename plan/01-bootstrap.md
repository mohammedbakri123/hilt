# Step 1 — Bootstrap

## Objective

Create a buildable Go module with pinned dependencies, version injection, and
repository hygiene. No product behavior yet.

## Files

```text
go.mod
go.sum
cmd/hilt/main.go
Makefile
.gitignore
.editorconfig
LICENSE
README.md
```

## Tasks

1. `git init -b main`; add origin `github.com/mohammedbakri123/hilt`.
2. `go mod init github.com/mohammedbakri123/hilt`.
3. Add dependencies:
   ```bash
   go get charm.land/bubbletea/v2@v2.1.0
   go get charm.land/bubbles/v2@v2.2.1
   go get charm.land/lipgloss/v2@v2.0.6
   go get github.com/spf13/cobra@v1.10.2
   go get golang.org/x/term
   ```
4. `cmd/hilt/main.go` declares `version`, `commit`, and `date` link-time
   variables and passes them to `cli.Execute`. It must not import TUI code
   directly beyond the CLI package.
5. `Makefile` targets:
   - `build` → `bin/hilt` with `-trimpath` and ldflags.
   - `test` → `go test ./...`.
   - `race` → `go test -race ./...`.
   - `lint` → `go vet ./...`.
   - `fmt` → `gofmt -w` over all Go files.
   - `run` → `go run ./cmd/hilt`.
6. `.gitignore`: `bin/`, `dist/`, `*.test`, `coverage.out`.
7. MIT license.

## Verification

- `go build ./...` exits 0.
- `go vet ./...` exits 0.
- `bin/hilt version` reports the injected version.

## Notes

- Charm v2 uses vanity import paths (`charm.land/...`). Do not copy v1 examples
  from `github.com/charmbracelet/...`; the API differs (`View() tea.View`,
  `tea.KeyPressMsg`, declarative alternate screen).
- The Go directive is `1.26.0` because Bubble Tea v2 requires it. The local
  toolchain is newer and builds it fine.
