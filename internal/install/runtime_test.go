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

type runtimeChannelSpec struct {
	channel     string
	runtime     string
	aspnetcore  string
	runtimeHash string
	aspHash     string
}

type runtimeServer struct {
	*httptest.Server
	channels       []runtimeChannelSpec
	netcoreArchive []byte
	aspArchive     []byte

	mu              sync.Mutex
	archiveRequests map[string]int
}

func newRuntimeServer(t *testing.T, channels []runtimeChannelSpec, netcoreArchive, aspArchive []byte) *runtimeServer {
	t.Helper()
	server := &runtimeServer{
		channels:        channels,
		netcoreArchive:  netcoreArchive,
		aspArchive:      aspArchive,
		archiveRequests: make(map[string]int),
	}
	server.Server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/archives/") {
			server.mu.Lock()
			server.archiveRequests[request.URL.Path]++
			server.mu.Unlock()
			switch request.URL.Path {
			case "/archives/netcore.tar.gz":
				_, _ = writer.Write(server.netcoreArchive)
			case "/archives/aspnetcore.tar.gz":
				_, _ = writer.Write(server.aspArchive)
			default:
				http.NotFound(writer, request)
			}
			return
		}
		for _, spec := range server.channels {
			if request.URL.Path == "/"+spec.channel+"/releases.json" {
				_, _ = writer.Write([]byte(server.channelMetadata(spec)))
				return
			}
		}
		http.NotFound(writer, request)
	}))
	return server
}

func (s *runtimeServer) channelMetadata(spec runtimeChannelSpec) string {
	netcoreHash := spec.runtimeHash
	if netcoreHash == "" {
		netcoreSum := checksum(s.netcoreArchive)
		netcoreHash = FormatSHA512(netcoreSum[:])
	}
	aspHash := spec.aspHash
	if aspHash == "" {
		aspSum := checksum(s.aspArchive)
		aspHash = FormatSHA512(aspSum[:])
	}
	products := make([]string, 0, 2)
	if spec.runtime != "" {
		products = append(products, fmt.Sprintf(`"runtime":{"version":%q,"files":[{"name":"dotnet-runtime-%s-linux-x64.tar.gz","rid":"linux-x64","url":%q,"hash":%q}]}`,
			spec.runtime, spec.runtime, s.URL+"/archives/netcore.tar.gz", netcoreHash))
	}
	if spec.aspnetcore != "" {
		products = append(products, fmt.Sprintf(`"aspnetcore-runtime":{"version":%q,"files":[{"name":"dotnet-aspnetcore-runtime-%s-linux-x64.tar.gz","rid":"linux-x64","url":%q,"hash":%q}]}`,
			spec.aspnetcore, spec.aspnetcore, s.URL+"/archives/aspnetcore.tar.gz", aspHash))
	}
	releaseVersion := spec.runtime
	if releaseVersion == "" {
		releaseVersion = spec.aspnetcore
	}
	return fmt.Sprintf(`{"releases":[{"release-version":%q,%s}]}`, releaseVersion, strings.Join(products, ","))
}

func (s *runtimeServer) archiveRequestCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.archiveRequests[path]
}

func checksum(data []byte) [sha512.Size]byte {
	return sha512.Sum512(data)
}

