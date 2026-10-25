package install

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mohammedbakri123/hilt/internal/process"
)

type installServer struct {
	*httptest.Server
	version string
	rid     string
	archive []byte
	hash    string

	mu               sync.Mutex
	metadataRequests int
	archiveRequests  int
}

func newInstallServer(t *testing.T, version, rid string, archive []byte, hash string) *installServer {
	t.Helper()
	server := &installServer{version: version, rid: rid, archive: archive, hash: hash}
	server.Server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		channel := strings.Join(strings.Split(version, ".")[:2], ".")
		switch request.URL.Path {
		case "/" + channel + "/releases.json":
			server.mu.Lock()
			server.metadataRequests++
			server.mu.Unlock()
			checksum := server.hash
			if checksum == "" {
				sum := sha512.Sum512(server.archive)
				checksum = FormatSHA512(sum[:])
			}
			fmt.Fprintf(writer, `{"releases":[{"sdk":{"version":%q,"files":[{"name":"dotnet-sdk-%s-%s.tar.gz","rid":%q,"url":%q,"hash":%q}]}}]}`,
				server.version, server.version, server.rid, server.rid, server.URL+"/archive.tar.gz", checksum)
		case "/archive.tar.gz":
			server.mu.Lock()
			server.archiveRequests++
			server.mu.Unlock()
			_, _ = writer.Write(server.archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	return server
}

func (s *installServer) requestCounts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadataRequests, s.archiveRequests
}

type fakeRunner struct {
	version string
	mu      sync.Mutex
	calls   []process.Spec
}

func (f *fakeRunner) Run(_ context.Context, spec process.Spec) (process.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, spec)
	f.mu.Unlock()
	if len(spec.Args) == 0 {
		return process.Result{}, nil
	}
	switch spec.Args[0] {
	case "--list-sdks":
		return process.Result{Stdout: []byte(fmt.Sprintf("%s [%s]\n", f.version, spec.Dir))}, nil
	case "--info":
		return process.Result{Stdout: []byte("Architecture: x64\nRID: linux-x64\n")}, nil
	default:
		return process.Result{}, nil
	}
}

func (f *fakeRunner) Stream(ctx context.Context, spec process.Spec, _, _ io.Writer) (process.Result, error) {
	return f.Run(ctx, spec)
}

func (f *fakeRunner) callList() []process.Spec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]process.Spec(nil), f.calls...)
}

func testInstaller(t *testing.T, server *installServer, runner process.Runner) Installer {
	t.Helper()
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	return Installer{
		ManagedRoot:  managedRoot,
		ManifestPath: filepath.Join(base, "installs.json"),
		LockPath:     filepath.Join(base, "installs.lock"),
		Metadata: &ReleaseClient{
			BaseURL:     server.URL,
			HTTPClient:  server.Client(),
			ValidateURL: allowTestURL,
		},
		Runner:      runner,
		HTTPClient:  server.Client(),
		ValidateURL: allowTestURL,
		Now: func() time.Time {
			return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
		},
	}
}

func TestInstallerPlanInstallAndIdempotency(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{
		{name: "dotnet", body: "#!/bin/sh\n", mode: 0o755, typeflag: tar.TypeReg},
		{name: "sdk/10.0.100/", mode: 0o755, typeflag: tar.TypeDir},
	})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, "")
	defer server.Close()
	runner := &fakeRunner{version: "10.0.100"}
	installer := testInstaller(t, server, runner)

	plan, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	if plan.AlreadyInstalled || plan.Version != "10.0.100" || plan.RID != "linux-x64" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Root != filepath.Join(installer.ManagedRoot, "10.0.100") {
		t.Errorf("root = %q", plan.Root)
	}

	var progress bytes.Buffer
	result, err := installer.Install(context.Background(), plan, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyInstalled || result.InstalledAt.IsZero() {
		t.Errorf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Root, "dotnet")); err != nil {
		t.Fatalf("published dotnet missing: %v", err)
	}
	for _, want := range []string{"Verified SHA-512", "Verifying staged toolchain", "Publishing to"} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("progress missing %q:\n%s", want, progress.String())
		}
	}

	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Owns(result.Root, "10.0.100") {
		t.Error("published root is not owned")
	}

	calls := runner.callList()
	if len(calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(calls))
	}
	if !strings.Contains(calls[0].Dir, ".staging") {
		t.Errorf("verification dir = %q, want staging", calls[0].Dir)
	}
	if !contains(calls[0].Env, "DOTNET_ROOT="+calls[0].Dir) || !contains(calls[0].Env, "DOTNET_MULTILEVEL_LOOKUP=0") {
		t.Errorf("verification env = %v", calls[0].Env)
	}

	_, archiveRequests := server.requestCounts()
	again, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyInstalled {
		t.Fatalf("second plan = %+v", again)
	}
	second, err := installer.Install(context.Background(), again, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyInstalled {
		t.Errorf("second result = %+v", second)
	}
	_, afterRequests := server.requestCounts()
	if afterRequests != archiveRequests {
		t.Errorf("idempotent install downloaded archive again: %d -> %d", archiveRequests, afterRequests)
	}
}

