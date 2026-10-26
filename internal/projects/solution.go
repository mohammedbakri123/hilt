package projects

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const solutionFolderType = "2150e333-8fdc-42a3-9474-1a3956d46de8"

var slnProjectLine = regexp.MustCompile(`(?i)^\s*Project\("(\{[^}]*\})"\)\s*=\s*"([^"]*)"\s*,\s*"([^"]*)"\s*,\s*"(\{[^}]*\})"`)

// ParseWorkspace reads a .sln, .slnx, or .slnf file and returns the projects it
// declares. Projects are not parsed or validated here.
func ParseWorkspace(path string) (Workspace, error) {
	kind, ok := KindForPath(path)
	if !ok {
		return Workspace{}, fmt.Errorf("parse workspace %s: unsupported file type", path)
	}
	if kind == KindProject {
		return Workspace{}, fmt.Errorf("parse workspace %s: %s is a project, not a solution", path, path)
	}
	absolute, err := absolutePath(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("parse workspace %s: %w", path, err)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return Workspace{}, fmt.Errorf("parse workspace %s: %w", absolute, err)
	}

	name := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	workspace := Workspace{
		Path:      absolute,
		Name:      name,
		Directory: filepath.Dir(absolute),
		Kind:      kind,
	}

	switch kind {
	case KindSolution:
		paths, err := parseSolution(absolute, data)
		if err != nil {
			return Workspace{}, fmt.Errorf("parse solution %s: %w", absolute, err)
		}
		workspace.ProjectPaths = paths
	case KindSolutionX:
		paths, err := parseSolutionX(absolute, data)
		if err != nil {
			return Workspace{}, fmt.Errorf("parse solution %s: %w", absolute, err)
		}
		workspace.ProjectPaths = paths
	case KindSolutionFilter:
		paths, solutionPath, err := parseSolutionFilter(absolute, data)
		if err != nil {
			return Workspace{}, fmt.Errorf("parse solution filter %s: %w", absolute, err)
		}
		workspace.ProjectPaths = paths
		workspace.SolutionPath = solutionPath
	}

	return workspace, nil
}

func parseSolution(path string, data []byte) ([]string, error) {
	base := filepath.Dir(path)
	directory := base
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	header := false
	var projects []string
	for number, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if number == 0 && strings.Contains(trimmed, "Microsoft Visual Studio Solution File") {
			header = true
		}
		if !strings.HasPrefix(trimmed, "Project(") {
			continue
		}
		match := slnProjectLine.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("line %d: malformed Project declaration", number+1)
		}
		if strings.EqualFold(normalizeGUID(match[1]), solutionFolderType) {
			continue
		}
		if resolved := resolvePath(directory, match[3]); resolved != "" {
			projects = append(projects, resolved)
		}
	}
	if !header && len(projects) == 0 {
		return nil, errors.New("not a Visual Studio solution file")
	}
	return uniqueStrings(projects), nil
}

func normalizeGUID(value string) string {
	return strings.Trim(strings.TrimSpace(value), "{}")
}

func parseSolutionX(path string, data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	rootSeen := false
	var projects []string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		element, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if !rootSeen {
			rootSeen = true
			if !strings.EqualFold(element.Name.Local, "Solution") {
				return nil, fmt.Errorf("root element is %q, want Solution", element.Name.Local)
			}
			continue
		}
		if !strings.EqualFold(element.Name.Local, "Project") {
			continue
		}
		for _, attribute := range element.Attr {
			if !strings.EqualFold(attribute.Name.Local, "Path") {
				continue
			}
			if resolved := resolvePath(filepath.Dir(path), attribute.Value); resolved != "" {
				projects = append(projects, resolved)
			}
		}
	}
	if !rootSeen {
		return nil, errors.New("empty XML document")
	}
	return uniqueStrings(projects), nil
}

type solutionFilterDocument struct {
	Solution *solutionFilterSolution `json:"solution"`
}

type solutionFilterSolution struct {
	Path     string   `json:"path"`
	Projects []string `json:"projects"`
}

func parseSolutionFilter(path string, data []byte) ([]string, string, error) {
	var document solutionFilterDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, "", err
	}
	if document.Solution == nil {
		return nil, "", errors.New("missing solution object")
	}
	base := filepath.Dir(path)
	solutionPath := ""
	if value := strings.TrimSpace(document.Solution.Path); value != "" {
		solutionPath = resolvePath(base, value)
		base = filepath.Dir(solutionPath)
	}
	if solutionPath == "" && len(document.Solution.Projects) == 0 {
		return nil, "", errors.New("solution filter declares neither a solution nor projects")
	}
	projects := make([]string, 0, len(document.Solution.Projects))
	for _, value := range document.Solution.Projects {
		if resolved := resolvePath(base, value); resolved != "" {
			projects = append(projects, resolved)
		}
	}
	return uniqueStrings(projects), solutionPath, nil
}