// runtimeTestInstaller builds an Installer pointed at an arbitrary metadata server.
func runtimeTestInstaller(t *testing.T, server *httptest.Server, runner process.Runner) Installer {
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

// runtimeTarGz builds a runtime archive containing host placeholder files and
// the requested shared framework directories.
func runtimeTarGz(t *testing.T, version string, frameworks []string, withHost bool) []byte {
	t.Helper()
	entries := make([]archiveEntry, 0, len(frameworks)+1)
	if withHost {
		entries = append(entries, archiveEntry{name: "dotnet", body: "#!/bin/sh\n", mode: 0o755, typeflag: tar.TypeReg})
	}
	for _, framework := range frameworks {
		entries = append(entries, archiveEntry{name: "shared/" + framework + "/" + version + "/", mode: 0o755, typeflag: tar.TypeDir})
	}
	archivePath := writeTarGz(t, entries)
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func aspnetcoreRunner() *fakeRunner {
	return &fakeRunner{
		version:         "10.0.12",
		runtimeVersions: []string{"Microsoft.NETCore.App", "Microsoft.AspNetCore.App"},
	}
}

func netcoreRunner() *fakeRunner {
	return &fakeRunner{
		version:         "10.0.12",
		runtimeVersions: []string{"Microsoft.NETCore.App"},
	}
}

func addRuntimeEntryFrameworks(t *testing.T, installer Installer, version string, frameworks []string) {
	t.Helper()
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	hash := checksum([]byte("archive"))
	if err := manifest.Add(Entry{
		Product:     ProductRuntime,
		Version:     version,
		RID:         "linux-x64",
		Root:        "runtimes/" + version,
		SourceURL:   "https://builds.dotnet.microsoft.com/dotnet/runtime.tar.gz",
		SHA512:      FormatSHA512(hash[:]),
		Artifacts:   []ArtifactRecord{{Name: "dotnet-runtime-" + version + "-linux-x64.tar.gz", URL: "https://builds.dotnet.microsoft.com/dotnet/runtime.tar.gz", SHA512: FormatSHA512(hash[:])}},
		Frameworks:  append([]string(nil), frameworks...),
		InstalledAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(); err != nil {
		t.Fatal(err)
	}
}

func createRuntimeRootFrameworks(t *testing.T, installer Installer, version string, frameworks []string) string {
	t.Helper()
	root := RuntimeRoot(installer.ManagedRoot, version)
	for _, framework := range frameworks {
		if err := os.MkdirAll(filepath.Join(root, "shared", framework, version), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "dotnet"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRuntimePlanAndInstallNetCore(t *testing.T) {
	channels := []runtimeChannelSpec{{channel: "10.0", runtime: "10.0.12", aspnetcore: "10.0.12"}}
	server := newRuntimeServer(t, channels,
		runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkNetCore}, true),
		runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore}, true))
	defer server.Close()
	installer := runtimeTestInstaller(t, server.Server, netcoreRunner())

	plan, err := installer.PlanRuntime(context.Background(), "10.0.12", FrameworkNetCore, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AlreadyInstalled || plan.Version != "10.0.12" || plan.RID != "linux-x64" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Framework != "netcore" || len(plan.Frameworks) != 1 || plan.Frameworks[0] != RuntimeFrameworkNetCore {
		t.Errorf("plan frameworks = %+v", plan)
	}
	if plan.Root != RuntimeRoot(installer.ManagedRoot, "10.0.12") {
		t.Errorf("root = %q", plan.Root)
	}
	if len(plan.Artifacts) != 1 || plan.Artifacts[0].Product != RuntimeArchiveRuntime {
		t.Errorf("artifacts = %+v", plan.Artifacts)
	}

	var progress bytes.Buffer
	result, err := installer.InstallRuntime(context.Background(), plan, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyInstalled || result.InstalledAt.IsZero() {
		t.Errorf("result = %+v", result)
	}
	if result.Framework != "netcore" || len(result.Frameworks) != 1 || result.Frameworks[0] != RuntimeFrameworkNetCore {
		t.Errorf("result frameworks = %+v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Name != "dotnet-runtime-10.0.12-linux-x64.tar.gz" {
		t.Errorf("result artifacts = %+v", result.Artifacts)
	}
	if !strings.Contains(result.VerifiedWith, ".staging") || !strings.HasSuffix(result.VerifiedWith, "/dotnet") {
		t.Errorf("verifiedWith = %q, want staging dotnet host", result.VerifiedWith)
	}
	if _, err := os.Stat(filepath.Join(result.Root, "shared", RuntimeFrameworkNetCore, "10.0.12")); err != nil {
		t.Fatalf("published shared framework missing: %v", err)
	}
	for _, want := range []string{"Verifying staged runtime", "Publishing to"} {
		if !strings.Contains(progress.String(), want) {
			t.Errorf("progress missing %q:\n%s", want, progress.String())
		}
	}

	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry := manifest.Find(ProductRuntime, "10.0.12")
	if entry == nil || entry.Root != "runtimes/10.0.12" || len(entry.Frameworks) != 1 {
		t.Errorf("manifest entry = %+v", entry)
	}

	// Idempotency: re-plan and re-install without downloading again.
	before := server.archiveRequestCount("/archives/netcore.tar.gz")
	again, err := installer.PlanRuntime(context.Background(), "10.0.12", FrameworkNetCore, false)
	if err != nil {
		t.Fatal(err)
	}
	if !again.AlreadyInstalled {
		t.Fatalf("second plan = %+v", again)
	}
	second, err := installer.InstallRuntime(context.Background(), again, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyInstalled {
		t.Errorf("second result = %+v", second)
	}
	if after := server.archiveRequestCount("/archives/netcore.tar.gz"); after != before {
		t.Errorf("idempotent install downloaded archive again: %d -> %d", before, after)
	}
}

func TestRuntimeInstallASPNETCoreVerifiesBothFrameworks(t *testing.T) {
	channels := []runtimeChannelSpec{{channel: "10.0", runtime: "10.0.12", aspnetcore: "10.0.12"}}
	server := newRuntimeServer(t, channels,
		runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkNetCore}, true),
		runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore}, true))
	defer server.Close()
	runner := aspnetcoreRunner()
	installer := runtimeTestInstaller(t, server.Server, runner)

	for _, flavor := range []FrameworkFlavor{FrameworkASPNETCore, FrameworkAll} {
		plan, err := installer.PlanRuntime(context.Background(), "10.0.12", flavor, false)
		if err != nil {
			t.Fatal(err)
		}
		if plan.AlreadyInstalled {
			continue
		}
		result, err := installer.InstallRuntime(context.Background(), plan, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Frameworks) != 2 {
			t.Errorf("flavor %s result = %+v", flavor, result)
		}
	}

	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry := manifest.Find(ProductRuntime, "10.0.12")
	if entry == nil || len(entry.Frameworks) != 2 {
		t.Fatalf("manifest entry = %+v", entry)
	}
	calls := runner.callList()
	if len(calls) == 0 || calls[0].Args[0] != "--list-runtimes" {
		t.Fatalf("runner calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Dir, ".staging") {
		t.Errorf("verification dir = %q, want staging", calls[0].Dir)
	}
	if !contains(calls[0].Env, "DOTNET_ROOT="+calls[0].Dir) || !contains(calls[0].Env, "DOTNET_MULTILEVEL_LOOKUP=0") {
		t.Errorf("verification env = %v", calls[0].Env)
	}
}

func TestRuntimeSecondArtifactChecksumFailureLeavesNoPublish(t *testing.T) {
	netcoreArchive := runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkNetCore}, true)
	aspArchive := runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore}, true)
	channels := []runtimeChannelSpec{{channel: "10.0", runtime: "10.0.12", aspnetcore: "10.0.12"}}
	server := newRuntimeServer(t, channels, netcoreArchive, aspArchive)
	defer server.Close()
	installer := runtimeTestInstaller(t, server.Server, aspnetcoreRunner())

	wrongHash := FormatSHA512(make([]byte, sha512.Size))
	netcoreSum := checksum(netcoreArchive)
	plan := RuntimePlan{
		Version:    "10.0.12",
		RID:        "linux-x64",
		Framework:  string(FrameworkAll),
		Frameworks: []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore},
		Root:       RuntimeRoot(installer.ManagedRoot, "10.0.12"),
		Artifacts: []RuntimeArtifact{
			{Product: RuntimeArchiveRuntime, URL: server.URL + "/archives/netcore.tar.gz", SHA512: FormatSHA512(netcoreSum[:])},
			{Product: RuntimeArchiveASPNETCore, URL: server.URL + "/archives/aspnetcore.tar.gz", SHA512: wrongHash},
		},
	}

	_, err := installer.InstallRuntime(context.Background(), plan, io.Discard)
	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) || !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if _, statErr := os.Stat(plan.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination exists after failed install: %v", statErr)
	}
	manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Installs) != 0 {
		t.Errorf("manifest entries = %d, want 0", len(manifest.Installs))
	}
}

