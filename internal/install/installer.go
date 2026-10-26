package install

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/process"
)

const (
	defaultMaxDownloadBytes  = 2 << 30
	defaultMaxExtractBytes   = 4 << 30
	defaultMaxArchiveEntries = 100000
)

// Plan describes an SDK installation before any download occurs.
type Plan struct {
	Version          string `json:"version"`
	Requested        string `json:"requested,omitempty"`
	RID              string `json:"rid"`
	Root             string `json:"root"`
	DotnetPath       string `json:"dotnetPath"`
	SourceURL        string `json:"sourceUrl,omitempty"`
	SHA512           string `json:"sha512,omitempty"`
	FileName         string `json:"fileName,omitempty"`
	AlreadyInstalled bool   `json:"alreadyInstalled"`
	SystemCopy       bool   `json:"systemCopy,omitempty"`
}

// Result describes a completed or already-present managed installation.
type Result struct {
	Version          string    `json:"version"`
	RID              string    `json:"rid"`
	Root             string    `json:"root"`
	DotnetPath       string    `json:"dotnetPath"`
	SourceURL        string    `json:"sourceUrl,omitempty"`
	SHA512           string    `json:"sha512,omitempty"`
	InstalledAt      time.Time `json:"installedAt"`
	AlreadyInstalled bool      `json:"alreadyInstalled"`
	SystemCopy       bool      `json:"systemCopy,omitempty"`
}

// Removal describes a completed managed SDK or runtime removal.
type Removal struct {
	Product   ProductKind `json:"product,omitempty"`
	Version   string      `json:"version"`
	RID       string      `json:"rid,omitempty"`
	Root      string      `json:"root"`
	Stale     bool        `json:"stale"`
	RemovedAt time.Time   `json:"removedAt"`
}

// UnownedRootError reports a destination that exists without a manifest entry.
type UnownedRootError struct {
	Root string
}

func (e *UnownedRootError) Error() string {
	return fmt.Sprintf("destination %s already exists but is not recorded in Hilt's install manifest; refusing to overwrite it", e.Root)
}

// NotOwnedError reports a managed root that is not recorded in the manifest.
type NotOwnedError struct {
	Product ProductKind
	Version string
	Root    string
}

func (e *NotOwnedError) Error() string {
	return fmt.Sprintf("%s %s at %s is not recorded in Hilt's install manifest; refusing to remove it", productLabel(e.Product), e.Version, e.Root)
}

// NotInstalledError reports a product with no manifest entry and no managed root.
type NotInstalledError struct {
	Product ProductKind
	Version string
}

func (e *NotInstalledError) Error() string {
	return fmt.Sprintf("%s %s is not installed in Hilt's managed root", productLabel(e.Product), e.Version)
}

// StaleInstallError reports a manifest entry whose root is missing.
type StaleInstallError struct {
	Product ProductKind
	Version string
	Root    string
}

func (e *StaleInstallError) Error() string {
	return fmt.Sprintf("install manifest records %s %s at %s, but that directory is missing", productLabel(e.Product), e.Version, e.Root)
}

// productLabel renders a product kind with an SDK fallback for omitted kinds.
func productLabel(product ProductKind) string {
	if product == ProductRuntime {
		return "runtime"
	}
	return "SDK"
}

// Installer downloads, verifies, and publishes managed SDKs and runtimes.
type Installer struct {
	ManagedRoot       string
	ManifestPath      string
	LockPath          string
	Metadata          *ReleaseClient
	Runner            process.Runner
	HTTPClient        *http.Client
	ValidateURL       func(*url.URL) error
	BaseEnv           []string
	MaxDownloadBytes  int64
	MaxExtractBytes   int64
	MaxArchiveEntries int
	RemoveAll         func(string) error
	Now               func() time.Time
	// FindHost resolves a fallback dotnet host used to verify staged runtimes
	// that do not ship their own host.
	FindHost func() (string, bool)
}

func (i Installer) host() (string, bool) {
	if i.FindHost != nil {
		return i.FindHost()
	}
	if path, err := exec.LookPath("dotnet"); err == nil {
		return path, true
	}
	return "", false
}

