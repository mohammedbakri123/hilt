package install

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

var stableVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

// ValidateVersion accepts exact stable .NET SDK versions only.
func ValidateVersion(value string) (dotnet.Version, error) {
	trimmed := strings.TrimSpace(value)
	if !stableVersionPattern.MatchString(trimmed) {
		return dotnet.Version{}, fmt.Errorf("invalid SDK version %q: expected a stable version such as 10.0.100", value)
	}
	version, err := dotnet.ParseVersion(trimmed)
	if err != nil {
		return dotnet.Version{}, err
	}
	if version.IsPrerelease() {
		return dotnet.Version{}, fmt.Errorf("invalid SDK version %q: prerelease versions are not supported", value)
	}
	return version, nil
}

// Channel returns the Microsoft release channel for a version.
func Channel(version dotnet.Version) string {
	return fmt.Sprintf("%d.%d", version.Major, version.Minor)
}

// CurrentRID returns the .NET runtime identifier for the host.
func CurrentRID() (string, error) {
	return RIDFor(runtime.GOOS, runtime.GOARCH)
}

// RIDFor maps a Go OS and architecture to a supported .NET runtime identifier.
func RIDFor(goos, goarch string) (string, error) {
	if goos != "linux" {
		return "", fmt.Errorf("managed SDK installs are not supported on %s", goos)
	}
	switch goarch {
	case "amd64":
		return "linux-x64", nil
	case "arm64":
		return "linux-arm64", nil
	default:
		return "", fmt.Errorf("managed SDK installs are not supported on linux/%s", goarch)
	}
}
