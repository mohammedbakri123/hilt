package dotnet

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SDK is a discovered .NET SDK.
type SDK struct {
	Version      string `json:"version"`
	Path         string `json:"path"`
	Root         string `json:"root"`
	Architecture string `json:"architecture,omitempty"`
	Managed      bool   `json:"managed"`
	Usable       bool   `json:"usable"`
	DetectedBy   string `json:"detectedBy"`
}

// Runtime is a discovered shared runtime.
type Runtime struct {
	Framework  string `json:"framework"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	Root       string `json:"root"`
	Managed    bool   `json:"managed"`
	DetectedBy string `json:"detectedBy"`
}

// Host is one .NET root and everything found inside it.
type Host struct {
	Root         string    `json:"root"`
	DotnetPath   string    `json:"dotnetPath"`
	Source       Source    `json:"source"`
	Managed      bool      `json:"managed"`
	Architecture string    `json:"architecture,omitempty"`
	RID          string    `json:"rid,omitempty"`
	SDKs         []SDK     `json:"sdks"`
	Runtimes     []Runtime `json:"runtimes"`
	ProbeError   string    `json:"probeError,omitempty"`
}

// Inventory is the complete detected .NET state.
type Inventory struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Hosts       []Host    `json:"hosts"`
	SDKs        []SDK     `json:"sdks"`
	Runtimes    []Runtime `json:"runtimes"`
	Warnings    []string  `json:"warnings"`
}

// Prober queries a specific .NET host.
type Prober interface {
	ListSDKs(ctx context.Context, installation Installation) ([]SDK, error)
	ListRuntimes(ctx context.Context, installation Installation) ([]Runtime, error)
	Info(ctx context.Context, installation Installation) (HostInfo, error)
}

// HostInfo is host-level metadata from `dotnet --info`.
type HostInfo struct {
	Architecture string
	RID          string
	BasePath     string
}

// Discover builds an inventory. It never returns an error: missing .NET and
// probe failures are represented as warnings and empty hosts.
func Discover(ctx context.Context, locator Locator, prober Prober, stat StatFunc) Inventory {
	if ctx == nil {
		ctx = context.Background()
	}
	if stat == nil {
		stat = os.Stat
	}

	inventory := Inventory{GeneratedAt: time.Now().UTC()}
	installations := locator.Locate()
	if len(installations) == 0 {
		inventory.Warnings = append(inventory.Warnings, "No .NET installation roots found; install the .NET SDK to use build and project features.")
		return inventory
	}

	sdkSeen := make(map[string]bool)
	runtimeSeen := make(map[string]bool)
	sdkLocations := make(map[string]map[string]bool)
	warnings := make(map[string]bool)

	for _, installation := range installations {
		host := Host{
			Root:       installation.Root,
			DotnetPath: installation.DotnetPath,
			Source:     installation.Source,
			Managed:    installation.Managed,
		}

		if installation.Usable && prober != nil {
			if sdks, err := prober.ListSDKs(ctx, installation); err != nil {
				host.ProbeError = err.Error()
				warnings["Failed to query SDKs in "+installation.Root+": "+err.Error()] = true
			} else {
				host.SDKs = sdks
			}

			if runtimes, err := prober.ListRuntimes(ctx, installation); err != nil {
				if host.ProbeError == "" {
					host.ProbeError = err.Error()
				}
				warnings["Failed to query runtimes in "+installation.Root+": "+err.Error()] = true
			} else {
				host.Runtimes = runtimes
			}

			if info, err := prober.Info(ctx, installation); err == nil {
				host.Architecture = info.Architecture
				host.RID = info.RID
			}
		}

		host.SDKs = mergeSDKs(host.SDKs, ScanSDKs(installation.Root, stat))
		host.Runtimes = mergeRuntimes(host.Runtimes, ScanRuntimes(installation.Root))

		for i := range host.SDKs {
			sdk := &host.SDKs[i]
			sdk.Root = installation.Root
			sdk.Managed = installation.Managed
			if sdk.Architecture == "" {
				sdk.Architecture = host.Architecture
			}
			if sdk.Usable = pathIsDir(sdk.Path, stat); sdk.DetectedBy == "" {
				sdk.DetectedBy = "filesystem"
			}
			key := sdk.Version + "|" + sdk.Path
			if sdkSeen[key] {
				continue
			}
			sdkSeen[key] = true
			if sdkLocations[sdk.Version] == nil {
				sdkLocations[sdk.Version] = make(map[string]bool)
			}
			sdkLocations[sdk.Version][sdk.Root] = true
			inventory.SDKs = append(inventory.SDKs, *sdk)
		}

		for i := range host.Runtimes {
			runtime := &host.Runtimes[i]
			runtime.Root = installation.Root
			runtime.Managed = installation.Managed
			if runtime.DetectedBy == "" {
				runtime.DetectedBy = "filesystem"
			}
			key := runtime.Framework + "|" + runtime.Version + "|" + runtime.Path
			if runtimeSeen[key] {
				continue
			}
			runtimeSeen[key] = true
			inventory.Runtimes = append(inventory.Runtimes, *runtime)
		}

		inventory.Hosts = append(inventory.Hosts, host)
	}

	for version, roots := range sdkLocations {
		if len(roots) > 1 {
			warnings["SDK "+version+" is present in multiple installation roots."] = true
		}
	}

	if len(inventory.SDKs) == 0 {
		warnings["No .NET SDKs were detected. Hilt can still run, but build and project features require an SDK."] = true
	}
	if len(inventory.Runtimes) == 0 {
		warnings["No .NET shared runtimes were detected."] = true
	}

	for warning := range warnings {
		inventory.Warnings = append(inventory.Warnings, warning)
	}
	sort.Strings(inventory.Warnings)
	sortInventory(&inventory)
	return inventory
}

func mergeSDKs(primary, fallback []SDK) []SDK {
	merged := make([]SDK, 0, len(primary)+len(fallback))
	seen := make(map[string]bool)
	for _, sdk := range append(primary, fallback...) {
		key := sdk.Version + "|" + sdk.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, sdk)
	}
	return merged
}

func mergeRuntimes(primary, fallback []Runtime) []Runtime {
	merged := make([]Runtime, 0, len(primary)+len(fallback))
	seen := make(map[string]bool)
	for _, runtime := range append(primary, fallback...) {
		key := runtime.Framework + "|" + runtime.Version + "|" + runtime.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, runtime)
	}
	return merged
}

func pathIsDir(path string, stat StatFunc) bool {
	if path == "" {
		return false
	}
	info, err := stat(path)
	return err == nil && info.IsDir()
}

func sortInventory(inventory *Inventory) {
	sort.SliceStable(inventory.SDKs, func(i, j int) bool {
		left, leftErr := ParseVersion(inventory.SDKs[i].Version)
		right, rightErr := ParseVersion(inventory.SDKs[j].Version)
		if leftErr == nil && rightErr == nil {
			if result := left.Compare(right); result != 0 {
				return result > 0
			}
		}
		return inventory.SDKs[i].Version > inventory.SDKs[j].Version
	})
	sort.SliceStable(inventory.Runtimes, func(i, j int) bool {
		if inventory.Runtimes[i].Framework != inventory.Runtimes[j].Framework {
			return inventory.Runtimes[i].Framework < inventory.Runtimes[j].Framework
		}
		left, leftErr := ParseVersion(inventory.Runtimes[i].Version)
		right, rightErr := ParseVersion(inventory.Runtimes[j].Version)
		if leftErr == nil && rightErr == nil {
			return left.Compare(right) > 0
		}
		return inventory.Runtimes[i].Version > inventory.Runtimes[j].Version
	})
	for i := range inventory.Hosts {
		sort.SliceStable(inventory.Hosts[i].SDKs, func(a, b int) bool {
			left, leftErr := ParseVersion(inventory.Hosts[i].SDKs[a].Version)
			right, rightErr := ParseVersion(inventory.Hosts[i].SDKs[b].Version)
			if leftErr == nil && rightErr == nil {
				return left.Compare(right) > 0
			}
			return inventory.Hosts[i].SDKs[a].Version > inventory.Hosts[i].SDKs[b].Version
		})
	}
}

// SDKVersions returns unique SDK versions, newest first.
func (i Inventory) SDKVersions() []string {
	seen := make(map[string]bool)
	versions := make([]string, 0, len(i.SDKs))
	for _, sdk := range i.SDKs {
		if !seen[sdk.Version] {
			seen[sdk.Version] = true
			versions = append(versions, sdk.Version)
		}
	}
	return versions
}

// FindSDK returns the first SDK with the exact version.
func (i Inventory) FindSDK(version string) *SDK {
	for index := range i.SDKs {
		if i.SDKs[index].Version == version {
			return &i.SDKs[index]
		}
	}
	return nil
}

// ResolvedDotnet returns the first host's executable dotnet path.
func (i Inventory) ResolvedDotnet() string {
	for _, host := range i.Hosts {
		if host.DotnetPath == "" {
			continue
		}
		if info, err := os.Stat(host.DotnetPath); err == nil && !info.IsDir() {
			return host.DotnetPath
		}
	}
	return ""
}

// HasUsableHost reports whether any host has an executable dotnet.
func (i Inventory) HasUsableHost() bool {
	for _, host := range i.Hosts {
		if host.DotnetPath == "" {
			continue
		}
		if info, err := os.Stat(host.DotnetPath); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// ScanSDKs reads <root>/sdk without invoking dotnet.
func ScanSDKs(root string, stat StatFunc) []SDK {
	if stat == nil {
		stat = os.Stat
	}
	sdkRoot := filepath.Join(root, "sdk")
	entries, err := os.ReadDir(sdkRoot)
	if err != nil {
		return nil
	}
	var sdks []SDK
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := ParseVersion(entry.Name()); err != nil {
			continue
		}
		path := filepath.Join(sdkRoot, entry.Name())
		sdks = append(sdks, SDK{
			Version:    entry.Name(),
			Path:       path,
			Root:       root,
			Usable:     pathIsDir(path, stat),
			DetectedBy: "filesystem",
		})
	}
	return sdks
}

// ScanRuntimes reads <root>/shared without invoking dotnet.
func ScanRuntimes(root string) []Runtime {
	shared := filepath.Join(root, "shared")
	frameworks, err := os.ReadDir(shared)
	if err != nil {
		return nil
	}
	var runtimes []Runtime
	for _, framework := range frameworks {
		if !framework.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(shared, framework.Name()))
		if err != nil {
			continue
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			if _, err := ParseVersion(version.Name()); err != nil {
				continue
			}
			runtimes = append(runtimes, Runtime{
				Framework:  framework.Name(),
				Version:    version.Name(),
				Path:       filepath.Join(shared, framework.Name(), version.Name()),
				Root:       root,
				DetectedBy: "filesystem",
			})
		}
	}
	return runtimes
}

// GroupRuntimes groups runtime versions by framework.
func GroupRuntimes(runtimes []Runtime) map[string][]string {
	grouped := make(map[string][]string)
	for _, runtime := range runtimes {
		grouped[runtime.Framework] = append(grouped[runtime.Framework], runtime.Version)
	}
	return grouped
}

// TrimRoot is a small helper used by renderers.
func TrimRoot(root string) string {
	if root == "" {
		return ""
	}
	return strings.TrimSuffix(root, string(filepath.Separator))
}
