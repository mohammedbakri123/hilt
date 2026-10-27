package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/process"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

type appFakeRunner struct {
	spec   process.Spec
	result process.Result
	err    error
}

func (f *appFakeRunner) Run(context.Context, process.Spec) (process.Result, error) {
	return f.result, f.err
}

func (f *appFakeRunner) Stream(_ context.Context, spec process.Spec, _, _ io.Writer) (process.Result, error) {
	f.spec = spec
	return f.result, f.err
}

func TestBuildProjectUsesExplicitToolchainAndStreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))
	runner := &appFakeRunner{result: process.Result{ExitCode: 0, Duration: time.Second, Started: true}}
	app := New("test")
	app.runner = runner

	var stdout bytes.Buffer
	result, err := app.BuildProject(context.Background(), execution.BuildRequest{
		DotnetPath:       "/opt/dotnet/dotnet",
		ProjectPath:      "/work/Shop/Shop.csproj",
		WorkingDirectory: "/work/Shop",
		Configuration:    "Release",
		Environment:      []string{"DOTNET_ROOT=/opt/dotnet"},
	}, &stdout, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Started || result.ExitCode != 0 {
		t.Fatalf("result = %+v", result)
	}
	if runner.spec.Name != "/opt/dotnet/dotnet" {
		t.Errorf("name = %q", runner.spec.Name)
	}
	if runner.spec.Dir != "/work/Shop" {
		t.Errorf("dir = %q", runner.spec.Dir)
	}
	if len(runner.spec.Args) < 6 || runner.spec.Args[0] != "build" {
		t.Errorf("args = %#v", runner.spec.Args)
	}
	if len(runner.spec.Env) != 1 || runner.spec.Env[0] != "DOTNET_ROOT=/opt/dotnet" {
		t.Errorf("env = %#v", runner.spec.Env)
	}
}

func TestRecordBuildTargetPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))
	app := New("test")

	target := state.BuildTarget{
		ProjectPath:   "/work/Shop/Shop.csproj",
		WorkspacePath: "/work/Shop",
		SDKVersion:    "10.0.100",
		Configuration: "Debug",
	}
	if err := app.RecordBuildTarget(target); err != nil {
		t.Fatal(err)
	}
	if app.State().LastBuild == nil || app.State().LastBuild.ProjectPath != target.ProjectPath {
		t.Fatalf("in-memory state = %+v", app.State().LastBuild)
	}

	loaded, warnings, err := (state.Store{Path: app.Paths().StateFile()}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if loaded.LastBuild == nil {
		t.Fatal("last build was not persisted")
	}
	if loaded.LastBuild.ProjectPath != target.ProjectPath || loaded.LastBuild.WorkspacePath != target.WorkspacePath {
		t.Errorf("persisted target = %+v", loaded.LastBuild)
	}
	if loaded.LastBuild.LastBuilt.IsZero() {
		t.Error("LastBuilt was not set")
	}
}

func TestStateCopiesLastBuild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))
	app := New("test")
	if err := app.RecordBuildTarget(state.BuildTarget{ProjectPath: "/work/Shop/Shop.csproj"}); err != nil {
		t.Fatal(err)
	}
	got := app.State()
	got.LastBuild.ProjectPath = "mutated"
	if app.State().LastBuild.ProjectPath != "/work/Shop/Shop.csproj" {
		t.Fatal("State exposed a mutable LastBuild pointer")
	}
}

type createRunner struct {
	specs []process.Spec
}

func (r *createRunner) Run(context.Context, process.Spec) (process.Result, error) {
	return process.Result{Started: true}, nil
}

func (r *createRunner) Stream(_ context.Context, spec process.Spec, _, _ io.Writer) (process.Result, error) {
	r.specs = append(r.specs, spec)
	return process.Result{Started: true}, nil
}

func TestCreateProjectsExecutesValidatedPlanInOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))
	app := New("test")
	runner := &createRunner{}
	app.runner = runner
	plan, err := projects.PlanCreate(projects.CreateRequest{
		TemplateID:       "webapi",
		Name:             "Orders",
		Destination:      filepath.Join(home, "src"),
		ExistingSolution: filepath.Join(home, "Orders.sln"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CreateProjects(context.Background(), plan, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(runner.specs) != 2 {
		t.Fatalf("executed specs = %d, want 2", len(runner.specs))
	}
	if runner.specs[0].Name != "dotnet" || runner.specs[0].Args[0] != "new" {
		t.Fatalf("generator spec = %+v", runner.specs[0])
	}
	if runner.specs[1].Args[0] != "sln" || runner.specs[1].Args[2] != "add" {
		t.Fatalf("solution spec = %+v", runner.specs[1])
	}
}

func TestDeleteProjectRemovesOnlyProjectFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HILT_HOME", filepath.Join(home, "hilt"))
	projectDir := filepath.Join(home, "src", "Orders")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(projectDir, "Orders.csproj")
	if err := os.WriteFile(projectPath, []byte("<Project Sdk=\"Microsoft.NET.Sdk\" />"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := New("test")
	if err := app.DeleteProject(projectPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		t.Fatalf("project file still exists, stat error = %v", err)
	}
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		t.Fatalf("project directory was removed: info=%v err=%v", info, err)
	}
}
