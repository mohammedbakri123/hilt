package install

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

const (
	defaultMetadataBaseURL = "https://builds.dotnet.microsoft.com/dotnet/release-metadata"
	maxMetadataBytes       = 16 << 20
)

// Artifact describes a downloadable SDK archive.
type Artifact struct {
	Version  string
	RID      string
	URL      string
	FileName string
	SHA512   []byte
}

// ReleaseClient fetches Microsoft release metadata. Fetched channel metadata
// and the release index are cached per client.
type ReleaseClient struct {
	BaseURL     string
	HTTPClient  *http.Client
	ValidateURL func(*url.URL) error

	cacheMu      sync.Mutex
	channels     map[string]releaseMetadata
	releaseIndex *releaseIndex
}

type releaseMetadata struct {
	Releases []release `json:"releases"`
}

type release struct {
	ReleaseVersion     string       `json:"release-version"`
	SDK                *sdkRelease  `json:"sdk"`
	SDKs               []sdkRelease `json:"sdks"`
	Runtime            *sdkRelease  `json:"runtime"`
	Runtimes           []sdkRelease `json:"runtimes"`
	ASPNETCoreRuntime  *sdkRelease  `json:"aspnetcore-runtime"`
	ASPNETCoreRuntimes []sdkRelease `json:"aspnetcore-runtimes"`
}

type sdkRelease struct {
	Version string        `json:"version"`
	Files   []releaseFile `json:"files"`
}

