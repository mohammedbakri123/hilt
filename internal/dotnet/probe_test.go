package dotnet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSDKList(t *testing.T) {
	output := []byte("10.0.112 [/usr/share/dotnet/sdk]\n8.0.404 [/usr/lib/dotnet/sdk]\n")
	sdks, err := ParseSDKList(output, "/usr/share/dotnet")
	if err != nil {
		t.Fatalf("ParseSDKList: %v", err)
	}
	if len(sdks) != 2 {
		t.Fatalf("got %d SDKs", len(sdks))
	}
	if sdks[0].Version != "10.0.112" {
		t.Errorf("version = %q", sdks[0].Version)
	}
	if sdks[0].Path != "/usr/share/dotnet/sdk/10.0.112" {
		t.Errorf("path = %q", sdks[0].Path)
	}
	if sdks[1].Path != "/usr/lib/dotnet/sdk/8.0.404" {
		t.Errorf("path = %q", sdks[1].Path)
	}
}

func TestParseSDKListRejectsMalformed(t *testing.T) {
	if _, err := ParseSDKList([]byte("not a valid line\n"), "/root"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := ParseSDKList([]byte("nope [/root/sdk]\n"), "/root"); err == nil {
		t.Fatal("expected a version error")
	}
}

func TestParseRuntimeList(t *testing.T) {
	output := []byte("Microsoft.AspNetCore.App 10.0.12 [/usr/share/dotnet/shared/Microsoft.AspNetCore.App]\nMicrosoft.NETCore.App 10.0.12 [/usr/share/dotnet/shared/Microsoft.NETCore.App]\n")
	runtimes, err := ParseRuntimeList(output, "/usr/share/dotnet")
	if err != nil {
		t.Fatalf("ParseRuntimeList: %v", err)
	}
	if len(runtimes) != 2 {
		t.Fatalf("got %d runtimes", len(runtimes))
	}
	if runtimes[0].Framework != "Microsoft.AspNetCore.App" || runtimes[0].Version != "10.0.12" {
		t.Errorf("runtime = %+v", runtimes[0])
	}
	if runtimes[0].Path != "/usr/share/dotnet/shared/Microsoft.AspNetCore.App/10.0.12" {
		t.Errorf("path = %q", runtimes[0].Path)
	}
}

func TestParseCapturedFixtures(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "tests", "testdata", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return data
	}

	sdks, err := ParseSDKList(read("dotnet-list-sdks.txt"), "/usr/share/dotnet")
	if err != nil {
		t.Fatalf("ParseSDKList: %v", err)
	}
	if len(sdks) != 1 || sdks[0].Version != "10.0.112" {
		t.Errorf("SDKs = %+v", sdks)
	}

	runtimes, err := ParseRuntimeList(read("dotnet-list-runtimes.txt"), "/usr/share/dotnet")
	if err != nil {
		t.Fatalf("ParseRuntimeList: %v", err)
	}
	if len(runtimes) != 2 || runtimes[0].Version != "10.0.12" {
		t.Errorf("runtimes = %+v", runtimes)
	}

	info := ParseInfo(read("dotnet-info.txt"))
	if info.Architecture != "x64" || info.RID != "arch-x64" {
		t.Errorf("info = %+v", info)
	}
}

func TestParseInfo(t *testing.T) {
	output := []byte(`.NET SDK:
 Version:           10.0.112

Runtime Environment:
 OS Name:     cachyos
 RID:         arch-x64
 Base Path:   /usr/share/dotnet/sdk/10.0.112/

Host:
  Version:      10.0.12
  Architecture: x64
`)
	info := ParseInfo(output)
	if info.Architecture != "x64" {
		t.Errorf("Architecture = %q", info.Architecture)
	}
	if info.RID != "arch-x64" {
		t.Errorf("RID = %q", info.RID)
	}
	if info.BasePath != "/usr/share/dotnet/sdk/10.0.112/" {
		t.Errorf("BasePath = %q", info.BasePath)
	}
}

func TestScanSDKs(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"10.0.112", "8.0.404", "not-a-version"} {
		if err := os.MkdirAll(filepath.Join(root, "sdk", dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sdks := ScanSDKs(root, os.Stat)
	if len(sdks) != 2 {
		t.Fatalf("SDKs = %+v", sdks)
	}
}

func TestScanRuntimes(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		"Microsoft.AspNetCore.App/10.0.12",
		"Microsoft.NETCore.App/10.0.12",
		"Microsoft.NETCore.App/not-a-version",
	} {
		if err := os.MkdirAll(filepath.Join(root, "shared", dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	runtimes := ScanRuntimes(root)
	if len(runtimes) != 2 {
		t.Fatalf("runtimes = %+v", runtimes)
	}
}

func TestGlobalJSON(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "src", "Api")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"sdk":{"version":"10.0.100","rollForward":"latestFeature","allowPrerelease":true}}`
	if err := os.WriteFile(filepath.Join(dir, "global.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	found, err := FindGlobalJSON(nested, os.Stat)
	if err != nil {
		t.Fatalf("FindGlobalJSON: %v", err)
	}
	if found == nil {
		t.Fatal("expected global.json")
	}
	if found.Version != "10.0.100" || found.RollForward != "latestFeature" {
		t.Errorf("global.json = %+v", found)
	}
	if found.AllowPrerelease == nil || !*found.AllowPrerelease {
		t.Errorf("AllowPrerelease = %+v", found.AllowPrerelease)
	}
}

func TestGlobalJSONMissing(t *testing.T) {
	found, err := FindGlobalJSON(t.TempDir(), os.Stat)
	if err != nil {
		t.Fatalf("FindGlobalJSON: %v", err)
	}
	if found != nil {
		t.Fatalf("expected nil, got %+v", found)
	}
}

func TestGlobalJSONMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "global.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadGlobalJSON(path); err == nil {
		t.Fatal("expected an error")
	}
}
