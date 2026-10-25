package dotnet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeProber struct {
	sdks     []SDK
	runtimes []Runtime
	info     HostInfo
	err      error
}

func (p fakeProber) ListSDKs(context.Context, Installation) ([]SDK, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.sdks, nil
}

func (p fakeProber) ListRuntimes(context.Context, Installation) ([]Runtime, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.runtimes, nil
}

func (p fakeProber) Info(context.Context, Installation) (HostInfo, error) {
	if p.err != nil {
		return HostInfo{}, p.err
	}
	return p.info, nil
}

func TestDiscoverFilesystemFallback(t *testing.T) {
	root := fakeRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "sdk", "8.0.404"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "shared", "Microsoft.NETCore.App", "8.0.20")
	if err := os.MkdirAll(runtimePath, 0o700); err != nil {
		t.Fatal(err)
	}

	locator := Locator{
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Stat:       os.Stat,
		Getenv:     func(string) string { return root },
		KnownRoots: []string{},
	}
	inventory := Discover(context.Background(), locator, fakeProber{err: errors.New("probe failed")}, os.Stat)

	if len(inventory.SDKs) != 2 {
		t.Fatalf("SDKs = %+v", inventory.SDKs)
	}
	if inventory.SDKs[0].Version != "10.0.100" {
		t.Errorf("first SDK = %+v", inventory.SDKs[0])
	}
	if len(inventory.Runtimes) != 1 || inventory.Runtimes[0].Framework != "Microsoft.NETCore.App" {
		t.Errorf("runtimes = %+v", inventory.Runtimes)
	}
	if len(inventory.Warnings) == 0 {
		t.Error("expected probe warnings")
	}
	if !inventory.HasUsableHost() {
		t.Error("expected a usable host")
	}
}

func TestDiscoverUsesProbeMetadata(t *testing.T) {
	root := fakeRoot(t)
	locator := Locator{
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Stat:       os.Stat,
		Getenv:     func(string) string { return root },
		KnownRoots: []string{},
	}
	prober := fakeProber{
		sdks:     []SDK{{Version: "10.0.100", Path: filepath.Join(root, "sdk", "10.0.100"), DetectedBy: "probe"}},
		runtimes: []Runtime{{Framework: "Microsoft.NETCore.App", Version: "10.0.0", Path: filepath.Join(root, "shared", "Microsoft.NETCore.App", "10.0.0")}},
		info:     HostInfo{Architecture: "x64", RID: "linux-x64"},
	}
	inventory := Discover(context.Background(), locator, prober, os.Stat)

	if len(inventory.Hosts) != 1 {
		t.Fatalf("hosts = %+v", inventory.Hosts)
	}
	host := inventory.Hosts[0]
	if host.Architecture != "x64" || host.RID != "linux-x64" {
		t.Errorf("host info = %+v", host)
	}
	if len(host.SDKs) != 1 || host.SDKs[0].DetectedBy != "probe" {
		t.Errorf("host SDKs = %+v", host.SDKs)
	}
	if inventory.SDKs[0].Architecture != "x64" {
		t.Errorf("SDK architecture = %q", inventory.SDKs[0].Architecture)
	}
	if inventory.ResolvedDotnet() != filepath.Join(root, "dotnet") {
		t.Errorf("ResolvedDotnet = %q", inventory.ResolvedDotnet())
	}
}

func TestDiscoverNoInstallations(t *testing.T) {
	locator := Locator{
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Stat:       os.Stat,
		KnownRoots: []string{},
	}
	inventory := Discover(context.Background(), locator, nil, os.Stat)
	if len(inventory.Hosts) != 0 {
		t.Fatalf("expected no hosts: %+v", inventory.Hosts)
	}
	if len(inventory.Warnings) == 0 {
		t.Fatal("expected a warning")
	}
	if inventory.HasUsableHost() {
		t.Fatal("expected no usable host")
	}
}

func TestDiscoverPreservesLocatorPriority(t *testing.T) {
	dotnetRoot := fakeRoot(t)
	pathRoot := fakeRoot(t)
	locator := Locator{
		LookPath: func(string) (string, error) {
			return filepath.Join(pathRoot, "dotnet"), nil
		},
		Stat: os.Stat,
		Getenv: func(key string) string {
			if key == "DOTNET_ROOT" {
				return dotnetRoot
			}
			return ""
		},
		KnownRoots: []string{},
	}
	inventory := Discover(context.Background(), locator, nil, os.Stat)
	if len(inventory.Hosts) != 2 {
		t.Fatalf("hosts = %+v", inventory.Hosts)
	}
	if inventory.Hosts[0].Root != dotnetRoot || inventory.Hosts[1].Root != pathRoot {
		t.Errorf("hosts lost discovery priority: %+v", inventory.Hosts)
	}
	if got := inventory.ResolvedDotnet(); got != filepath.Join(dotnetRoot, "dotnet") {
		t.Errorf("ResolvedDotnet = %q, want DOTNET_ROOT host", got)
	}
}

func TestDiscoverDuplicateVersionWarning(t *testing.T) {
	rootA := fakeRoot(t)
	rootB := fakeRoot(t)
	locator := Locator{
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Stat:       os.Stat,
		KnownRoots: []string{rootA, rootB},
	}
	inventory := Discover(context.Background(), locator, nil, os.Stat)
	if len(inventory.Warnings) == 0 {
		t.Fatal("expected warnings")
	}
	found := false
	for _, warning := range inventory.Warnings {
		if warning == "SDK 10.0.100 is present in multiple installation roots." {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v", inventory.Warnings)
	}
}
