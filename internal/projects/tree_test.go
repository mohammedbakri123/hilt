package projects

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanTreeSkipsGeneratedAndHiddenDirectories(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"src", "bin", "obj", ".git", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, directory, "file.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "App.csproj"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.cs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanTree(context.Background(), root, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.RelativePath == "bin" || entry.RelativePath == "obj" || entry.RelativePath == ".git" || entry.RelativePath == "node_modules" {
			t.Fatalf("excluded directory appeared: %+v", entry)
		}
	}
	if len(entries) != 4 {
		t.Fatalf("entries = %+v, want App.csproj, src, src/file.txt, src/main.cs", entries)
	}
}

func TestScanTreeHonorsDepthAndCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "one", "two"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "one", "two", "three.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanTree(context.Background(), root, 1)
	if err != nil || len(entries) != 1 || entries[0].RelativePath != "one" {
		t.Fatalf("depth entries = %+v err=%v", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScanTree(ctx, root, 0); err == nil {
		t.Fatal("cancelled tree scan unexpectedly succeeded")
	}
}
