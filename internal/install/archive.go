package install

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractLimits bounds archive extraction.
type ExtractLimits struct {
	MaxBytes   int64
	MaxEntries int
}

// DefaultExtractLimits are conservative limits for SDK archives.
var DefaultExtractLimits = ExtractLimits{
	MaxBytes:   4 << 30,
	MaxEntries: 100000,
}

// ExtractTarGz safely extracts an SDK archive into dest.
func ExtractTarGz(archivePath, dest string, limits ExtractLimits) error {
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = DefaultExtractLimits.MaxBytes
	}
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = DefaultExtractLimits.MaxEntries
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	dest = canonicalPath(dest)
	if dest == "" {
		return errors.New("extraction directory is empty")
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open SDK archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip archive: %w", err)
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	var total int64
	entries := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive entry: %w", err)
		}
		entries++
		if entries > limits.MaxEntries {
			return fmt.Errorf("archive contains more than %d entries", limits.MaxEntries)
		}

		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive entry %q escapes the extraction directory", header.Name)
		}
		target := filepath.Join(dest, name)
		if !withinRoot(dest, target) {
			return fmt.Errorf("archive entry %q escapes the extraction directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent directory for %q: %w", name, err)
			}
			mode := os.FileMode(header.Mode) & 0o777
			if mode == 0 {
				mode = 0o644
			}
			entry, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("create file %q: %w", name, err)
			}
			remaining := limits.MaxBytes - total
			if remaining < 0 {
				_ = entry.Close()
				return fmt.Errorf("archive exceeds %d bytes", limits.MaxBytes)
			}
			written, copyErr := io.Copy(entry, io.LimitReader(reader, remaining+1))
			if copyErr != nil {
				_ = entry.Close()
				return fmt.Errorf("write file %q: %w", name, copyErr)
			}
			if written > remaining {
				_ = entry.Close()
				return fmt.Errorf("archive exceeds %d bytes", limits.MaxBytes)
			}
			total += written
			if err := entry.Chmod(mode); err != nil {
				_ = entry.Close()
				return fmt.Errorf("set permissions on %q: %w", name, err)
			}
			if err := entry.Close(); err != nil {
				return fmt.Errorf("close file %q: %w", name, err)
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("archive symlink %q has an absolute target", name)
			}
			resolved := filepath.Join(filepath.Dir(target), header.Linkname)
			if !withinRoot(dest, resolved) {
				return fmt.Errorf("archive symlink %q escapes the extraction directory", name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent directory for symlink %q: %w", name, err)
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("create symlink %q: %w", name, err)
			}
		case tar.TypeLink:
			return fmt.Errorf("archive hard link %q is not supported", name)
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("archive entry %q has unsupported type %q", name, header.Typeflag)
		default:
			return fmt.Errorf("archive entry %q has unsupported type %q", name, header.Typeflag)
		}
	}
	if entries == 0 {
		return errors.New("archive is empty")
	}
	return nil
}