func TestInstallerInstallThenRemove(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{
		{name: "dotnet", body: "#!/bin/sh\n", mode: 0o755, typeflag: tar.TypeReg},
		{name: "sdk/10.0.100/", mode: 0o755, typeflag: tar.TypeDir},
	})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})

	plan, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	result, err := installer.Install(context.Background(), plan, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	removal, err := installer.Remove(context.Background(), result.Version)
	if err != nil {
		t.Fatal(err)
	}
	if removal.Stale || removal.Root != result.Root {
		t.Errorf("removal = %+v", removal)
	}
	if _, err := os.Stat(result.Root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("root still exists after removal: %v", err)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Find(result.Version) != nil || manifest.Owns(result.Root, result.Version) {
		t.Error("manifest still owns removed SDK")
	}
}

func TestInstallerChecksumMismatchLeavesNoInstall(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{{name: "dotnet", body: "binary", mode: 0o755, typeflag: tar.TypeReg}})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	wrongHash := FormatSHA512(make([]byte, sha512.Size))
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, wrongHash)
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})

	plan, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.Install(context.Background(), plan, io.Discard)
	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) || !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(plan.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination exists after failed install: %v", statErr)
	}
	if _, statErr := os.Stat(installer.ManagedRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("managed root exists after checksum failure: %v", statErr)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Installs) != 0 {
		t.Errorf("manifest entries = %d, want 0", len(manifest.Installs))
	}
}

func TestInstallerRefusesUnownedDestination(t *testing.T) {
	server := newInstallServer(t, "10.0.100", "linux-x64", nil, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})
	root := filepath.Join(installer.ManagedRoot, "10.0.100")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := installer.Plan(context.Background(), "10.0.100")
	var unowned *UnownedRootError
	if !errors.As(err, &unowned) {
		t.Fatalf("error = %v, want unowned root", err)
	}
}

func TestInstallerStaleManifest(t *testing.T) {
	server := newInstallServer(t, "10.0.100", "linux-x64", nil, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(testEntry("10.0.100", hash[:])); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
	_, err = installer.Plan(context.Background(), "10.0.100")
	var stale *StaleInstallError
	if !errors.As(err, &stale) {
		t.Fatalf("error = %v, want stale install", err)
	}
}

func TestInstallerRejectsUnsafeArchive(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{{name: "../escape", body: "bad", mode: 0o644, typeflag: tar.TypeReg}})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})
	plan, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), plan, io.Discard); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(plan.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination exists after unsafe archive: %v", statErr)
	}
}

