package projects

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options controls project discovery.
type Options struct {
	// Root is the directory or supported file to scan. An empty root means the
	// current working directory.
	Root string
	// Exclusions are directory names skipped at any depth. Nil uses
	// DefaultExclusions.
	Exclusions []string
	// MaxDepth limits directory recursion. Zero or negative means unlimited.
	MaxDepth int
	// IncludeHidden includes dot-prefixed files and directories.
	IncludeHidden bool
}

// Scan discovers projects and workspaces under root. Unreadable directories and
// malformed project files are reported as warnings; cancellation is returned as
// an error.
func Scan(ctx context.Context, opts Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root := opts.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := absolutePath(root)
	if err != nil {
		return Result{}, fmt.Errorf("scan projects: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Result{}, fmt.Errorf("scan projects: %w", err)
	}
	result := Result{
		Root:       absolute,
		Workspaces: []Workspace{},
		Projects:   []Project{},
		Warnings:   []string{},
	}

	if !info.IsDir() {
		workspace, parsed, warnings, err := inspectFile(ctx, absolute)
		if err != nil {
			return Result{}, err
		}
		result.Workspaces = append(result.Workspaces, workspace)
		result.Projects = append(result.Projects, parsed...)
		result.Warnings = append(result.Warnings, warnings...)
		return result, nil
	}

	exclusions := opts.Exclusions
	if exclusions == nil {
		exclusions = DefaultExclusions()
	}
	excluded := make(map[string]bool, len(exclusions))
	for _, name := range exclusions {
		name = strings.TrimSpace(name)
		if name != "" {
			excluded[name] = true
		}
	}

	projectsByPath := make(map[string]Project)
	var workspaces []Workspace
	walkErr := filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", path, err))
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		relative, relErr := filepath.Rel(absolute, path)
		if relErr != nil {
			relative = path
		}
		depth := 0
		if relative != "." {
			depth = len(strings.Split(relative, string(filepath.Separator)))
		}

		if entry.IsDir() {
			if path != absolute {
				if !opts.IncludeHidden && strings.HasPrefix(entry.Name(), ".") {
					return fs.SkipDir
				}
				if excluded[entry.Name()] {
					return fs.SkipDir
				}
			}
			if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if opts.MaxDepth > 0 && depth > opts.MaxDepth {
			return nil
		}
		if !opts.IncludeHidden && strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		kind, ok := KindForPath(entry.Name())
		if !ok {
			return nil
		}

		switch kind {
		case KindProject:
			project, parseErr := ParseProject(path)
			if parseErr != nil {
				result.Warnings = append(result.Warnings, parseErr.Error())
				return nil
			}
			projectsByPath[project.Path] = project
		default:
			workspace, parseErr := ParseWorkspace(path)
			if parseErr != nil {
				result.Warnings = append(result.Warnings, parseErr.Error())
				return nil
			}
			workspaces = append(workspaces, workspace)
		}
		return nil
	})
	if walkErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
			return Result{}, walkErr
		}
		return Result{}, fmt.Errorf("scan projects: %w", walkErr)
	}

	referenced := make(map[string]bool)
	for _, workspace := range workspaces {
		for _, projectPath := range workspace.ProjectPaths {
			referenced[filepath.Clean(projectPath)] = true
		}
	}
	for path, project := range projectsByPath {
		result.Projects = append(result.Projects, project)
		if referenced[filepath.Clean(path)] {
			continue
		}
		workspaces = append(workspaces, Workspace{
			Path:         project.Path,
			Name:         project.Name,
			Directory:    project.Directory,
			Kind:         KindProject,
			ProjectPaths: []string{project.Path},
		})
	}

	sort.Slice(workspaces, func(left, right int) bool {
		return workspaces[left].Path < workspaces[right].Path
	})
	sort.Slice(result.Projects, func(left, right int) bool {
		return result.Projects[left].Path < result.Projects[right].Path
	})
	sort.Strings(result.Warnings)
	result.Workspaces = append(result.Workspaces, workspaces...)
	sort.Slice(result.Workspaces, func(left, right int) bool {
		return result.Workspaces[left].Path < result.Workspaces[right].Path
	})
	return result, nil
}

// Inspect parses a single project or workspace file.
func Inspect(ctx context.Context, path string) (Inspection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Inspection{}, err
	}
	absolute, err := absolutePath(path)
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect %s: %w", absolute, err)
	}
	if info.IsDir() {
		return Inspection{}, fmt.Errorf("inspect %s: is a directory; use --path to scan a directory", absolute)
	}
	workspace, parsed, warnings, err := inspectFile(ctx, absolute)
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{
		Kind:      workspace.Kind,
		Workspace: workspace,
		Projects:  parsed,
		Warnings:  warnings,
	}, nil
}

func inspectFile(ctx context.Context, path string) (Workspace, []Project, []string, error) {
	if err := ctx.Err(); err != nil {
		return Workspace{}, nil, nil, err
	}
	kind, ok := KindForPath(path)
	if !ok {
		return Workspace{}, nil, nil, fmt.Errorf("inspect %s: unsupported file type", path)
	}
	if kind == KindProject {
		project, err := ParseProject(path)
		if err != nil {
			return Workspace{}, nil, nil, err
		}
		workspace := Workspace{
			Path:         project.Path,
			Name:         project.Name,
			Directory:    project.Directory,
			Kind:         KindProject,
			ProjectPaths: []string{project.Path},
		}
		return workspace, []Project{project}, []string{}, nil
	}

	workspace, err := ParseWorkspace(path)
	if err != nil {
		return Workspace{}, nil, nil, err
	}
	projects := make([]Project, 0, len(workspace.ProjectPaths))
	warnings := []string{}
	for _, projectPath := range workspace.ProjectPaths {
		if err := ctx.Err(); err != nil {
			return Workspace{}, nil, nil, err
		}
		project, parseErr := ParseProject(projectPath)
		if parseErr != nil {
			warnings = append(warnings, parseErr.Error())
			continue
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(left, right int) bool {
		return projects[left].Path < projects[right].Path
	})
	sort.Strings(warnings)
	return workspace, projects, warnings, nil
}
