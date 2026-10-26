package projects

import (
	"path/filepath"
	"strings"
)

func absolutePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

// resolvePath resolves a possibly Windows-style relative path against base.
// MSBuild property expressions are left untouched because Hilt does not
// evaluate them.
func resolvePath(base, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "$(") {
		return value
	}
	value = strings.ReplaceAll(value, `\`, "/")
	value = filepath.FromSlash(value)
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	if absolute, err := filepath.Abs(value); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(value)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
