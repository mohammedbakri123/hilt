package install

import (
	"crypto/sha512"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManifestRoundTripOwnershipAndPermissions(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	hash := sha512.Sum512([]byte("archive"))
	installedAt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

	manifest, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Installs) != 0 {
		t.Fatalf("new manifest has %d entries", len(manifest.Installs))
	}
	entry := testEntry("10.0.100", hash[:])
	entry.InstalledAt = installedAt
	if err := manifest.Add(entry); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("manifest mode = %o, want 600", info.Mode().Perm())
	}

	loaded, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	found := loaded.Find("10.0.100")
	if found == nil {
		t.Fatal("entry not found")
	}
	if found.SHA512 != FormatSHA512(hash[:]) || found.RID != "linux-x64" {
		t.Errorf("entry = %+v", found)
	}
	root := filepath.Join(managedRoot, "10.0.100")
	if !loaded.Owns(root, "10.0.100") {
		t.Error("expected exact version ownership")
	}
	if loaded.Owns(root, "9.0.100") {
		t.Error("unexpected ownership for another version")
	}
	if loaded.Owns(managedRoot, "") {
		t.Error("managed root itself should not be owned")
	}
	roots := loaded.ManagedRoots()
	if len(roots) != 1 || roots[0] != root {
		t.Errorf("ManagedRoots = %v", roots)
	}
}

func TestManifestRemove(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifest := NewManifest(managedRoot)
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(testEntry("10.0.100", hash[:])); err != nil {
		t.Fatal(err)
	}
	removed, ok := manifest.Remove("10.0.100")
	if !ok || removed.Version != "10.0.100" {
		t.Errorf("Remove() = %+v, %v", removed, ok)
	}
	if manifest.Find("10.0.100") != nil {
		t.Error("entry still present after Remove")
	}
	if _, ok := manifest.Remove("10.0.100"); ok {
		t.Error("Remove succeeded for a missing entry")
	}
}

func TestManifestRejectsUnsafeOrInvalidEntries(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	hash := sha512.Sum512([]byte("archive"))

	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{name: "traversal", entry: Entry{Version: "10.0.100", RID: "linux-x64", Root: "../escape", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: FormatSHA512(hash[:])}, want: "escapes"},
		{name: "absolute", entry: Entry{Version: "10.0.100", RID: "linux-x64", Root: "/tmp/escape", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: FormatSHA512(hash[:])}, want: "relative"},
		{name: "missing hash", entry: Entry{Version: "10.0.100", RID: "linux-x64", Root: "10.0.100", SourceURL: "https://example.microsoft.com/sdk.tar.gz"}, want: "checksum"},
		{name: "root mismatch", entry: Entry{Version: "10.0.100", RID: "linux-x64", Root: "10.0.101", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: FormatSHA512(hash[:])}, want: "must match"},
		{name: "prerelease", entry: Entry{Version: "10.0.100-rc.1", RID: "linux-x64", Root: "10.0.100-rc.1", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: FormatSHA512(hash[:])}, want: "stable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(Manifest{SchemaVersion: 1, Installs: []Entry{test.entry}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(manifestPath, managedRoot); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestManifestRejectsDuplicateAndNewerSchema(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	hash := sha512.Sum512([]byte("archive"))
	entry := testEntry("10.0.100", hash[:])

	data, err := json.Marshal(Manifest{SchemaVersion: 1, Installs: []Entry{entry, entry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(manifestPath, managedRoot); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}

	data, err = json.Marshal(Manifest{SchemaVersion: ManifestSchemaVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(manifestPath, managedRoot); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("schema error = %v", err)
	}
}

func testEntry(version string, hash []byte) Entry {
	return Entry{
		Version:     version,
		RID:         "linux-x64",
		Root:        version,
		SourceURL:   "https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz",
		SHA512:      FormatSHA512(hash),
		InstalledAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
	}
}
