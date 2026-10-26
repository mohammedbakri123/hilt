# Installation and version-management safety

Hilt follows a read-wide, write-private model: it discovers and uses every .NET
installation it can find, but it writes only to its own user-owned managed root.
It never modifies system or external installations.

## Read-only discovery

- `DOTNET_ROOT`, `PATH`, known installation roots, and the Hilt-managed root are
  inspected read-only.
- `dotnet --list-sdks`, `dotnet --list-runtimes`, and `dotnet --info` are invoked
  with telemetry disabled, first-run experience skipped, and multilevel lookup
  disabled.
- If probing fails, Hilt falls back to scanning `sdk/` and `shared/` directories
  and reports the probe error instead of hiding it.
- A missing .NET installation is a warning, never a startup failure.
- A root is marked `managed` only when it is inside the configured managed root
  (`<data>/hilt/dotnet` by default). `owned` requires an install-manifest entry.
  System installations are never guessed to be managed.
- Duplicate SDK versions across roots produce a warning, not a failure.

## Managed SDK installs

Hilt installs only exact stable SDK versions into
`<data>/hilt/dotnet/<version>`. The manifest and lock are siblings of that
directory: `installs.json` and `installs.lock`.

Installation behavior:

- Release metadata comes from Microsoft over HTTPS. Metadata and archive URLs,
  including redirects, are restricted to Microsoft hosts.
- Only the host RID's `.tar.gz` archive is used. Downloads have a 2 GiB limit;
  extraction has a 4 GiB and 100,000-entry limit.
- Archives are downloaded to a temporary directory outside the managed root,
  never piped to a shell, and verified against the published SHA-512 checksum
  before extraction.
- Extraction rejects absolute paths, traversal, unsafe links, and special
  files. The staged toolchain is verified with `dotnet --list-sdks` and
  `dotnet --info` using `DOTNET_ROOT` set to the staging directory and
  `DOTNET_MULTILEVEL_LOOKUP=0`.
- Publishing is an atomic rename inside the managed root. The manifest entry is
  written last. If the manifest write fails, the published directory is rolled
  back.
- Installs are idempotent and guarded by a lock. Checksum, download, extraction,
  verification, cancellation, and manifest failures do not leave a published
  toolchain behind.
- If the destination already exists without manifest ownership, Hilt refuses to
  overwrite it.
- If the same version exists in a system root, Hilt still installs the private
  copy and says so.

## Managed runtime installs

Hilt installs .NET runtime archives into the managed root alongside SDKs.
`hilt runtime install <version> --framework all|netcore|aspnetcore` downloads
the runtime (and ASP.NET Core runtime when requested), verifies both, and
publishes them atomically. Runtimes are verified with `dotnet --list-runtimes`.

## Managed SDK discovery

`hilt sdk available` and the TUI install picker read official Microsoft release
metadata. They perform no downloads and make no filesystem changes; installed
versions are hidden by default.

## Managed SDK updates

`hilt sdk update <version> [--remove-old]` installs the newest stable SDK in
the same release channel without touching the previous one. With `--remove-old`,
the previous SDK is removed only after the new one is installed and verified;
a removal failure leaves the new SDK installed and reports partial success.

## Removal

`hilt sdk remove <version>` removes only an SDK that is both inside the managed
root and recorded in Hilt's install manifest.

- System installations, external installations, and unowned directories under
  the managed root are refused with specific messages.
- A stale manifest entry whose directory is already gone is reconciled without
  deleting anything else.
- The directory is renamed to a temporary removal path, deleted, and only then
  removed from the manifest. If deletion fails, the rename is rolled back and
  the manifest entry is kept.
- A manifest entry whose root escapes the managed root is refused.

## Runtime updates

`hilt runtime outdated` checks managed runtimes for newer stable releases in their channel. It is read-only.

## Update checks

`hilt sdk outdated` is explicit and read-only. It checks the newest stable SDK
in the same `major.minor` release channel as each manifest-owned installation.
It reports availability; it never downloads, installs, or applies an update.
Update checks require network access to Microsoft release metadata.

## What Hilt does not do

- It does not run `sudo`.
- It does not run package-manager commands or write system packages.
- It does not modify `PATH` or edit shell startup files.
- It does not modify, upgrade, or remove system or external SDKs.
- It does not update automatically.
- It does not write project files, NuGet state, or templates yet.

System package-manager installation remains an explicit alternative, not the
default. Offline operation reports what is unavailable instead of failing
silently.

## Configuration and data

Configuration is written only when a save is requested. Saves are atomic and
mode `0600`; malformed files are preserved with a `.corrupt-*` suffix. The
install manifest is also written atomically with mode `0600`.
