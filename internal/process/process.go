package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Spec describes a single external command invocation.
type Spec struct {
	Name  string
	Args  []string
	Dir   string
	Env   []string
	Stdin io.Reader
}

// Result preserves process output and exit information.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Duration time.Duration
	Started  bool
}

// ExitError reports a non-zero process exit.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("process exited with code %d", e.Code)
}

// Runner executes external commands. Implementations must not use a shell.
type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
	Stream(ctx context.Context, spec Spec, stdout, stderr io.Writer) (Result, error)
}

// ExecRunner executes commands using os/exec.
type ExecRunner struct{}

// Run captures stdout and stderr.
func (ExecRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	var stdout, stderr strings.Builder
	result, err := run(ctx, spec, &stdout, &stderr)
	result.Stdout = []byte(stdout.String())
	result.Stderr = []byte(stderr.String())
	return result, err
}

// Stream forwards stdout and stderr to the supplied writers.
func (ExecRunner) Stream(ctx context.Context, spec Spec, stdout, stderr io.Writer) (Result, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return run(ctx, spec, stdout, stderr)
}

func run(ctx context.Context, spec Spec, stdout, stderr io.Writer) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	result := Result{ExitCode: -1}

	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	if spec.Env != nil {
		cmd.Env = spec.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Stdin = spec.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		return killProcessGroup(cmd.Process)
	}
	configureSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		result.Duration = time.Since(started)
		return result, fmt.Errorf("start %s: %w", spec.Name, err)
	}
	result.Started = true

	err := cmd.Wait()
	result.Duration = time.Since(started)
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, &ExitError{Code: result.ExitCode}
		}
		return result, fmt.Errorf("wait %s: %w", spec.Name, err)
	}
	return result, nil
}

// EnvWith merges overrides into base, replacing existing keys and sorting the
// result for deterministic command environments. Passing a nil base means an
// empty environment; callers that want inheritance must pass os.Environ().
func EnvWith(base []string, overrides map[string]string) []string {
	if base == nil && len(overrides) == 0 {
		return []string{}
	}

	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		values[key] = value
	}
	for key, value := range overrides {
		if key == "" {
			continue
		}
		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
