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
const ManifestSchemaVersion = 2

// ProductKind names the managed product recorded by a manifest entry.
type ProductKind string

const (
	// ProductSDK records a managed .NET SDK installation.
	ProductSDK ProductKind = "sdk"
	// ProductRuntime records a managed .NET runtime installation.
	ProductRuntime ProductKind = "runtime"
)

// Valid reports whether the product kind is one of the supported products.
func (p ProductKind) Valid() bool {
	return p == ProductSDK || p == ProductRuntime
}

// label renders a human-readable product name for diagnostics.
func (p ProductKind) label() string {
	if p == ProductRuntime {
		return "runtime"
	}
	return "SDK"
}

// ArtifactRecord records one downloaded archive behind an installation.
type ArtifactRecord struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA512 string `json:"sha512"`
}

// Entry records one managed SDK or runtime installation.
type Entry struct {
	Product     ProductKind      `json:"product"`
	Version     string           `json:"version"`
	RID         string           `json:"rid"`
	Root        string           `json:"root"`
	SourceURL   string           `json:"sourceUrl"`
	SHA512      string           `json:"sha512"`
	Artifacts   []ArtifactRecord `json:"artifacts,omitempty"`
	Frameworks  []string         `json:"frameworks,omitempty"`
	InstalledAt time.Time        `json:"installedAt"`
}

// Manifest is Hilt's record of managed SDK and runtime installations. Roots are
// stored relative to the configured managed root so the manifest stays
// self-contained. Entries are keyed by product and version: an SDK and a
// runtime may share the same version.
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
		if stored.SchemaVersion < ManifestSchemaVersion && entry.Product == "" {
			// Schema 1 recorded only SDKs; migrate those entries.
			entry.Product = ProductSDK
		}
		normalized, err := normalizeEntry(entry, manifest.managedRoot)
		if err != nil {
			return nil, fmt.Errorf("install manifest %s: %w", path, err)
		}
		key := entryKey(normalized.Product, normalized.Version)
		if seen[key] {
			return nil, fmt.Errorf("install manifest %s: duplicate entry for %s %s", path, normalized.Product.label(), normalized.Version)
		}
		seen[key] = true
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

// entryKey builds the (product, version) key used for duplicate detection.
func entryKey(product ProductKind, version string) string {
	return string(product) + "\x00" + version
}

// Find returns the entry for an exact product and version.
func (m *Manifest) Find(product ProductKind, version string) *Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for index := range m.Installs {
		if m.Installs[index].Product != product || m.Installs[index].Version != version {
			continue
		}
		entry := m.Installs[index]
		return &entry
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

// Remove deletes the entry for an exact product and version.
func (m *Manifest) Remove(product ProductKind, version string) (Entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.Installs {
		if m.Installs[index].Product != product || m.Installs[index].Version != version {
			continue
		}
		removed := m.Installs[index]
		m.Installs = append(m.Installs[:index], m.Installs[index+1:]...)
		return removed, true
	}
	return Entry{}, false
}

// Add appends a validated entry keyed by product and version.
func (m *Manifest) Add(entry Entry) error {
	normalized, err := normalizeEntry(entry, m.managedRoot)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.Installs {
		if existing.Product == normalized.Product && existing.Version == normalized.Version {
			return fmt.Errorf("%s %s is already recorded in the install manifest", normalized.Product.label(), normalized.Version)
		}
	}
	m.Installs = append(m.Installs, normalized)
	return nil
}

// Owns reports whether a root and, when non-empty, a version are recorded.
// Ownership is product-agnostic: SDK and runtime roots differ, so the root
// alone disambiguates the product.
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
	product := entry.Product
	if !product.Valid() {
		return Entry{}, fmt.Errorf("product must be %q or %q, got %q", ProductSDK, ProductRuntime, string(product))
	}
	version, err := ValidateVersion(entry.Version)
	if err != nil {
		return Entry{}, err
	}
	entry.Version = version.String()

	if strings.TrimSpace(entry.RID) == "" {
		return Entry{}, fmt.Errorf("%s %s: RID is empty", product.label(), entry.Version)
	}
	if strings.TrimSpace(entry.SourceURL) == "" {
		return Entry{}, fmt.Errorf("%s %s: source URL is empty", product.label(), entry.Version)
	}
	hash, err := ParseSHA512(entry.SHA512)
	if err != nil {
		return Entry{}, fmt.Errorf("%s %s: %w", product.label(), entry.Version, err)
	}
	entry.SHA512 = FormatSHA512(hash)

	root := strings.TrimSpace(entry.Root)
	if root == "" {
		return Entry{}, fmt.Errorf("%s %s: root is empty", product.label(), entry.Version)
	}
	if filepath.IsAbs(root) {
		return Entry{}, fmt.Errorf("%s %s: root must be relative to the managed root", product.label(), entry.Version)
	}
	root = filepath.Clean(filepath.FromSlash(root))
	if root == "." || root == ".." || strings.HasPrefix(root, ".."+string(filepath.Separator)) {
		return Entry{}, fmt.Errorf("%s %s: root escapes the managed root", product.label(), entry.Version)
	}
	absolute := filepath.Join(managedRoot, root)
	if !withinRoot(managedRoot, absolute) {
		return Entry{}, fmt.Errorf("%s %s: root escapes the managed root", product.label(), entry.Version)
	}
	if product == ProductSDK {
		if filepath.ToSlash(root) != entry.Version {
			return Entry{}, fmt.Errorf("SDK %s: root %q must match the SDK version", entry.Version, entry.Root)
		}
	} else {
		if filepath.ToSlash(root) != "runtimes/"+entry.Version {
			return Entry{}, fmt.Errorf("runtime %s: root %q must match runtimes/<version>", entry.Version, entry.Root)
		}
	}
	entry.Root = filepath.ToSlash(root)

	frameworks, err := normalizeFrameworks(entry.Product, entry.Frameworks)
	if err != nil {
		return Entry{}, fmt.Errorf("%s %s: %w", product.label(), entry.Version, err)
	}
	entry.Frameworks = frameworks

	artifacts := make([]ArtifactRecord, 0, len(entry.Artifacts))
	for _, artifact := range entry.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			return Entry{}, fmt.Errorf("%s %s: artifact name is empty", product.label(), entry.Version)
		}
		if strings.TrimSpace(artifact.URL) == "" {
			return Entry{}, fmt.Errorf("%s %s: artifact %s URL is empty", product.label(), entry.Version, artifact.Name)
		}
		hash, err := ParseSHA512(artifact.SHA512)
		if err != nil {
			return Entry{}, fmt.Errorf("%s %s: artifact %s: %w", product.label(), entry.Version, artifact.Name, err)
		}
		artifact.Name = strings.TrimSpace(artifact.Name)
		artifact.URL = strings.TrimSpace(artifact.URL)
		artifact.SHA512 = FormatSHA512(hash)
		artifacts = append(artifacts, artifact)
	}
	entry.Artifacts = artifacts
	return entry, nil
}

func normalizeFrameworks(product ProductKind, frameworks []string) ([]string, error) {
	if len(frameworks) > 0 && product == ProductSDK {
		return nil, fmt.Errorf("frameworks are only valid for runtime entries")
	}
	cleaned := make([]string, 0, len(frameworks))
	seen := make(map[string]bool, len(frameworks))
	for _, framework := range frameworks {
		trimmed := strings.TrimSpace(framework)
		if trimmed == "" {
			return nil, fmt.Errorf("framework is empty")
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	return cleaned, nil
}