func TestRuntimeCancellationLeavesNoPublish(t *testing.T) {
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
		Runner:       netcoreRunner(),
	}
	hash := checksum([]byte("archive"))
	plan := RuntimePlan{
		Version:    "10.0.12",
		RID:        "linux-x64",
		Framework:  string(FrameworkNetCore),
		Frameworks: []string{RuntimeFrameworkNetCore},
		Root:       RuntimeRoot(managedRoot, "10.0.12"),
		Artifacts: []RuntimeArtifact{{
			Product: RuntimeArchiveRuntime,
			URL:     server.URL + "/archives/netcore.tar.gz",
			SHA512:  FormatSHA512(hash[:]),
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_, err := installer.InstallRuntime(ctx, plan, io.Discard)
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

func TestRuntimeRemove(t *testing.T) {
	t.Run("owned", func(t *testing.T) {
		installer := testRemovalInstaller(t)
		root := createRuntimeRootFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore})
		addRuntimeEntryFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore})

		removal, err := installer.RemoveRuntime(context.Background(), "10.0.12")
		if err != nil {
			t.Fatal(err)
		}
		if removal.Stale || removal.Version != "10.0.12" || removal.Root != root || removal.Product != ProductRuntime {
			t.Errorf("removal = %+v", removal)
		}
		if _, statErr := os.Stat(root); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("root still exists after removal: %v", statErr)
		}
		manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Find(ProductRuntime, "10.0.12") != nil {
			t.Error("manifest entry still present")
		}
	})

	t.Run("stale", func(t *testing.T) {
		installer := testRemovalInstaller(t)
		addRuntimeEntryFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkNetCore})
		_, err := installer.RemoveRuntime(context.Background(), "10.0.12")
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Find(ProductRuntime, "10.0.12") != nil {
			t.Error("stale manifest entry still present")
		}
	})

	t.Run("unowned", func(t *testing.T) {
		installer := testRemovalInstaller(t)
		root := createRuntimeRootFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkNetCore})
		_, err := installer.RemoveRuntime(context.Background(), "10.0.12")
		var notOwned *NotOwnedError
		if !errors.As(err, &notOwned) {
			t.Fatalf("error = %v, want not owned", err)
		}
		if _, statErr := os.Stat(root); statErr != nil {
			t.Errorf("unowned root removed: %v", statErr)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		installer := testRemovalInstaller(t)
		_, err := installer.RemoveRuntime(context.Background(), "10.0.12")
		var notInstalled *NotInstalledError
		if !errors.As(err, &notInstalled) {
			t.Fatalf("error = %v, want not installed", err)
		}
	})

	t.Run("sdk version is not a runtime", func(t *testing.T) {
		installer := testRemovalInstaller(t)
		createInstalledRoot(t, installer, "10.0.100")
		addManifestEntry(t, installer, "10.0.100")

		_, err := installer.RemoveRuntime(context.Background(), "10.0.100")
		var notInstalled *NotInstalledError
		if !errors.As(err, &notInstalled) {
			t.Fatalf("error = %v, want not installed", err)
		}
		manifest, err := Load(installer.ManifestPath, installer.ManagedRoot)
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Find(ProductSDK, "10.0.100") == nil {
			t.Error("SDK entry removed by runtime removal")
		}
	})
}

