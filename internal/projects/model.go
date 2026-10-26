// Package projects discovers .NET projects, solutions, and solution filters.
// It parses declared metadata only; it does not evaluate MSBuild properties.
package projects

import (
	"path/filepath"
	"strings"
)

// Kind identifies a discovered project or workspace file.
type Kind string

const (
	// KindSolution is a classic Visual Studio .sln file.
	KindSolution Kind = "sln"
	// KindSolutionX is an XML .slnx solution file.
	KindSolutionX Kind = "slnx"
	// KindSolutionFilter is a filtered solution .slnf file.
	KindSolutionFilter Kind = "slnf"
	// KindProject is an MSBuild .csproj project file.
	KindProject Kind = "csproj"
)

// Project is a .csproj file and its declared metadata.
type Project struct {
	Path              string   `json:"path"`
	Name              string   `json:"name"`
	Directory         string   `json:"directory"`
	Kind              Kind     `json:"kind"`
	TargetFrameworks  []string `json:"targetFrameworks"`
	SDK               string   `json:"sdk,omitempty"`
	OutputType        string   `json:"outputType,omitempty"`
	TestProject       bool     `json:"testProject"`
	ProjectReferences []string `json:"projectReferences"`
}

// Workspace is a solution, solution filter, or standalone project. ProjectPaths
// are absolute paths exactly as declared by the workspace file.
type Workspace struct {
	Path         string   `json:"path"`
	Name         string   `json:"name"`
	Directory    string   `json:"directory"`
	Kind         Kind     `json:"kind"`
	ProjectPaths []string `json:"projectPaths"`
	SolutionPath string   `json:"solutionPath,omitempty"`
}

// Result is the outcome of a recursive discovery scan.
type Result struct {
	Root       string      `json:"root"`
	Workspaces []Workspace `json:"workspaces"`
	Projects   []Project   `json:"projects"`
	Warnings   []string    `json:"warnings"`
}

// Inspection describes one project or workspace file in detail.
type Inspection struct {
	Kind      Kind      `json:"kind"`
	Workspace Workspace `json:"workspace"`
	Projects  []Project `json:"projects"`
	Warnings  []string  `json:"warnings"`
}

// DefaultExclusions returns directory names skipped by default.
func DefaultExclusions() []string {
	return []string{"bin", "obj", ".git", "node_modules"}
}

// KindForPath returns the file kind for a supported extension.
func KindForPath(path string) (Kind, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".sln":
		return KindSolution, true
	case ".slnx":
		return KindSolutionX, true
	case ".slnf":
		return KindSolutionFilter, true
	case ".csproj":
		return KindProject, true
	default:
		return "", false
	}
}
