# Step 3 — Process execution

## Objective

Provide a single, testable abstraction for running external programs with
contexts, explicit arguments, controlled environments, streaming output, and
reliable cancellation of child process groups.

## Package

`internal/process`

## API

```go
type Spec struct {
    Name  string
    Args  []string
    Dir   string
    Env   []string // nil means inherit os.Environ()
    Stdin io.Reader
}

type Result struct {
    ExitCode int
    Stdout   []byte
    Stderr   []byte
    Duration time.Duration
}

type ExitError struct{ Code int }

type Runner interface {
    Run(ctx context.Context, spec Spec) (Result, error)
    Stream(ctx context.Context, spec Spec, stdout, stderr io.Writer) (Result, error)
}

type ExecRunner struct{}

func EnvWith(base []string, overrides map[string]string) []string
```

## Behavior

- No shell interpolation. `Name` and `Args` are passed directly to `exec`.
- `Run` captures stdout/stderr; `Stream` forwards to writers.
- Non-zero exits return `*ExitError`, with the exit code also present in the
  result. Start failures return the underlying error.
- On context cancellation the process group is killed (Linux sets
  `Setpgid`). `WaitDelay` bounds cleanup.
- Environment overrides replace existing keys, remove duplicates, and sort for
  deterministic output.
- `EnvWith(nil, ...)` is treated as "inherit nothing"; callers must pass
  `os.Environ()` when inheritance is desired.

## Tests

- Exit code and stdout/stderr capture via a temporary shell script.
- Argument with spaces is delivered as one argument.
- `EnvWith` replaces, deduplicates, and sorts.
- Cancellation returns promptly and terminates a sleeping child.
- Streaming writes appear in the provided writers.

## Acceptance

All tests run without a real .NET installation and without network access.
