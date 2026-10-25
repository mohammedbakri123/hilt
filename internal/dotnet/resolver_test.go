package dotnet

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/process"
)

type fakeRunner struct {
	calls   []process.Spec
	respond func(process.Spec) (process.Result, error)
}

func (f *fakeRunner) Run(ctx context.Context, spec process.Spec) (process.Result, error) {
	f.calls = append(f.calls, spec)
	if err := ctx.Err(); err != nil {
		return process.Result{}, err
	}
	if f.respond != nil {
		return f.respond(spec)
	}
	return process.Result{}, nil
}

func (f *fakeRunner) Stream(ctx context.Context, spec process.Spec, _, _ io.Writer) (process.Result, error) {
	return f.Run(ctx, spec)
}

func resolverHost(root string, provenance Provenance, versions ...string) Host {
	host := Host{
		Root:       root,
		DotnetPath: filepath.Join(root, "dotnet"),
		Source:     SourceKnown,
		Provenance: provenance,
		Managed:    provenance == ProvenanceManaged || provenance == ProvenanceOwned,
		Owned:      provenance == ProvenanceOwned,
		Usable:     true,
	}
	for _, version := range versions {
		host.SDKs = append(host.SDKs, SDK{
			Version:    version,
			Path:       filepath.Join(root, "sdk", version),
			Root:       root,
			Source:     SourceKnown,
			Provenance: provenance,
			Managed:    host.Managed,
			Owned:      host.Owned,
			Usable:     true,
		})
	}
	return host
}

func writeGlobalJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveSystemOnly(t *testing.T) {
	runner := &fakeRunner{}
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112")}},
		Dir:       t.TempDir(),
		Runner:    runner,
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Root != "/usr/share/dotnet" || selection.SDKVersion != "10.0.112" {
		t.Errorf("selection = %+v", selection)
	}
	if selection.Provenance != ProvenanceSystem {
		t.Errorf("provenance = %q", selection.Provenance)
	}
	if selection.Verified {
		t.Error("selection without global.json should not run dotnet --version")
	}
	if len(runner.calls) != 0 {
		t.Errorf("unexpected process calls: %+v", runner.calls)
	}
}

func TestResolveManagedWinsOverNewerSystem(t *testing.T) {
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112"),
			resolverHost("/home/u/.local/share/hilt/dotnet", ProvenanceManaged, "9.0.300"),
		}},
		Dir: t.TempDir(),
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Root != "/home/u/.local/share/hilt/dotnet" || selection.SDKVersion != "9.0.300" {
		t.Errorf("selection = %+v", selection)
	}
	if selection.Provenance != ProvenanceManaged {
		t.Errorf("provenance = %q", selection.Provenance)
	}
	if !strings.Contains(selection.Rationale, "managed") {
		t.Errorf("rationale = %q", selection.Rationale)
	}
}

func TestResolveNewestManagedWithinManagedGroup(t *testing.T) {
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost("/managed/a", ProvenanceManaged, "8.0.404", "9.0.300"),
			resolverHost("/managed/b", ProvenanceOwned, "9.0.200"),
			resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112"),
		}},
		Dir: t.TempDir(),
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.SDKVersion != "9.0.300" || selection.Root != "/managed/a" {
		t.Errorf("selection = %+v", selection)
	}
}

func TestResolveGlobalJSONFallsBackToSystem(t *testing.T) {
	dir := t.TempDir()
	writeGlobalJSON(t, dir, `{"sdk":{"version":"10.0.112"}}`)

	managedRoot := "/home/u/.local/share/hilt/dotnet"
	systemRoot := "/usr/share/dotnet"
	runner := &fakeRunner{respond: func(spec process.Spec) (process.Result, error) {
		if strings.HasPrefix(spec.Name, managedRoot) {
			return process.Result{ExitCode: 1, Stderr: []byte("compatible SDK not found")}, errors.New("exit status 1")
		}
		return process.Result{ExitCode: 0, Stdout: []byte("10.0.112\n")}, nil
	}}
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost(systemRoot, ProvenanceSystem, "10.0.112"),
			resolverHost(managedRoot, ProvenanceManaged, "9.0.300"),
		}},
		Dir:     dir,
		Runner:  runner,
		BaseEnv: []string{},
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Root != systemRoot || selection.SDKVersion != "10.0.112" {
		t.Errorf("selection = %+v", selection)
	}
	if !selection.Verified {
		t.Error("global.json selection should be verified")
	}
	if len(selection.Attempts) != 2 {
		t.Fatalf("attempts = %+v", selection.Attempts)
	}
	if selection.Attempts[0].Error == "" || selection.Attempts[1].Version != "10.0.112" {
		t.Errorf("attempts = %+v", selection.Attempts)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("process calls = %+v", runner.calls)
	}
	for _, call := range runner.calls {
		if call.Dir != dir {
			t.Errorf("call Dir = %q, want %q", call.Dir, dir)
		}
		if !containsEnv(call.Env, "DOTNET_MULTILEVEL_LOOKUP=0") {
			t.Errorf("call env missing multilevel opt-out: %v", call.Env)
		}
	}
}

