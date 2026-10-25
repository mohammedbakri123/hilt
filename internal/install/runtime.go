package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/process"
)

// Shared-framework identifiers recorded by managed runtime installs.
const (
	RuntimeFrameworkNetCore    = "Microsoft.NETCore.App"
	RuntimeFrameworkASPNETCore = "Microsoft.AspNetCore.App"
)

const runtimeRootName = "runtimes"

// FrameworkFlavor selects which runtime frameworks a managed runtime install
// targets.
type FrameworkFlavor string

const (
	// FrameworkAll installs both frameworks. The ASP.NET Core runtime archive
	// normally supplies Microsoft.AspNetCore.App and Microsoft.NETCore.App.
	FrameworkAll FrameworkFlavor = "all"
	// FrameworkNetCore installs only Microsoft.NETCore.App, using the runtime
	// archive.
	FrameworkNetCore FrameworkFlavor = "netcore"
	// FrameworkASPNETCore installs Microsoft.AspNetCore.App, normally together
	// with Microsoft.NETCore.App, using the ASP.NET Core runtime archive.
	FrameworkASPNETCore FrameworkFlavor = "aspnetcore"
)

func (f FrameworkFlavor) Valid() bool {
	switch f {
	case FrameworkAll, FrameworkNetCore, FrameworkASPNETCore:
		return true
	}
	return false
}

// archiveProduct names the Microsoft artifact downloaded for this flavor.
// aspnetcore/all use the ASP.NET Core runtime archive; it normally supplies
// both frameworks. netcore uses the smaller runtime archive.
func (f FrameworkFlavor) archiveProduct() string {
	if f == FrameworkNetCore {
		return RuntimeArchiveRuntime
	}
	return RuntimeArchiveASPNETCore
}

// targetFrameworks names the shared frameworks that must be present after
// extraction. aspnetcore/all verify both frameworks; netcore only the base one.
func (f FrameworkFlavor) targetFrameworks() []string {
	if f == FrameworkNetCore {
		return []string{RuntimeFrameworkNetCore}
	}
	return []string{RuntimeFrameworkASPNETCore, RuntimeFrameworkNetCore}
}

// parseFrameworkFlavor converts user-facing framework spellings.
func parseFrameworkFlavor(value string) (FrameworkFlavor, error) {
	flavor := FrameworkFlavor(strings.TrimSpace(value))
	if !flavor.Valid() {
		return FrameworkAll, fmt.Errorf("invalid framework %q: expected all, netcore, or aspnetcore", value)
	}
	return flavor, nil
}

// RuntimeArtifact describes one runtime archive planned for download.
type RuntimeArtifact struct {
	Product  string `json:"product"`
	URL      string `json:"url"`
	FileName string `json:"fileName,omitempty"`
	SHA512   string `json:"sha512"`
}

// RuntimePlan describes a runtime installation before any download occurs.
type RuntimePlan struct {
	Version          string            `json:"version"`
	RID              string            `json:"rid"`
	Framework        string            `json:"framework"`
	Frameworks       []string          `json:"frameworks"`
	Root             string            `json:"root"`
	Artifacts        []RuntimeArtifact `json:"artifacts,omitempty"`
	AlreadyInstalled bool              `json:"alreadyInstalled"`
	// SystemCopy reports whether the same runtime version also exists in a
	// non-managed root. Hilt never touches it.
	SystemCopy bool `json:"systemCopy,omitempty"`
}

// RuntimeResult describes a completed or already-present managed runtime install.
type RuntimeResult struct {
	Version          string           `json:"version"`
	RID              string           `json:"rid"`
	Framework        string           `json:"framework"`
	Frameworks       []string         `json:"frameworks"`
	Root             string           `json:"root"`
	Artifacts        []ArtifactRecord `json:"artifacts,omitempty"`
	InstalledAt      time.Time        `json:"installedAt"`
	AlreadyInstalled bool             `json:"alreadyInstalled"`
	// VerifiedWith records the dotnet host that produced the --list-runtimes
	// output used to verify the install.
	VerifiedWith string `json:"verifiedWith,omitempty"`
}

// RuntimeRoot returns the managed install root for a runtime version.
func RuntimeRoot(managedRoot, version string) string {
	return filepath.Join(managedRoot, runtimeRootName, version)
}

