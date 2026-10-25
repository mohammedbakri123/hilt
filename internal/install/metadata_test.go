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