func TestResolveGlobalJSONManagedSatisfiesWithoutTryingSystem(t *testing.T) {
	dir := t.TempDir()
	writeGlobalJSON(t, dir, `{"sdk":{"version":"9.0.300"}}`)

	managedRoot := "/home/u/.local/share/hilt/dotnet"
	runner := &fakeRunner{respond: func(spec process.Spec) (process.Result, error) {
		return process.Result{Stdout: []byte("9.0.300\n")}, nil
	}}
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112"),
			resolverHost(managedRoot, ProvenanceManaged, "9.0.300"),
		}},
		Dir:     dir,
		Runner:  runner,
		BaseEnv: []string{},
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.Root != managedRoot || selection.SDKVersion != "9.0.300" {
		t.Errorf("selection = %+v", selection)
	}
	if len(runner.calls) != 1 {
		t.Errorf("process calls = %+v", runner.calls)
	}
	if !containsEnv(runner.calls[0].Env, "DOTNET_ROOT="+managedRoot) {
		t.Errorf("call env missing selected root: %v", runner.calls[0].Env)
	}
}

func TestResolveGlobalJSONRollForwardUsesActualVersion(t *testing.T) {
	dir := t.TempDir()
	writeGlobalJSON(t, dir, `{"sdk":{"version":"8.0.100","rollForward":"latestFeature"}}`)

	runner := &fakeRunner{respond: func(spec process.Spec) (process.Result, error) {
		return process.Result{Stdout: []byte("10.0.112\n")}, nil
	}}
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112"),
		}},
		Dir:     dir,
		Runner:  runner,
		BaseEnv: []string{},
	}
	selection, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if selection.SDKVersion != "10.0.112" || selection.RequestedVersion != "8.0.100" {
		t.Errorf("selection = %+v", selection)
	}
	if !strings.Contains(selection.Rationale, "10.0.112") {
		t.Errorf("rationale = %q", selection.Rationale)
	}
}

func TestResolveGlobalJSONMissingReportsAttempts(t *testing.T) {
	dir := t.TempDir()
	writeGlobalJSON(t, dir, `{"sdk":{"version":"8.0.100"}}`)

	managedRoot := "/home/u/.local/share/hilt/dotnet"
	systemRoot := "/usr/share/dotnet"
	runner := &fakeRunner{respond: func(spec process.Spec) (process.Result, error) {
		return process.Result{ExitCode: 1, Stderr: []byte("not found")}, errors.New("exit status 1")
	}}
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost(systemRoot, ProvenanceSystem, "10.0.112"),
			resolverHost(managedRoot, ProvenanceManaged, "9.0.300"),
		}},
		Dir:     dir,
		Runner:  runner,
		BaseEnv: []string{},
	}
	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	var missing *MissingSDKError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %T %v, want *MissingSDKError", err, err)
	}
	if missing.RequestedVersion != "8.0.100" {
		t.Errorf("requested version = %q", missing.RequestedVersion)
	}
	if len(missing.Attempts) != 2 {
		t.Fatalf("attempts = %+v", missing.Attempts)
	}
	for _, want := range []string{"8.0.100", managedRoot, systemRoot, "not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestResolveNoSDKsReturnsTypedError(t *testing.T) {
	resolver := Resolver{Dir: t.TempDir()}
	_, err := resolver.Resolve(context.Background())
	var missing *MissingSDKError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %T %v, want *MissingSDKError", err, err)
	}
	if missing.RequestedVersion != "" {
		t.Errorf("requested version = %q", missing.RequestedVersion)
	}
}

func TestResolvePropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := Resolver{
		Inventory: Inventory{Hosts: []Host{
			resolverHost("/usr/share/dotnet", ProvenanceSystem, "10.0.112"),
		}},
		Dir: t.TempDir(),
	}
	if _, err := resolver.Resolve(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
