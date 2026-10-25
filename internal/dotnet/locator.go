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

// Provenance describes where a toolchain came from and whether Hilt owns it.
// Managed is path-based; owned additionally requires manifest ownership.
type Provenance string

const (
	ProvenanceSystem   Provenance = "system"
	ProvenanceExternal Provenance = "external"
	ProvenanceManaged  Provenance = "managed"
	ProvenanceOwned    Provenance = "owned"
)

// Ownership reports whether a managed root, or a specific version inside it, is
// recorded in Hilt's install manifest. An empty version asks whether the root
// contains any owned entry. Implementations must be read-only.
type Ownership interface {
	Owns(root, version string) bool
}

// OwnershipFunc adapts a function to Ownership.
type OwnershipFunc func(root, version string) bool

func (f OwnershipFunc) Owns(root, version string) bool { return f(root, version) }

// ManagedRootProvider optionally exposes additional Hilt-owned roots, such as
// per-version managed install roots recorded in the install manifest.
type ManagedRootProvider interface {
	ManagedRoots() []string
}

// StatFunc is the subset of os.Stat used for discovery.
type StatFunc func(string) (os.FileInfo, error)

// Installation is a discovered .NET root.
type Installation struct {
	Root       string     `json:"root"`
	DotnetPath string     `json:"dotnetPath"`
	Source     Source     `json:"source"`
	Managed    bool       `json:"managed"`
	Owned      bool       `json:"owned"`
	Provenance Provenance `json:"provenance"`
	Usable     bool       `json:"usable"`
	Reason     string     `json:"reason,omitempty"`
}

// EffectiveProvenance returns the recorded provenance or a conservative
// fallback derived from the path-based managed flag.
func (i Installation) EffectiveProvenance() Provenance {
	return effectiveProvenance(i.Provenance, i.Managed, i.Owned)
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
	SystemRoots  []string
	Ownership    Ownership
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

		managed := isWithin(root, l.ManagedRoot)
		owned := managed && l.Ownership != nil && l.Ownership.Owns(root, "")
		provenance := effectiveProvenance(Provenance(""), managed, owned)
		if !managed && !l.isSystemRoot(root) {
			provenance = ProvenanceExternal
		}

		installations = append(installations, Installation{
			Root:       root,
			DotnetPath: dotnetPath,
			Source:     source,
			Managed:    managed,
			Owned:      owned,
			Provenance: provenance,
			Usable:     usable,
			Reason:     reason,
		})
	}

	// DOTNET_ROOT is a discovery input only. Hilt never treats it as an install
	// destination and never writes to it.
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
	roots := l.KnownRoots
	if roots == nil {
		roots = []string{
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
	}
	if provider, ok := l.Ownership.(ManagedRootProvider); ok {
		roots = append(append([]string(nil), roots...), provider.ManagedRoots()...)
	}
	return roots
}

func (l Locator) isSystemRoot(root string) bool {
	for _, systemRoot := range l.systemRoots() {
		if isWithin(root, systemRoot) {
			return true
		}
	}
	return false
}

func (l Locator) systemRoots() []string {
	if l.SystemRoots != nil {
		return l.SystemRoots
	}
	return []string{
		"/usr/share/dotnet",
		"/usr/lib/dotnet",
		"/usr/lib64/dotnet",
		"/opt/dotnet",
	}
}

func effectiveProvenance(provenance Provenance, managed, owned bool) Provenance {
	if provenance != "" {
		return provenance
	}
	if managed {
		if owned {
			return ProvenanceOwned
		}
		return ProvenanceManaged
	}
	return ProvenanceSystem
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