type releaseFile struct {
	Name string `json:"name"`
	RID  string `json:"rid"`
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

// Runtime archive products accepted by ResolveRuntime.
const (
	// RuntimeArchiveRuntime is the base .NET runtime archive. It supplies
	// Microsoft.NETCore.App only.
	RuntimeArchiveRuntime = "runtime"
	// RuntimeArchiveASPNETCore is the ASP.NET Core runtime archive. It normally
	// supplies Microsoft.AspNetCore.App and Microsoft.NETCore.App.
	RuntimeArchiveASPNETCore = "aspnetcore-runtime"
)

// VersionNotFoundError reports a version absent from a release channel.
type VersionNotFoundError struct {
	Product string
	Version string
	Channel string
}

func (e *VersionNotFoundError) Error() string {
	return fmt.Sprintf("%s %s was not found in the .NET %s release metadata", metadataProductLabel(e.Product), e.Version, e.Channel)
}

// RIDNotFoundError reports a missing archive for the host runtime identifier.
type RIDNotFoundError struct {
	Product string
	Version string
	RID     string
}

func (e *RIDNotFoundError) Error() string {
	return fmt.Sprintf("%s %s has no .tar.gz archive for %s", metadataProductLabel(e.Product), e.Version, e.RID)
}

// ChecksumMissingError reports metadata without a SHA-512 hash.
type ChecksumMissingError struct {
	Product string
	Version string
	RID     string
}

func (e *ChecksumMissingError) Error() string {
	return fmt.Sprintf("%s %s for %s has no SHA-512 checksum", metadataProductLabel(e.Product), e.Version, e.RID)
}

// metadataProductLabel renders an archive product for diagnostics. An empty
// product keeps the historical SDK wording.
func metadataProductLabel(product string) string {
	switch product {
	case RuntimeArchiveRuntime:
		return "runtime"
	case RuntimeArchiveASPNETCore:
		return "ASP.NET Core runtime"
	default:
		return "SDK"
	}
}

// ErrChecksumMismatch is returned when a downloaded archive does not match its
// published checksum.
var ErrChecksumMismatch = errors.New("checksum mismatch")

// ChecksumMismatchError reports the expected and actual archive hashes.
type ChecksumMismatchError struct {
	Expected string
	Actual   string
}

func (e *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch: expected %s, got %s", e.Expected, e.Actual)
}

func (e *ChecksumMismatchError) Unwrap() error { return ErrChecksumMismatch }

// ResolveSDK returns the archive metadata for an exact SDK version and RID.
// Prerelease versions are accepted here; callers gate prerelease installs on
// the --prerelease flag.
func (c *ReleaseClient) ResolveSDK(ctx context.Context, version, rid string) (Artifact, error) {
	parsed, err := dotnet.ParseVersion(version)
	if err != nil {
		return Artifact{}, err
	}
	if rid == "" {
		return Artifact{}, errors.New("runtime identifier is empty")
	}

	channel := Channel(parsed)
	metadata, err := c.channelMetadata(ctx, parsed)
	if err != nil {
		return Artifact{}, err
	}
	sdk := metadata.findSDK(parsed.String())
	if sdk == nil {
		return Artifact{}, &VersionNotFoundError{Version: parsed.String(), Channel: channel}
	}
	file, ok := sdk.findArchive(rid)
	if !ok {
		return Artifact{}, &RIDNotFoundError{Version: parsed.String(), RID: rid}
	}
	if strings.TrimSpace(file.Hash) == "" {
		return Artifact{}, &ChecksumMissingError{Version: parsed.String(), RID: rid}
	}
	hash, err := ParseSHA512(file.Hash)
	if err != nil {
		return Artifact{}, fmt.Errorf("parse checksum for SDK %s: %w", parsed.String(), err)
	}
	archiveURL, err := url.Parse(file.URL)
	if err != nil {
		return Artifact{}, fmt.Errorf("parse SDK archive URL: %w", err)
	}
	if err := c.validator()(archiveURL); err != nil {
		return Artifact{}, err
	}
	fileName := strings.TrimSpace(file.Name)
	if fileName == "" {
		fileName = path.Base(archiveURL.Path)
	}
	return Artifact{
		Version:  parsed.String(),
		RID:      rid,
		URL:      archiveURL.String(),
		FileName: fileName,
		SHA512:   hash,
	}, nil
}

// LatestSDK returns the newest stable SDK in the same release channel.
func (c *ReleaseClient) LatestSDK(ctx context.Context, version string) (dotnet.Version, error) {
	parsed, err := ValidateVersion(version)
	if err != nil {
		return dotnet.Version{}, err
	}
	metadata, err := c.channelMetadata(ctx, parsed)
	if err != nil {
		return dotnet.Version{}, err
	}
	latest, ok := metadata.latestSDK(parsed)
	if !ok {
		return dotnet.Version{}, &VersionNotFoundError{Version: parsed.String(), Channel: Channel(parsed)}
	}
	return latest, nil
}

// ResolveRuntime returns the archive metadata for an exact runtime version,
// archive product, and RID. Product is RuntimeArchiveRuntime or
// RuntimeArchiveASPNETCore. Prerelease versions are accepted here; callers
// gate prerelease installs on the --prerelease flag.
func (c *ReleaseClient) ResolveRuntime(ctx context.Context, version, product, rid string) (Artifact, error) {
	parsed, err := dotnet.ParseVersion(version)
	if err != nil {
		return Artifact{}, err
	}
	if rid == "" {
		return Artifact{}, errors.New("runtime identifier is empty")
	}
	if product != RuntimeArchiveRuntime && product != RuntimeArchiveASPNETCore {
		return Artifact{}, fmt.Errorf("unknown runtime archive product %q", product)
	}

	channel := Channel(parsed)
	metadata, err := c.channelMetadata(ctx, parsed)
	if err != nil {
		return Artifact{}, err
	}
	archive := metadata.findRuntimeProduct(product, parsed.String())
	if archive == nil {
		return Artifact{}, &VersionNotFoundError{Product: product, Version: parsed.String(), Channel: channel}
	}
	file, ok := archive.findArchive(rid)
	if !ok {
		return Artifact{}, &RIDNotFoundError{Product: product, Version: parsed.String(), RID: rid}
	}
	if strings.TrimSpace(file.Hash) == "" {
		return Artifact{}, &ChecksumMissingError{Product: product, Version: parsed.String(), RID: rid}
	}
	hash, err := ParseSHA512(file.Hash)
	if err != nil {
		return Artifact{}, fmt.Errorf("parse checksum for %s %s: %w", metadataProductLabel(product), parsed.String(), err)
	}
	archiveURL, err := url.Parse(file.URL)
	if err != nil {
		return Artifact{}, fmt.Errorf("parse %s archive URL: %w", metadataProductLabel(product), err)
	}
	if err := c.validator()(archiveURL); err != nil {
		return Artifact{}, err
	}
	fileName := strings.TrimSpace(file.Name)
	if fileName == "" {
		fileName = path.Base(archiveURL.Path)
	}
	return Artifact{
		Version:  parsed.String(),
		RID:      rid,
		URL:      archiveURL.String(),
		FileName: fileName,
		SHA512:   hash,
	}, nil
}

// LatestRuntime returns the newest stable runtime version for an archive
// product in the same release channel.
func (c *ReleaseClient) LatestRuntime(ctx context.Context, version, product string) (dotnet.Version, error) {
	parsed, err := ValidateVersion(version)
	if err != nil {
		return dotnet.Version{}, err
	}
	if product != RuntimeArchiveRuntime && product != RuntimeArchiveASPNETCore {
		return dotnet.Version{}, fmt.Errorf("unknown runtime archive product %q", product)
	}
	metadata, err := c.channelMetadata(ctx, parsed)
	if err != nil {
		return dotnet.Version{}, err
	}
	latest, ok := metadata.latestRuntime(product, parsed)
	if !ok {
		return dotnet.Version{}, &VersionNotFoundError{Product: product, Version: parsed.String(), Channel: Channel(parsed)}
	}
	return latest, nil
}

func (c *ReleaseClient) channelMetadata(ctx context.Context, version dotnet.Version) (releaseMetadata, error) {
	return c.channel(ctx, Channel(version))
}

// channel fetches (or returns the cached) release metadata for a channel.
func (c *ReleaseClient) channel(ctx context.Context, channel string) (releaseMetadata, error) {
	if channel == "" {
		return releaseMetadata{}, errors.New("release channel is empty")
	}
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.channels != nil {
		if metadata, ok := c.channels[channel]; ok {
			return metadata, nil
		}
	} else {
		c.channels = make(map[string]releaseMetadata)
	}
	metadata, err := c.fetchChannel(ctx, channel)
	if err != nil {
		return releaseMetadata{}, err
	}
	c.channels[channel] = metadata
	return metadata, nil
}

func (c *ReleaseClient) fetchChannel(ctx context.Context, channel string) (releaseMetadata, error) {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultMetadataBaseURL
	}
	metadataURL := baseURL + "/" + channel + "/releases.json"
	parsedURL, err := url.Parse(metadataURL)
	if err != nil {
		return releaseMetadata{}, fmt.Errorf("parse release metadata URL: %w", err)
	}
	if err := c.validator()(parsedURL); err != nil {
		return releaseMetadata{}, err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := c.fetch(fetchCtx, parsedURL, maxMetadataBytes)
	if err != nil {
		return releaseMetadata{}, err
	}

	var metadata releaseMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return releaseMetadata{}, fmt.Errorf("parse release metadata: %w", err)
	}
	return metadata, nil
}

