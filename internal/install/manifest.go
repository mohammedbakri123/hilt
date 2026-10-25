package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ManifestSchemaVersion is the current install manifest schema.
const ManifestSchemaVersion = 1

// Entry records one managed SDK installation.
type Entry struct {
	Version     string    `json:"version"`
	RID         string    `json:"rid"`
	Root        string    `json:"root"`
	SourceURL   string    `json:"sourceUrl"`
	SHA512      string    `json:"sha512"`
	InstalledAt time.Time `json:"installedAt"`
}

// Manifest is Hilt's record of managed SDK installations. Roots are stored
// relative to the configured managed root so the manifest stays self-contained.
type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	Installs      []Entry `json:"installs"`

	path        string
	managedRoot string
	mu          sync.RWMutex
}

// Load reads an install manifest. A missing file is an empty manifest.
func Load(path, managedRoot string) (*Manifest, error) {
	manifest := newManifest(managedRoot)
	manifest.path = path

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifest, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read install manifest: %w", err)
	}

	var stored Manifest
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parse install manifest %s: %w", path, err)
	}
	if stored.SchemaVersion == 0 {
		stored.SchemaVersion = ManifestSchemaVersion
	}
	if stored.SchemaVersion > ManifestSchemaVersion {
		return nil, fmt.Errorf("install manifest schema %d is newer than supported schema %d", stored.SchemaVersion, ManifestSchemaVersion)
	}

	manifest.SchemaVersion = stored.SchemaVersion
	manifest.Installs = make([]Entry, 0, len(stored.Installs))
	seen := make(map[string]bool)
	for _, entry := range stored.Installs {
		normalized, err := normalizeEntry(entry, manifest.managedRoot)
		if err != nil {
			return nil, fmt.Errorf("install manifest %s: %w", path, err)
		}
		if seen[normalized.Version] {
			return nil, fmt.Errorf("install manifest %s: duplicate entry for SDK %s", path, normalized.Version)
		}
		seen[normalized.Version] = true
		manifest.Installs = append(manifest.Installs, normalized)
	}
	return manifest, nil
}

func newManifest(managedRoot string) *Manifest {
	return &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Installs:      []Entry{},
		managedRoot:   canonicalPath(managedRoot),
	}
}

// NewManifest returns an empty in-memory manifest for a managed root.
func NewManifest(managedRoot string) *Manifest {
	return newManifest(managedRoot)
}

// ManagedRoot returns the canonical managed root this manifest describes.
func (m *Manifest) ManagedRoot() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.managedRoot
}

// Find returns the entry for an exact version.
func (m *Manifest) Find(version string) *Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for index := range m.Installs {
		if m.Installs[index].Version == version {
			entry := m.Installs[index]
			return &entry
		}
	}
	return nil
}

// RootFor returns the absolute root for an entry.
func (m *Manifest) RootFor(entry Entry) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rootFor(entry)
}

func (m *Manifest) rootFor(entry Entry) string {
	return filepath.Join(m.managedRoot, filepath.FromSlash(entry.Root))
}

// Add appends a validated entry.
func (m *Manifest) Add(entry Entry) error {
	normalized, err := normalizeEntry(entry, m.managedRoot)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.Installs {
		if existing.Version == normalized.Version {
			return fmt.Errorf("SDK %s is already recorded in the install manifest", normalized.Version)
		}
	}
	m.Installs = append(m.Installs, normalized)
	return nil
}

// Owns reports whether a root and, when non-empty, a version are recorded.
func (m *Manifest) Owns(root, version string) bool {
	if m == nil || root == "" {
		return false
	}
	root = canonicalPath(root)
	if root == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, entry := range m.Installs {
		if version != "" && entry.Version != version {
			continue
		}
		if canonicalPath(m.rootFor(entry)) == root {
			return true
		}
	}
	return false
}

// ManagedRoots returns every recorded installation root.
func (m *Manifest) ManagedRoots() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	roots := make([]string, 0, len(m.Installs))
	for _, entry := range m.Installs {
		roots = append(roots, m.rootFor(entry))
	}
	return roots
}

// Save writes the manifest atomically with private permissions.
func (m *Manifest) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.path == "" {
		return errors.New("install manifest path is empty")
	}
	m.SchemaVersion = ManifestSchemaVersion
	if m.Installs == nil {
		m.Installs = []Entry{}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install manifest: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create install manifest directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".installs-*.tmp")
	if err != nil {
		return fmt.Errorf("create install manifest: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set install manifest permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write install manifest: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync install manifest: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close install manifest: %w", err)
	}
	if err := os.Rename(tempName, m.path); err != nil {
		return fmt.Errorf("publish install manifest: %w", err)
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func normalizeEntry(entry Entry, managedRoot string) (Entry, error) {
	version, err := ValidateVersion(entry.Version)
	if err != nil {
		return Entry{}, err
	}
	entry.Version = version.String()

	if strings.TrimSpace(entry.RID) == "" {
		return Entry{}, fmt.Errorf("SDK %s: RID is empty", entry.Version)
	}
	if strings.TrimSpace(entry.SourceURL) == "" {
		return Entry{}, fmt.Errorf("SDK %s: source URL is empty", entry.Version)
	}
	hash, err := ParseSHA512(entry.SHA512)
	if err != nil {
		return Entry{}, fmt.Errorf("SDK %s: %w", entry.Version, err)
	}
	entry.SHA512 = FormatSHA512(hash)

	root := strings.TrimSpace(entry.Root)
	if root == "" {
		return Entry{}, fmt.Errorf("SDK %s: root is empty", entry.Version)
	}
	if filepath.IsAbs(root) {
		return Entry{}, fmt.Errorf("SDK %s: root must be relative to the managed root", entry.Version)
	}
	root = filepath.Clean(filepath.FromSlash(root))
	if root == "." || root == ".." || strings.HasPrefix(root, ".."+string(filepath.Separator)) {
		return Entry{}, fmt.Errorf("SDK %s: root escapes the managed root", entry.Version)
	}
	absolute := filepath.Join(managedRoot, root)
	if !withinRoot(managedRoot, absolute) {
		return Entry{}, fmt.Errorf("SDK %s: root escapes the managed root", entry.Version)
	}
	if filepath.ToSlash(root) != entry.Version {
		return Entry{}, fmt.Errorf("SDK %s: root %q must match the SDK version", entry.Version, entry.Root)
	}
	entry.Root = filepath.ToSlash(root)
	return entry, nil
}
