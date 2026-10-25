package dotnet

import (
	"os"

	"github.com/mohammedbakri123/hilt/internal/process"
)

// ToolchainEnvironment returns the explicit environment overrides for a
// selected .NET root. Multilevel lookup is disabled so the selected root is the
// only source of SDKs and runtimes.
func ToolchainEnvironment(root string) map[string]string {
	return map[string]string{
		"DOTNET_ROOT":                       root,
		"DOTNET_MULTILEVEL_LOOKUP":          "0",
		"DOTNET_CLI_TELEMETRY_OPTOUT":       "1",
		"DOTNET_NOLOGO":                     "1",
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE": "1",
	}
}

// BuildToolchainEnvironment merges the explicit toolchain overrides into base.
// A nil base means no inherited environment; callers that want inheritance must
// pass os.Environ().
func BuildToolchainEnvironment(base []string, root string) []string {
	return process.EnvWith(base, ToolchainEnvironment(root))
}

// Environment returns the explicit environment for the selected toolchain. A
// nil base inherits the current process environment.
func (s Selection) Environment(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	return BuildToolchainEnvironment(base, s.Root)
}
