# Hilt ⚡

[![Go Report Card](https://goreportcard.com/badge/github.com/mohammedbakri123/hilt)](https://goreportcard.com/report/github.com/mohammedbakri123/hilt)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Hilt** is a fast, terminal-first management dashboard and CLI for the .NET development ecosystem on Linux. 

Built in pure Go with [Charm](https://charm.sh) (Bubble Tea, Lip Gloss, Bubbles) and [Cobra](https://github.com/spf13/cobra), Hilt is distributed as a **single, self-contained executable** that starts immediately and diagnoses your environment **even when no .NET SDK or runtime is installed**.

---

## Key Highlights

- **Zero-.NET Dependency:** Inspects system installations, diagnoses missing dependencies, and bootstraps .NET SDKs and runtimes from scratch without needing .NET pre-installed.
- **Side-by-Side Managed Toolchains:** Download and install official Microsoft .NET SDKs and runtimes into an isolated, user-owned directory (`~/.hilt/data/dotnet`) without administrative privileges or modifying system files.
- **Non-Destructive & Safe by Default:** System packages and external installations are discovered with read-wide access, but Hilt never touches `sudo`, package managers, or `PATH` automatically.
- **Project Explorer & Details:** A focused solution/project picker opens a dedicated detail page for target frameworks, references, packages, files, EF Core, and build output.
- **Interactive Streaming Build:** Build projects directly inside the TUI (`b`) with streamed stdout/stderr output into an expandable, scrollable output drawer (`o`).
- **Comprehensive CLI & Automation:** Every management action is available through consistent CLI commands with `--json` support for seamless shell scripting and CI/CD.

---

## Screenshots & Interface

```text
 HILT v0.1.0 / Projects & Solutions                 linux-x64 · Arch Linux · .NET SDK 10.0.100
─────────────────────────────────────────────────────────────────────────────────────────────
 WORKSPACE            WORKSPACES
   1 Home               Shop                                  [sln] /home/user/src/Shop.sln
 ▌ 2 Projects
   3 SDKs             PROJECTS & SOLUTIONS                         [2 projects]
   4 Diagnostics        Shop.sln [sln] /home/user/src/Shop.sln
   5 Settings             > Shop       net10.0, net8.0  src/Shop/Shop.csproj
                          Shop.Tests   net10.0          tests/Shop.Tests/Shop.Tests.csproj

                      Press / or f to browse folders, then enter or double-click to open.
                      Detail page: Overview · Packages · Files · EF · Output
─────────────────────────────────────────────────────────────────────────────────────────────
 [ok] Active project: Shop.
 enter open  / find  p root  a add  e fold  n new  x delete  ? help  q quit
```

---

## Installation & Requirements

### Requirements
- **OS:** Linux (x86_64 / amd64 or aarch64 / arm64)
- **Go:** 1.24+ (only required to compile from source)
- **.NET:** Optional (Hilt can install it for you!)

### Build from Source
```bash
git clone https://github.com/mohammedbakri123/hilt.git
cd hilt
make build
```
The compiled executable will be placed in `bin/hilt`.

To install to your `GOPATH` or local bin:
```bash
go install ./cmd/hilt
```

---

## Quickstart

### Launch the Interactive TUI
```bash
hilt
```
> **Note:** The TUI requires an interactive terminal (TTY). Running `hilt` in non-interactive environments exits cleanly with code `2`.

### Verify & Diagnose Host Environment
```bash
hilt doctor
```
Checks OS detection, architecture, Git availability, PATH resolution, active .NET hosts, and `global.json` compatibility.

---

## CLI Reference

### 1. SDK Management
```bash
# List all detected SDKs (both system-installed and Hilt-managed)
hilt sdk list

# Show which SDK is actively resolved for current directory
hilt sdk which

# Discover downloadable SDK versions from Microsoft release metadata
hilt sdk available
hilt sdk available --all --prerelease

# Install a managed SDK into user isolation
hilt sdk install 10.0.100
hilt sdk install latest

# Check for updates within the same major/minor release channel
hilt sdk outdated

# Upgrade an existing managed SDK
hilt sdk update 10.0.100 --remove-old

# Remove a Hilt-managed SDK (manifest-guarded)
hilt sdk remove 10.0.100
```

### 2. Runtime Management
```bash
# List installed runtimes across all hosts
hilt runtime list

# Install a managed runtime flavor (all, netcore, or aspnetcore)
hilt runtime install 10.0.1 --framework aspnetcore

# Check for runtime updates
hilt runtime outdated

# Remove a managed runtime
hilt runtime remove 10.0.1
```

### 3. Project Discovery & Inspection
```bash
# Scan a directory for solutions and projects
hilt projects list
hilt projects list --path ~/Projects --all

# Inspect project declared metadata (references, frameworks, test status)
hilt projects inspect path/to/App.csproj
hilt projects inspect path/to/App.sln --json
```

### 4. JSON Output for Automation
Every query command supports `--json`:
```bash
hilt doctor --json
hilt sdk list --json
hilt projects list --json
```

---

## TUI Keyboard Shortcuts

| Key | Context | Action |
| --- | --- | --- |
| `1` – `5` | Global | Jump directly to screen (Home, Projects, SDKs, Doctor, Settings) |
| `]` / `[` | Global | Cycle next / previous screen |
| `o` | Global | Toggle build & command output drawer |
| `r` | Global | Rescan / refresh inventory & active screen data |
| `?` | Global | Toggle keyboard shortcut reference modal |
| `q` / `ctrl+c`| Global | Quit Hilt (prompts if a background build is running) |
| `b` | Projects | Build selected project with live streaming output |
| `p` | Projects | Open in-place scan root folder picker |
| `a` | Projects | Add and pin a project, solution, or directory from any path |
| `u` | Projects | Remove the selected imported source (files are never deleted) |
| `e` | Projects | Expand or collapse the selected solution in the explorer |
| `t` | Projects | Inspect the selected project's source tree |
| `m` | Projects | Open project actions (build, clean, restore, test, run) |
| `n` | Projects | Create a project from a Hilt or `dotnet new` preset |
| `x` | Projects | Delete the selected `.csproj` file after confirmation (source preserved) |
| `/` / `f` | Projects | Open file-explorer dialog for folders, projects, and solutions |
| `,` / `.` | Projects | Switch active project detail tabs |
| `tab` | Projects / SDKs| Toggle pane focus / switch between SDKs and Runtimes |
| Mouse | Projects | Click navigation, explorer rows, detail tabs, and double-click to open |
| `i` | SDKs / Home | Install managed SDK or runtime |
| `u` | SDKs | Check for updates / apply update to selected item |
| `x` | SDKs | Remove selected managed version (guarded with confirmation) |

---

## Directory Layout & Storage

Hilt organizes user data inside a clean, isolated root (`~/hilt` or custom `$HILT_HOME`):

```text
~/hilt/
├── apps/                 Default project workspace root
└── .hilt/
    ├── config.json       User configuration (read-only in UI, editable in JSON)
    ├── state.json        Active project, recents, and pinned targets
    ├── cache/            Disposable Microsoft release metadata caches
    └── data/
        ├── installs.json Managed installation manifest
        ├── installs.lock Mutex lock file protecting concurrent operations
        └── dotnet/       Per-version isolated .NET toolchains
```

- **Atomic Writes:** Configuration and state files use atomic rename with `0600` permissions.
- **Corruption Resilience:** Corrupt JSON files are backed up to `.corrupt-<timestamp>` and safely regenerated with defaults.

---

## Roadmap

| Phase | Milestone | Status |
| --- | --- | :---: |
| **Phase 1** | Standalone foundation, platform detection, `hilt doctor`, TUI shell | ✅ Complete |
| **Phase 2** | Managed SDK/runtime install, verification, updates, manifests | ✅ Complete |
| **Phase 3** | Solution/project explorer, import, templates, actions, tree inspection, streaming execution | ✅ Complete |
| **Phase 4** | Interactive NuGet package manager (search, add, update, audit) | 🚧 In Progress |
| **Phase 5** | Advanced project templating and custom blueprint discovery | 🚧 In Progress |
| **Phase 6** | Binary distribution (x64/ARM64 packages, CI/CD, shell completions) | 📋 Planned |

See [docs/roadmap.md](docs/roadmap.md) for detailed progress.

---

## Contributing & Testing

Run the full verification suite (tests, vet, format check, and build):
```bash
./scripts/verify.sh
```

Run unit tests directly:
```bash
go test -count=1 ./...
```

Run integration test against real host .NET (if installed):
```bash
HILT_TEST_REAL_DOTNET=1 go test ./internal/dotnet -run TestRealDotnetInventory -v
```

---

## Documentation

- [Architecture & Design](docs/architecture.md) — Package boundaries, dependency flow, and components.
- [Safety & Isolation Policy](docs/safety.md) — Security rules, elevation guarantees, and mutation boundaries.
- [Project Roadmap](docs/roadmap.md) — Detailed phase delivery schedule.

---

## License

[MIT](LICENSE) © Mohammed Bakri
