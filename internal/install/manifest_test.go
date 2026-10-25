package install

import (
	"crypto/sha512"
	"encoding/json"
	"fmt"
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
	found := loaded.Find(ProductSDK, "10.0.100")
	if found == nil {
		t.Fatal("entry not found")
	}
	if found.Product != ProductSDK || found.SHA512 != FormatSHA512(hash[:]) || found.RID != "linux-x64" {
		t.Errorf("entry = %+v", found)
	}
	if len(found.Artifacts) != 1 || found.Artifacts[0].Name != "dotnet-sdk-10.0.100-linux-x64.tar.gz" || found.Artifacts[0].URL != found.SourceURL {
		t.Errorf("artifacts = %+v", found.Artifacts)
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
	removed, ok := manifest.Remove(ProductSDK, "10.0.100")
	if !ok || removed.Version != "10.0.100" {
		t.Errorf("Remove() = %+v, %v", removed, ok)
	}
	if manifest.Find(ProductSDK, "10.0.100") != nil {
		t.Error("entry still present after Remove")
	}
	if _, ok := manifest.Remove(ProductSDK, "10.0.100"); ok {
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

func TestManifestMigratesSchema1ToSchema2(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	sum := sha512.Sum512([]byte("archive"))
	hash := FormatSHA512(sum[:])
	legacy := fmt.Sprintf(`{"schemaVersion":1,"installs":[{"version":"10.0.100","rid":"linux-x64","root":"10.0.100","sourceUrl":"https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz","sha512":%q,"installedAt":"2026-01-02T03:04:05Z"}]}`, hash)
	if err := os.WriteFile(manifestPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatalf("schema 1 load failed: %v", err)
	}
	if manifest.SchemaVersion != 1 {
		t.Errorf("loaded schema version = %d, want 1 until saved", manifest.SchemaVersion)
	}
	entry := manifest.Find(ProductSDK, "10.0.100")
	if entry == nil {
		t.Fatal("migrated SDK entry not found")
	}
	if entry.Product != ProductSDK || !entry.InstalledAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Errorf("migrated entry = %+v", entry)
	}

	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schemaVersion": 2`) {
		t.Errorf("saved manifest did not record schema 2:\n%s", raw)
	}
	reloaded, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SchemaVersion != ManifestSchemaVersion || reloaded.Find(ProductSDK, "10.0.100") == nil {
		t.Errorf("reloaded manifest = %+v", reloaded)
	}
}

func TestManifestSDKAndRuntimeCoexist(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	hash := sha512.Sum512([]byte("archive"))
	manifest := NewManifest(managedRoot)

	sdkEntry := testEntry("10.0.12", hash[:])
	runtimeEntry := testRuntimeEntry("10.0.12", hash[:])
	if err := manifest.Add(sdkEntry); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Add(runtimeEntry); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Add(runtimeEntry); err == nil || !strings.Contains(err.Error(), "already recorded") {
		t.Fatalf("duplicate runtime Add error = %v", err)
	}

	sdk := manifest.Find(ProductSDK, "10.0.12")
	runtime := manifest.Find(ProductRuntime, "10.0.12")
	if sdk == nil || runtime == nil {
		t.Fatalf("expected both products for 10.0.12, sdk=%v runtime=%v", sdk, runtime)
	}
	if sdk.Root != "10.0.12" || runtime.Root != "runtimes/10.0.12" {
		t.Errorf("roots = %q / %q", sdk.Root, runtime.Root)
	}
	if len(runtime.Frameworks) != 2 {
		t.Errorf("runtime frameworks = %v", runtime.Frameworks)
	}
	if len(runtime.Artifacts) != 0 {
		t.Errorf("runtime artifacts = %v", runtime.Artifacts)
	}

	persisted, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := persisted.Add(sdkEntry); err != nil {
		t.Fatal(err)
	}
	if err := persisted.Add(runtimeEntry); err != nil {
		t.Fatal(err)
	}
	if err := persisted.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded := loaded.Find(ProductRuntime, "10.0.12"); reloaded == nil || len(reloaded.Frameworks) != 2 {
		t.Errorf("runtime entry after round trip = %+v", reloaded)
	}
	if reloaded := loaded.Find(ProductSDK, "10.0.12"); reloaded == nil {
		t.Error("SDK entry after round trip missing")
	}
	if !loaded.Owns(filepath.Join(managedRoot, "10.0.12"), "10.0.12") {
		t.Error("SDK root not owned")
	}
	if !loaded.Owns(filepath.Join(managedRoot, "runtimes", "10.0.12"), "10.0.12") {
		t.Error("runtime root not owned")
	}
	roots := loaded.ManagedRoots()
	if len(roots) != 2 {
		t.Fatalf("ManagedRoots = %v, want 2", roots)
	}
}

func createRuntimeRootWithFrameworks(t *testing.T, manifestPath, managedRoot, version string, frameworks []string) {
	t.Helper()
	manifest, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	entry := testRuntimeEntry(version, hash[:])
	entry.Frameworks = frameworks
	if err := manifest.Add(entry); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRuntimeRootAndFieldValidation(t *testing.T) {
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	manifestPath := filepath.Join(base, "installs.json")
	sum := sha512.Sum512([]byte("archive"))
	hash := FormatSHA512(sum[:])

	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{name: "missing product", entry: Entry{Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}, want: "product"},
		{name: "sdk root on runtime", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}, want: "runtimes/<version>"},
		{name: "other runtime version root", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.13", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}, want: "runtimes/<version>"},
		{name: "runtime traversal", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/../../escape", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}, want: "escapes"},
		{name: "frameworks on sdk", entry: Entry{Product: ProductSDK, Version: "10.0.100", RID: "linux-x64", Root: "10.0.100", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: hash, Frameworks: []string{"Microsoft.NETCore.App"}}, want: "only valid for runtime"},
		{name: "empty framework", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash, Frameworks: []string{"  "}}, want: "framework is empty"},
		{name: "artifact missing name", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash, Artifacts: []ArtifactRecord{{URL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}}}, want: "artifact name is empty"},
		{name: "artifact missing url", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash, Artifacts: []ArtifactRecord{{Name: "runtime.tar.gz", SHA512: hash}}}, want: "URL is empty"},
		{name: "artifact bad checksum", entry: Entry{Product: ProductRuntime, Version: "10.0.12", RID: "linux-x64", Root: "runtimes/10.0.12", SourceURL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash, Artifacts: []ArtifactRecord{{Name: "runtime.tar.gz", URL: "https://example.microsoft.com/runtime.tar.gz", SHA512: "abc"}}}, want: "invalid SHA-512"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(Manifest{SchemaVersion: 2, Installs: []Entry{test.entry}})
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

	valid := Entry{
		Product:    ProductRuntime,
		Version:    "10.0.12",
		RID:        "linux-x64",
		Root:       "runtimes/10.0.12",
		SourceURL:  "https://example.microsoft.com/runtime.tar.gz",
		SHA512:     hash,
		Artifacts:  []ArtifactRecord{{Name: "dotnet-runtime-10.0.12-linux-x64.tar.gz", URL: "https://example.microsoft.com/runtime.tar.gz", SHA512: hash}},
		Frameworks: []string{"Microsoft.NETCore.App", "Microsoft.NETCore.App", "Microsoft.AspNetCore.App"},
	}
	data, err := json.Marshal(Manifest{SchemaVersion: 2, Installs: []Entry{valid}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(manifestPath, managedRoot)
	if err != nil {
		t.Fatalf("valid runtime entry rejected: %v", err)
	}
	entry := loaded.Find(ProductRuntime, "10.0.12")
	if entry == nil {
		t.Fatal("valid runtime entry missing")
	}
	if len(entry.Frameworks) != 2 {
		t.Errorf("frameworks = %v, want deduplicated", entry.Frameworks)
	}
}

func TestManifestAddRejectsInvalidProduct(t *testing.T) {
	manifest := NewManifest(filepath.Join(t.TempDir(), "dotnet"))
	sum := sha512.Sum512([]byte("archive"))
	hash := FormatSHA512(sum[:])
	err := manifest.Add(Entry{Product: "toolchain", Version: "10.0.100", RID: "linux-x64", Root: "10.0.100", SourceURL: "https://example.microsoft.com/sdk.tar.gz", SHA512: hash})
	if err == nil || !strings.Contains(err.Error(), "product") {
		t.Fatalf("error = %v, want product kind rejection", err)
	}
}

func testEntry(version string, hash []byte) Entry {
	return Entry{
		Product:   ProductSDK,
		Version:   version,
		RID:       "linux-x64",
		Root:      version,
		SourceURL: "https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz",
		SHA512:    FormatSHA512(hash),
		Artifacts: []ArtifactRecord{{
			Name:   fmt.Sprintf("dotnet-sdk-%s-linux-x64.tar.gz", version),
			URL:    "https://builds.dotnet.microsoft.com/dotnet/sdk.tar.gz",
			SHA512: FormatSHA512(hash),
		}},
		InstalledAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
	}
}

func testRuntimeEntry(version string, hash []byte) Entry {
	return Entry{
		Product:     ProductRuntime,
		Version:     version,
		RID:         "linux-x64",
		Root:        "runtimes/" + version,
		SourceURL:   "https://builds.dotnet.microsoft.com/dotnet/runtime.tar.gz",
		SHA512:      FormatSHA512(hash),
		Frameworks:  []string{"Microsoft.NETCore.App", "Microsoft.AspNetCore.App"},
		InstalledAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
	}
}
