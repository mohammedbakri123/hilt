package install

import (
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

func TestParseVersionRequest(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		allowPrerelease bool
		kind            RequestKind
		requested       string
		version         dotnet.Version
		wantErr         string
	}{
		{name: "exact", input: "10.0.112", kind: RequestExact, requested: "10.0.112", version: mustParseVersion(t, "10.0.112")},
		{name: "exact with revision", input: "10.0.112.1", kind: RequestExact, requested: "10.0.112.1", version: mustParseVersion(t, "10.0.112.1")},
		{name: "channel", input: "10.0", kind: RequestChannel, requested: "10.0", version: dotnet.Version{Major: 10, Minor: 0}},
		{name: "latest", input: "latest", kind: RequestLatest, requested: "latest"},
		{name: "latest case-insensitive", input: "Latest", kind: RequestLatest, requested: "Latest"},
		{name: "lts", input: "LTS", kind: RequestLTS, requested: "LTS"},
		{name: "trims spaces", input: " 10.0.112 ", kind: RequestExact, requested: "10.0.112", version: mustParseVersion(t, "10.0.112")},
		{name: "prerelease allowed", input: "10.0.112-rc.1", allowPrerelease: true, kind: RequestExact, requested: "10.0.112-rc.1", version: mustParseVersion(t, "10.0.112-rc.1")},
		{name: "prerelease rejected", input: "10.0.112-rc.1", wantErr: "not supported without --prerelease"},
		{name: "channel prerelease flag ok", input: "10.0", allowPrerelease: true, kind: RequestChannel, requested: "10.0", version: dotnet.Version{Major: 10, Minor: 0}},
		{name: "alias with prerelease reject", input: "latest", allowPrerelease: true, wantErr: "not supported with the latest alias"},
		{name: "lts with prerelease reject", input: "lts", allowPrerelease: true, wantErr: "not supported with the lts alias"},
		{name: "empty", input: "   ", wantErr: "empty"},
		{name: "single segment", input: "10", wantErr: "invalid version request"},
		{name: "malformed", input: "not-a.version", wantErr: "invalid version request"},
		{name: "unknown alias", input: "stable", wantErr: "invalid version request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := ParseVersionRequest(test.input, test.allowPrerelease)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if request.Kind != test.kind || request.Requested != test.requested {
				t.Fatalf("request = %+v", request)
			}
			if request.Version != test.version {
				t.Errorf("version = %+v, want %+v", request.Version, test.version)
			}
			if request.AllowPrerelease != test.allowPrerelease {
				t.Errorf("allowPrerelease = %v", request.AllowPrerelease)
			}
		})
	}
}

func mustParseVersion(t *testing.T, value string) dotnet.Version {
	t.Helper()
	parsed, err := dotnet.ParseVersion(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// requestServer serves a release index plus per-channel release metadata.
func newRequestServer(t *testing.T, indexJSON string, channels map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/releases-index.json" {
			if indexJSON == "" {
				http.NotFound(writer, request)
				return
			}
			_, _ = writer.Write([]byte(indexJSON))
			return
		}
		for path, body := range channels {
			if request.URL.Path == path {
				_, _ = writer.Write([]byte(body))
				return
			}
		}
		http.NotFound(writer, request)
	}))
	return server
}

func newRequestClient(server *httptest.Server) *ReleaseClient {
	return &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
}

const requestTestIndex = `{"releases-index":[
	{"channel-version":"10.0","latest-release":"10.0.0","latest-sdk":"10.0.112","latest-runtime":"10.0.12","support-phase":"active","type":"lts"},
	{"channel-version":"9.0","latest-sdk":"9.0.100","latest-runtime":"9.0.6","support-phase":"active","type":"sts"},
	{"channel-version":"8.0","latest-sdk":"8.0.100","latest-runtime":"8.0.1","support-phase":"eol","type":"lts"},
	{"channel-version":"11.0","latest-sdk":"11.0.1","latest-runtime":"11.0.2","support-phase":"preview","type":"preview"}
]}`

