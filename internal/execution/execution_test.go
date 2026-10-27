package execution

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mohammedbakri123/hilt/internal/process"
)

type fakeRunner struct {
	spec    process.Spec
	result  process.Result
	err     error
	stdout  string
	stderr  string
	started chan struct{}
}

func (f *fakeRunner) Run(context.Context, process.Spec) (process.Result, error) {
	return f.result, f.err
}

func (f *fakeRunner) Stream(_ context.Context, spec process.Spec, stdout, stderr io.Writer) (process.Result, error) {
	f.spec = spec
	if f.started != nil {
		close(f.started)
	}
	if f.stdout != "" && stdout != nil {
		_, _ = io.WriteString(stdout, f.stdout)
	}
	if f.stderr != "" && stderr != nil {
		_, _ = io.WriteString(stderr, f.stderr)
	}
	return f.result, f.err
}

func TestBuildArgs(t *testing.T) {
	args, err := BuildArgs(BuildRequest{
		DotnetPath:    "/usr/share/dotnet/dotnet",
		ProjectPath:   "/work/Shop/Shop.csproj",
		Configuration: "Release",
		NoRestore:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"build", "/work/Shop/Shop.csproj", "--nologo", "--verbosity", "minimal",
		"--configuration", "Release", "--no-restore",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildArgsOmitsOptionalFlags(t *testing.T) {
	args, err := BuildArgs(BuildRequest{
		DotnetPath:  "/usr/share/dotnet/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"build", "/work/Shop/Shop.csproj", "--nologo", "--verbosity", "minimal"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildArgsIncludesFrameworkAndExtraArguments(t *testing.T) {
	args, err := BuildArgs(BuildRequest{
		DotnetPath:  "/usr/share/dotnet/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
		Framework:   "net10.0",
		Arguments:   []string{"--no-restore", "-p:WarnAsError=true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"build", "/work/Shop/Shop.csproj", "--nologo", "--verbosity", "minimal",
		"--framework", "net10.0", "--no-restore", "-p:WarnAsError=true",
	}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestRunArgs(t *testing.T) {
	args, err := RunArgs("/work/Shop/Shop.csproj", "Release", "net9.0", []string{"--", "--port", "8080"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"run", "/work/Shop/Shop.csproj", "--nologo", "--configuration", "Release", "--framework", "net9.0", "--", "--port", "8080"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildArgsValidation(t *testing.T) {
	tests := []BuildRequest{
		{ProjectPath: "/work/Shop/Shop.csproj"},
		{DotnetPath: "/usr/share/dotnet/dotnet"},
		{DotnetPath: "  ", ProjectPath: "  "},
	}
	for _, request := range tests {
		if _, err := BuildArgs(request); err == nil {
			t.Errorf("BuildArgs(%+v) did not fail", request)
		}
	}
}

func TestBuildStreamsAndUsesExplicitToolchain(t *testing.T) {
	runner := &fakeRunner{
		result: process.Result{ExitCode: 0, Duration: 1250 * time.Millisecond, Started: true},
		stdout: "Build succeeded.\n",
		stderr: "warning: something\n",
	}
	var stdout, stderr bytes.Buffer
	result, err := Builder{Runner: runner}.Build(context.Background(), BuildRequest{
		DotnetPath:       "/opt/dotnet/dotnet",
		ProjectPath:      "/work/Shop/Shop.csproj",
		WorkingDirectory: "/work/Shop",
		Configuration:    "Debug",
		NoRestore:        true,
		Environment:      []string{"DOTNET_ROOT=/opt/dotnet"},
	}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !result.Started || result.Cancelled {
		t.Fatalf("result = %+v", result)
	}
	if result.Duration != 1250*time.Millisecond {
		t.Fatalf("duration = %s", result.Duration)
	}
	if runner.spec.Name != "/opt/dotnet/dotnet" {
		t.Errorf("name = %q", runner.spec.Name)
	}
	if runner.spec.Dir != "/work/Shop" {
		t.Errorf("dir = %q", runner.spec.Dir)
	}
	wantArgs := []string{"build", "/work/Shop/Shop.csproj", "--nologo", "--verbosity", "minimal", "--configuration", "Debug", "--no-restore"}
	if strings.Join(runner.spec.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("args = %#v", runner.spec.Args)
	}
	if len(runner.spec.Env) != 1 || runner.spec.Env[0] != "DOTNET_ROOT=/opt/dotnet" {
		t.Errorf("env = %#v", runner.spec.Env)
	}
	if stdout.String() != "Build succeeded.\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
	if stderr.String() != "warning: something\n" {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestBuildDefaultsWorkingDirectory(t *testing.T) {
	runner := &fakeRunner{result: process.Result{ExitCode: 0, Started: true}}
	if _, err := (Builder{Runner: runner}).Build(context.Background(), BuildRequest{
		DotnetPath:  "/usr/share/dotnet/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
	}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if runner.spec.Dir != "/work/Shop" {
		t.Fatalf("dir = %q, want project directory", runner.spec.Dir)
	}
}

func TestBuildNonZeroExitIsNotDiscarded(t *testing.T) {
	runner := &fakeRunner{
		result: process.Result{ExitCode: 2, Duration: time.Second, Started: true},
		stderr: "error CS0001\n",
		err:    &process.ExitError{Code: 2},
	}
	var stderr bytes.Buffer
	result, err := Builder{Runner: runner}.Build(context.Background(), BuildRequest{
		DotnetPath:  "/usr/share/dotnet/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
	}, io.Discard, &stderr)
	if err == nil {
		t.Fatal("non-zero exit must return an error")
	}
	var exitErr *process.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("error = %v, want ExitError 2", err)
	}
	if result.ExitCode != 2 || !result.Started || result.Cancelled {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(stderr.String(), "error CS0001") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestBuildCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeRunner{result: process.Result{ExitCode: -1, Started: true}, err: context.Canceled}
	cancel()
	result, err := Builder{Runner: runner}.Build(ctx, BuildRequest{
		DotnetPath:  "/usr/share/dotnet/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
	}, io.Discard, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if !result.Cancelled {
		t.Fatal("result should record cancellation")
	}
	if !result.Started {
		t.Fatal("fake result should preserve started state")
	}
}

func TestBuildStartFailureIsNotStarted(t *testing.T) {
	runner := &fakeRunner{result: process.Result{ExitCode: -1}, err: errors.New("start failed")}
	result, err := Builder{Runner: runner}.Build(context.Background(), BuildRequest{
		DotnetPath:  "/missing/dotnet",
		ProjectPath: "/work/Shop/Shop.csproj",
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("error = %v", err)
	}
	if result.Started {
		t.Fatal("start failure must not report Started")
	}
}

func TestBuildRequiresProjectPath(t *testing.T) {
	runner := &fakeRunner{}
	if _, err := (Builder{Runner: runner}).Build(context.Background(), BuildRequest{
		DotnetPath: "/usr/share/dotnet/dotnet",
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("empty project path should fail before running")
	}
	if runner.spec.Name != "" {
		t.Fatal("validation failure must not invoke the runner")
	}
}

func TestCommandRunsDirectDotnetArguments(t *testing.T) {
	runner := &fakeRunner{result: process.Result{ExitCode: 0, Started: true}}
	result, err := Command(context.Background(), CommandRequest{
		DotnetPath:       "/opt/dotnet/dotnet",
		ProjectPath:      "/work/App/App.csproj",
		WorkingDirectory: "/work/App",
		Arguments:        []string{"test", "/work/App/App.csproj", "--nologo"},
		Environment:      []string{"DOTNET_ROOT=/opt/dotnet"},
	}, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Started || runner.spec.Name != "/opt/dotnet/dotnet" || runner.spec.Dir != "/work/App" {
		t.Fatalf("result=%+v spec=%+v", result, runner.spec)
	}
	if strings.Join(runner.spec.Args, " ") != "test /work/App/App.csproj --nologo" {
		t.Fatalf("args=%v", runner.spec.Args)
	}
}