func TestRuntimeFallbackHostUsedWhenStagingLacksDotnet(t *testing.T) {
	netcoreArchive := runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkNetCore}, false)
	channels := []runtimeChannelSpec{{channel: "10.0", runtime: "10.0.12", aspnetcore: "10.0.12"}}
	server := newRuntimeServer(t, channels, netcoreArchive, nil)
	defer server.Close()
	runner := netcoreRunner()
	installer := runtimeTestInstaller(t, server.Server, runner)
	installer.FindHost = func() (string, bool) {
		return "/usr/bin/known-host", true
	}

	plan, err := installer.PlanRuntime(context.Background(), "10.0.12", FrameworkNetCore, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.InstallRuntime(context.Background(), plan, io.Discard); err != nil {
		t.Fatal(err)
	}

	calls := runner.callList()
	if len(calls) == 0 || calls[0].Name != "/usr/bin/known-host" {
		t.Fatalf("runner calls = %+v, want fallback host", calls)
	}
	if !strings.Contains(calls[0].Dir, ".staging") {
		t.Errorf("verification dir = %q, want staging", calls[0].Dir)
	}
	if !contains(calls[0].Env, "DOTNET_ROOT="+calls[0].Dir) {
		t.Errorf("verification env = %v", calls[0].Env)
	}
}

