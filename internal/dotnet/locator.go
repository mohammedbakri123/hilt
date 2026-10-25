package dotnet

import (
	"os"
	"path/filepath"
	"strings"
)

// Source records where an installation root was discovered.
type Source string

const (
	SourcePath       Source = "path"
	SourceDotnetRoot Source = "dotnet_root"
	SourceKnown      Source = "known"
	SourceManaged    Source = "managed"
)

// StatFunc is the subset of os.Stat used for discovery.
type StatFunc func(string) (os.FileInfo, error)

// Installation is a discovered .NET root.
type Installation struct {
	Root       string `json:"root"`
	DotnetPath string `json:"dotnetPath"`
	Source     Source `json:"source"`
	Managed    bool   `json:"managed"`
	Usable     bool   `json:"usable"`
	Reason     string `json:"reason,omitempty"`
}

// Locator discovers .NET installation roots.
type Locator struct {
	LookPath     func(string) (string, error)
	EvalSymlinks func(string) (string, error)
	Stat         StatFunc
	Getenv       func(string) string
	ManagedRoot  string
	Home         string
	KnownRoots   []string
}

// Locate returns installation candidates in priority order, deduplicated by
// canonical root. Roots that do not exist as directories are skipped.
func (l Locator) Locate() []Installation {
	if l.Stat == nil {
		l.Stat = os.Stat
	}
	if l.Getenv == nil {
		l.Getenv = func(string) string { return "" }
	}
	if l.LookPath == nil {
		l.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	}
	if l.EvalSymlinks == nil {
		l.EvalSymlinks = filepath.EvalSymlinks
	}

	var installations []Installation
	seen := make(map[string]bool)

	add := func(root string, source Source) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		abs, err := filepath.Abs(root)
		if err == nil {
			root = abs
		}
		root = filepath.Clean(root)
		if canonical, err := l.EvalSymlinks(root); err == nil {
			root = filepath.Clean(canonical)
		}
		if seen[root] {
			return
		}
		info, err := l.Stat(root)
		if err != nil || !info.IsDir() {
			return
		}
		seen[root] = true

		dotnetPath := filepath.Join(root, "dotnet")
		usable := false
		reason := ""
		if info, err := l.Stat(dotnetPath); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			usable = true
		} else {
			reason = "dotnet executable not found in root"
		}

		installations = append(installations, Installation{
			Root:       root,
			DotnetPath: dotnetPath,
			Source:     source,
			Managed:    isWithin(root, l.ManagedRoot),
			Usable:     usable,
			Reason:     reason,
		})
	}

	add(l.Getenv("DOTNET_ROOT"), SourceDotnetRoot)

	if binary, err := l.LookPath("dotnet"); err == nil && binary != "" {
		if canonical, err := l.EvalSymlinks(binary); err == nil {
			binary = canonical
		}
		add(filepath.Dir(binary), SourcePath)
	}

	for _, root := range l.knownRoots() {
		source := SourceKnown
		if isWithin(root, l.ManagedRoot) {
			source = SourceManaged
		}
		add(root, source)
	}

	return installations
}

func (l Locator) knownRoots() []string {
	if l.KnownRoots != nil {
		return l.KnownRoots
	}
	roots := []string{
		"/usr/share/dotnet",
		"/usr/lib/dotnet",
		"/usr/lib64/dotnet",
		"/opt/dotnet",
	}
	if l.Home != "" {
		roots = append(roots, filepath.Join(l.Home, ".dotnet"))
	}
	if l.ManagedRoot != "" {
		roots = append(roots, l.ManagedRoot)
	}
	return roots
}

func isWithin(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
