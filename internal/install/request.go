package install

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

// RequestKind classifies a version request.
type RequestKind string

const (
	// RequestExact is an unambiguous version such as 10.0.112.
	RequestExact RequestKind = "exact"
	// RequestChannel is a release channel such as 10.0.
	RequestChannel RequestKind = "channel"
	// RequestLatest resolves to the newest actively supported channel.
	RequestLatest RequestKind = "latest"
	// RequestLTS resolves to the newest actively supported long-term-support
	// channel, verifiable only through the release index.
	RequestLTS RequestKind = "lts"
)

// Supported release-index support phases.
var activeSupportPhases = map[string]bool{
	"active":  true,
	"go-live": true,
}

const (
	// ChannelTypeLTS marks a long-term-support channel in the release index.
	ChannelTypeLTS = "lts"
	// ChannelTypeSTS marks a standard-term-support channel.
	ChannelTypeSTS = "sts"
)

// VersionRequest is a parsed user version input.
type VersionRequest struct {
	Kind RequestKind
	// Requested is the exact user input.
	Requested string
	// Version holds exact version numbers. Channel requests only populate
	// Major and Minor.
	Version dotnet.Version
	// AllowPrerelease was requested by the caller.
	AllowPrerelease bool
}

// ParseVersionRequest classifies a user version input. Prerelease versions are
// only accepted when allowPrerelease is set, and apply to exact or channel
// requests. Aliases and channels are resolved by the metadata client.
func ParseVersionRequest(input string, allowPrerelease bool) (VersionRequest, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return VersionRequest{}, errors.New("version request is empty")
	}

	switch strings.ToLower(trimmed) {
	case "latest":
		if allowPrerelease {
			return VersionRequest{}, fmt.Errorf("--prerelease is not supported with the latest alias")
		}
		return VersionRequest{Kind: RequestLatest, Requested: trimmed}, nil
	case "lts":
		if allowPrerelease {
			return VersionRequest{}, fmt.Errorf("--prerelease is not supported with the lts alias")
		}
		return VersionRequest{Kind: RequestLTS, Requested: trimmed}, nil
	}

	parsed, err := dotnet.ParseVersion(trimmed)
	if err != nil {
		return VersionRequest{}, fmt.Errorf("invalid version request %q: %w", trimmed, err)
	}
	if parsed.IsPrerelease() && !allowPrerelease {
		return VersionRequest{}, fmt.Errorf("prerelease version %q is not supported without --prerelease", trimmed)
	}

	if strings.Count(trimmed, ".") == 1 {
		return VersionRequest{
			Kind:            RequestChannel,
			Requested:       trimmed,
			Version:         dotnet.Version{Major: parsed.Major, Minor: parsed.Minor},
			AllowPrerelease: allowPrerelease,
		}, nil
	}

	// Exact requests need three or four numeric segments.
	if strings.Count(trimmed, ".") < 2 {
		return VersionRequest{}, fmt.Errorf("invalid version request %q: expected an alias (latest, lts), a channel such as 10.0, or an exact version such as 10.0.112", trimmed)
	}
	return VersionRequest{
		Kind:            RequestExact,
		Requested:       trimmed,
		Version:         parsed,
		AllowPrerelease: allowPrerelease,
	}, nil
}

// releaseIndex mirrors Microsoft's releases-index.json.
type releaseIndex struct {
	Channels []releaseIndexChannel `json:"releases-index"`
}

type releaseIndexChannel struct {
	ChannelVersion string `json:"channel-version"`
	LatestSDK      string `json:"latest-sdk"`
	LatestRuntime  string `json:"latest-runtime"`
	SupportPhase   string `json:"support-phase"`
	Type           string `json:"type"`
}

// active reports whether the channel is in an actively supported phase.
func (c releaseIndexChannel) active() bool {
	return activeSupportPhases[strings.ToLower(strings.TrimSpace(c.SupportPhase))]
}

// channelNumber parses the numeric channel-version.
func (c releaseIndexChannel) channelNumber() (dotnet.Version, error) {
	parts := strings.Split(strings.TrimSpace(c.ChannelVersion), ".")
	if len(parts) != 2 {
		return dotnet.Version{}, fmt.Errorf("channel-version %q is not a version channel", c.ChannelVersion)
	}
	parsed, err := dotnet.ParseVersion(c.ChannelVersion)
	if err != nil {
		return dotnet.Version{}, fmt.Errorf("parse channel-version %q: %w", c.ChannelVersion, err)
	}
	return dotnet.Version{Major: parsed.Major, Minor: parsed.Minor}, nil
}