// PlanRuntime resolves metadata and checks for an existing runtime installation.
func (i Installer) PlanRuntime(ctx context.Context, version string, flavor FrameworkFlavor) (RuntimePlan, error) {
	if !flavor.Valid() {
		return RuntimePlan{}, fmt.Errorf("invalid framework %q: expected all, netcore, or aspnetcore", flavor)
	}
	parsed, err := ValidateVersion(version)
	if err != nil {
		return RuntimePlan{}, err
	}
	rid, err := CurrentRID()
	if err != nil {
		return RuntimePlan{}, err
	}
	managedRoot, err := i.managedRoot()
	if err != nil {
		return RuntimePlan{}, err
	}
	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return RuntimePlan{}, err
	}

	canonical := parsed.String()
	root := RuntimeRoot(managedRoot, canonical)
	if entry := manifest.Find(ProductRuntime, canonical); entry != nil {
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRuntimeRoot(root, canonical, entry.Frameworks) {
			return RuntimePlan{}, &StaleInstallError{Product: ProductRuntime, Version: canonical, Root: root}
		}
		return runtimePlanFromEntry(*entry, root, true), nil
	}

	if info, statErr := os.Lstat(root); statErr == nil {
		if !info.IsDir() {
			return RuntimePlan{}, fmt.Errorf("destination %s exists and is not a directory", root)
		}
		return RuntimePlan{}, &UnownedRootError{Root: root}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return RuntimePlan{}, fmt.Errorf("inspect destination %s: %w", root, statErr)
	}

	artifacts, err := i.resolveRuntimeArtifacts(ctx, canonical, rid, flavor)
	if err != nil {
		return RuntimePlan{}, err
	}
	return RuntimePlan{
		Version:    canonical,
		RID:        rid,
		Framework:  string(flavor),
		Frameworks: flavor.targetFrameworks(),
		Root:       root,
		Artifacts:  artifacts,
	}, nil
}

func (i Installer) resolveRuntimeArtifacts(ctx context.Context, version, rid string, flavor FrameworkFlavor) ([]RuntimeArtifact, error) {
	artifact, err := i.metadata().ResolveRuntime(ctx, version, flavor.archiveProduct(), rid)
	if err != nil {
		return nil, err
	}
	return []RuntimeArtifact{{
		Product:  flavor.archiveProduct(),
		URL:      artifact.URL,
		FileName: artifact.FileName,
		SHA512:   FormatSHA512(artifact.SHA512),
	}}, nil
}

