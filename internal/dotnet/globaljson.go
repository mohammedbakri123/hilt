package dotnet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalJSON is the subset of global.json Hilt understands.
type GlobalJSON struct {
	Path            string `json:"path"`
	Version         string `json:"version"`
	RollForward     string `json:"rollForward"`
	AllowPrerelease *bool  `json:"allowPrerelease,omitempty"`
}

type globalJSONDocument struct {
	SDK struct {
		Version         string `json:"version"`
		RollForward     string `json:"rollForward"`
		AllowPrerelease *bool  `json:"allowPrerelease"`
	} `json:"sdk"`
}

// ReadGlobalJSON parses a global.json file.
func ReadGlobalJSON(path string) (*GlobalJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read global.json: %w", err)
	}
	var document globalJSONDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &GlobalJSON{
		Path:            path,
		Version:         document.SDK.Version,
		RollForward:     document.SDK.RollForward,
		AllowPrerelease: document.SDK.AllowPrerelease,
	}, nil
}

// FindGlobalJSON walks upward from start looking for the nearest global.json.
// It returns (nil, nil) when none is found.
func FindGlobalJSON(start string, stat func(string) (os.FileInfo, error)) (*GlobalJSON, error) {
	if stat == nil {
		stat = os.Stat
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("resolve start directory: %w", err)
	}

	for depth := 0; depth < 64; depth++ {
		candidate := filepath.Join(dir, "global.json")
		if info, err := stat(candidate); err == nil && !info.IsDir() {
			return ReadGlobalJSON(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, nil
}
