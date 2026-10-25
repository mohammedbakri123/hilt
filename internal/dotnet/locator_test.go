package dotnet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocatorDeduplicatesAndClassifies(t *testing.T) {
	rootA := fakeRoot(t)
	rootB := fakeRoot(t)
	managedRoot := rootB

	locator := Locator{
		LookPath: func(name string) (string, error) {
			if name == "dotnet" {
				return filepath.Join(rootA, "dotnet"), nil
			}
			return "", errors.New("not found")
		},
		Stat:        os.Stat,
		Getenv:      func(key string) string { return "" },
		ManagedRoot: managedRoot,
		KnownRoots:  []string{rootA, rootB},
	}
	installations := locator.Locate()
	if len(installations) != 2 {
		t.Fatalf("Locate() returned %d installations: %+v", len(installations), installations)
	}
	if installations[0].Root != rootA || installations[0].Source != SourcePath {
		t.Errorf("first installation = %+v", installations[0])
	}
	if !installations[1].Managed {
		t.Errorf("expected %s to be managed: %+v", rootB, installations[1])
	}
	if !installations[0].Usable {
		t.Errorf("expected usable root: %+v", installations[0])
	}
}

func TestLocatorDotnetRootPriority(t *testing.T) {
	root := fakeRoot(t)
	locator := Locator{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Stat:     os.Stat,
		Getenv: func(key string) string {
			if key == "DOTNET_ROOT" {
				return root
			}
			return ""
		},
		KnownRoots: []string{},
	}
	installations := locator.Locate()
	if len(installations) != 1 {
		t.Fatalf("got %d installations", len(installations))
	}
	if installations[0].Source != SourceDotnetRoot {
		t.Errorf("source = %q", installations[0].Source)
	}
}

func TestLocatorOwnershipSeparateFromManaged(t *testing.T) {
	managedRoot := t.TempDir()
	ownedRoot := filepath.Join(managedRoot, "owned")
	unownedRoot := filepath.Join(managedRoot, "unowned")
	for _, root := range []string{ownedRoot, unownedRoot} {
		createRoot(t, root, "10.0.100")
	}

	locator := Locator{
		LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		Stat:        os.Stat,
		Getenv:      func(string) string { return "" },
		ManagedRoot: managedRoot,
		KnownRoots:  []string{ownedRoot, unownedRoot},
		SystemRoots: []string{},
		Ownership: OwnershipFunc(func(root, version string) bool {
			return root == ownedRoot && version == ""
		}),
	}
	installations := locator.Locate()
	if len(installations) != 2 {
		t.Fatalf("Locate() returned %d installations: %+v", len(installations), installations)
	}
	if installations[0].Provenance != ProvenanceOwned || !installations[0].Owned {
		t.Errorf("owned root = %+v", installations[0])
	}
	if installations[1].Provenance != ProvenanceManaged || installations[1].Owned {
		t.Errorf("unowned managed root = %+v", installations[1])
	}
}

func TestLocatorClassifiesSystemAndExternal(t *testing.T) {
	systemRoot := t.TempDir()
	externalRoot := t.TempDir()
	createRoot(t, systemRoot, "10.0.100")
	createRoot(t, externalRoot, "9.0.300")

	locator := Locator{
		LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		Stat:        os.Stat,
		Getenv:      func(string) string { return "" },
		KnownRoots:  []string{systemRoot, externalRoot},
		SystemRoots: []string{systemRoot},
	}
	installations := locator.Locate()
	if len(installations) != 2 {
		t.Fatalf("Locate() returned %d installations", len(installations))
	}
	if installations[0].Provenance != ProvenanceSystem {
		t.Errorf("system root = %+v", installations[0])
	}
	if installations[1].Provenance != ProvenanceExternal {
		t.Errorf("external root = %+v", installations[1])
	}
}

func TestLocatorSkipsMissingRoots(t *testing.T) {
	locator := Locator{
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
		Stat:       os.Stat,
		KnownRoots: []string{filepath.Join(t.TempDir(), "missing")},
	}
	if got := locator.Locate(); len(got) != 0 {
		t.Fatalf("expected no installations, got %+v", got)
	}
}

func TestIsWithin(t *testing.T) {
	root := "/home/user/.local/share/hilt/dotnet"
	tests := []struct {
		path string
		want bool
	}{
		{root, true},
		{root + "/sdk/10.0.100", true},
		{root + "-other", false},
		{"/home/user/.local/share/hilt", false},
		{"", false},
	}
	for _, test := range tests {
		if got := isWithin(test.path, root); got != test.want {
			t.Errorf("isWithin(%q) = %v, want %v", test.path, got, test.want)
		}
	}
}

func fakeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	createRoot(t, root, "10.0.100")
	return root
}

func createRoot(t *testing.T, root, sdkVersion string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sdk", sdkVersion), 0o700); err != nil {
		t.Fatal(err)
	}
}