// Plan resolves metadata and checks for an existing installation. The version
// request may be exact or a channel/alias request; aliases and channels are
// resolved to an exact version before anything else happens.
func (i Installer) Plan(ctx context.Context, input string, allowPrerelease bool) (Plan, error) {
	request, err := ParseVersionRequest(input, allowPrerelease)
	if err != nil {
		return Plan{}, err
	}
	resolved, err := i.metadata().ResolveVersion(ctx, request)
	if err != nil {
		return Plan{}, err
	}
	return i.planVersion(ctx, resolved.String(), request.Requested, request.AllowPrerelease)
}

func (i Installer) planVersion(ctx context.Context, canonical, requested string, allowPrerelease bool) (Plan, error) {
	parsed, err := dotnet.ParseVersion(canonical)
	if err != nil {
		return Plan{}, err
	}
	if parsed.IsPrerelease() && !allowPrerelease {
		return Plan{}, fmt.Errorf("prerelease version %q is not supported without --prerelease", canonical)
	}
	rid, err := CurrentRID()
	if err != nil {
		return Plan{}, err
	}
	managedRoot, err := i.managedRoot()
	if err != nil {
		return Plan{}, err
	}
	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return Plan{}, err
	}

	if entry := manifest.Find(ProductSDK, canonical); entry != nil {
		root := manifest.RootFor(*entry)
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRoot(root, canonical) {
			return Plan{}, &StaleInstallError{Version: canonical, Root: root}
		}
		plan := planFromEntry(*entry, root, true)
		plan.Requested = requested
		return plan, nil
	}

	root := filepath.Join(managedRoot, canonical)
	if info, statErr := os.Stat(root); statErr == nil {
		if !info.IsDir() {
			return Plan{}, fmt.Errorf("destination %s exists and is not a directory", root)
		}
		return Plan{}, &UnownedRootError{Root: root}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Plan{}, fmt.Errorf("inspect destination %s: %w", root, statErr)
	}

	artifact, err := i.metadata().ResolveSDK(ctx, canonical, rid)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Version:    artifact.Version,
		RID:        artifact.RID,
		Root:       root,
		DotnetPath: filepath.Join(root, "dotnet"),
		SourceURL:  artifact.URL,
		SHA512:     FormatSHA512(artifact.SHA512),
		FileName:   artifact.FileName,
		Requested:  requested,
	}, nil
}

