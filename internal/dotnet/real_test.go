package dotnet

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mohammedbakri123/hilt/internal/process"
)

// TestRealDotnetInventory exercises discovery against an installed SDK. It is
// opt-in so the regular suite never depends on .NET.
func TestRealDotnetInventory(t *testing.T) {
	if os.Getenv("HILT_TEST_REAL_DOTNET") != "1" {
		t.Skip("set HILT_TEST_REAL_DOTNET=1 to probe the installed .NET SDK")
	}
	binary, err := exec.LookPath("dotnet")
	if err != nil {
		t.Skip("dotnet is not on PATH")
	}
	root := filepath.Dir(binary)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	locator := Locator{
		LookPath:     exec.LookPath,
		EvalSymlinks: filepath.EvalSymlinks,
		Stat:         os.Stat,
		Getenv:       os.Getenv,
		KnownRoots:   []string{root},
	}
	inventory := Discover(ctx, locator, CLIProber{Runner: process.ExecRunner{}}, os.Stat)
	if len(inventory.Hosts) == 0 {
		t.Fatal("expected at least one installation root")
	}
	if len(inventory.SDKs) == 0 {
		t.Fatalf("expected at least one SDK: %+v", inventory.Warnings)
	}
	if inventory.ResolvedDotnet() == "" {
		t.Fatal("expected a resolved dotnet executable")
	}
}