// releaseIndex caches the cross-channel release index.
func (c *ReleaseClient) index(ctx context.Context) (releaseIndex, error) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if c.releaseIndex != nil {
		return *c.releaseIndex, nil
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultMetadataBaseURL
	}
	indexURL := baseURL + "/releases-index.json"
	parsedURL, err := url.Parse(indexURL)
	if err != nil {
		return releaseIndex{}, fmt.Errorf("parse release index URL: %w", err)
	}
	if err := c.validator()(parsedURL); err != nil {
		return releaseIndex{}, err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := c.fetch(fetchCtx, parsedURL, maxMetadataBytes)
	if err != nil {
		return releaseIndex{}, fmt.Errorf("fetch release index: %w", err)
	}
	var parsed releaseIndex
	if err := json.Unmarshal(data, &parsed); err != nil {
		return releaseIndex{}, fmt.Errorf("parse release index: %w", err)
	}
	if len(parsed.Channels) == 0 {
		return releaseIndex{}, fmt.Errorf("parse release index: no channels were found")
	}
	c.releaseIndex = &parsed
	return parsed, nil
}

// productReleases returns the singular and plural metadata entries carrying
// versions for a product.
func (r release) productReleases(product string) []sdkRelease {
	var entries []sdkRelease
	switch product {
	case "sdk":
		entries = append(entries, r.SDKs...)
		if r.SDK != nil {
			entries = append(entries, *r.SDK)
		}
	case RuntimeArchiveRuntime:
		entries = append(entries, r.Runtimes...)
		if r.Runtime != nil {
			entries = append(entries, *r.Runtime)
		}
	case RuntimeArchiveASPNETCore:
		entries = append(entries, r.ASPNETCoreRuntimes...)
		if r.ASPNETCoreRuntime != nil {
			entries = append(entries, *r.ASPNETCoreRuntime)
		}
	default:
		return nil
	}
	return entries
}

