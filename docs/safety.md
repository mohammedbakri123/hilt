# Installation and version-management safety

Phase 1 is read-only. Hilt discovers .NET installations and reports findings; it
does not install, remove, or modify SDKs, runtimes, projects, packages, or
templates. It does not use `sudo`, download files, or change `PATH`.

## Current behavior

- `DOTNET_ROOT`, `PATH`, known installation roots, and the Hilt-managed root are
  inspected read-only.
- `dotnet --list-sdks`, `dotnet --list-runtimes`, and `dotnet --info` are invoked
  with telemetry disabled, first-run experience skipped, and multilevel lookup
  disabled.
- If probing fails, Hilt falls back to scanning `sdk/` and `shared/` directories
  and reports the probe error instead of hiding it.
- A missing .NET installation is a warning, never a startup failure.
- Configuration is written only when a save is requested. Saves are atomic and
  mode `0600`; malformed files are preserved with a `.corrupt-*` suffix.

## Managed-root classification

A root is marked `managed` only when it is inside the configured managed root
(`<data>/hilt/dotnet` by default). System installations are never guessed to be
managed. Duplicate SDK versions across roots produce a warning, not a failure.

## Policy for later phases

When installation is implemented, Hilt will:

- prefer isolated, user-owned installations;
- resolve version aliases to explicit versions before acting;
- show the target version, destination, and any package-manager command before
  changing the system;
- never pipe a network download directly into a shell;
- verify published checksums or signatures when available;
- never run `sudo` silently and never modify `PATH` automatically;
- remove only roots recorded as Hilt-managed;
- keep failed downloads, temporary files, and partial installs from accumulating;
- preserve project files unless the user explicitly confirms a change.

System package-manager installation remains an explicit alternative, not the
default. Offline operation reports what is unavailable instead of failing
silently.
