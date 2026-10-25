# Hilt — A Standalone TUI for Managing the .NET Ecosystem

## 1. Mission

Build **Hilt**, a standalone, terminal-first application for managing the .NET development ecosystem.

Hilt combines the useful workflows of a .NET SDK manager, project explorer, build runner, NuGet package manager, and project scaffolder into one cohesive application.

The user launches it with:

```bash
hilt
```

The command opens a full-screen, keyboard-driven TUI inspired by applications such as lazygit and lazydotnet.

Hilt must also support non-interactive CLI commands so it can be used in scripts, shell workflows, and automation.

### The defining requirement

**Hilt must work even when .NET is not installed on the user's machine.**

The application itself must not depend on the .NET runtime. It must be distributed as a standalone Go executable that can detect, install, and manage .NET installations.

Its initial target is Linux, particularly Arch-based distributions such as CachyOS. Keep the design extensible enough to support other Linux distributions, Windows, and macOS later.

Hilt is a .NET management application, not merely a project scaffolder and not an IDE replacement.

It does not need to implement a C# language server, debugger, compiler, MSBuild, or NuGet itself. It should orchestrate official .NET tooling and existing development utilities wherever practical.

## 2. Technology Stack

Use the following stack.

### Language

**Go**

Use a current stable Go release supported by the development environment.

Prefer the standard library for filesystem operations, process management, HTTP requests, archive handling, JSON, concurrency, and platform detection. Introduce external dependencies only when they provide meaningful functionality.

### TUI

Use the Charmbracelet ecosystem:

- **Bubble Tea** for application state, events, keyboard handling, and the TUI lifecycle.
- **Bubbles** for reusable UI elements such as lists, tables, inputs, progress indicators, spinners, and viewports.
- **Lip Gloss** for styling, layout, borders, colors, and visual hierarchy.

Check the current stable versions and corresponding documentation before implementation. These libraries have versioned APIs, so do not mix examples from incompatible major versions.

The interface should be polished, keyboard-first, responsive to terminal resizing, and usable entirely without a mouse.

### CLI

Use Cobra or another well-maintained Go CLI framework if it provides a clear benefit.

The CLI must support nested commands, flags, help text, predictable exit codes, and non-interactive execution.

### Configuration and state

Use JSON configuration initially. Prefer Go's standard library rather than adding a configuration framework without a specific need.

Follow the XDG Base Directory Specification on Linux:

- Configuration under the user's configuration directory.
- Persistent state under the user's state directory.
- Download caches and temporary artifacts under an appropriate cache directory.

Use platform-appropriate locations on other operating systems.

### Testing

Use Go's built-in testing package.

Use temporary directories, fake executables, dependency injection, and controlled HTTP test servers for tests involving the filesystem, processes, package managers, and downloads.

The application must be buildable without having .NET installed.

## 3. Product Principles

Follow these principles throughout the implementation.

1. **Standalone:** launching Hilt does not require a .NET runtime or SDK.
2. **Safe by default:** no silent elevation, destructive system operations, or implicit overwrites.
3. **Respect existing installations:** distinguish system-managed installations from Hilt-managed installations.
4. **Use official tooling:** delegate to the .NET CLI, MSBuild, NuGet, and Git where appropriate.
5. **TUI-first, CLI-capable:** every important operation should be accessible interactively and, where sensible, through commands.
6. **Linux-first:** make CachyOS/Arch a first-class initial platform without hardcoding Linux assumptions into shared logic.
7. **Honest diagnostics:** report detected facts, failures, and uncertainties rather than guessing.
8. **Incremental delivery:** each implementation phase must produce a buildable, tested result.

## 4. Main Dashboard

When the user runs `hilt`, display a dashboard containing:

- Hilt version.
- Operating system and CPU architecture.
- Whether .NET is installed.
- Detected SDK and runtime counts.
- The currently resolved .NET executable, if available.
- Git availability.
- Recent projects, if any.
- Relevant environment warnings.

Provide these main sections:

1. Dashboard
2. SDKs and Runtimes
3. Projects and Solutions
4. NuGet Packages
5. Create Project
6. Templates and Architectures
7. Diagnostics
8. Settings
9. Exit

Use contextual screens rather than displaying everything simultaneously.

Support keyboard navigation, contextual shortcuts, searchable lists, confirmation dialogs, progress indicators, and scrollable command output.

