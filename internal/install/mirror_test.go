package install

import (
	"context"
	"crypto/sha512"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func parseTestURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func TestMirrorArchiveHostRewritesBuildsHost(t *testing.T) {
	target, _ := parseTestURL("https://builds.dotnet.microsoft.com/dotnet/Sdk/10.0.112/x.tar.gz")
	mirrorArchiveHost(target)
	if target.Hostname() != "dotnetcli.blob.core.windows.net" {
		t.Fatalf("host = %s", target.Hostname())
	}
	if target.Path != "/dotnet/Sdk/10.0.112/x.tar.gz" {
		t.Fatalf("path = %s", target.Path)
	}

	nonMicrosoft, _ := parseTestURL("https://example.com/x.tar.gz")
	mirrorArchiveHost(nonMicrosoft)
	if nonMicrosoft.Hostname() != "example.com" {
		t.Fatalf("host changed: %s", nonMicrosoft.Hostname())
	}
}

func TestArchiveMirrorURLsOrder(t *testing.T) {
	got := archiveMirrorURLs("https://builds.dotnet.microsoft.com/dotnet/Sdk/1/x.tar.gz")
	if len(got) != len(archiveMirrorHosts) {
		t.Fatalf("got %v", got)
	}
	for index, host := range archiveMirrorHosts {
		parsed, err := url.Parse(got[index])
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Hostname() != host {
			t.Fatalf("mirror %d = %s, want %s", index, parsed.Hostname(), host)
		}
		if parsed.Path != "/dotnet/Sdk/1/x.tar.gz" {
			t.Fatalf("mirror %d path = %s", index, parsed.Path)
		}
	}

	unknown := archiveMirrorURLs("https://example.com/x.tar.gz")
	if len(unknown) != 1 || unknown[0] != "https://example.com/x.tar.gz" {
		t.Fatalf("unknown host got %v", unknown)
	}
}

func TestMetadataMirrorFallback(t *testing.T) {
	var badHits atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	var goodHits atomic.Int32
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goodHits.Add(1)
		switch r.URL.Path {
		case "/releases-index.json":
			_, _ = w.Write([]byte(requestTestIndex))
		case "/10.0/releases.json":
			_, _ = w.Write([]byte(`{"releases":[{"sdk":{"version":"10.0.112","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://dotnetcli.blob.core.windows.net/dotnet/Sdk/10.0.112/dotnet-sdk-linux-x64.tar.gz","hash":"` + strings.Repeat("a", 128) + `"}]}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer good.Close()

	original := metadataBaseURLCandidates
	metadataBaseURLCandidates = []string{bad.URL, good.URL}
	defer func() { metadataBaseURLCandidates = original }()

	client := &ReleaseClient{HTTPClient: good.Client(), ValidateURL: allowTestURL}
	index, err := client.index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Channels) == 0 {
		t.Fatal("no channels in fallback index")
	}
	if badHits.Load() == 0 || goodHits.Load() == 0 {
		t.Fatalf("bad hits = %d, good hits = %d", badHits.Load(), goodHits.Load())
	}

	metadata, err := client.channel(context.Background(), "10.0")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.findSDK("10.0.112") == nil {
		t.Fatal("channel metadata was not served by the fallback mirror")
	}
}

func TestArchiveDownloadFallsBackOnServerError(t *testing.T) {
	body := []byte("archive contents")
	expected := sha512.Sum512(body)

	var firstHits, secondHits atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		_, _ = w.Write(body)
	}))
	defer second.Close()

	original := archiveMirrorHosts
	archiveMirrorHosts = []string{testHost(first.URL), testHost(second.URL)}
	defer func() { archiveMirrorHosts = original }()

	installer := Installer{HTTPClient: second.Client(), ValidateURL: allowTestURL}
	name, err := installer.download(context.Background(),
		first.URL+"/dotnet/Sdk/1/x.tar.gz",
		expected[:], t.TempDir(), nil, "SDK")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(name)
	if firstHits.Load() == 0 || secondHits.Load() == 0 {
		t.Fatalf("first hits = %d, second hits = %d", firstHits.Load(), secondHits.Load())
	}
}

func TestArchiveChecksumMismatchDoesNotFallBack(t *testing.T) {
	expected := sha512.Sum512([]byte("good archive"))

	var firstHits, secondHits atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits.Add(1)
		_, _ = w.Write([]byte("corrupt archive"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		_, _ = w.Write([]byte("good archive"))
	}))
	defer second.Close()

	original := archiveMirrorHosts
	archiveMirrorHosts = []string{testHost(first.URL), testHost(second.URL)}
	defer func() { archiveMirrorHosts = original }()

	installer := Installer{HTTPClient: second.Client(), ValidateURL: allowTestURL}
	_, err := installer.download(context.Background(),
		first.URL+"/dotnet/Sdk/1/x.tar.gz",
		expected[:], t.TempDir(), nil, "SDK")
	var mismatch *ChecksumMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if secondHits.Load() != 0 {
		t.Fatalf("checksum mismatch fell back to the second mirror (%d hits)", secondHits.Load())
	}
	if firstHits.Load() != 1 {
		t.Fatalf("first mirror hits = %d, want 1", firstHits.Load())
	}
}

func testHost(serverURL string) string {
	parsed, _ := url.Parse(serverURL)
	return parsed.Host
}

func TestReleaseTypeParsingAndStableAliasSelection(t *testing.T) {
	index := `{"releases-index":[
		{"channel-version":"11.0","latest-sdk":"11.0.100-rc.1","support-phase":"go-live","release-type":"sts"},
		{"channel-version":"10.0","latest-sdk":"10.0.112","support-phase":"active","release-type":"lts"}
	]}`
	channels := map[string]string{
		"/10.0/releases.json": `{"releases":[{"sdk":{"version":"10.0.112","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://dotnetcli.blob.core.windows.net/sdk.tar.gz","hash":"` + strings.Repeat("a", 128) + `"}]}}]}`,
	}
	server := newRequestServer(t, index, channels)
	defer server.Close()
	client := newRequestClient(server)

	channel, err := client.channelForAlias(context.Background(), VersionRequest{Kind: RequestLatest, Requested: "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if channel.ChannelVersion != "10.0" {
		t.Fatalf("latest channel = %s, want 10.0", channel.ChannelVersion)
	}
	if !channel.channelTypeLTS() {
		t.Fatal("release-type was not parsed as lts")
	}

	request, _ := ParseVersionRequest("latest", false)
	version, err := client.ResolveVersion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if version.String() != "10.0.112" {
		t.Fatalf("latest resolved = %s, want 10.0.112", version)
	}

	ltsRequest, _ := ParseVersionRequest("lts", false)
	ltsVersion, err := client.ResolveVersion(context.Background(), ltsRequest)
	if err != nil {
		t.Fatal(err)
	}
	if ltsVersion.String() != "10.0.112" {
		t.Fatalf("lts resolved = %s, want 10.0.112", ltsVersion)
	}
}

func TestRealMetadataOptional(t *testing.T) {
	if os.Getenv("HILT_TEST_REAL_METADATA") != "1" {
		t.Skip("set HILT_TEST_REAL_METADATA=1 to run")
	}
	client := &ReleaseClient{}
	index, err := client.index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Channels) == 0 {
		t.Fatal("no channels")
	}
	var channel releaseIndexChannel
	for _, candidate := range index.Channels {
		if candidate.ChannelVersion == "10.0" {
			channel = candidate
			break
		}
	}
	if channel.ChannelVersion == "" {
		t.Fatal("10.0 channel missing from release index")
	}
	if channel.Type == "" {
		t.Fatal("release-type was not parsed")
	}

	metadata, err := client.channel(context.Background(), "10.0")
	if err != nil {
		t.Fatal(err)
	}
	sdk := metadata.findSDK("10.0.112")
	if sdk == nil || len(sdk.Files) == 0 {
		t.Fatal("SDK 10.0.112 missing or has no files")
	}
	rid := runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOARCH == "amd64" {
		rid = runtime.GOOS + "-x64"
	}
	artifact, err := client.ResolveSDK(context.Background(), "10.0.112", rid)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(artifact.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "dotnetcli.blob.core.windows.net" {
		t.Fatalf("live metadata URL was not mirrored: %s", artifact.URL)
	}
}