// InstallRuntime acquires the install lock, verifies every planned archive, and
// publishes the staged runtime atomically. All archives are downloaded before
// extraction, and the manifest entry is written last. A failure anywhere before
// publication leaves the managed root unchanged.
func (i Installer) InstallRuntime(ctx context.Context, plan RuntimePlan, progress io.Writer) (RuntimeResult, error) {
	if progress == nil {
		progress = io.Discard
	}
	flavor, err := parseFrameworkFlavor(plan.Framework)
	if err != nil {
		return RuntimeResult{}, err
	}
	parsed, err := ValidateVersion(plan.Version)
	if err != nil {
		return RuntimeResult{}, err
	}
	if strings.TrimSpace(plan.RID) == "" {
		return RuntimeResult{}, errors.New("runtime identifier is empty")
	}
	managedRoot, err := i.managedRoot()
	if err != nil {
		return RuntimeResult{}, err
	}
	release, err := acquireLock(ctx, i.lockPath(managedRoot))
	if err != nil {
		return RuntimeResult{}, err
	}
	defer release()

	manifest, err := Load(i.manifestPath(managedRoot), managedRoot)
	if err != nil {
		return RuntimeResult{}, err
	}
	canonical := parsed.String()
	if entry := manifest.Find(ProductRuntime, canonical); entry != nil {
		root := manifest.RootFor(*entry)
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRuntimeRoot(root, canonical, entry.Frameworks) {
			return RuntimeResult{}, &StaleInstallError{Product: ProductRuntime, Version: canonical, Root: root}
		}
		result := runtimeResultFromEntry(*entry, root, flavor)
		result.AlreadyInstalled = true
		return result, nil
	}

	finalRoot := RuntimeRoot(managedRoot, canonical)
	if plan.Root != "" && canonicalPath(plan.Root) != finalRoot {
		return RuntimeResult{}, fmt.Errorf("plan destination %s does not match managed root destination %s", plan.Root, finalRoot)
	}
	if _, statErr := os.Lstat(finalRoot); statErr == nil {
		return RuntimeResult{}, &UnownedRootError{Root: finalRoot}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return RuntimeResult{}, fmt.Errorf("inspect destination %s: %w", finalRoot, statErr)
	}

	artifacts := plan.Artifacts
	if len(artifacts) == 0 {
		resolved, resolveErr := i.resolveRuntimeArtifacts(ctx, canonical, plan.RID, flavor)
		if resolveErr != nil {
			return RuntimeResult{}, resolveErr
		}
		artifacts = resolved
	}
	if len(artifacts) == 0 {
		return RuntimeResult{}, errors.New("planned runtime install has no artifacts")
	}
	expected := make([][]byte, len(artifacts))
	for index, artifact := range artifacts {
		digest, digestErr := ParseSHA512(artifact.SHA512)
		if digestErr != nil {
			return RuntimeResult{}, fmt.Errorf("parse planned checksum for %s: %w", artifactLabel(artifact.Product), digestErr)
		}
		expected[index] = digest
	}

	downloadParent := filepath.Dir(managedRoot)
	if err := os.MkdirAll(downloadParent, 0o700); err != nil {
		return RuntimeResult{}, fmt.Errorf("create download parent directory: %w", err)
	}
	downloadDir, err := os.MkdirTemp(downloadParent, ".hilt-download-")
	if err != nil {
		return RuntimeResult{}, fmt.Errorf("create download directory: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	downloadPaths := make([]string, len(artifacts))
	for index, artifact := range artifacts {
		label := artifactLabel(artifact.Product)
		path, downloadErr := i.download(ctx, artifact.URL, expected[index], downloadDir, progress, label)
		if downloadErr != nil {
			return RuntimeResult{}, downloadErr
		}
		downloadPaths[index] = path
	}

	if err := os.MkdirAll(managedRoot, 0o700); err != nil {
		return RuntimeResult{}, fmt.Errorf("create managed root: %w", err)
	}
	stagingParent := filepath.Join(managedRoot, ".staging")
	if err := os.MkdirAll(stagingParent, 0o700); err != nil {
		return RuntimeResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	staging, err := os.MkdirTemp(stagingParent, canonical+"-")
	if err != nil {
		return RuntimeResult{}, fmt.Errorf("create staging root: %w", err)
	}
	defer os.RemoveAll(staging)

	for index, artifact := range artifacts {
		progressf(progress, "Extracting %s", valueOr(artifact.FileName, artifactLabel(artifact.Product)+" archive"))
		if err := ExtractTarGz(downloadPaths[index], staging, ExtractLimits{
			MaxBytes:   i.maxExtractBytes(),
			MaxEntries: i.maxArchiveEntries(),
		}); err != nil {
			return RuntimeResult{}, fmt.Errorf("extract %s archive: %w", artifactLabel(artifact.Product), err)
		}
	}

	progressf(progress, "Verifying staged runtime")
	statement, verifyErr := i.verifyRuntime(ctx, staging, canonical, flavor.targetFrameworks())
	if verifyErr != nil {
		return RuntimeResult{}, verifyErr
	}

	if _, statErr := os.Lstat(finalRoot); statErr == nil {
		return RuntimeResult{}, &UnownedRootError{Root: finalRoot}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return RuntimeResult{}, fmt.Errorf("inspect destination %s: %w", finalRoot, statErr)
	}
	progressf(progress, "Publishing to %s", finalRoot)
	if err := os.MkdirAll(filepath.Dir(finalRoot), 0o700); err != nil {
		return RuntimeResult{}, fmt.Errorf("create runtime parent directory: %w", err)
	}
	if err := os.Rename(staging, finalRoot); err != nil {
		return RuntimeResult{}, fmt.Errorf("publish runtime: %w", err)
	}

	installedAt := i.now().UTC()
	records := make([]ArtifactRecord, 0, len(artifacts))
	for index, artifact := range artifacts {
		records = append(records, ArtifactRecord{
			Name:   artifactName(artifact.FileName, artifact.URL),
			URL:    artifact.URL,
			SHA512: FormatSHA512(expected[index]),
		})
	}
	entry := Entry{
		Product:     ProductRuntime,
		Version:     canonical,
		RID:         plan.RID,
		Root:        filepath.ToSlash(filepath.Join(runtimeRootName, canonical)),
		SourceURL:   artifacts[0].URL,
		SHA512:      FormatSHA512(expected[0]),
		Artifacts:   records,
		Frameworks:  flavor.targetFrameworks(),
		InstalledAt: installedAt,
	}
	if err := manifest.Add(entry); err != nil {
		_ = os.RemoveAll(finalRoot)
		return RuntimeResult{}, err
	}
	if err := manifest.Save(); err != nil {
		rollbackErr := os.RemoveAll(finalRoot)
		if rollbackErr != nil {
			return RuntimeResult{}, fmt.Errorf("write install manifest: %w (also failed to roll back %s: %v)", err, finalRoot, rollbackErr)
		}
		return RuntimeResult{}, fmt.Errorf("write install manifest: %w", err)
	}

	return RuntimeResult{
		Version:      canonical,
		RID:          plan.RID,
		Framework:    string(flavor),
		Frameworks:   flavor.targetFrameworks(),
		Root:         finalRoot,
		Artifacts:    records,
		InstalledAt:  installedAt,
		VerifiedWith: statement,
	}, nil
}

// RemoveRuntime deletes a manifest-owned runtime and then its manifest entry.
// Stale entries are reconciled without deleting anything else.
func (i Installer) RemoveRuntime(ctx context.Context, version string) (Removal, error) {
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
	entry := manifest.Find(ProductRuntime, canonical)
	if entry == nil {
		root := RuntimeRoot(managedRoot, canonical)
		if _, statErr := os.Lstat(root); statErr == nil {
			return Removal{}, &NotOwnedError{Product: ProductRuntime, Version: canonical, Root: root}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return Removal{}, fmt.Errorf("inspect managed runtime root %s: %w", root, statErr)
		}
		return Removal{}, &NotInstalledError{Product: ProductRuntime, Version: canonical}
	}

	root := manifest.RootFor(*entry)
	if !withinRoot(managedRoot, root) {
		return Removal{}, fmt.Errorf("manifest root %s escapes the managed root", root)
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		if _, removed := manifest.Remove(ProductRuntime, canonical); !removed {
			return Removal{}, fmt.Errorf("runtime %s disappeared from the install manifest during removal", canonical)
		}
		if err := manifest.Save(); err != nil {
			return Removal{}, fmt.Errorf("remove stale runtime manifest entry: %w", err)
		}
		return Removal{
			Product:   ProductRuntime,
			Version:   canonical,
			RID:       entry.RID,
			Root:      root,
			Stale:     true,
			RemovedAt: i.now().UTC(),
		}, nil
	}
	if err != nil {
		return Removal{}, fmt.Errorf("inspect managed runtime root %s: %w", root, err)
	}
	if !info.IsDir() {
		return Removal{}, fmt.Errorf("recorded runtime root %s is not a directory; refusing to remove it", root)
	}

	trash := filepath.Join(managedRoot, ".removing-runtime-"+canonical+"-"+time.Now().Format("20060102T150405.000000000"))
	if err := os.Rename(root, trash); err != nil {
		return Removal{}, fmt.Errorf("stage runtime removal: %w", err)
	}
	if err := i.removeAll()(trash); err != nil {
		if rollbackErr := os.Rename(trash, root); rollbackErr != nil {
			return Removal{}, fmt.Errorf("remove runtime directory: %w (also failed to restore %s: %v)", err, root, rollbackErr)
		}
		return Removal{}, fmt.Errorf("remove runtime directory: %w", err)
	}

	if _, removed := manifest.Remove(ProductRuntime, canonical); !removed {
		return Removal{}, fmt.Errorf("runtime %s disappeared from the install manifest during removal", canonical)
	}
	if err := manifest.Save(); err != nil {
		return Removal{}, fmt.Errorf("remove runtime from install manifest: %w", err)
	}
	return Removal{
		Product:   ProductRuntime,
		Version:   canonical,
		RID:       entry.RID,
		Root:      root,
		RemovedAt: i.now().UTC(),
	}, nil
}

// verifyRuntime confirms that the staged root reports every expected shared
// framework at the exact version through `dotnet --list-runtimes` with the
// explicit toolchain environment. When the staged root has no usable dotnet
// host, a host resolved by FindHost runs with DOTNET_ROOT pointed at staging.
// The returned string names the host that performed verification.
func (i Installer) verifyRuntime(ctx context.Context, root, version string, expected []string) (string, error) {
	runner := i.Runner
	if runner == nil {
		runner = process.ExecRunner{}
	}
	baseEnv := i.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	env := dotnet.BuildToolchainEnvironment(baseEnv, root)
	host := filepath.Join(root, "dotnet")
	if info, err := os.Stat(host); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		fallback, found := i.host()
		if !found {
			return "", fmt.Errorf("staged runtime %s has no dotnet host and no alternate host was found to verify the installation", root)
		}
		host = fallback
	}

	result, err := runner.Run(ctx, process.Spec{
		Name: host,
		Args: []string{"--list-runtimes"},
		Dir:  root,
		Env:  env,
	})
	if err != nil {
		return "", fmt.Errorf("run staged dotnet --list-runtimes: %w", err)
	}
	runtimes, err := dotnet.ParseRuntimeList(result.Stdout, root)
	if err != nil {
		return "", fmt.Errorf("parse staged dotnet --list-runtimes: %w", err)
	}
	present := make(map[string]map[string]bool)
	for _, runtime := range runtimes {
		byVersion, ok := present[runtime.Framework]
		if !ok {
			byVersion = make(map[string]bool)
			present[runtime.Framework] = byVersion
		}
		byVersion[runtime.Version] = true
	}
	for _, framework := range expected {
		if !present[framework][version] {
			return "", fmt.Errorf("staged runtime %s did not report %s %s in --list-runtimes", root, framework, version)
		}
	}
	return host, nil
}

// CheckRuntimeUpdates checks manifest-owned runtimes for newer stable releases
// in the same release channel. It never installs or modifies anything.
func (i Installer) CheckRuntimeUpdates(ctx context.Context) ([]Update, error) {
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
		if entry.Product != ProductRuntime {
			continue
		}
		root := manifest.RootFor(entry)
		info, statErr := os.Stat(root)
		if statErr != nil || !info.IsDir() || !usableRuntimeRoot(root, entry.Version, entry.Frameworks) {
			continue
		}
		parsed, parseErr := ValidateVersion(entry.Version)
		if parseErr != nil {
			continue
		}
		product := runtimeProductForFrameworks(entry.Frameworks)
		cacheKey := Channel(parsed) + "\x00" + product
		latest, ok := latestByChannel[cacheKey]
		if !ok {
			latest, err = client.LatestRuntime(ctx, entry.Version, product)
			if err != nil {
				return nil, fmt.Errorf("check updates for runtime %s: %w", entry.Version, err)
			}
			latestByChannel[cacheKey] = latest
		}
		updates = append(updates, Update{
			Version:         entry.Version,
			LatestVersion:   latest.String(),
			RID:             entry.RID,
			Root:            root,
			UpdateAvailable: latest.Compare(parsed) > 0,
		})
	}
	sortRuntimeUpdatesStable(updates)
	return updates, nil
}

// runtimeProductForFrameworks maps recorded frameworks back to the archive
// product that supplies them.
func runtimeProductForFrameworks(frameworks []string) string {
	for _, framework := range frameworks {
		if framework == RuntimeFrameworkASPNETCore {
			return RuntimeArchiveASPNETCore
		}
	}
	return RuntimeArchiveRuntime
}

func artifactLabel(product string) string {
	return metadataProductLabel(product)
}

// flavorForEntry infers the install flavor of an already-recorded runtime.
// Only-netcore entries were built from the runtime archive; everything else
// carries both frameworks and is shown as all.
func flavorForEntry(entry Entry) string {
	if runtimeProductForFrameworks(entry.Frameworks) == RuntimeArchiveRuntime {
		return string(FrameworkNetCore)
	}
	return string(FrameworkAll)
}

func runtimePlanFromEntry(entry Entry, root string, installed bool) RuntimePlan {
	return RuntimePlan{
		Version:          entry.Version,
		RID:              entry.RID,
		Framework:        flavorForEntry(entry),
		Frameworks:       append([]string(nil), entry.Frameworks...),
		Root:             root,
		AlreadyInstalled: installed,
	}
}

func runtimeResultFromEntry(entry Entry, root string, flavor FrameworkFlavor) RuntimeResult {
	records := make([]ArtifactRecord, len(entry.Artifacts))
	copy(records, entry.Artifacts)
	return RuntimeResult{
		Version:     entry.Version,
		RID:         entry.RID,
		Framework:   string(flavor),
		Frameworks:  append([]string(nil), entry.Frameworks...),
		Root:        root,
		Artifacts:   records,
		InstalledAt: entry.InstalledAt,
	}
}

func sortRuntimeUpdatesStable(updates []Update) {
	sort.SliceStable(updates, func(left, right int) bool {
		leftVersion, leftErr := dotnet.ParseVersion(updates[left].Version)
		rightVersion, rightErr := dotnet.ParseVersion(updates[right].Version)
		if leftErr == nil && rightErr == nil {
			return leftVersion.Compare(rightVersion) > 0
		}
		return updates[left].Version > updates[right].Version
	})
}
