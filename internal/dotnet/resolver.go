package dotnet

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mohammedbakri123/hilt/internal/process"
)

// Selection is the toolchain Hilt will use for a workspace.
type Selection struct {
	Root             string     `json:"root"`
	DotnetPath       string     `json:"dotnetPath"`
	SDKVersion       string     `json:"sdkVersion"`
	Source           Source     `json:"source"`
	Provenance       Provenance `json:"provenance"`
	Owned            bool       `json:"owned"`
	Rationale        string     `json:"rationale"`
	GlobalJSONPath   string     `json:"globalJsonPath,omitempty"`
	RequestedVersion string     `json:"requestedVersion,omitempty"`
	Verified         bool       `json:"verified"`
	Attempts         []Attempt  `json:"attempts,omitempty"`
}

// EffectiveProvenance returns the recorded provenance or a conservative
// fallback derived from ownership.
func (s Selection) EffectiveProvenance() Provenance {
	if s.Provenance != "" {
		return s.Provenance
	}
	if s.Owned {
		return ProvenanceOwned
	}
	return ProvenanceSystem
}

// Attempt records one candidate root considered while resolving global.json.
type Attempt struct {
	Root       string     `json:"root"`
	Source     Source     `json:"source"`
	Provenance Provenance `json:"provenance"`
	Version    string     `json:"version,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// MissingSDKError reports that no usable root satisfied the request.
type MissingSDKError struct {
	RequestedVersion string
	GlobalJSONPath   string
	Attempts         []Attempt
}

func (e *MissingSDKError) Error() string {
	if e == nil {
		return "no .NET SDK is available"
	}
	if e.RequestedVersion == "" {
		return "no .NET SDK is available in any usable installation root"
	}
	message := fmt.Sprintf("no .NET SDK satisfies global.json version %s", e.RequestedVersion)
	if e.GlobalJSONPath != "" {
		message += " (" + e.GlobalJSONPath + ")"
	}
	if len(e.Attempts) == 0 {
		return message + "; no usable installation roots were found"
	}
	details := make([]string, 0, len(e.Attempts))
	for _, attempt := range e.Attempts {
		detail := attempt.Root
		if attempt.Source != "" {
			detail += " (" + string(attempt.Source) + ")"
		}
		switch {
		case attempt.Error != "":
			detail += ": " + attempt.Error
		case attempt.Version != "":
			detail += ": reported " + attempt.Version
		}
		details = append(details, detail)
	}
	return message + "; tried " + strings.Join(details, "; ")
}

// Resolver selects a toolchain from an inventory. It never consults ambient
// dotnet resolution after a root has been chosen.
type Resolver struct {
	Inventory Inventory
	Dir       string
	Runner    process.Runner
	Stat      StatFunc
	BaseEnv   []string
}

// Resolve selects a toolchain using this order:
//
//  1. the workspace global.json requirement;
//  2. the newest managed SDK;
//  3. the newest non-managed SDK.
//
// When global.json selects a version, each candidate root is asked for
// `dotnet --version` in the workspace so the host applies roll-forward and
// prerelease rules. Hilt does not reimplement those rules.
func (r Resolver) Resolve(ctx context.Context) (Selection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Selection{}, err
	}

	stat := r.Stat
	if stat == nil {
		stat = os.Stat
	}
	globalJSON, err := FindGlobalJSON(r.Dir, stat)
	if err != nil {
		return Selection{}, err
	}

	hosts := r.usableHosts(stat)
	if globalJSON != nil && globalJSON.Version != "" {
		return r.resolveGlobalJSON(ctx, hosts, globalJSON)
	}
	return r.resolveNewest(hosts)
}

func (r Resolver) usableHosts(stat StatFunc) []Host {
	hosts := make([]Host, 0, len(r.Inventory.Hosts))
	for _, host := range r.Inventory.Hosts {
		if len(host.SDKs) == 0 {
			continue
		}
		if !hostUsable(host, stat) {
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts
}

func hostUsable(host Host, stat StatFunc) bool {
	if host.Usable {
		return true
	}
	if host.DotnetPath == "" {
		return false
	}
	info, err := stat(host.DotnetPath)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func (r Resolver) resolveGlobalJSON(ctx context.Context, hosts []Host, globalJSON *GlobalJSON) (Selection, error) {
	ordered := r.orderedHosts(hosts)
	attempts := make([]Attempt, 0, len(ordered))

	for _, host := range ordered {
		if err := ctx.Err(); err != nil {
			return Selection{}, err
		}
		attempt := Attempt{
			Root:       host.Root,
			Source:     host.Source,
			Provenance: host.EffectiveProvenance(),
		}
		version, err := r.runVersion(ctx, host)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Selection{}, ctxErr
			}
			attempt.Error = err.Error()
			attempts = append(attempts, attempt)
			continue
		}

		attempt.Version = version
		attempts = append(attempts, attempt)

		provenance := host.EffectiveProvenance()
		owned := host.Owned
		if sdk := findSDKInHost(host, version); sdk != nil {
			provenance = sdk.EffectiveProvenance()
			owned = sdk.Owned
		}
		rationale := fmt.Sprintf(
			"global.json requires %s; %s root %s satisfied it with dotnet --version %s",
			globalJSON.Version, provenance, host.Root, version,
		)
		if len(attempts) > 1 {
			rationale += fmt.Sprintf(" after %d other root(s) did not", len(attempts)-1)
		}
		return Selection{
			Root:             host.Root,
			DotnetPath:       host.DotnetPath,
			SDKVersion:       version,
			Source:           host.Source,
			Provenance:       provenance,
			Owned:            owned,
			Rationale:        rationale,
			GlobalJSONPath:   globalJSON.Path,
			RequestedVersion: globalJSON.Version,
			Verified:         true,
			Attempts:         attempts,
		}, nil
	}

	return Selection{}, &MissingSDKError{
		RequestedVersion: globalJSON.Version,
		GlobalJSONPath:   globalJSON.Path,
		Attempts:         attempts,
	}
}

func (r Resolver) resolveNewest(hosts []Host) (Selection, error) {
	ordered := r.orderedHosts(hosts)
	if len(ordered) == 0 {
		return Selection{}, &MissingSDKError{}
	}

	host := ordered[0]
	sdk := newestSDK(host)
	provenance := sdk.EffectiveProvenance()
	return Selection{
		Root:       host.Root,
		DotnetPath: host.DotnetPath,
		SDKVersion: sdk.Version,
		Source:     host.Source,
		Provenance: provenance,
		Owned:      sdk.Owned,
		Rationale:  newestRationale(provenance, sdk.Version),
	}, nil
}

func (r Resolver) orderedHosts(hosts []Host) []Host {
	managed := make([]Host, 0, len(hosts))
	other := make([]Host, 0, len(hosts))
	for _, host := range hosts {
		if isManagedProvenance(host.EffectiveProvenance()) {
			managed = append(managed, host)
			continue
		}
		other = append(other, host)
	}
	sortHosts(managed)
	sortHosts(other)
	return append(managed, other...)
}

func sortHosts(hosts []Host) {
	sort.SliceStable(hosts, func(i, j int) bool {
		left := newestSDK(hosts[i])
		right := newestSDK(hosts[j])
		if result := compareVersionStrings(left.Version, right.Version); result != 0 {
			return result > 0
		}
		leftRank := provenanceRank(hosts[i].EffectiveProvenance())
		rightRank := provenanceRank(hosts[j].EffectiveProvenance())
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return hosts[i].Root < hosts[j].Root
	})
}

func (r Resolver) runVersion(ctx context.Context, host Host) (string, error) {
	if r.Runner == nil {
		return "", fmt.Errorf("no process runner configured")
	}
	if host.DotnetPath == "" {
		return "", fmt.Errorf("no dotnet executable for %s", host.Root)
	}
	base := r.BaseEnv
	if base == nil {
		base = os.Environ()
	}
	result, err := r.Runner.Run(ctx, process.Spec{
		Name: host.DotnetPath,
		Args: []string{"--version"},
		Dir:  r.Dir,
		Env:  BuildToolchainEnvironment(base, host.Root),
	})
	if err != nil {
		if detail := strings.TrimSpace(string(result.Stderr)); detail != "" {
			return "", fmt.Errorf("%s --version: %w (%s)", host.DotnetPath, err, detail)
		}
		return "", fmt.Errorf("%s --version: %w", host.DotnetPath, err)
	}
	version := strings.TrimSpace(string(result.Stdout))
	if version == "" {
		return "", fmt.Errorf("%s --version returned no version", host.DotnetPath)
	}
	if _, err := ParseVersion(version); err != nil {
		return "", fmt.Errorf("%s --version returned %q: %w", host.DotnetPath, version, err)
	}
	return version, nil
}

func newestSDK(host Host) SDK {
	newest := host.SDKs[0]
	for _, sdk := range host.SDKs[1:] {
		if compareVersionStrings(sdk.Version, newest.Version) > 0 {
			newest = sdk
		}
	}
	return newest
}

func findSDKInHost(host Host, version string) *SDK {
	for index := range host.SDKs {
		if host.SDKs[index].Version == version {
			return &host.SDKs[index]
		}
	}
	return nil
}

func compareVersionStrings(left, right string) int {
	leftVersion, leftErr := ParseVersion(left)
	rightVersion, rightErr := ParseVersion(right)
	if leftErr == nil && rightErr == nil {
		return leftVersion.Compare(rightVersion)
	}
	return strings.Compare(left, right)
}

func isManagedProvenance(provenance Provenance) bool {
	return provenance == ProvenanceManaged || provenance == ProvenanceOwned
}

func provenanceRank(provenance Provenance) int {
	switch provenance {
	case ProvenanceOwned:
		return 0
	case ProvenanceManaged:
		return 1
	case ProvenanceSystem:
		return 2
	case ProvenanceExternal:
		return 3
	default:
		return 4
	}
}

func newestRationale(provenance Provenance, version string) string {
	switch provenance {
	case ProvenanceOwned:
		return fmt.Sprintf("owned SDK %s selected as the newest managed installation", version)
	case ProvenanceManaged:
		return fmt.Sprintf("managed SDK %s selected as the newest managed installation", version)
	case ProvenanceExternal:
		return fmt.Sprintf("external SDK %s selected as the newest user installation; no managed or system SDK is available", version)
	default:
		return fmt.Sprintf("system SDK %s selected as the newest system installation; no managed SDK is available", version)
	}
}