func TestInstallerEnforcesDownloadLimit(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{{name: "dotnet", body: strings.Repeat("x", 128), mode: 0o755, typeflag: tar.TypeReg}})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})
	installer.MaxDownloadBytes = 16
	plan, err := installer.Plan(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(context.Background(), plan, io.Discard); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallerCancellationLeavesNoInstall(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	installer := Installer{
		ManagedRoot:  managedRoot,
		ManifestPath: filepath.Join(base, "installs.json"),
		LockPath:     filepath.Join(base, "installs.lock"),
		HTTPClient:   server.Client(),
		ValidateURL:  allowTestURL,
		Runner:       &fakeRunner{version: "10.0.100"},
	}
	hash := sha512.Sum512([]byte("archive"))
	plan := Plan{
		Version:   "10.0.100",
		RID:       "linux-x64",
		Root:      filepath.Join(managedRoot, "10.0.100"),
		SourceURL: server.URL + "/archive.tar.gz",
		SHA512:    FormatSHA512(hash[:]),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_, err := installer.Install(ctx, plan, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if _, statErr := os.Stat(plan.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination exists after cancellation: %v", statErr)
	}
	if _, statErr := os.Stat(managedRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("managed root exists after cancellation: %v", statErr)
	}
}

func TestInstallerResolvesMetadataWhenPlanLacksSource(t *testing.T) {
	archivePath := writeTarGz(t, []archiveEntry{{name: "dotnet", body: "binary", mode: 0o755, typeflag: tar.TypeReg}})
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	server := newInstallServer(t, "10.0.100", "linux-x64", archive, "")
	defer server.Close()
	installer := testInstaller(t, server, &fakeRunner{version: "10.0.100"})
	plan := Plan{
		Version: "10.0.100",
		RID:     "linux-x64",
		Root:    filepath.Join(installer.ManagedRoot, "10.0.100"),
	}
	result, err := installer.Install(context.Background(), plan, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceURL == "" {
		t.Error("expected resolved source URL")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func testRemovalInstaller(t *testing.T) Installer {
	t.Helper()
	base := t.TempDir()
	managedRoot := filepath.Join(base, "dotnet")
	return Installer{
		ManagedRoot:  managedRoot,
		ManifestPath: filepath.Join(base, "installs.json"),
		LockPath:     filepath.Join(base, "installs.lock"),
		Now: func() time.Time {
			return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
		},
	}
}

func createInstalledRoot(t *testing.T, installer Installer, version string) string {
	t.Helper()
	root := filepath.Join(installer.ManagedRoot, version)
	if err := os.MkdirAll(filepath.Join(root, "sdk", version), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func addManifestEntry(t *testing.T, installer Installer, version string) {
	t.Helper()
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha512.Sum512([]byte("archive"))
	if err := manifest.Add(testEntry(version, hash[:])); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerRemoveOwned(t *testing.T) {
	installer := testRemovalInstaller(t)
	version := "10.0.100"
	root := createInstalledRoot(t, installer, version)
	addManifestEntry(t, installer, version)

	removal, err := installer.Remove(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if removal.Stale || removal.Version != version || removal.Root != root || removal.RemovedAt.IsZero() {
		t.Errorf("removal = %+v", removal)
	}
	if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("root still exists after removal: %v", statErr)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Find(version) != nil {
		t.Error("manifest entry still present")
	}
}

func TestInstallerRemoveStaleEntryKeepsUnrelatedDirectories(t *testing.T) {
	installer := testRemovalInstaller(t)
	version := "10.0.100"
	addManifestEntry(t, installer, version)
	unrelated := filepath.Join(installer.ManagedRoot, "unrelated")
	if err := os.MkdirAll(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}

	removal, err := installer.Remove(context.Background(), version)
	if err != nil {
		t.Fatal(err)
	}
	if !removal.Stale || removal.Version != version {
		t.Errorf("removal = %+v", removal)
	}
	if _, statErr := os.Stat(unrelated); statErr != nil {
		t.Errorf("unrelated directory removed: %v", statErr)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Find(version) != nil {
		t.Error("stale manifest entry still present")
	}
}

func TestInstallerRemoveRefusesUnownedRoot(t *testing.T) {
	installer := testRemovalInstaller(t)
	version := "10.0.100"
	root := createInstalledRoot(t, installer, version)

	_, err := installer.Remove(context.Background(), version)
	var notOwned *NotOwnedError
	if !errors.As(err, &notOwned) {
		t.Fatalf("error = %v, want not owned", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Errorf("unowned root removed: %v", statErr)
	}
}

func TestInstallerRemoveNotInstalled(t *testing.T) {
	installer := testRemovalInstaller(t)
	_, err := installer.Remove(context.Background(), "10.0.100")
	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) {
		t.Fatalf("error = %v, want not installed", err)
	}
}

func TestInstallerRemoveFailureKeepsManifest(t *testing.T) {
	installer := testRemovalInstaller(t)
	version := "10.0.100"
	root := createInstalledRoot(t, installer, version)
	addManifestEntry(t, installer, version)
	installer.RemoveAll = func(string) error { return errors.New("busy") }

	_, err := installer.Remove(context.Background(), version)
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("error = %v, want busy", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Errorf("root was not restored after failed removal: %v", statErr)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Find(version) == nil {
		t.Error("manifest entry removed after failed removal")
	}
}

func TestInstallerCheckUpdates(t *testing.T) {
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests[request.URL.Path]++
		mu.Unlock()
		switch request.URL.Path {
		case "/10.0/releases.json":
			_, _ = writer.Write([]byte(`{"releases":[{"sdk":{"version":"10.0.100"}},{"sdk":{"version":"10.0.102"}}]}`))
		case "/9.0/releases.json":
			_, _ = writer.Write([]byte(`{"releases":[{"sdk":{"version":"9.0.100"}}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	installer := testRemovalInstaller(t)
	installer.Metadata = &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	for _, version := range []string{"10.0.100", "9.0.100"} {
		createInstalledRoot(t, installer, version)
		addManifestEntry(t, installer, version)
	}
	addManifestEntry(t, installer, "8.0.100")

	updates, err := installer.CheckUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 {
		t.Fatalf("updates = %+v, want 2 usable entries", updates)
	}
	byVersion := make(map[string]Update)
	for _, update := range updates {
		byVersion[update.Version] = update
	}
	if update := byVersion["10.0.100"]; update.LatestVersion != "10.0.102" || !update.UpdateAvailable {
		t.Errorf("10.0.100 update = %+v", update)
	}
	if update := byVersion["9.0.100"]; update.LatestVersion != "9.0.100" || update.UpdateAvailable {
		t.Errorf("9.0.100 update = %+v", update)
	}
	if _, ok := byVersion["8.0.100"]; ok {
		t.Error("stale entry was included in update check")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests["/10.0/releases.json"] != 1 || requests["/9.0/releases.json"] != 1 {
		t.Errorf("metadata requests = %v, want one per channel", requests)
	}
}
