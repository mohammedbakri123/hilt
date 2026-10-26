// Package execution runs .NET build commands against a resolved toolchain.
// Commands are executed directly, never through a shell, and always accept a
// context so callers can cancel them.
package execution

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mohammedbakri123/hilt/internal/process"
)

// BuildRequest describes one dotnet build invocation.
type BuildRequest struct {
	DotnetPath       string
	ProjectPath      string
	WorkingDirectory string
	Configuration    string
	NoRestore        bool
	Environment      []string
}

// Result reports the outcome of a build invocation.
type Result struct {
	ExitCode  int
	Duration  time.Duration
	Cancelled bool
	Started   bool
}

// Builder executes build requests with a process runner.
type Builder struct {
	Runner process.Runner
}

// BuildArgs returns the deterministic argument list for a request. It does not
// consult the environment or the filesystem.
func BuildArgs(request BuildRequest) ([]string, error) {
	if strings.TrimSpace(request.DotnetPath) == "" {
		return nil, fmt.Errorf("dotnet path is required")
	}
	if strings.TrimSpace(request.ProjectPath) == "" {
		return nil, fmt.Errorf("project path is required")
	}
	args := []string{
		"build",
		request.ProjectPath,
		"--nologo",
		"--verbosity", "minimal",
	}
	if configuration := strings.TrimSpace(request.Configuration); configuration != "" {
		args = append(args, "--configuration", configuration)
	}
	if request.NoRestore {
		args = append(args, "--no-restore")
	}
	return args, nil
}

// Build runs a build request and streams combined stdout and stderr to the
// supplied writers. A non-zero exit is returned as a process.ExitError with the
// result intact. Cancellation is reported in the result and via ctx.Err.
func (b Builder) Build(ctx context.Context, request BuildRequest, stdout, stderr io.Writer) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args, err := BuildArgs(request)
	if err != nil {
		return Result{}, err
	}
	workingDirectory := request.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(request.ProjectPath)
	}

	var runner process.Runner = b.Runner
	if runner == nil {
		runner = process.ExecRunner{}
	}

	processResult, runErr := runner.Stream(ctx, process.Spec{
		Name: request.DotnetPath,
		Args: args,
		Dir:  workingDirectory,
		Env:  request.Environment,
	}, stdout, stderr)

	result := Result{
		ExitCode: processResult.ExitCode,
		Duration: processResult.Duration,
		Started:  processResult.Started,
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.Cancelled = true
		return result, ctxErr
	}
	if runErr != nil {
		return result, fmt.Errorf("dotnet build: %w", runErr)
	}
	return result, nil
}