// sdkChannelJSON returns channel metadata where only the resolved stable and
// prerelease entries carry downloadable files.
func sdkChannelJSON(hash [sha512.Size]byte, stable, prerelease string) string {
	return fmt.Sprintf(`{"releases":[
		{"sdk":{"version":"10.0.100"}},
		{"sdks":[{"version":"10.0.101"},{"version":%q}]},
		{"sdk":{"version":%q,"files":[{"name":"dotnet-sdk-%s-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/sdk-%s.tar.gz","hash":"%s"}]}},
		{"sdk":{"version":%q,"files":[{"name":"dotnet-sdk-%s-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/sdk-%s.tar.gz","hash":"%s"}]}}
	]}`,
		prerelease,
		stable, stable, stable, FormatSHA512(hash[:]),
		prerelease, prerelease, prerelease, FormatSHA512(hash[:]))
}

func TestLatestAliasResolvesNewestActiveChannelDeterministically(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	channels := map[string]string{
		"/10.0/releases.json": sdkChannelJSON(hash, "10.0.102", "10.0.104-rc.1"),
		"/9.0/releases.json":  `{"releases":[{"sdk":{"version":"9.0.47"}}]}`,
	}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("latest", false)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		resolved, err := client.ResolveVersion(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.String() != "10.0.102" {
			t.Fatalf("attempt %d resolved = %s, want 10.0.102", attempt, resolved)
		}
	}
}

func TestLTSAliasResolvesVerifiedLTSChannel(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	channels := map[string]string{
		"/10.0/releases.json": sdkChannelJSON(hash, "10.0.102", "10.0.104-rc.1"),
		"/9.0/releases.json":  `{"releases":[{"sdk":{"version":"9.0.47"}}]}`,
	}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("lts", false)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveVersion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.String() != "10.0.102" {
		t.Fatalf("resolved = %s, want 10.0.102", resolved)
	}
}

func TestChannelRequestResolvesNewestStable(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	channels := map[string]string{"/10.0/releases.json": sdkChannelJSON(hash, "10.0.102", "10.0.104-rc.1")}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("10.0", false)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveVersion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.String() != "10.0.102" {
		t.Fatalf("resolved = %s, want 10.0.102", resolved)
	}
}

