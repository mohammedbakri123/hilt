package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func testdataPath(t *testing.T, parts ...string) string {
	t.Helper()
	base := append([]string{"testdata"}, parts...)
	absolute, err := filepath.Abs(filepath.Join(base...))
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func TestParseProjectDeclaredMetadata(t *testing.T) {
	path := testdataPath(t, "scan", "src", "Shop", "Shop.csproj")
	project, err := ParseProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if project.Path != path {
		t.Errorf("Path = %q, want %q", project.Path, path)
	}
	if project.Name != "Shop" {
		t.Errorf("Name = %q", project.Name)
	}
	if project.Directory != filepath.Dir(path) {
		t.Errorf("Directory = %q", project.Directory)
	}
	if project.Kind != KindProject {
		t.Errorf("Kind = %q", project.Kind)
	}
	if project.SDK != "Microsoft.NET.Sdk" {
		t.Errorf("SDK = %q", project.SDK)
	}
	if project.OutputType != "Exe" {
		t.Errorf("OutputType = %q", project.OutputType)
	}
	if project.TestProject {
		t.Error("TestProject = true, want false")
	}
	wantFrameworks := []string{"net10.0", "net9.0", "net8.0"}
	if strings.Join(project.TargetFrameworks, ",") != strings.Join(wantFrameworks, ",") {
		t.Errorf("TargetFrameworks = %v, want %v", project.TargetFrameworks, wantFrameworks)
	}
	library := filepath.Join(filepath.Dir(path), "..", "Library", "Library.csproj")
	library, err = filepath.Abs(library)
	if err != nil {
		t.Fatal(err)
	}
	wantReferences := []string{library, "$(GeneratedProject)"}
	sort.Strings(wantReferences)
	gotReferences := append([]string(nil), project.ProjectReferences...)
	sort.Strings(gotReferences)
	if strings.Join(gotReferences, ",") != strings.Join(wantReferences, ",") {
		t.Errorf("ProjectReferences = %v, want %v", gotReferences, wantReferences)
	}
}

func TestParseProjectNamespaceAndSDKElement(t *testing.T) {
	library, err := ParseProject(testdataPath(t, "scan", "src", "Library", "Library.csproj"))
	if err != nil {
		t.Fatal(err)
	}
	if library.SDK != "Microsoft.NET.Sdk" {
		t.Errorf("SDK = %q", library.SDK)
	}
	if strings.Join(library.TargetFrameworks, ",") != "netstandard2.0" {
		t.Errorf("TargetFrameworks = %v", library.TargetFrameworks)
	}

	utility, err := ParseProject(testdataPath(t, "scan", "standalone", "Utility", "Utility.csproj"))
	if err != nil {
		t.Fatal(err)
	}
	if utility.SDK != "Microsoft.NET.Sdk" {
		t.Errorf("SDK element hint = %q", utility.SDK)
	}
	if utility.OutputType != "Library" {
		t.Errorf("OutputType = %q", utility.OutputType)
	}
}

func TestParseProjectDetectsTestProjects(t *testing.T) {
	project, err := ParseProject(testdataPath(t, "scan", "tests", "Shop.Tests", "Shop.Tests.csproj"))
	if err != nil {
		t.Fatal(err)
	}
	if !project.TestProject {
		t.Error("TestProject = false, want true")
	}
	if len(project.ProjectReferences) != 1 {
		t.Errorf("ProjectReferences = %v", project.ProjectReferences)
	}
}

func TestParseProjectErrors(t *testing.T) {
	for _, path := range []string{
		testdataPath(t, "malformed", "broken.csproj"),
		testdataPath(t, "missing.csproj"),
	} {
		if _, err := ParseProject(path); err == nil {
			t.Errorf("ParseProject(%q) returned no error", path)
		} else if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not name %q", err, path)
		}
	}
}