// latestSDKVersion returns the newest version in the channel for the SDK. When
// allowPrerelease is set, entries from the plural sdks array also compete.
func (m releaseMetadata) latestSDKVersion(reference dotnet.Version, allowPrerelease bool) (dotnet.Version, bool) {
	return latestVersionInChannel(m, "sdk", reference, allowPrerelease)
}

// latestRuntimeVersion returns the newest version in the channel for a runtime
// archive product. When allowPrerelease is set, plural entries also compete.
func (m releaseMetadata) latestRuntimeVersion(product string, reference dotnet.Version, allowPrerelease bool) (dotnet.Version, bool) {
	return latestVersionInChannel(m, product, reference, allowPrerelease)
}

func latestVersionInChannel(metadata releaseMetadata, product string, reference dotnet.Version, allowPrerelease bool) (dotnet.Version, bool) {
	var latest dotnet.Version
	found := false
	add := func(version string) {
		parsed, err := dotnet.ParseVersion(version)
		if err != nil || parsed.Major != reference.Major || parsed.Minor != reference.Minor {
			return
		}
		if !allowPrerelease && parsed.IsPrerelease() {
			return
		}
		if !found || parsed.Compare(latest) > 0 {
			latest = parsed
			found = true
		}
	}
	for _, release := range metadata.Releases {
		for _, entry := range release.productReleases(product) {
			add(entry.Version)
		}
	}
	return latest, found
}

func (m releaseMetadata) latestSDK(reference dotnet.Version) (dotnet.Version, bool) {
	var latest dotnet.Version
	found := false
	add := func(value string) {
		candidate, err := ValidateVersion(value)
		if err != nil || candidate.Major != reference.Major || candidate.Minor != reference.Minor {
			return
		}
		if !found || candidate.Compare(latest) > 0 {
			latest = candidate
			found = true
		}
	}
	for _, release := range m.Releases {
		if release.SDK != nil {
			add(release.SDK.Version)
		}
		for _, sdk := range release.SDKs {
			add(sdk.Version)
		}
	}
	return latest, found
}

func (m releaseMetadata) findSDK(version string) *sdkRelease {
	var fallback *sdkRelease
	for index := range m.Releases {
		current := &m.Releases[index]
		if current.SDK != nil && current.SDK.Version == version {
			if len(current.SDK.Files) > 0 {
				return current.SDK
			}
			if fallback == nil {
				fallback = current.SDK
			}
		}
		for sdkIndex := range current.SDKs {
			if current.SDKs[sdkIndex].Version == version {
				if len(current.SDKs[sdkIndex].Files) > 0 {
					return &current.SDKs[sdkIndex]
				}
				if fallback == nil {
					fallback = &current.SDKs[sdkIndex]
				}
			}
		}
	}
	return fallback
}

// productRelease returns the release entry for a runtime archive product. An
// unknown product yields nil.
func (r release) productRelease(product string) *sdkRelease {
	switch product {
	case RuntimeArchiveRuntime:
		return r.Runtime
	case RuntimeArchiveASPNETCore:
		return r.ASPNETCoreRuntime
	default:
		return nil
	}
}