Display operation status clearly: pending, running, succeeded, failed, or cancelled.

If .NET is missing, the dashboard must remain functional and offer an SDK installation workflow.

## 5. .NET SDK and Runtime Management

This is a core component and should be implemented before advanced project management.

### Discovery

Detect installed .NET SDKs and runtimes without assuming that the first `dotnet` found on PATH represents every installation.

Use `dotnet --list-sdks` and `dotnet --list-runtimes` where possible. Supplement these with filesystem discovery and known installation locations.

Display:

- Version.
- Installation path.
- SDK or runtime type.
- Architecture where known.
- Installation source when identifiable.
- Management status: system-managed, Hilt-managed, or unknown.
- Whether the installation is accessible and appears usable.

Keep SDKs, shared runtimes, and ASP.NET Core runtimes distinct.

Detection must work even when no `dotnet` command exists.

### Installation methods

Support two installation paths.

**A. Operating-system package manager**

Begin with CachyOS and Arch Linux.

Detect the distribution and available package manager. Verify actual package availability and naming using the detected system's package metadata or other reliable information before offering installation.

When supported, allow Hilt to install .NET through the OS package manager.

Make system-wide installation an explicit user choice. Explain the package manager command and privilege requirements before proceeding.

Never invoke `sudo` or equivalent silently. Allow the native privilege prompt to occur only after the user explicitly approves the operation.

Add adapters for other package managers later, such as APT, DNF, or Zypper. Do not assume every distribution packages .NET in the same way.

**B. Official Microsoft installation**

Support installing SDKs and runtimes into a Hilt-managed, user-owned directory.

Use official Microsoft distribution metadata and installation mechanisms. The official `dotnet-install.sh` mechanism is one possible option for non-admin Linux installation.

Do not pipe network downloads directly into a shell. Download scripts or archives to temporary files, use HTTPS, validate expected sources, check published integrity information when available, and display what will be executed.

Do not assume that every .NET release, architecture, or runtime is available from every distribution endpoint.

Allow the user to choose the installation version and destination.

Show download progress, installation output, verification results, and useful errors. Clean up incomplete temporary files safely.

### Side-by-side version management

Support installing multiple SDK versions without replacing one another.

Provide operations to:

- List installed SDKs.
- List installed runtimes.
- Inspect installation details.
- Install a specific version.
- Install a release channel when it can be resolved unambiguously.
- Verify a new installation.
- Remove Hilt-managed versions.
- Diagnose inaccessible or conflicting installations.

Never silently delete or overwrite system-managed installations.

Resolve version aliases to explicit version numbers before carrying out installation or removal operations. Show the actual version to the user before proceeding.

### SDK selection and `global.json`

Explain and support the .NET SDK selection mechanism used by the CLI.

Provide operations for:

- Inspecting a project's current SDK selection.
- Detecting a `global.json`.
- Creating or updating `global.json` with a selected SDK version.
- Detecting missing SDK requirements.
- Recommending an installation when a project selects an unavailable SDK.
- Explaining roll-forward and prerelease behavior when relevant.

Do not confuse selecting an SDK with changing a project's target framework or runtime.

Do not silently modify PATH or shell startup files. Offer explicit configuration instructions or a reversible integration mechanism.

Hilt-managed installations should be invokable through explicit paths or a controlled process environment without unexpectedly overriding unrelated system installations.

## 6. Project and Solution Explorer

Create an interactive project explorer inspired by lazygit and lazydotnet.

### Discovery

Allow users to open the current directory, choose another directory, or pass a project or solution path to Hilt.

Recognize these common files where supported:

- `.sln`
- `.slnx`
- `.slnf`
- `.csproj`

Discover related projects and solution references.

Avoid scanning the entire filesystem by default. Exclude build outputs and version-control internals, avoid following symlinks into loops, and make large scans cancellable.

### Project details

Display:

- Project name and path.
- Project type.
- Target framework or frameworks.
- SDK requirements.
- Project references.
- Direct package references.
- Test-project status when it can be determined.
- Relevant configuration files.

Parse project metadata using XML parsing and existing .NET tools where appropriate.

MSBuild project files can contain conditions, imported properties, and generated values. Do not pretend simple XML parsing resolves all evaluated properties. Use the actual build tooling when evaluated information is needed, and report unresolved details accurately.

