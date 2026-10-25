package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type archiveEntry struct {
	name     string
	body     string
	mode     int64
	typeflag byte
	linkname string
}

func TestExtractTarGz(t *testing.T) {
	archive := writeTarGz(t, []archiveEntry{
		{name: "dotnet", body: "#!/bin/sh\n", mode: 0o755, typeflag: tar.TypeReg},
		{name: "sdk/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "sdk/10.0.100/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "sdk/10.0.100/file.txt", body: "hello", mode: 0o644, typeflag: tar.TypeReg},
		{name: "sdk/current", linkname: "10.0.100", mode: 0o777, typeflag: tar.TypeSymlink},
	})
	dest := t.TempDir()
	if err := ExtractTarGz(archive, dest, ExtractLimits{}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "sdk", "10.0.100", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
	info, err := os.Stat(filepath.Join(dest, "dotnet"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("dotnet mode = %o, want executable", info.Mode().Perm())
	}
	link, err := os.Readlink(filepath.Join(dest, "sdk", "current"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "10.0.100" {
		t.Errorf("link = %q", link)
	}
}

func TestExtractTarGzRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []archiveEntry
		want    string
	}{
		{
			name:    "traversal",
			entries: []archiveEntry{{name: "../escape", body: "bad", mode: 0o644, typeflag: tar.TypeReg}},
			want:    "escapes",
		},
		{
			name:    "absolute",
			entries: []archiveEntry{{name: "/tmp/escape", body: "bad", mode: 0o644, typeflag: tar.TypeReg}},
			want:    "escapes",
		},
		{
			name:    "escaping symlink",
			entries: []archiveEntry{{name: "link", linkname: "../../escape", mode: 0o777, typeflag: tar.TypeSymlink}},
			want:    "escapes",
		},
		{
			name:    "absolute symlink",
			entries: []archiveEntry{{name: "link", linkname: "/tmp/escape", mode: 0o777, typeflag: tar.TypeSymlink}},
			want:    "absolute",
		},
		{
			name:    "hard link",
			entries: []archiveEntry{{name: "link", linkname: "dotnet", mode: 0o777, typeflag: tar.TypeLink}},
			want:    "hard link",
		},
		{
			name:    "empty",
			entries: nil,
			want:    "empty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := writeTarGz(t, test.entries)
			err := ExtractTarGz(archive, t.TempDir(), ExtractLimits{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestExtractTarGzLimits(t *testing.T) {
	archive := writeTarGz(t, []archiveEntry{
		{name: "one", body: "12345", mode: 0o644, typeflag: tar.TypeReg},
		{name: "two", body: "12345", mode: 0o644, typeflag: tar.TypeReg},
	})
	if err := ExtractTarGz(archive, t.TempDir(), ExtractLimits{MaxBytes: 8}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("byte limit error = %v", err)
	}
	if err := ExtractTarGz(archive, t.TempDir(), ExtractLimits{MaxEntries: 1}); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("entry limit error = %v", err)
	}
}

func writeTarGz(t *testing.T, entries []archiveEntry) string {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Typeflag: typeflag,
			Linkname: entry.linkname,
			Size:     int64(len(entry.body)),
		}
		if typeflag != tar.TypeReg && typeflag != tar.TypeRegA {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
