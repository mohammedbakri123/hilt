package platform

import (
	"os"
	"runtime"
	"strings"
)

// Info describes the host platform Hilt is running on.
type Info struct {
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	Distro        string   `json:"distro"`
	DistroName    string   `json:"distroName"`
	DistroVersion string   `json:"distroVersion"`
	DistroLike    []string `json:"distroLike"`
	Kernel        string   `json:"kernel"`
	IsLinux       bool     `json:"isLinux"`
}

// Detect inspects the current host. Missing files are not treated as fatal.
func Detect() Info {
	return DetectFrom("/etc/os-release", "/proc/sys/kernel/osrelease")
}

// DetectFrom is the testable form of Detect.
func DetectFrom(osReleasePath, kernelPath string) Info {
	info := Info{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		IsLinux: runtime.GOOS == "linux",
	}

	if data, err := os.ReadFile(osReleasePath); err == nil {
		if values, err := ParseOSRelease(data); err == nil {
			info.Distro = values["ID"]
			info.DistroName = values["PRETTY_NAME"]
			info.DistroVersion = values["VERSION_ID"]
			info.DistroLike = strings.Fields(values["ID_LIKE"])
		}
	}

	if data, err := os.ReadFile(kernelPath); err == nil {
		info.Kernel = strings.TrimSpace(string(data))
	}

	return info
}

// ParseOSRelease parses the simple KEY=VALUE format used by os-release.
func ParseOSRelease(data []byte) (map[string]string, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			quote := value[0]
			value = value[1 : len(value)-1]
			if quote == '"' && value != "" {
				value = unescape(value)
			}
		}
		values[key] = value
	}
	return values, nil
}

// unescape resolves the escapes permitted inside double-quoted os-release
// values. Single-quoted values are literal.
func unescape(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			switch value[index+1] {
			case '"', '\\', '$', '`':
				builder.WriteByte(value[index+1])
				index++
				continue
			}
		}
		builder.WriteByte(value[index])
	}
	return builder.String()
}

// DisplayName returns the most useful distribution label available.
func (i Info) DisplayName() string {
	if i.DistroName != "" {
		return i.DistroName
	}
	if i.Distro != "" {
		name := strings.ToUpper(i.Distro[:1]) + i.Distro[1:]
		if i.DistroVersion != "" {
			return name + " " + i.DistroVersion
		}
		return name
	}
	if i.OS != "" {
		return i.OS
	}
	return "unknown"
}

// PlatformLabel returns "linux/amd64" style output.
func (i Info) PlatformLabel() string {
	return i.OS + "/" + i.Arch
}