// Update reports whether a managed SDK has a newer stable version in its
// release channel.
type Update struct {
	Version         string `json:"version"`
	LatestVersion   string `json:"latestVersion"`
	RID             string `json:"rid"`
	Root            string `json:"root"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

// CheckUpdates checks manifest-owned SDKs for newer stable releases. It never
// installs or modifies anything.
func (i Installer) CheckUpdates(ctx context.Context) ([]Update, error) {
	managedRoot, err := i.managedRoot()
	if err != nil {
		return nil, err
	}
	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return nil, err
	}
	client := i.metadata()
	latestByChannel := make(map[string]dotnet.Version)
	updates := make([]Update, 0, len(manifest.Installs))
	for _, entry := range manifest.Installs {
		if entry.Product != ProductSDK {
			// Runtime entries are checked by the runtime update flow; an
			// SDK channel lookup would misresolve runtime versions.
			continue
		}
		root := manifest.RootFor(entry)
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRoot(root, entry.Version) {
			continue
		}
		parsed, parseErr := ValidateVersion(entry.Version)
		if parseErr != nil {
			continue
		}
		channel := Channel(parsed)
		latest, ok := latestByChannel[channel]
		if !ok {
			latest, err = client.LatestSDK(ctx, entry.Version)
			if err != nil {
				return nil, fmt.Errorf("check updates for SDK %s: %w", entry.Version, err)
			}
			latestByChannel[channel] = latest
		}
		updates = append(updates, Update{
			Version:         entry.Version,
			LatestVersion:   latest.String(),
			RID:             entry.RID,
			Root:            root,
			UpdateAvailable: latest.Compare(parsed) > 0,
		})
	}
	sort.SliceStable(updates, func(left, right int) bool {
		leftVersion, leftErr := dotnet.ParseVersion(updates[left].Version)
		rightVersion, rightErr := dotnet.ParseVersion(updates[right].Version)
		if leftErr == nil && rightErr == nil {
			return leftVersion.Compare(rightVersion) > 0
		}
		return updates[left].Version > updates[right].Version
	})
	return updates, nil
}

// Install acquires the install lock, verifies the archive, and publishes the SDK.
func (i Installer) Install(ctx context.Context, plan Plan, progress io.Writer) (Result, error) {
	if progress == nil {
		progress = io.Discard
	}
	parsed, err := ValidateVersion(plan.Version)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(plan.RID) == "" {
		return Result{}, errors.New("runtime identifier is empty")
	}
	managedRoot, err := i.managedRoot()
	if err != nil {
		return Result{}, err
	}
	release, err := acquireLock(ctx, i.lockPath(managedRoot))
	if err != nil {
		return Result{}, err
	}
	defer release()

	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return Result{}, err
	}
	canonical := parsed.String()
	if entry := manifest.Find(ProductSDK, canonical); entry != nil {
		root := manifest.RootFor(*entry)
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRoot(root, canonical) {
			return Result{}, &StaleInstallError{Version: canonical, Root: root}
		}
		result := resultFromEntry(*entry, root, plan.SystemCopy)
		result.AlreadyInstalled = true
		return result, nil
	}

	finalRoot := filepath.Join(managedRoot, canonical)
	if plan.Root != "" && canonicalPath(plan.Root) != finalRoot {
		return Result{}, fmt.Errorf("plan destination %s does not match managed root destination %s", plan.Root, finalRoot)
	}
	if _, statErr := os.Lstat(finalRoot); statErr == nil {
		return Result{}, &UnownedRootError{Root: finalRoot}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect destination %s: %w", finalRoot, statErr)
	}

	if plan.SourceURL == "" || plan.SHA512 == "" {
		artifact, resolveErr := i.metadata().ResolveSDK(ctx, canonical, plan.RID)
		if resolveErr != nil {
			return Result{}, resolveErr
		}
		plan.SourceURL = artifact.URL
		plan.SHA512 = FormatSHA512(artifact.SHA512)
		plan.FileName = artifact.FileName
	}
	expected, err := ParseSHA512(plan.SHA512)
	if err != nil {
		return Result{}, fmt.Errorf("parse planned checksum: %w", err)
	}

	downloadParent := filepath.Dir(managedRoot)
	if err := os.MkdirAll(downloadParent, 0o700); err != nil {
		return Result{}, fmt.Errorf("create download parent directory: %w", err)
	}
	downloadDir, err := os.MkdirTemp(downloadParent, ".hilt-download-")
	if err != nil {
		return Result{}, fmt.Errorf("create download directory: %w", err)
	}
	defer os.RemoveAll(downloadDir)
	downloadPath, err := i.download(ctx, plan.SourceURL, expected, downloadDir, progress, "SDK")
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(managedRoot, 0o700); err != nil {
		return Result{}, fmt.Errorf("create managed root: %w", err)
	}
	stagingParent := filepath.Join(managedRoot, ".staging")
	if err := os.MkdirAll(stagingParent, 0o700); err != nil {
		return Result{}, fmt.Errorf("create staging directory: %w", err)
	}
	staging, err := os.MkdirTemp(stagingParent, canonical+"-")
	if err != nil {
		return Result{}, fmt.Errorf("create staging root: %w", err)
	}
	defer os.RemoveAll(staging)

	progressf(progress, "Extracting %s", valueOr(plan.FileName, "SDK archive"))
	if err := ExtractTarGz(downloadPath, staging, ExtractLimits{
		MaxBytes:   i.maxExtractBytes(),
		MaxEntries: i.maxArchiveEntries(),
	}); err != nil {
		return Result{}, fmt.Errorf("extract SDK archive: %w", err)
	}

	progressf(progress, "Verifying staged toolchain")
	if err := i.verify(ctx, staging, canonical); err != nil {
		return Result{}, err
	}

	if _, statErr := os.Lstat(finalRoot); statErr == nil {
		return Result{}, &UnownedRootError{Root: finalRoot}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect destination %s: %w", finalRoot, statErr)
	}
	progressf(progress, "Publishing to %s", finalRoot)
	if err := os.Rename(staging, finalRoot); err != nil {
		return Result{}, fmt.Errorf("publish SDK: %w", err)
	}

	installedAt := i.now().UTC()
	entry := Entry{
		Product:   ProductSDK,
		Version:   canonical,
		RID:       plan.RID,
		Root:      canonical,
		SourceURL: plan.SourceURL,
		SHA512:    FormatSHA512(expected),
		Artifacts: []ArtifactRecord{{
			Name:   artifactName(plan.FileName, plan.SourceURL),
			URL:    plan.SourceURL,
			SHA512: FormatSHA512(expected),
		}},
		InstalledAt: installedAt,
	}
	if err := manifest.Add(entry); err != nil {
		_ = os.RemoveAll(finalRoot)
		return Result{}, err
	}
	if err := manifest.Save(); err != nil {
		rollbackErr := os.RemoveAll(finalRoot)
		if rollbackErr != nil {
			return Result{}, fmt.Errorf("write install manifest: %w (also failed to roll back %s: %v)", err, finalRoot, rollbackErr)
		}
		return Result{}, fmt.Errorf("write install manifest: %w", err)
	}

	return Result{
		Version:     canonical,
		RID:         plan.RID,
		Root:        finalRoot,
		DotnetPath:  filepath.Join(finalRoot, "dotnet"),
		SourceURL:   plan.SourceURL,
		SHA512:      FormatSHA512(expected),
		InstalledAt: installedAt,
		SystemCopy:  plan.SystemCopy,
	}, nil
}

// Remove deletes a manifest-owned SDK and then its manifest entry. Stale
// entries are reconciled without deleting anything else.
func (i Installer) Remove(ctx context.Context, version string) (Removal, error) {
	parsed, err := ValidateVersion(version)
	if err != nil {
		return Removal{}, err
	}
	managedRoot, err := i.managedRoot()
	if err != nil {
		return Removal{}, err
	}
	release, err := acquireLock(ctx, i.lockPath(managedRoot))
	if err != nil {
		return Removal{}, err
	}
	defer release()

	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return Removal{}, err
	}
	canonical := parsed.String()
	entry := manifest.Find(ProductSDK, canonical)
	if entry == nil {
		root := filepath.Join(managedRoot, canonical)
		if _, statErr := os.Lstat(root); statErr == nil {
			return Removal{}, &NotOwnedError{Version: canonical, Root: root}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return Removal{}, fmt.Errorf("inspect managed SDK root %s: %w", root, statErr)
		}
		return Removal{}, &NotInstalledError{Version: canonical}
	}

	root := manifest.RootFor(*entry)
	if !withinRoot(managedRoot, root) {
		return Removal{}, fmt.Errorf("manifest root %s escapes the managed root", root)
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		if _, removed := manifest.Remove(ProductSDK, canonical); !removed {
			return Removal{}, fmt.Errorf("SDK %s disappeared from the install manifest during removal", canonical)
		}
		if err := manifest.Save(); err != nil {
			return Removal{}, fmt.Errorf("remove stale SDK manifest entry: %w", err)
		}
		return Removal{
			Product:   ProductSDK,
			Version:   canonical,
			RID:       entry.RID,
			Root:      root,
			Stale:     true,
			RemovedAt: i.now().UTC(),
		}, nil
	}
	if err != nil {
		return Removal{}, fmt.Errorf("inspect managed SDK root %s: %w", root, err)
	}
	if !info.IsDir() {
		return Removal{}, fmt.Errorf("recorded SDK root %s is not a directory; refusing to remove it", root)
	}

	trash := filepath.Join(managedRoot, ".removing-"+canonical+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.Rename(root, trash); err != nil {
		return Removal{}, fmt.Errorf("stage SDK removal: %w", err)
	}
	if err := i.removeAll()(trash); err != nil {
		if rollbackErr := os.Rename(trash, root); rollbackErr != nil {
			return Removal{}, fmt.Errorf("remove SDK directory: %w (also failed to restore %s: %v)", err, root, rollbackErr)
		}
		return Removal{}, fmt.Errorf("remove SDK directory: %w", err)
	}

	if _, removed := manifest.Remove(ProductSDK, canonical); !removed {
		return Removal{}, fmt.Errorf("SDK %s disappeared from the install manifest during removal", canonical)
	}
	if err := manifest.Save(); err != nil {
		return Removal{}, fmt.Errorf("remove SDK from install manifest: %w", err)
	}
	return Removal{
		Product:   ProductSDK,
		Version:   canonical,
		RID:       entry.RID,
		Root:      root,
		RemovedAt: i.now().UTC(),
	}, nil
}

func (i Installer) download(ctx context.Context, rawURL string, expected []byte, directory string, progress io.Writer, label string) (string, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse %s archive URL: %w", label, err)
	}
	if err := i.validator()(target); err != nil {
		return "", err
	}

	file, err := os.CreateTemp(directory, "archive-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	name := file.Name()
	success := false
	defer func() {
		if !success {
			_ = file.Close()
			_ = os.Remove(name)
		}
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create %s download request: %w", label, err)
	}
	request.Header.Set("User-Agent", "hilt")
	response, err := i.httpClient().Do(request)
	if err != nil {
		return "", fmt.Errorf("download %s archive: %w", label, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return "", fmt.Errorf("download %s archive: %s: %s", label, response.Status, strings.TrimSpace(string(snippet)))
	}
	limit := i.maxDownloadBytes()
	if response.ContentLength > limit {
		return "", fmt.Errorf("%s archive exceeds %d bytes", label, limit)
	}

	progressf(progress, "Downloading %s", target.Redacted())
	hasher := sha512.New()
	written, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("download %s archive: %w", label, err)
	}
	if written > limit {
		return "", fmt.Errorf("%s archive exceeds %d bytes", label, limit)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync %s archive: %w", label, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close %s archive: %w", label, err)
	}

	actual := hasher.Sum(nil)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return "", &ChecksumMismatchError{Expected: FormatSHA512(expected), Actual: FormatSHA512(actual)}
	}
	success = true
	progressf(progress, "Downloaded %s", humanBytes(written))
	progressf(progress, "Verified SHA-512")
	return name, nil
}

func (i Installer) verify(ctx context.Context, root, version string) error {
	runner := i.Runner
	if runner == nil {
		runner = process.ExecRunner{}
	}
	baseEnv := i.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	env := dotnet.BuildToolchainEnvironment(baseEnv, root)
	dotnetPath := filepath.Join(root, "dotnet")

	result, err := runner.Run(ctx, process.Spec{
		Name: dotnetPath,
		Args: []string{"--list-sdks"},
		Dir:  root,
		Env:  env,
	})
	if err != nil {
		return fmt.Errorf("run staged dotnet --list-sdks: %w", err)
	}
	sdks, err := dotnet.ParseSDKList(result.Stdout, root)
	if err != nil {
		return fmt.Errorf("parse staged dotnet --list-sdks: %w", err)
	}
	found := false
	for _, sdk := range sdks {
		if sdk.Version == version {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("staged SDK %s did not report itself in --list-sdks", version)
	}

	result, err = runner.Run(ctx, process.Spec{
		Name: dotnetPath,
		Args: []string{"--info"},
		Dir:  root,
		Env:  env,
	})
	if err != nil {
		return fmt.Errorf("run staged dotnet --info: %w", err)
	}
	info := dotnet.ParseInfo(result.Stdout)
	if info.Architecture == "" && info.RID == "" {
		return errors.New("staged dotnet --info did not report host information")
	}
	return nil
}

func (i Installer) managedRoot() (string, error) {
	root := canonicalPath(i.ManagedRoot)
	if root == "" {
		return "", errors.New("managed root is empty")
	}
	return root, nil
}

func (i Installer) manifestPath(managedRoot string) string {
	if i.ManifestPath != "" {
		return i.ManifestPath
	}
	return filepath.Join(filepath.Dir(managedRoot), "installs.json")
}

func (i Installer) lockPath(managedRoot string) string {
	if i.LockPath != "" {
		return i.LockPath
	}
	return filepath.Join(filepath.Dir(managedRoot), "installs.lock")
}

func (i Installer) metadata() *ReleaseClient {
	if i.Metadata != nil {
		return i.Metadata
	}
	return &ReleaseClient{}
}

func (i Installer) httpClient() *http.Client {
	base := i.HTTPClient
	if base == nil {
		base = &http.Client{Transport: defaultTransport()}
	}
	client := *base
	original := client.CheckRedirect
	validate := i.validator()
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := validate(request.URL); err != nil {
			return err
		}
		if original != nil {
			return original(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func (i Installer) validator() func(*url.URL) error {
	if i.ValidateURL != nil {
		return i.ValidateURL
	}
	return ValidateMicrosoftURL
}

func (i Installer) maxDownloadBytes() int64 {
	if i.MaxDownloadBytes > 0 {
		return i.MaxDownloadBytes
	}
	return defaultMaxDownloadBytes
}

func (i Installer) maxExtractBytes() int64 {
	if i.MaxExtractBytes > 0 {
		return i.MaxExtractBytes
	}
	return defaultMaxExtractBytes
}

func (i Installer) maxArchiveEntries() int {
	if i.MaxArchiveEntries > 0 {
		return i.MaxArchiveEntries
	}
	return defaultMaxArchiveEntries
}

func (i Installer) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

func (i Installer) removeAll() func(string) error {
	if i.RemoveAll != nil {
		return i.RemoveAll
	}
	return os.RemoveAll
}

func usableRoot(root, version string) bool {
	dotnetInfo, err := os.Stat(filepath.Join(root, "dotnet"))
	if err != nil || dotnetInfo.IsDir() || dotnetInfo.Mode()&0o111 == 0 {
		return false
	}
	sdkInfo, err := os.Stat(filepath.Join(root, "sdk", version))
	return err == nil && sdkInfo.IsDir()
}

// usableRuntimeRoot statically checks that every recorded framework directory
// exists under <root>/shared/<framework>/<version>.
func usableRuntimeRoot(root, version string, frameworks []string) bool {
	if len(frameworks) == 0 {
		return false
	}
	for _, framework := range frameworks {
		if strings.ContainsRune(framework, '/') {
			continue
		}
		info, err := os.Stat(filepath.Join(root, "shared", framework, version))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func planFromEntry(entry Entry, root string, installed bool) Plan {
	return Plan{
		Version:          entry.Version,
		RID:              entry.RID,
		Root:             root,
		DotnetPath:       filepath.Join(root, "dotnet"),
		SourceURL:        entry.SourceURL,
		SHA512:           entry.SHA512,
		AlreadyInstalled: installed,
	}
}

func resultFromEntry(entry Entry, root string, systemCopy bool) Result {
	return Result{
		Version:     entry.Version,
		RID:         entry.RID,
		Root:        root,
		DotnetPath:  filepath.Join(root, "dotnet"),
		SourceURL:   entry.SourceURL,
		SHA512:      entry.SHA512,
		InstalledAt: entry.InstalledAt,
		SystemCopy:  systemCopy,
	}
}

func progressf(writer io.Writer, format string, args ...any) {
	if writer == nil {
		return
	}
	_, _ = fmt.Fprintf(writer, format+"\n", args...)
}

func humanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := int64(unit), 0
	for quotient := value / unit; quotient >= unit; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func artifactName(fileName, rawURL string) string {
	if trimmed := strings.TrimSpace(fileName); trimmed != "" {
		return trimmed
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		if base := path.Base(parsed.Path); base != "" && base != "/" && base != "." {
			return base
		}
	}
	return rawURL
}