func TestParseWorkspaceFormats(t *testing.T) {
	cases := []struct {
		name        string
		file        string
		kind        Kind
		solutionSet bool
	}{
		{name: "sln", file: "Shop.sln", kind: KindSolution},
		{name: "slnx", file: "Shop.slnx", kind: KindSolutionX},
		{name: "slnf", file: "Shop.slnf", kind: KindSolutionFilter, solutionSet: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := testdataPath(t, "scan", test.file)
			workspace, err := ParseWorkspace(path)
			if err != nil {
				t.Fatal(err)
			}
			if workspace.Kind != test.kind {
				t.Errorf("Kind = %q, want %q", workspace.Kind, test.kind)
			}
			if workspace.Name != "Shop" {
				t.Errorf("Name = %q", workspace.Name)
			}
			if len(workspace.ProjectPaths) != 2 {
				t.Fatalf("ProjectPaths = %v", workspace.ProjectPaths)
			}
			for _, projectPath := range workspace.ProjectPaths {
				if !filepath.IsAbs(projectPath) {
					t.Errorf("project path is not absolute: %q", projectPath)
				}
				if strings.Contains(projectPath, `\`) {
					t.Errorf("project path still contains a backslash: %q", projectPath)
				}
			}
			if test.solutionSet && workspace.SolutionPath == "" {
				t.Error("SolutionPath is empty")
			}
			if !test.solutionSet && workspace.SolutionPath != "" {
				t.Errorf("SolutionPath = %q, want empty", workspace.SolutionPath)
			}
		})
	}
}

func TestParseWorkspaceRejectsProjectsAndMalformedFiles(t *testing.T) {
	if _, err := ParseWorkspace(testdataPath(t, "scan", "src", "Shop", "Shop.csproj")); err == nil {
		t.Error("ParseWorkspace accepted a csproj")
	}
	for _, file := range []string{"broken.sln", "broken.slnx", "broken.slnf", "not-a-solution.sln"} {
		path := testdataPath(t, "malformed", file)
		if _, err := ParseWorkspace(path); err == nil {
			t.Errorf("ParseWorkspace(%q) returned no error", path)
		} else if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not name %q", err, path)
		}
	}
	if _, err := ParseWorkspace(testdataPath(t, "malformed", "missing.sln")); err == nil {
		t.Error("ParseWorkspace accepted a missing file")
	}
}

func TestScanFindsProjectsAndWorkspaces(t *testing.T) {
	result, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 5 {
		t.Fatalf("Projects = %d: %v", len(result.Projects), projectNames(result.Projects))
	}
	if len(result.Workspaces) != 6 {
		t.Fatalf("Workspaces = %d: %v", len(result.Workspaces), workspaceNames(result.Workspaces))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v", result.Warnings)
	}
	for _, project := range result.Projects {
		for _, excluded := range []string{string(filepath.Separator) + "bin" + string(filepath.Separator), string(filepath.Separator) + "obj" + string(filepath.Separator), "node_modules", string(filepath.Separator) + ".hidden" + string(filepath.Separator)} {
			if strings.Contains(project.Path, excluded) {
				t.Errorf("excluded project discovered: %s", project.Path)
			}
		}
	}
	kinds := make(map[Kind]bool)
	for _, workspace := range result.Workspaces {
		kinds[workspace.Kind] = true
	}
	for _, kind := range []Kind{KindSolution, KindSolutionX, KindSolutionFilter, KindProject} {
		if !kinds[kind] {
			t.Errorf("missing workspace kind %q", kind)
		}
	}
}

func TestScanIncludesHiddenWhenRequested(t *testing.T) {
	result, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan"), IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, project := range result.Projects {
		if project.Name == "Hidden" {
			found = true
		}
	}
	if !found {
		t.Errorf("hidden project not discovered: %v", projectNames(result.Projects))
	}
}

func TestScanHonorsMaxDepthAndCustomExclusions(t *testing.T) {
	shallow, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan"), MaxDepth: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range shallow.Projects {
		if project.Name == "Deep" {
			t.Errorf("Deep project found at max depth 3: %s", project.Path)
		}
	}
	if len(shallow.Projects) != 4 {
		t.Errorf("Projects = %v", projectNames(shallow.Projects))
	}

	excluded, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan"), Exclusions: []string{"src"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range excluded.Projects {
		if strings.Contains(project.Path, string(filepath.Separator)+"src"+string(filepath.Separator)) {
			t.Errorf("src project discovered despite exclusion: %s", project.Path)
		}
	}
}

func TestScanStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, Options{Root: testdataPath(t, "scan")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan error = %v, want context.Canceled", err)
	}
}

func TestScanGuardsSymlinkLoops(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "Loop.csproj")
	if err := os.WriteFile(projectPath, []byte(`<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup></Project>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, err := Scan(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 1 {
		t.Errorf("Projects = %v", projectNames(result.Projects))
	}
}

func TestScanSingleFile(t *testing.T) {
	projectResult, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan", "src", "Shop", "Shop.csproj")})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectResult.Projects) != 1 || projectResult.Projects[0].Name != "Shop" {
		t.Errorf("Projects = %v", projectNames(projectResult.Projects))
	}
	if len(projectResult.Workspaces) != 1 || projectResult.Workspaces[0].Kind != KindProject {
		t.Errorf("Workspaces = %v", workspaceNames(projectResult.Workspaces))
	}

	solutionResult, err := Scan(context.Background(), Options{Root: testdataPath(t, "scan", "Shop.sln")})
	if err != nil {
		t.Fatal(err)
	}
	if len(solutionResult.Projects) != 2 {
		t.Errorf("Projects = %v", projectNames(solutionResult.Projects))
	}
}

func TestScanReportsMalformedFilesAsWarnings(t *testing.T) {
	result, err := Scan(context.Background(), Options{Root: testdataPath(t, "malformed")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != 0 {
		t.Errorf("Projects = %v", projectNames(result.Projects))
	}
	if len(result.Workspaces) != 0 {
		t.Errorf("Workspaces = %v", workspaceNames(result.Workspaces))
	}
	for _, file := range []string{"broken.csproj", "broken.sln", "broken.slnx", "broken.slnf", "not-a-solution.sln"} {
		found := false
		for _, warning := range result.Warnings {
			if strings.Contains(warning, file) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no warning names %s: %v", file, result.Warnings)
		}
	}
}

func TestInspectProjectAndSolution(t *testing.T) {
	project, err := Inspect(context.Background(), testdataPath(t, "scan", "src", "Shop", "Shop.csproj"))
	if err != nil {
		t.Fatal(err)
	}
	if project.Kind != KindProject || len(project.Projects) != 1 {
		t.Errorf("Inspection = %+v", project)
	}

	solution, err := Inspect(context.Background(), testdataPath(t, "scan", "Shop.sln"))
	if err != nil {
		t.Fatal(err)
	}
	if solution.Kind != KindSolution || len(solution.Projects) != 2 {
		t.Errorf("Inspection = %+v", solution)
	}
	if len(solution.Warnings) != 0 {
		t.Errorf("Warnings = %v", solution.Warnings)
	}
}

func TestInspectWarnsAboutMissingMembers(t *testing.T) {
	root := t.TempDir()
	solutionPath := filepath.Join(root, "Missing.sln")
	content := "Microsoft Visual Studio Solution File, Format Version 12.00\n" +
		"Project(\"{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}\") = \"Missing\", \"Missing.csproj\", \"{11111111-1111-1111-1111-111111111111}\"\nEndProject\n"
	if err := os.WriteFile(solutionPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(context.Background(), solutionPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projects) != 0 {
		t.Errorf("Projects = %v", projectNames(inspection.Projects))
	}
	if len(inspection.Warnings) != 1 || !strings.Contains(inspection.Warnings[0], "Missing.csproj") {
		t.Errorf("Warnings = %v", inspection.Warnings)
	}
}

func TestInspectErrors(t *testing.T) {
	if _, err := Inspect(context.Background(), testdataPath(t, "malformed", "broken.csproj")); err == nil {
		t.Error("Inspect accepted a malformed project")
	}
	if _, err := Inspect(context.Background(), testdataPath(t, "scan")); err == nil {
		t.Error("Inspect accepted a directory")
	}
	if _, err := Inspect(context.Background(), testdataPath(t, "missing.csproj")); err == nil {
		t.Error("Inspect accepted a missing file")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, testdataPath(t, "scan", "Shop.sln")); !errors.Is(err, context.Canceled) {
		t.Errorf("Inspect error = %v, want context.Canceled", err)
	}
}

func projectNames(projects []Project) []string {
	names := make([]string, 0, len(projects))
	for _, project := range projects {
		names = append(names, project.Name)
	}
	sort.Strings(names)
	return names
}

func workspaceNames(workspaces []Workspace) []string {
	names := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		names = append(names, string(workspace.Kind)+":"+workspace.Name)
	}
	sort.Strings(names)
	return names
}
