package install

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseSHA512HexAndBase64(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	for name, encoded := range map[string]string{
		"hex":    FormatSHA512(hash[:]),
		"base64": base64.StdEncoding.EncodeToString(hash[:]),
		"raw":    base64.RawStdEncoding.EncodeToString(hash[:]),
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := ParseSHA512(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if FormatSHA512(parsed) != FormatSHA512(hash[:]) {
				t.Errorf("parsed hash = %s", FormatSHA512(parsed))
			}
		})
	}
	if _, err := ParseSHA512("not-a-hash"); err == nil {
		t.Fatal("expected invalid checksum error")
	}
}

func TestValidateMicrosoftURL(t *testing.T) {
	allowed := []string{
		"https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json",
		"https://dotnetcli.azureedge.net/dotnet/Sdk/10.0.100/dotnet-sdk-10.0.100-linux-x64.tar.gz",
		"https://dotnetbuilds.blob.core.windows.net/public/sdk.tar.gz",
	}
	for _, raw := range allowed {
		parsed, _ := url.Parse(raw)
		if err := ValidateMicrosoftURL(parsed); err != nil {
			t.Errorf("ValidateMicrosoftURL(%s) = %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://builds.dotnet.microsoft.com/sdk.tar.gz",
		"https://example.com/sdk.tar.gz",
		"https://microsoft.com.evil.example/sdk.tar.gz",
	} {
		parsed, _ := url.Parse(raw)
		if err := ValidateMicrosoftURL(parsed); err == nil {
			t.Errorf("ValidateMicrosoftURL(%s) succeeded", raw)
		}
	}
}

func TestResolveSDK(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/10.0/releases.json" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, `{
			"releases": [{
				"release-version": "10.0.100",
				"sdk": {
					"version": "10.0.100",
					"files": [
						{"name": "dotnet-sdk-10.0.100-linux-x64.zip", "rid": "linux-x64", "url": "%s/archive.zip", "hash": "%s"},
						{"name": "dotnet-sdk-10.0.100-linux-x64.tar.gz", "rid": "linux-x64", "url": "%s/archive.tar.gz", "hash": "%s"}
					]
				}
			}]
		}`, server.URL, FormatSHA512(hash[:]), server.URL, FormatSHA512(hash[:]))
	}))
	defer server.Close()

	client := &ReleaseClient{
		BaseURL:     server.URL,
		HTTPClient:  server.Client(),
		ValidateURL: allowTestURL,
	}
	artifact, err := client.ResolveSDK(context.Background(), "10.0.100", "linux-x64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Version != "10.0.100" || artifact.RID != "linux-x64" {
		t.Errorf("artifact = %+v", artifact)
	}
	if artifact.URL != server.URL+"/archive.tar.gz" {
		t.Errorf("URL = %q", artifact.URL)
	}
	if artifact.FileName != "dotnet-sdk-10.0.100-linux-x64.tar.gz" {
		t.Errorf("FileName = %q", artifact.FileName)
	}
	if FormatSHA512(artifact.SHA512) != FormatSHA512(hash[:]) {
		t.Errorf("SHA512 = %s", FormatSHA512(artifact.SHA512))
	}
}

func TestResolveSDKErrors(t *testing.T) {
	hash := sha512.Sum512([]byte("archive"))
	tests := []struct {
		name     string
		metadata string
		version  string
		rid      string
		want     string
	}{
		{
			name:     "missing version",
			metadata: `{"releases":[{"sdk":{"version":"9.0.100","files":[]}}]}`,
			version:  "10.0.100",
			rid:      "linux-x64",
			want:     "was not found",
		},
		{
			name:     "missing rid",
			metadata: `{"releases":[{"sdk":{"version":"10.0.100","files":[{"name":"dotnet-sdk-10.0.100-linux-x64.tar.gz","rid":"linux-x64","url":"https://example.microsoft.com/a.tar.gz","hash":"` + FormatSHA512(hash[:]) + `"}]}}]}`,
			version:  "10.0.100",
			rid:      "linux-arm64",
			want:     "no .tar.gz archive",
		},
		{
			name:     "missing checksum",
			metadata: `{"releases":[{"sdk":{"version":"10.0.100","files":[{"name":"dotnet-sdk-10.0.100-linux-x64.tar.gz","rid":"linux-x64","url":"https://example.microsoft.com/a.tar.gz"}]}}]}`,
			version:  "10.0.100",
			rid:      "linux-x64",
			want:     "no SHA-512 checksum",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write([]byte(test.metadata))
			}))
			defer server.Close()
			client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
			_, err := client.ResolveSDK(context.Background(), test.version, test.rid)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestReleaseClientRejectsMalformedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("{bad"))
	}))
	defer server.Close()
	client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	if _, err := client.ResolveSDK(context.Background(), "10.0.100", "linux-x64"); err == nil || !strings.Contains(err.Error(), "parse release metadata") {
		t.Fatalf("error = %v", err)
	}
}

func allowTestURL(*url.URL) error { return nil }

func TestLatestSDK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/10.0/releases.json" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{"releases":[
			{"sdk":{"version":"10.0.100"}},
			{"sdks":[{"version":"10.0.101"},{"version":"10.0.103-rc.1"}]},
			{"sdk":{"version":"10.0.102"}},
			{"sdk":{"version":"9.0.200"}}
		]}`))
	}))
	defer server.Close()
	client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	latest, err := client.LatestSDK(context.Background(), "10.0.100")
	if err != nil {
		t.Fatal(err)
	}
	if latest.String() != "10.0.102" {
		t.Errorf("latest = %s, want 10.0.102", latest)
	}
}

func TestLatestSDKMissingStableVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"releases":[{"sdk":{"version":"10.0.103-rc.1"}}]}`))
	}))
	defer server.Close()
	client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	if _, err := client.LatestSDK(context.Background(), "10.0.100"); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestAvailableSDKsFiltersInstalledCandidatesForRID(t *testing.T) {
	hash := strings.Repeat("a", 128)
	index := `{"releases-index":[
		{"channel-version":"11.0","latest-sdk":"11.0.100-rc.1","support-phase":"go-live","release-type":"sts"},
		{"channel-version":"10.0","latest-sdk":"10.0.112","support-phase":"active","release-type":"lts"},
		{"channel-version":"9.0","latest-sdk":"9.0.300","support-phase":"maintenance","release-type":"sts"},
		{"channel-version":"8.0","latest-sdk":"8.0.100","support-phase":"eol","release-type":"lts"}
	]}`
	archive := func(version, rid string) string {
		return fmt.Sprintf(`{"name":"dotnet-sdk-%s-%s.tar.gz","rid":%q,"url":"https://a.microsoft.com/%s.tar.gz","hash":"%s"}`, version, rid, rid, version, hash)
	}
	channels := map[string]string{
		"/10.0/releases.json": fmt.Sprintf(`{"releases":[
			{"sdk":{"version":"10.0.112","files":[%s]}},
			{"sdks":[
				{"version":"10.0.111","files":[%s]},
				{"version":"10.0.113-rc.1","files":[%s]},
				{"version":"10.0.110","files":[{"name":"dotnet-sdk-10.0.110-linux-x64.zip","rid":"linux-x64","url":"https://a.microsoft.com/10.0.110.zip","hash":"%s"}]}
			]}
		]}`, archive("10.0.112", "linux-x64"), archive("10.0.111", "linux-x64"), archive("10.0.113-rc.1", "linux-x64"), hash),
		"/9.0/releases.json":  fmt.Sprintf(`{"releases":[{"sdk":{"version":"9.0.300","files":[%s]}}]}`, archive("9.0.300", "linux-x64")),
		"/8.0/releases.json":  fmt.Sprintf(`{"releases":[{"sdk":{"version":"8.0.100","files":[%s]}}]}`, archive("8.0.100", "linux-x64")),
		"/11.0/releases.json": fmt.Sprintf(`{"releases":[{"sdk":{"version":"11.0.100-rc.1","files":[%s]}}]}`, archive("11.0.100-rc.1", "linux-x64")),
	}
	server := newRequestServer(t, index, channels)
	defer server.Close()
	client := newRequestClient(server)

	available, err := client.AvailableSDKs(context.Background(), "linux-x64", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 4 {
		t.Fatalf("available = %+v, want 4 stable SDKs", available)
	}
	for index, want := range []struct {
		version string
		latest  bool
	}{
		{"10.0.112", true},
		{"10.0.111", false},
		{"9.0.300", true},
		{"8.0.100", true},
	} {
		got := available[index]
		if got.Version != want.version || got.Latest != want.latest {
			t.Errorf("available[%d] = %+v, want version %s latest=%v", index, got, want.version, want.latest)
		}
		if got.Channel == "" || got.ReleaseType == "" || got.SupportPhase == "" {
			t.Errorf("available[%d] missing channel metadata: %+v", index, got)
		}
	}
	for _, sdk := range available {
		if sdk.Version == "10.0.110" {
			t.Error("zip-only SDK was listed")
		}
		if strings.HasPrefix(sdk.Version, "11.") {
			t.Errorf("prerelease channel leaked into stable list: %+v", sdk)
		}
	}

	withPrerelease, err := client.AvailableSDKs(context.Background(), "linux-x64", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withPrerelease) != 6 {
		t.Fatalf("available with prerelease = %+v, want 6 SDKs", withPrerelease)
	}
	if withPrerelease[0].Version != "11.0.100-rc.1" || !withPrerelease[0].Latest {
		t.Errorf("newest prerelease = %+v", withPrerelease[0])
	}

	arm64, err := client.AvailableSDKs(context.Background(), "linux-arm64", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(arm64) != 0 {
		t.Fatalf("arm64 available = %+v, want no x64 archives", arm64)
	}

}

func TestAvailableSDKsFallsBackToLatestWhenChannelMetadataFails(t *testing.T) {
	index := `{"releases-index":[
		{"channel-version":"9.0","latest-sdk":"9.0.300","support-phase":"maintenance","release-type":"sts"},
		{"channel-version":"8.0","latest-sdk":"8.0.400","support-phase":"maintenance","release-type":"lts"}
	]}`
	server := newRequestServer(t, index, map[string]string{})
	defer server.Close()
	client := newRequestClient(server)

	available, err := client.AvailableSDKs(context.Background(), "linux-x64", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 2 {
		t.Fatalf("available = %+v, want index fallbacks", available)
	}
	for index, want := range []string{"9.0.300", "8.0.400"} {
		if available[index].Version != want || !available[index].Latest {
			t.Errorf("available[%d] = %+v, want %s latest", index, available[index], want)
		}
	}
}

func TestResolveRuntime(t *testing.T) {
	hash := sha512.Sum512([]byte("runtime archive"))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/10.0/releases.json" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, `{"releases":[
			{"runtime":{"version":"10.0.12","files":[{"name":"dotnet-runtime-10.0.12-linux-x64.tar.gz","rid":"linux-x64","url":"%s/archive-runtime.tar.gz","hash":"%s"}]}},
			{"aspnetcore-runtime":{"version":"10.0.12","files":[{"name":"dotnet-aspnetcore-runtime-10.0.12-linux-x64.tar.gz","rid":"linux-x64","url":"%s/archive-aspnetcore.tar.gz","hash":"%s"}]}}
		]}`, server.URL, FormatSHA512(hash[:]), server.URL, FormatSHA512(hash[:]))
	}))
	defer server.Close()

	client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	artifact, err := client.ResolveRuntime(context.Background(), "10.0.12", RuntimeArchiveRuntime, "linux-x64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.URL != server.URL+"/archive-runtime.tar.gz" || artifact.FileName != "dotnet-runtime-10.0.12-linux-x64.tar.gz" {
		t.Errorf("runtime artifact = %+v", artifact)
	}
	if FormatSHA512(artifact.SHA512) != FormatSHA512(hash[:]) {
		t.Errorf("SHA512 = %s", FormatSHA512(artifact.SHA512))
	}
	aspnetcore, err := client.ResolveRuntime(context.Background(), "10.0.12", RuntimeArchiveASPNETCore, "linux-x64")
	if err != nil {
		t.Fatal(err)
	}
	if aspnetcore.URL != server.URL+"/archive-aspnetcore.tar.gz" || aspnetcore.FileName != "dotnet-aspnetcore-runtime-10.0.12-linux-x64.tar.gz" {
		t.Errorf("aspnetcore artifact = %+v", aspnetcore)
	}
}

func TestResolveRuntimeErrors(t *testing.T) {
	hash := sha512.Sum512([]byte("runtime archive"))
	tests := []struct {
		name    string
		product string
		want    string
	}{
		{name: "unknown product", product: "sdk", want: `unknown runtime archive product "sdk"`},
		{name: "missing product in metadata", product: RuntimeArchiveRuntime, want: "runtime 10.0.12 was not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"releases":[{"sdk":{"version":"10.0.100"}}]}`
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write([]byte(body))
			}))
			defer server.Close()
			client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
			_, err := client.ResolveRuntime(context.Background(), "10.0.12", test.product, "linux-x64")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}

	ridBody := fmt.Sprintf(`{"releases":[{"runtime":{"version":"10.0.12","files":[{"name":"dotnet-runtime-10.0.12-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/r.tar.gz","hash":"%s"}]}}]}`, FormatSHA512(hash[:]))
	ridServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(ridBody))
	}))
	defer ridServer.Close()
	ridClient := &ReleaseClient{BaseURL: ridServer.URL, HTTPClient: ridServer.Client(), ValidateURL: allowTestURL}
	if _, err := ridClient.ResolveRuntime(context.Background(), "10.0.12", RuntimeArchiveRuntime, "linux-arm64"); err == nil || !strings.Contains(err.Error(), "no .tar.gz archive") {
		t.Fatalf("rid error = %v", err)
	}

	noHashBody := `{"releases":[{"runtime":{"version":"10.0.12","files":[{"name":"dotnet-runtime-10.0.12-linux-x64.tar.gz","rid":"linux-x64","url":"https://a.microsoft.com/r.tar.gz"}]}}]}`
	noHashServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(noHashBody))
	}))
	defer noHashServer.Close()
	noHashClient := &ReleaseClient{BaseURL: noHashServer.URL, HTTPClient: noHashServer.Client(), ValidateURL: allowTestURL}
	if _, err := noHashClient.ResolveRuntime(context.Background(), "10.0.12", RuntimeArchiveRuntime, "linux-x64"); err == nil || !strings.Contains(err.Error(), "no SHA-512 checksum") {
		t.Fatal("wanted checksum missing error")
	}
}

func TestLatestRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/10.0/releases.json" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{"releases":[
			{"runtime":{"version":"10.0.12"}},
			{"runtimes":[{"version":"10.0.13"},{"version":"10.0.14-rc.1"}]},
			{"runtime":{"version":"10.0.15"}},
			{"sdk":{"version":"10.0.200"}},
			{"aspnetcore-runtime":{"version":"10.0.13"}}
		]}`))
	}))
	defer server.Close()
	client := &ReleaseClient{BaseURL: server.URL, HTTPClient: server.Client(), ValidateURL: allowTestURL}
	latest, err := client.LatestRuntime(context.Background(), "10.0.12", RuntimeArchiveRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if latest.String() != "10.0.15" {
		t.Errorf("runtime latest = %s, want 10.0.15", latest)
	}
	aspnetcore, err := client.LatestRuntime(context.Background(), "10.0.12", RuntimeArchiveASPNETCore)
	if err != nil {
		t.Fatal(err)
	}
	if aspnetcore.String() != "10.0.13" {
		t.Errorf("aspnetcore latest = %s, want 10.0.13", aspnetcore)
	}
}