### Project operations

Support:

- Build project.
- Build solution.
- Clean.
- Restore.
- Run.
- Stop running project.
- Run tests.
- Inspect project references.
- Add and remove project references.
- Open a project directory in the system's file manager.
- Open source or project files in a detected editor where possible.

Allow the user to choose a startup project when a solution contains multiple executable projects.

Stream stdout and stderr while processes run. Preserve exit codes and provide useful failure summaries with access to full output.

Handle cancellation without leaving child processes running unintentionally.

## 7. NuGet Package Management

Provide an interactive NuGet management interface for selected projects.

### Package inventory

Display direct package references, including:

- Package name.
- Requested version or version range.
- Resolved version where known.
- Available update information where known.

Support transitive dependency listings when the installed SDK supports the necessary operation.

Make the distinction between direct references and transitive dependencies clear.

### Package operations

Implement:

- Search configured NuGet feeds.
- Add a package.
- Select an explicit package version.
- Update a package.
- Remove a package.
- Restore packages.
- Inspect NuGet source configuration.
- Inspect package versions.
- Display audit or vulnerability information when supported.

Use official NuGet sources and the existing CLI wherever practical.

Detect CLI capabilities and use command forms supported by the selected SDK rather than assuming a single syntax works across every SDK version.

Before changing a project, display the target project, package, and version and ask for confirmation.

After an operation, check its result and report success only when the operation actually succeeds.

Do not require internet access simply to list local project references or display cached information.

Clearly identify which search and update features require network access.

## 8. Build, Run, and Test

Create a unified execution screen.

It should support:

- Building projects and solutions.
- Running projects.
- Running tests.
- Stopping processes.
- Passing application arguments.
- Selecting configuration and target framework where supported.
- Streaming output.
- Navigating logs.
- Viewing failure summaries.
- Re-running failed operations.

Use subprocess execution with structured arguments, not shell-string concatenation.

Set the working directory deliberately, pass a controlled environment, and preserve relevant environment variables where necessary.

Provide cancellation, process termination handling, and cleanup.

If an SDK is missing, show a clear diagnostic and offer the user a suitable installation workflow rather than just displaying an opaque CLI failure.

## 9. Project Creation and Architecture Templates

Project creation is an important later module, but it must not delay the initial SDK-management release.

Use the real .NET template engine for standard projects. Use Hilt-owned templates or generation logic for additional project structures.

### Standard templates

Support common installed templates where available, such as:

- ASP.NET Core Web API.
- ASP.NET Core MVC.
- Blazor.
- Class library.
- Console application.
- Worker service.
- Test project.

Discover installed templates dynamically where possible. Indicate which templates require a particular SDK or package.

### Clean Architecture preset

Generate a configurable solution with four projects:

```text
MyApp/
├── MyApp.sln
├── src/
│   ├── MyApp.Domain/
│   ├── MyApp.Application/
│   ├── MyApp.Infrastructure/
│   └── MyApp.Api/
└── tests/
    ├── MyApp.UnitTests/
    └── MyApp.IntegrationTests/
```

Configure project references to preserve the intended dependency direction.

Create initial folders, configuration files, and tests as selected.

Do not add unwanted frameworks, databases, or architectural patterns without making them configurable.

### Vertical Slice preset

Support feature-based project organization, for example:

```text
Features/
├── Books/
│   ├── Create/
│   ├── GetById/
│   └── List/
└── Authors/
    ├── Create/
    └── List/
```

Optionally scaffold endpoints, request and response types, handlers, validators, and tests.

Allow the user to choose between a single-project Vertical Slice architecture and appropriate multi-project variations.

Allow Clean Architecture and Vertical Slice approaches to coexist when selected.

### Custom solution generation

Eventually allow users to select projects, references, target frameworks, packages, initial folders, and conventions.

Validate project names, destinations, and the reference graph before writing files.

Do not overwrite existing files or directories without explicit approval.

After generation, offer restore, build, and test operations.

A generated project must not be reported as verified unless the relevant checks actually pass.

## 10. CLI Command Surface

Provide a CLI alongside the TUI.

The following is an initial command design. Adjust individual commands where necessary for consistency, and implement them only when functional.

