package projects

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TreeEntry is one filesystem item in a project tree.
type TreeEntry struct {
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Directory    bool   `json:"directory"`
	Depth        int    `json:"depth"`
}

// ScanTree returns a bounded, source-oriented tree for a project directory.
// Build outputs, VCS metadata, node_modules, hidden paths, and symlinks are
// skipped so inspecting a project remains safe and predictable.
func ScanTree(ctx context.Context, projectPath string, maxDepth int) ([]TreeEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	absolute, err := absolutePath(projectPath)
	if err != nil {
		return nil, fmt.Errorf("scan project tree: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("scan project tree: %w", err)
	}
	if !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	root := absolute
	entries := make([]TreeEntry, 0, 64)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() && excludedTreeDirectory(entry.Name()) {
			return fs.SkipDir
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		depth := len(strings.Split(relative, string(filepath.Separator)))
		if maxDepth > 0 && depth > maxDepth {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if hiddenPath(relative) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		entries = append(entries, TreeEntry{
			Name:         entry.Name(),
			RelativePath: filepath.ToSlash(relative),
			Directory:    entry.IsDir(),
			Depth:        depth,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan project tree: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].RelativePath < entries[right].RelativePath
	})
	return entries, nil
}

func excludedTreeDirectory(name string) bool {
	switch strings.ToLower(name) {
	case "bin", "obj", ".git", ".vs", "node_modules":
		return true
	default:
		return false
	}
}

func hiddenPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}
