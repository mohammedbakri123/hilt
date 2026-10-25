package install

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLockContentionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installs.lock")
	release, err := acquireLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := acquireLock(waitCtx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended lock error = %v, want deadline exceeded", err)
	}

	release()
	releaseAgain, err := acquireLock(context.Background(), path)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	releaseAgain()
}