```bash
hilt
hilt --help
hilt doctor

hilt sdk list
hilt sdk install <version>
hilt sdk remove <version>

hilt runtime list
hilt runtime install <version>

hilt project list
hilt project inspect <path>

hilt build <project-or-solution>
hilt run <project>
hilt test <project-or-solution>

hilt package list <project>
hilt package search <query>
hilt package add <name> --project <path>
hilt package remove <name> --project <path>

hilt new <name>
hilt template list
```

Support non-interactive use with predictable exit codes. Add JSON output for inventory and diagnostic commands where useful.

A missing argument in an interactive workflow may open the TUI, but scripts must have reliable, non-interactive behavior when a terminal is unavailable.

Do not make CLI commands depend on starting the full-screen UI.

## 11. Diagnostics

Implement `hilt doctor` in both CLI and TUI forms.

Check:

- OS and architecture detection.
- PATH and executable resolution.
- SDK and runtime discovery.
- Hilt-managed installation health.
- SDK selection through `global.json`.
- Package-manager availability.
- Git availability.
- Known installation dependency issues.
- Relevant build or restore errors when the user asks for diagnosis.

For each finding, include:

- Severity.
- What was detected.
- Why it may matter.
- Suggested remediation.
- Whether a remediation can be performed safely by Hilt.

Do not claim that an issue was fixed unless verification proves that the fix succeeded.

## 12. Configuration and State

Use versioned JSON configuration.

Store settings in platform-appropriate user directories and respect XDG on Linux.

Possible configuration includes:

- Managed SDK installation directory.
- Recent projects.
- Default project directory.
- Scan exclusions.
- Preferred SDK selection behavior.
- UI preferences and keybindings.
- Template sources and trusted origins.
- Package-source preferences.
- Optional editor integration.

Keep configuration separate from cache and persistent state.

Handle invalid configuration safely and provide a way to regenerate defaults without deleting unrelated data.

Hilt should not require a database.

## 13. Security and Safety

Treat SDK installation and package operations as sensitive system changes.

- Never invoke `sudo` or equivalent silently.
- Never remove system-managed SDKs or runtimes automatically.
- Never execute arbitrary template hooks without explaining and obtaining approval.
- Never use shell interpolation with untrusted input.
- Never pipe a network download directly into a shell.
- Validate expected download domains and verify published checksums or signatures when available.
- Show installation paths and package-manager commands before modifying the system.
- Never overwrite project files without explicit confirmation.
- Avoid printing secrets from environment variables or configuration.
- Use temporary files and atomic replacement where appropriate.
- Clean up failed downloads and temporary files safely.
- Distinguish detected facts from assumptions.
- Handle offline operation gracefully.
- Clearly report unsupported distributions and architectures.

The default installation strategy should prioritize isolated user-owned installations. System package-manager installation remains an explicit alternative.

## 14. Architecture and Repository Structure

Use a modular Go codebase without overengineering the initial release.

A reasonable starting point is:

```text
hilt/
├── cmd/
│   └── hilt/
├── internal/
│   ├── app/
│   ├── cli/
│   ├── tui/
│   ├── config/
│   ├── dotnet/
│   ├── install/
│   ├── platform/
│   ├── process/
│   ├── projects/
│   ├── packages/
│   ├── templates/
│   └── diagnostics/
├── tests/
├── docs/
├── scripts/
├── go.mod
├── go.sum
├── Makefile
├── .editorconfig
├── .gitignore
├── README.md
└── LICENSE
```

Change this structure if repository size or dependency direction warrants a simpler layout.

Keep UI rendering separate from system operations. Operational code must be testable independently of the TUI.

Define interfaces around external processes, filesystem access, network downloads, installation strategies, and platform integration when they provide meaningful testability.

Avoid deep abstractions and unnecessary interfaces that have only one trivial implementation and no practical testing benefit.

## 15. Development Phases

Implement the project incrementally.

### Phase 1 — Standalone foundation

- Go module and build setup.
- CLI entry point.
- Bubble Tea application.
- Dashboard and navigation.
- Platform and architecture detection.
- .NET executable discovery.
- SDK and runtime inventory.
- Initial `hilt doctor`.
- Unit tests.

The phase must result in a working executable that starts and displays the UI even without .NET installed.

### Phase 2 — SDK installation and management

