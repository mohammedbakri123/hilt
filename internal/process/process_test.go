package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	runner := ExecRunner{}
	result, err := runner.Run(context.Background(), Spec{
		Name: "sh",
		Args: []string{"-c", "echo out; echo err >&2; exit 7"},
	})

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %v, want ExitError", err)
	}
	if exitErr.Code != 7 {
		t.Errorf("exit error code = %d, want 7", exitErr.Code)
	}
	if result.ExitCode != 7 {
		t.Errorf("result exit code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(string(result.Stdout), "out") {
		t.Errorf("stdout = %q", result.Stdout)
	}
	if !strings.Contains(string(result.Stderr), "err") {
		t.Errorf("stderr = %q", result.Stderr)
	}
}

func TestRunPreservesArgumentsWithoutShell(t *testing.T) {
	script := writeScript(t, "for arg in \"$@\"; do printf '<%s>\\n' \"$arg\"; done\n")

	result, err := ExecRunner{}.Run(context.Background(), Spec{
		Name: "sh",
		Args: []string{script, "a b", "c;d", "$HOME"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "<a b>\n<c;d>\n<$HOME>\n"
	if string(result.Stdout) != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
}

func TestRunAppliesEnvironment(t *testing.T) {
	result, err := ExecRunner{}.Run(context.Background(), Spec{
		Name: "sh",
		Args: []string{"-c", "printf '%s' \"$HILT_TEST_VALUE\""},
		Env:  EnvWith(os.Environ(), map[string]string{"HILT_TEST_VALUE": "hello"}),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(result.Stdout) != "hello" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := ExecRunner{}.Run(ctx, Spec{Name: "sh", Args: []string{"-c", "sleep 5"}})
	elapsed := time.Since(started)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

func TestStream(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result, err := ExecRunner{}.Stream(context.Background(), Spec{
		Name: "sh",
		Args: []string{"-c", "echo hello; echo warning >&2"},
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d", result.ExitCode)
	}
	if stdout.String() != "hello\n" {
		t.Errorf("stdout = %q", stdout.String())
	}
	if stderr.String() != "warning\n" {
		t.Errorf("stderr = %q", stderr.String())
	}
	if result.Duration <= 0 {
		t.Error("expected a positive duration")
	}
}

func TestRunStartFailure(t *testing.T) {
	_, err := ExecRunner{}.Run(context.Background(), Spec{Name: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected an error")
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("start failure should not be an ExitError: %v", err)
	}
}

func TestEnvWith(t *testing.T) {
	base := []string{"PATH=/bin", "DUP=one", "DUP=two", "MALFORMED"}
	got := EnvWith(base, map[string]string{"DUP": "three", "NEW": "value"})
	want := []string{"DUP=three", "NEW=value", "PATH=/bin"}
	if len(got) != len(want) {
		t.Fatalf("EnvWith = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EnvWith = %v, want %v", got, want)
		}
	}
	if got := EnvWith(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("nil base and overrides should return an empty non-nil environment, got %#v", got)
	}
}

func writeScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