func (m releaseMetadata) findRuntimeProduct(product, version string) *sdkRelease {
	var fallback *sdkRelease
	for index := range m.Releases {
		current := &m.Releases[index]
		entry := current.productRelease(product)
		if entry != nil && entry.Version == version {
			if len(entry.Files) > 0 {
				return entry
			}
			if fallback == nil {
				fallback = entry
			}
		}
		for _, plural := range current.productPluralPointers(product) {
			if plural.Version == version {
				if len(plural.Files) > 0 {
					return plural
				}
				if fallback == nil {
					fallback = plural
				}
			}
		}
	}
	return fallback
}

// productPluralPointers returns pointers to the plural array entries for a
// runtime archive product.
func (r release) productPluralPointers(product string) []*sdkRelease {
	var collection []sdkRelease
	switch product {
	case RuntimeArchiveRuntime:
		collection = r.Runtimes
	case RuntimeArchiveASPNETCore:
		collection = r.ASPNETCoreRuntimes
	default:
		return nil
	}
	entries := make([]*sdkRelease, 0, len(collection))
	for index := range collection {
		entries = append(entries, &collection[index])
	}
	return entries
}

func (m releaseMetadata) latestRuntime(product string, reference dotnet.Version) (dotnet.Version, bool) {
	var latest dotnet.Version
	found := false
	for _, release := range m.Releases {
		entry := release.productRelease(product)
		if entry == nil {
			continue
		}
		candidate, err := ValidateVersion(entry.Version)
		if err != nil || candidate.Major != reference.Major || candidate.Minor != reference.Minor {
			continue
		}
		if !found || candidate.Compare(latest) > 0 {
			latest = candidate
			found = true
		}
	}
	return latest, found
}

func (s sdkRelease) findArchive(rid string) (releaseFile, bool) {
	for _, file := range s.Files {
		if file.RID != rid {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if name == "" {
			if parsed, err := url.Parse(file.URL); err == nil {
				name = strings.ToLower(path.Base(parsed.Path))
			}
		}
		if strings.HasSuffix(name, ".tar.gz") {
			return file, true
		}
	}
	return releaseFile{}, false
}

func (c *ReleaseClient) fetch(ctx context.Context, target *url.URL, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create release metadata request: %w", err)
	}
	request.Header.Set("User-Agent", "hilt")
	response, err := c.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch release metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, fmt.Errorf("fetch release metadata: %s: %s", response.Status, strings.TrimSpace(string(snippet)))
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("release metadata exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read release metadata: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("release metadata exceeds %d bytes", limit)
	}
	return data, nil
}

func (c *ReleaseClient) client() *http.Client {
	base := c.HTTPClient
	if base == nil {
		base = &http.Client{Transport: defaultTransport()}
	}
	client := *base
	original := client.CheckRedirect
	validate := c.validator()
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

func (c *ReleaseClient) validator() func(*url.URL) error {
	if c.ValidateURL != nil {
		return c.ValidateURL
	}
	return ValidateMicrosoftURL
}

func defaultTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.IdleConnTimeout = 90 * time.Second
	return transport
}

// ValidateMicrosoftURL restricts downloads to Microsoft HTTPS hosts.
func ValidateMicrosoftURL(target *url.URL) error {
	if target == nil {
		return errors.New("URL is nil")
	}
	if target.Scheme != "https" {
		return fmt.Errorf("refusing non-HTTPS URL %s", target.Redacted())
	}
	host := strings.ToLower(target.Hostname())
	if isMicrosoftHost(host) {
		return nil
	}
	return fmt.Errorf("refusing non-Microsoft URL %s", target.Redacted())
}

func isMicrosoftHost(host string) bool {
	for _, suffix := range []string{
		".microsoft.com",
		".azureedge.net",
		".blob.core.windows.net",
	} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// ParseSHA512 accepts a 64-byte SHA-512 digest encoded as hex or base64.
func ParseSHA512(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, errors.New("checksum is empty")
	}
	if len(trimmed) == sha512.Size*2 {
		if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == sha512.Size {
			return decoded, nil
		}
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(trimmed); err == nil && len(decoded) == sha512.Size {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid SHA-512 checksum %q", value)
}

// FormatSHA512 renders a digest as lowercase hex.
func FormatSHA512(hash []byte) string {
	return hex.EncodeToString(hash)
}