- Official installation mechanism.
- Version selection.
- User-owned installation directory.
- Verification after installation.
- Multiple SDK versions.
- Safe removal of Hilt-managed versions.
- Arch/CachyOS package-manager adapter.
- Explicit system-installation confirmation.

Test discovery and installation failures, not only successful paths.

### Phase 3 — Project explorer and execution

- Discover projects and solutions.
- Inspect project metadata.
- Build, restore, clean, run, stop, and test.
- Stream output.
- Handle `global.json`.
- Offer SDK installation when project requirements are missing.

### Phase 4 — NuGet management

- Direct dependency listing.
- Transitive dependency inspection where supported.
- Package search.
- Add, update, and remove.
- Restore and package-source inspection.
- Useful failure messages.

### Phase 5 — Project creation

- Standard templates.
- Clean Architecture preset.
- Vertical Slice preset.
- Configurable project references.
- Test projects.
- Git initialization.
- Post-generation verification.

### Phase 6 — Distribution and hardening

- Release binaries for Linux x64 and ARM64 where feasible.
- Installation documentation for CachyOS and other supported distributions.
- CI and automated tests.
- Release versioning and checksums.
- Shell completions.
- Additional platform support after the Linux implementation is stable.

Do not attempt to implement every phase in one enormous change. At each phase, keep the repository compiling, tests passing, and documentation synchronized with the actual behavior.

## 16. Testing Requirements

Use Go unit tests for:

- Version parsing.
- Platform detection.
- Configuration handling.
- SDK inventory parsing.
- Project metadata parsing.
- Command argument construction.
- Installation-plan validation.
- Dependency-graph validation.

Use temporary directories and fake executables to test external process interactions.

Use HTTP test servers for download logic and failure handling.

Where appropriate, use integration tests against a real .NET SDK when one is available, but do not require .NET to be installed to run Hilt's core unit tests.

Do not make ordinary unit tests depend on network access or administrator privileges.

Include tests for cancellation, malformed metadata, partial downloads, unsupported architectures, process failures, and existing-file protection.

## 17. Engineering Standards

- Use idiomatic Go and the standard library where practical.
- Keep functions focused and packages cohesive.
- Handle errors explicitly with useful context.
- Use contexts for cancellation of long-running operations.
- Avoid global mutable state.
- Do not block the TUI's event loop on downloads, builds, or package operations.
- Preserve stdout, stderr, and process exit codes.
- Avoid logging secrets.
- Pin dependencies to known compatible versions.
- Follow current library documentation.
- Provide useful error messages instead of panics for expected failures.
- Keep README instructions accurate.
- Do not claim compilation, tests, or installation were successful unless they were actually verified.

## 18. Required Deliverables

Deliver a working Git repository containing:

1. Go source code.
2. The TUI and CLI.
3. SDK and runtime discovery.
4. The implemented installation workflow.
5. Unit tests.
6. A README with setup, build, run, and installation instructions.
7. Documentation explaining installation safety and version management.
8. Build and test scripts.
9. A roadmap identifying incomplete functionality.

At the end of the task, summarize:

- Features implemented.
- Commands available.
- Build results.
- Test results.
- How to run Hilt on CachyOS.
- Known limitations and next steps.

Never fabricate test results.

## 19. Instructions to the Coding Agent

Start by inspecting the current repository and environment.

If an existing repository is present, preserve user work and follow its conventions where reasonable.

Then:

1. Inspect available Go versions and the current project state.
2. Verify the current Bubble Tea, Bubbles, Lip Gloss, and CLI framework APIs.
3. Select stable, compatible dependency versions.
4. Create the smallest complete working foundation.
5. Implement the first phase immediately instead of stopping after a plan.
6. Build and run tests.
7. Fix actual compilation and test failures.
8. Verify the executable starts without .NET installed when the environment allows it.
9. Document the commands used and their actual results.
10. Identify the next phase without claiming future functionality already exists.

Make reasonable engineering decisions without asking unnecessary questions. If an external service, SDK, package source, or permission is unavailable, implement what can be safely completed and document the limitation.

**The first milestone is a standalone Go executable with a functional TUI, reliable .NET discovery, SDK/runtime inventory, and diagnostics.** All other features must build on that foundation.

Hilt's distinctive value is that it can start before .NET exists, help install and manage .NET safely, and eventually provide one interactive interface for the wider .NET development workflow.