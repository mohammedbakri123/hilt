package dotnet

import (
	"strings"
	"testing"
)

func TestToolchainEnvironmentIsExplicit(t *testing.T) {
	values := ToolchainEnvironment("/opt/managed/dotnet")
	want := map[string]string{
		"DOTNET_ROOT":                       "/opt/managed/dotnet",
		"DOTNET_MULTILEVEL_LOOKUP":          "0",
		"DOTNET_CLI_TELEMETRY_OPTOUT":       "1",
		"DOTNET_NOLOGO":                     "1",
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE": "1",
	}
	for key, value := range want {
		if values[key] != value {
			t.Errorf("%s = %q, want %q", key, values[key], value)
		}
	}
}

func TestBuildToolchainEnvironmentMergesWithoutMutatingBase(t *testing.T) {
	base := []string{"PATH=/usr/bin", "DOTNET_ROOT=/old", "HOME=/home/u"}
	env := BuildToolchainEnvironment(base, "/new/root")
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"DOTNET_ROOT=/new/root",
		"DOTNET_MULTILEVEL_LOOKUP=0",
		"DOTNET_CLI_TELEMETRY_OPTOUT=1",
		"DOTNET_NOLOGO=1",
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("environment missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "DOTNET_ROOT=/old") {
		t.Errorf("old DOTNET_ROOT was not replaced:\n%s", joined)
	}
	if base[1] != "DOTNET_ROOT=/old" {
		t.Errorf("base environment was mutated: %v", base)
	}
}

func TestSelectionEnvironmentUsesSelectedRoot(t *testing.T) {
	selection := Selection{Root: "/selected/root"}
	env := selection.Environment([]string{})
	if len(env) != 5 {
		t.Fatalf("environment = %v", env)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "DOTNET_ROOT=") && entry != "DOTNET_ROOT=/selected/root" {
			t.Errorf("DOTNET_ROOT entry = %q", entry)
		}
	}
}
