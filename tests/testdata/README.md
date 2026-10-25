# Test fixtures

Captured, sanitized command output used by parser tests. Tests never run the
network and never require .NET.

- `dotnet-list-sdks.txt` — `dotnet --list-sdks` from .NET SDK 10.0.112.
- `dotnet-list-runtimes.txt` — `dotnet --list-runtimes` from .NET 10.0.12.
- `dotnet-info.txt` — `dotnet --info` from the same host.
- `os-release-cachyos` — `/etc/os-release` on CachyOS.

Paths are limited to `/usr/share/dotnet`. No home directories, secrets, or
hostnames are included. Fixtures are static; update them only when the parser
contract changes or a new .NET output format must be supported.