// channelTypeLTS reports whether the channel is designated long-term support.
func (c releaseIndexChannel) channelTypeLTS() bool {
	return strings.EqualFold(strings.TrimSpace(c.Type), ChannelTypeLTS)
}

// channelForAlias returns the release-index channel an alias resolves to.
func (c *ReleaseClient) channelForAlias(ctx context.Context, request VersionRequest) (releaseIndexChannel, error) {
	index, err := c.index(ctx)
	if err != nil {
		return releaseIndexChannel{}, err
	}
	type candidate struct {
		channel releaseIndexChannel
		number  dotnet.Version
	}
	var eligible []candidate
	for _, channel := range index.Channels {
		number, err := channel.channelNumber()
		if err != nil {
			continue
		}
		if !channel.active() {
			continue
		}
		if request.Kind == RequestLTS && !channel.channelTypeLTS() {
			continue
		}
		eligible = append(eligible, candidate{channel: channel, number: number})
	}
	if len(eligible) == 0 {
		if request.Kind == RequestLTS {
			return releaseIndexChannel{}, &UnsupportedAliasError{
				Alias:   "lts",
				Reason:  "no actively supported LTS channel could be verified in the release index",
				Channel: Channel(dotnet.Version{Major: 0, Minor: 0}),
			}
		}
		return releaseIndexChannel{}, fmt.Errorf("release index contains no actively supported channels")
	}
	best := eligible[0]
	for _, other := range eligible[1:] {
		if betterChannel(other.number, best.number) {
			best = other
		}
	}
	return best.channel, nil
}

func betterChannel(candidate, incumbent dotnet.Version) bool {
	return candidate.Major > incumbent.Major || (candidate.Major == incumbent.Major && candidate.Minor > incumbent.Minor)
}

// UnsupportedAliasError reports an alias whose support cannot be verified.
type UnsupportedAliasError struct {
	Alias   string
	Reason  string
	Channel string
}

func (e *UnsupportedAliasError) Error() string {
	return fmt.Sprintf("%s alias is unsupported: %s", e.Alias, e.Reason)
}

// ResolveVersion resolves a parsed request to an exact SDK version.
func (c *ReleaseClient) ResolveVersion(ctx context.Context, request VersionRequest) (dotnet.Version, error) {
	channel, err := c.resolveChannel(ctx, request)
	if err != nil {
		return dotnet.Version{}, err
	}
	if request.Kind == RequestExact {
		return request.Version, nil
	}
	metadata, err := c.channel(ctx, channel)
	if err != nil {
		return dotnet.Version{}, err
	}
	reference := dotnet.Version{Major: request.Version.Major, Minor: request.Version.Minor}
	if channelNumber, channelErr := dotnet.ParseVersion(channel); channelErr == nil {
		reference = dotnet.Version{Major: channelNumber.Major, Minor: channelNumber.Minor}
	}
	resolved, ok := metadata.latestSDKVersion(reference, request.AllowPrerelease)
	if !ok {
		return dotnet.Version{}, &VersionNotFoundError{Version: channel, Channel: channel}
	}
	return resolved, nil
}

// ResolveRuntimeVersion resolves a parsed request to an exact runtime version
// for the given runtime archive product.
func (c *ReleaseClient) ResolveRuntimeVersion(ctx context.Context, request VersionRequest, product string) (dotnet.Version, error) {
	channel, err := c.resolveChannel(ctx, request)
	if err != nil {
		return dotnet.Version{}, err
	}
	if request.Kind == RequestExact {
		return request.Version, nil
	}
	metadata, err := c.channel(ctx, channel)
	if err != nil {
		return dotnet.Version{}, err
	}
	reference := dotnet.Version{Major: request.Version.Major, Minor: request.Version.Minor}
	resolved, ok := metadata.latestRuntimeVersion(product, reference, request.AllowPrerelease)
	if !ok {
		return dotnet.Version{}, &VersionNotFoundError{Product: product, Version: channel, Channel: channel}
	}
	return resolved, nil
}

// resolveChannel returns the channel string backing the request. Exact
// requests resolve to their own channel without any metadata access.
func (c *ReleaseClient) resolveChannel(ctx context.Context, request VersionRequest) (string, error) {
	switch request.Kind {
	case RequestExact, RequestChannel:
		return fmt.Sprintf("%d.%d", request.Version.Major, request.Version.Minor), nil
	case RequestLatest, RequestLTS:
		channel, err := c.channelForAlias(ctx, request)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(channel.ChannelVersion), nil
	default:
		return "", fmt.Errorf("unsupported version request %q", request.Requested)
	}
}
