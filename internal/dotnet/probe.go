package dotnet

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mohammedbakri123/hilt/internal/process"
)

// CLIProber invokes a specific dotnet host.
type CLIProber struct {
	Runner process.Runner
}

func (p CLIProber) ListSDKs(ctx context.Context, installation Installation) ([]SDK, error) {
	output, err := p.run(ctx, installation, "--list-sdks")
	if err != nil {
		return nil, err
	}
	return ParseSDKList(output, installation.Root)
}

func (p CLIProber) ListRuntimes(ctx context.Context, installation Installation) ([]Runtime, error) {
	output, err := p.run(ctx, installation, "--list-runtimes")
	if err != nil {
		return nil, err
	}
	return ParseRuntimeList(output, installation.Root)
}

func (p CLIProber) Info(ctx context.Context, installation Installation) (HostInfo, error) {
	output, err := p.run(ctx, installation, "--info")
	if err != nil {
		return HostInfo{}, err
	}
	return ParseInfo(output), nil
}

func (p CLIProber) run(ctx context.Context, installation Installation, args ...string) ([]byte, error) {
	if p.Runner == nil {
		return nil, fmt.Errorf("no process runner configured")
	}
	env := BuildToolchainEnvironment(os.Environ(), installation.Root)
	result, err := p.Runner.Run(ctx, process.Spec{
		Name: installation.DotnetPath,
		Args: args,
		Env:  env,
	})
	if err != nil {
		return nil, fmt.Errorf("dotnet %s: %w", strings.Join(args, " "), err)
	}
	return result.Stdout, nil
}

// ParseSDKList parses `dotnet --list-sdks`.
func ParseSDKList(output []byte, root string) ([]SDK, error) {
	var sdks []SDK
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		version, basePath, err := parseListLine(line)
		if err != nil {
			return nil, fmt.Errorf("parse SDK line %q: %w", line, err)
		}
		if _, err := ParseVersion(version); err != nil {
			return nil, fmt.Errorf("parse SDK line %q: %w", line, err)
		}
		sdks = append(sdks, SDK{
			Version:    version,
			Path:       versionedPath(basePath, version),
			Root:       root,
			Usable:     true,
			DetectedBy: "probe",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SDK list: %w", err)
	}
	return sdks, nil
}

// ParseRuntimeList parses `dotnet --list-runtimes`.
func ParseRuntimeList(output []byte, root string) ([]Runtime, error) {
	var runtimes []Runtime
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("parse runtime line %q: expected framework, version, and path", line)
		}
		framework := fields[0]
		version := fields[1]
		if _, err := ParseVersion(version); err != nil {
			return nil, fmt.Errorf("parse runtime line %q: %w", line, err)
		}
		open := strings.LastIndex(line, " [")
		if open < 0 || !strings.HasSuffix(line, "]") {
			return nil, fmt.Errorf("parse runtime line %q: missing bracketed path", line)
		}
		basePath := strings.TrimSpace(line[open+2 : len(line)-1])
		runtimes = append(runtimes, Runtime{
			Framework:  framework,
			Version:    version,
			Path:       versionedPath(basePath, version),
			Root:       root,
			DetectedBy: "probe",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read runtime list: %w", err)
	}
	return runtimes, nil
}

// ParseInfo extracts host-level fields from `dotnet --info`.
func ParseInfo(output []byte) HostInfo {
	var info HostInfo
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Architecture":
			if info.Architecture == "" {
				info.Architecture = value
			}
		case "RID":
			if info.RID == "" {
				info.RID = value
			}
		case "Base Path":
			if info.BasePath == "" {
				info.BasePath = value
			}
		}
	}
	return info
}

func parseListLine(line string) (string, string, error) {
	open := strings.LastIndex(line, " [")
	if open < 0 || !strings.HasSuffix(line, "]") {
		return "", "", fmt.Errorf("expected 'version [path]'")
	}
	version := strings.TrimSpace(line[:open])
	path := strings.TrimSpace(line[open+2 : len(line)-1])
	if version == "" || path == "" {
		return "", "", fmt.Errorf("empty version or path")
	}
	return version, path, nil
}

func versionedPath(basePath, version string) string {
	basePath = filepath.Clean(basePath)
	if filepath.Base(basePath) == version {
		return basePath
	}
	return filepath.Join(basePath, version)
}