func TestRuntimeVerifyFailsWithoutAnyHost(t *testing.T) {
	netcoreArchive := runtimeTarGz(t, "10.0.12", []string{RuntimeFrameworkNetCore}, false)
	channels := []runtimeChannelSpec{{channel: "10.0", runtime: "10.0.12", aspnetcore: "10.0.12"}}
	server := newRuntimeServer(t, channels, netcoreArchive, nil)
	defer server.Close()
	installer := runtimeTestInstaller(t, server.Server, netcoreRunner())
	installer.FindHost = func() (string, bool) {
		return "", false
	}

	plan, err := installer.PlanRuntime(context.Background(), "10.0.12", FrameworkNetCore, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = installer.InstallRuntime(context.Background(), plan, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no alternate host") {
		t.Fatalf("error = %v, want no alternate host", err)
	}
	if _, statErr := os.Stat(plan.Root); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination exists after failed verification: %v", statErr)
	}
}

func TestRuntimeCheckUpdates(t *testing.T) {
	channels := []runtimeChannelSpec{
		{channel: "10.0", runtime: "10.0.14", aspnetcore: "10.0.14"},
		{channel: "9.0", runtime: "9.0.99", aspnetcore: "9.0.99"},
	}
	server := newRuntimeServer(t, channels, nil, nil)
	defer server.Close()

	installer := testRemovalInstaller(t)
	installer.Metadata = &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}

	netcore := createRuntimeRootFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkNetCore})
	addRuntimeEntryFrameworks(t, installer, "10.0.12", []string{RuntimeFrameworkNetCore})
	aspnetcore := createRuntimeRootFrameworks(t, installer, "10.0.13", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore})
	addRuntimeEntryFrameworks(t, installer, "10.0.13", []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore})
	older := createRuntimeRootFrameworks(t, installer, "9.0.99", []string{RuntimeFrameworkNetCore})
	addRuntimeEntryFrameworks(t, installer, "9.0.99", []string{RuntimeFrameworkNetCore})

	updates, err := installer.CheckRuntimeUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %+v, want 3", updates)
	}
	if updates[0].Version != "10.0.13" || updates[1].Version != "10.0.12" || updates[2].Version != "9.0.99" {
		t.Errorf("update order = %v", []string{updates[0].Version, updates[1].Version, updates[2].Version})
	}
	byVersion := make(map[string]Update)
	for _, update := range updates {
		byVersion[update.Version] = update
	}
	if update := byVersion["10.0.12"]; !update.UpdateAvailable || update.LatestVersion != "10.0.14" || update.Root != netcore {
		t.Errorf("10.0.12 update = %+v", update)
	}
	if update := byVersion["10.0.13"]; !update.UpdateAvailable || update.LatestVersion != "10.0.14" || update.Root != aspnetcore {
		t.Errorf("10.0.13 update = %+v", update)
	}
	if update := byVersion["9.0.99"]; update.UpdateAvailable || update.LatestVersion != "9.0.99" || update.Root != older {
		t.Errorf("9.0.99 update = %+v", update)
	}

	// SDK updates must not be affected by runtime entries.
	sdkUpdates, err := installer.CheckUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sdkUpdates) != 0 {
		t.Errorf("sdk updates = %+v, want 0", sdkUpdates)
	}
}