func TestChannelRequestResolvesPrereleaseWhenAllowed(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	channels := map[string]string{"/10.0/releases.json": sdkChannelJSON(hash, "10.0.102", "10.0.104-rc.1")}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("10.0", true)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveVersion(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.String() != "10.0.104-rc.1" {
		t.Fatalf("resolved = %s, want 10.0.104-rc.1", resolved)
	}
}

func TestLTSAliasUnsupportedWithoutVerifiedChannel(t *testing.T) {
	index := `{"releases-index":[
		{"channel-version":"9.0","support-phase":"active","type":"sts"},
		{"channel-version":"8.0","support-phase":"eol","type":"lts"}
	]}`
	server := newRequestServer(t, index, nil)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("lts", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ResolveVersion(context.Background(), request)
	var unsupported *UnsupportedAliasError
	if !errors.As(err, &unsupported) || !strings.Contains(unsupported.Reason, "LTS") {
		t.Fatalf("error = %v, want unsupported alias", err)
	}
}

func TestLatestAliasWithoutActiveChannelsFails(t *testing.T) {
	index := `{"releases-index":[{"channel-version":"8.0","support-phase":"eol","type":"lts"}]}`
	server := newRequestServer(t, index, nil)
	defer server.Close()
	client := newRequestClient(server)

	request, err := ParseVersionRequest("latest", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ResolveVersion(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "no actively supported channels") {
		t.Fatalf("error = %v, want no channels error", err)
	}
}

func TestOfflineAndMalformedIndexErrors(t *testing.T) {
	request, err := ParseVersionRequest("latest", false)
	if err != nil {
		t.Fatal(err)
	}

	// Offline: index unavailable.
	server := newRequestServer(t, "", nil)
	defer server.Close()
	client := newRequestClient(server)
	_, err = client.ResolveVersion(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "fetch release index") {
		t.Fatalf("offline error = %v", err)
	}

	// Malformed index.
	malformed := newRequestServer(t, `{"releases-index": "not-an-array"`, nil)
	defer malformed.Close()
	badClient := newRequestClient(malformed)
	_, err = badClient.ResolveVersion(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "parse release index") {
		t.Fatalf("malformed error = %v", err)
	}
}

func TestRuntimeRequestResolvesNewestRuntimeInChannel(t *testing.T) {
	hash := sha512.Sum512([]byte("runtime archive"))
	channels := map[string]string{"/10.0/releases.json": fmt.Sprintf(`{"releases":[
		{"runtime":{"version":"10.0.12"}},
		{"runtimes":[{"version":"10.0.15-rc.2","files":[{"name":"dotnet-runtime-10.0.15-rc.2-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/runtime-rc.tar.gz","hash":"%s"}]}]},
		{"runtime":{"version":"10.0.14","files":[{"name":"dotnet-runtime-10.0.14-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/runtime-10.0.14.tar.gz","hash":"%s"}]}}
	]}`, FormatSHA512(hash[:]), FormatSHA512(hash[:]))}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	installer := runtimeTestInstaller(t, server, netcoreRunner())

	plan, err := installer.PlanRuntime(context.Background(), "10.0", FrameworkNetCore, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Requested != "10.0" || plan.Version != "10.0.14" {
		t.Fatalf("plan = %+v, want 10.0 resolved to 10.0.14", plan)
	}
	if len(plan.Artifacts) != 1 || plan.Artifacts[0].URL != "https://a.microsoft.com/runtime-10.0.14.tar.gz" {
		t.Errorf("artifacts = %+v", plan.Artifacts)
	}

	prereleasePlan, err := installer.PlanRuntime(context.Background(), "10.0", FrameworkNetCore, true)
	if err != nil {
		t.Fatal(err)
	}
	if prereleasePlan.Version != "10.0.15-rc.2" {
		t.Errorf("prerelease plan = %+v", prereleasePlan)
	}
	if len(prereleasePlan.Artifacts) != 1 || prereleasePlan.Artifacts[0].URL != "https://a.microsoft.com/runtime-rc.tar.gz" {
		t.Errorf("prerelease artifacts = %+v", prereleasePlan.Artifacts)
	}
}

func TestSDKPlanCarriesRequestedAndResolved(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	channels := map[string]string{
		"/10.0/releases.json": sdkChannelJSON(hash, "10.0.102", "10.0.104-rc.1"),
	}
	server := newRequestServer(t, requestTestIndex, channels)
	defer server.Close()
	installer := runtimeTestInstaller(t, server, &fakeRunner{version: "10.0.102"})

	plan, err := installer.Plan(context.Background(), "10.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != "10.0.102" || plan.Requested != "10.0" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.SourceURL != "https://a.microsoft.com/sdk-10.0.102.tar.gz" {
		t.Errorf("sourceURL = %q", plan.SourceURL)
	}

	// Alias request resolves through the index and carries the alias tag.
	aliasPlan, err := installer.Plan(context.Background(), "latest", false)
	if err != nil {
		t.Fatal(err)
	}
	if aliasPlan.Version != "10.0.102" || aliasPlan.Requested != "latest" {
		t.Fatalf("plan = %+v", aliasPlan)
	}

	// Prerelease channel requests resolve to the newest prerelease artifacts.
	allowedPlan, err := installer.Plan(context.Background(), "10.0", true)
	if err != nil {
		t.Fatal(err)
	}
	if allowedPlan.Version != "10.0.104-rc.1" || allowedPlan.SourceURL != "https://a.microsoft.com/sdk-10.0.104-rc.1.tar.gz" {
		t.Fatalf("plan = %+v", allowedPlan)
	}
}
