package tui

import (
	"fmt"
	"strings"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

func (m Model) projectLines(width int) []string {
	if m.projectDetailOpen {
		return m.projectDetailLines(width)
	}
	lines := sectionHeader("Projects & Solutions", width)
	if m.backend == nil {
		return append(lines, "No backend configured.")
	}
	if m.projectsLoading && m.projectResult == nil {
		return append(lines, m.spinner.View()+" scanning projects...")
	}
	if m.projectsErr != nil {
		for _, line := range styledWrapped("Project scan failed: "+m.projectsErr.Error(), width-2, styleError) {
			lines = append(lines, line)
		}
	}
	if m.projectResult == nil {
		return append(lines, "Projects not loaded. Press r to scan.")
	}

	result := m.projectResult
	projectList := m.filteredProjects()
	summary := fmt.Sprintf("  %d solutions  ·  %d projects", len(result.Workspaces), len(projectList))
	if pinned := len(m.backend.State().PinnedExternalProjects); pinned > 0 {
		summary += fmt.Sprintf("  ·  %d imported", pinned)
	}
	if m.projectFilter != "" {
		summary = fmt.Sprintf("  %d workspaces  ·  %d/%d projects", len(result.Workspaces), len(projectList), len(result.Projects))
	}
	if m.projectsLoading {
		summary += "  " + styleMuted.Render(m.spinner.View()+" rescanning...")
	}
	if m.projectFiltering || m.projectFilter != "" {
		filter := "  filter: /" + m.projectFilter
		if m.projectFiltering {
			filter += "▌"
		}
		summary += "  ·" + styleMuted.Render(filter)
	}
	if root := m.projectWorkspaceRoot(); root != "" {
		summary += "  ·  " + styleMuted.Render("workspace "+pickerPath(root, width-24))
	}
	if result.Root != "" {
		summary += "  ·  " + styleMuted.Render("root "+pickerPath(result.Root, width-12))
	}
	lines = append(lines, styleMuted.Render(summary))
	listContent, _ := m.projectListContent()
	lines = append(lines, "")
	lines = append(lines, listContent...)
	lines = projectWarningContent(lines, result.Warnings, width)
	return lines
}

func (m Model) projectRow(project projects.Project, index int, compact bool) string {
	marker := " "
	name := fmt.Sprintf("%-22s", project.Name)
	if index == m.projectCursor {
		marker = styleAccent.Render(">")
		name = styleValue.Render(name)
	}
	frameworks := valueOr(strings.Join(project.TargetFrameworks, ", "), "none")
	line := "  " + marker + " " + name
	if compact {
		line += styleMuted.Render(frameworks)
	} else {
		line += " " + styleMuted.Render(fmt.Sprintf("%-28s", frameworks))
	}
	if project.TestProject {
		line += " " + styleOK.Render("[test]")
	}
	if m.projectIsExternal(project.Path) {
		line += " " + styleWarning.Render("[external]")
	}
	if !compact {
		line += " " + styleFaint.Render(project.Path)
	}
	return line
}

func (m Model) projectListContent() ([]string, int) {
	result := m.projectResult
	if result == nil {
		return []string{"Projects not loaded."}, 0
	}
	workspaceLabel := "HILT WORKSPACE"
	if root := m.projectWorkspaceRoot(); root != "" {
		workspaceLabel += "  ·  " + pickerPath(root, 44)
	}
	lines := []string{styleLabel.Render(workspaceLabel)}
	lines = append(lines, styleLabel.Render("WORKSPACES  ·  press e to fold"))
	if sources := m.backend.State().PinnedExternalProjects; len(sources) > 0 {
		lines = append(lines, "", styleLabel.Render("IMPORTED SOURCES"))
		for _, source := range sources {
			lines = append(lines, "  "+styleAccent.Render("+")+" "+styleMuted.Render(source))
		}
	}
	if len(result.Workspaces) == 0 {
		lines = append(lines, "  "+styleMuted.Render("none found"))
	}
	projectList := m.filteredProjects()
	listed := make(map[string]bool)
	for _, workspace := range result.Workspaces {
		marker := "▾"
		if !m.workspaceExpanded(workspace.Path) {
			marker = "▸"
		}
		line := "  " + styleAccent.Render(marker) + " " + styleValue.Render(workspace.Name) + "  " +
			styleMuted.Render("["+string(workspace.Kind)+"] "+workspace.Path)
		line += styleFaint.Render(fmt.Sprintf("  (%d projects)", len(workspace.ProjectPaths)))
		lines = append(lines, line)
		for _, project := range projectList {
			if containsPath(workspace.ProjectPaths, project.Path) {
				listed[project.Path] = true
			}
		}
		if !m.workspaceExpanded(workspace.Path) {
			continue
		}
		for index, project := range projectList {
			if !containsPath(workspace.ProjectPaths, project.Path) {
				continue
			}
			lines = append(lines, "    "+m.projectRow(project, index, false))
		}
	}
	lines = append(lines, "", styleLabel.Render("INDEPENDENT PROJECTS"))
	selectedLine := len(lines)
	if len(projectList) == 0 {
		if m.projectFilter != "" {
			lines = append(lines,
				"  "+styleMuted.Render(fmt.Sprintf("No projects match %q.", m.projectFilter)),
				"  "+styleMuted.Render("Press esc to clear or / to edit."),
			)
		} else {
			lines = append(lines, "  "+styleMuted.Render("No .csproj files found."))
		}
		return lines, selectedLine
	}
	independent := 0
	for _, project := range projectList {
		if !listed[project.Path] {
			independent++
		}
	}
	if independent == 0 {
		lines = append(lines, "  "+styleMuted.Render("none"))
		return lines, selectedLine
	}
	lines = append(lines, styleFaint.Render("  NAME                  FRAMEWORKS"))
	for index, project := range projectList {
		if listed[project.Path] {
			continue
		}
		if index == m.projectCursor {
			selectedLine = len(lines)
		}
		lines = append(lines, "  "+m.projectRow(project, index, false))
	}
	return lines, selectedLine
}

// projectListProjectAt maps a rendered project-list line back to the stable
// filtered project index. Keeping this semantic mapping beside the renderer
// makes pointer selection follow the same hierarchy as keyboard selection.
func (m Model) projectListProjectAt(target int) (int, bool) {
	if m.projectResult == nil || target < 0 {
		return 0, false
	}
	line := 2 // HILT WORKSPACE and WORKSPACES labels.
	if sources := m.backend.State().PinnedExternalProjects; len(sources) > 0 {
		line += 2 + len(sources) // blank, label, and one row per source.
	}
	projectList := m.filteredProjects()
	listed := make(map[string]bool)
	for workspaceIndex, workspace := range m.projectResult.Workspaces {
		if line == target {
			return -workspaceIndex - 1, true
		}
		line++
		for projectIndex, project := range projectList {
			if !containsPath(workspace.ProjectPaths, project.Path) {
				continue
			}
			listed[project.Path] = true
			if !m.workspaceExpanded(workspace.Path) {
				continue
			}
			if line == target {
				return projectIndex, true
			}
			line++
		}
	}
	line += 2 // blank and INDEPENDENT PROJECTS.
	if len(projectList) == 0 {
		return 0, false
	}
	line++ // column headings.
	for projectIndex, project := range projectList {
		if listed[project.Path] {
			continue
		}
		if line == target {
			return projectIndex, true
		}
		line++
	}
	return 0, false
}

func (m Model) projectListWorkspaceAt(target int) (int, bool) {
	if m.projectResult == nil || target < 0 {
		return 0, false
	}
	line := 2
	if sources := m.backend.State().PinnedExternalProjects; len(sources) > 0 {
		line += 2 + len(sources)
	}
	for index := range m.projectResult.Workspaces {
		if line == target {
			return index, true
		}
		line++
		if !m.workspaceExpanded(m.projectResult.Workspaces[index].Path) {
			continue
		}
		for _, project := range m.filteredProjects() {
			if containsPath(m.projectResult.Workspaces[index].ProjectPaths, project.Path) {
				line++
			}
		}
	}
	return 0, false
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func projectPaneWidths(width int) (int, int) {
	if width < 20 {
		return width, width
	}
	left := (width - 1) / 2
	return left, width - left - 1
}

func joinPanes(left, right []string) []string {
	lines := make([]string, 0, len(left))
	for index := range left {
		rightLine := ""
		if index < len(right) {
			rightLine = right[index]
		}
		lines = append(lines, left[index]+" "+rightLine)
	}
	return lines
}

func panelWindow(title string, content []string, width, height, offset int, focused bool) []string {
	if height < 3 {
		height = 3
	}
	if width < 12 {
		width = 12
	}
	innerWidth := width - 4
	innerHeight := height - 2
	titleLine := styleLabel.Render(title)
	if focused {
		titleLine = styleAccent.Render(title) + styleMuted.Render(" [focus]")
	}
	body := []string{titleLine}
	body = append(body, windowLines(content, innerWidth, innerHeight-1, offset)...)
	box := stylePanel.Width(width).Render(strings.Join(body, "\n"))
	return strings.Split(box, "\n")
}

func (m Model) projectDetailContent(project projects.Project, width int) []string {
	content := []string{
		kvSized("Path", project.Path, 12),
		kvSized("Frameworks", valueOr(strings.Join(project.TargetFrameworks, ", "), "none"), 12),
		kvSized("SDK", valueOr(project.SDK, "unknown"), 12),
		kvSized("Output", valueOr(project.OutputType, "default"), 12),
	}
	if project.TestProject {
		content = append(content, kvSized("Test project", styleOK.Render("yes"), 12))
	}
	if m.projectIsExternal(project.Path) {
		content = append(content, kvSized("Location", styleWarning.Render("external"), 12))
		if source := m.externalSourceFor(project); source != "" {
			content = append(content, styleMuted.Render("Press u to remove the imported source."))
		}
	}
	if len(project.ProjectReferences) > 0 {
		content = append(content, kvSized("References", fmt.Sprintf("%d", len(project.ProjectReferences)), 12))
		for _, reference := range project.ProjectReferences {
			for _, line := range wrap("  "+reference, width) {
				content = append(content, styleMuted.Render(line))
			}
		}
	}
	return content
}

func (m Model) projectDetailPanel(width int) []string {
	return panelBox("SELECTED PROJECT", m.projectDetailBody(width-6), width)
}

func (m Model) projectDetailBody(width int) []string {
	lines := []string{}
	if project, ok := m.selectedProject(); ok {
		lines = append(lines, m.projectTabBar(), "")
		lines = append(lines, m.projectTabContent(project, width)...)
	} else {
		lines = append(lines, styleMuted.Render("Select a project to see its details."))
	}
	if m.projectResult != nil {
		lines = projectWarningContent(lines, m.projectResult.Warnings, width)
	}
	return lines
}

func (m Model) projectTabBar() string {
	parts := make([]string, 0, len(projectTabs))
	for index, tab := range projectTabs {
		if projectTab(index) == m.projectTab {
			parts = append(parts, styleValue.Render("["+tab.label+"]"))
			continue
		}
		parts = append(parts, styleMuted.Render(tab.label))
	}
	return "  " + strings.Join(parts, "  ") + "  " + styleFaint.Render("· esc returns to projects")
}

func (m Model) projectTabContent(project projects.Project, width int) []string {
	switch m.projectTab {
	case projectTabTree:
		if m.projectTreeLoading {
			return []string{m.spinner.View() + " inspecting project tree..."}
		}
		if m.projectTreeErr != nil {
			return []string{styleError.Render("Tree inspection failed: " + m.projectTreeErr.Error())}
		}
		if len(m.projectTree) == 0 {
			return []string{styleMuted.Render("No source files found."), "", styleMuted.Render("Generated and hidden folders are omitted.")}
		}
		lines := []string{styleMuted.Render("Source tree · generated and hidden folders omitted")}
		for _, entry := range m.projectTree {
			marker := "·"
			if entry.Directory {
				marker = "▾"
			}
			indent := entry.Depth - 1
			if indent < 0 {
				indent = 0
			}
			lines = append(lines, strings.Repeat("  ", indent)+styleAccent.Render(marker)+" "+entry.Name)
		}
		return lines
	case projectTabPackages:
		if len(project.PackageReferences) == 0 {
			return []string{styleMuted.Render("No declared package references."), "", styleFaint.Render("Package references are read from the project file.")}
		}
		lines := []string{styleMuted.Render(fmt.Sprintf("Declared NuGet packages · %d", len(project.PackageReferences)))}
		for _, packageReference := range project.PackageReferences {
			version := "version managed centrally"
			if packageReference.Version != "" {
				version = packageReference.Version
			}
			lines = append(lines, "  "+styleValue.Render(packageReference.Name)+"  "+styleFaint.Render(version))
		}
		return lines
	case projectTabData:
		lines := []string{styleMuted.Render("Entity Framework Core")}
		efPackages := make([]projects.PackageReference, 0)
		for _, packageReference := range project.PackageReferences {
			if projects.IsEntityFrameworkCorePackage(packageReference.Name) {
				efPackages = append(efPackages, packageReference)
			}
		}
		if len(efPackages) == 0 {
			return append(lines, "", styleMuted.Render("No EF Core package references detected."), styleFaint.Render("Hilt will not install dotnet-ef automatically."))
		}
		packageWord := "packages"
		if len(efPackages) == 1 {
			packageWord = "package"
		}
		lines = append(lines, styleOK.Render(fmt.Sprintf("EF Core detected · %d %s", len(efPackages), packageWord)), "")
		for _, packageReference := range efPackages {
			version := "version managed centrally"
			if packageReference.Version != "" {
				version = packageReference.Version
			}
			lines = append(lines, "  "+styleValue.Render(packageReference.Name)+"  "+styleFaint.Render(version))
		}
		if m.projectTreeLoading {
			return append(lines, "", m.spinner.View()+" checking migration files...")
		}
		if m.projectTreeErr != nil {
			return append(lines, "", styleError.Render("Migration inspection failed: "+m.projectTreeErr.Error()))
		}
		migrations := make([]string, 0)
		for _, entry := range m.projectTree {
			if !entry.Directory && isMigrationPath(entry.RelativePath) {
				migrations = append(migrations, entry.RelativePath)
			}
		}
		if len(migrations) == 0 {
			return append(lines, "", styleMuted.Render("No migration files detected."), styleFaint.Render("Use the action menu for EF commands when dotnet-ef is available."))
		}
		lines = append(lines, "", styleLabel.Render(fmt.Sprintf("MIGRATIONS · %d", len(migrations))))
		for _, migration := range migrations {
			lines = append(lines, "  "+styleValue.Render(migration))
		}
		return lines
	case projectTabOutput:
		if m.hasBuildTarget && m.buildTarget.Path == project.Path {
			lines := []string{styleMuted.Render("Build output streams in the bottom drawer.")}
			if m.buildResult != nil {
				lines = append(lines, "", kvSized("Last build", buildDrawerStatus(*m.buildResult, m.buildErr), 14))
			}
			lines = append(lines, "", styleMuted.Render("Press o to open or close the output drawer."))
			return lines
		}
		return []string{
			styleMuted.Render("No build or run output yet."),
			"",
			styleMuted.Render("Press o to open the output drawer."),
		}
	}
	return m.projectDetailContent(project, width)
}

func isMigrationPath(path string) bool {
	path = strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "migrations" {
			return true
		}
	}
	base := parts[len(parts)-1]
	return strings.HasSuffix(base, "modelsnapshot.cs")
}

func projectWarningContent(lines []string, warnings []string, width int) []string {
	if len(warnings) == 0 {
		return lines
	}
	lines = append(lines, "", styleLabel.Render("WARNINGS"))
	for _, warning := range warnings {
		for _, line := range wrap(warning, width) {
			lines = append(lines, styleWarning.Render("  ! "+line))
		}
	}
	return lines
}

func (m Model) projectPaneHeight() int {
	height := m.bodyHeight() - 4
	if height < 5 {
		height = 5
	}
	return height
}

func (m Model) projectListVisibleHeight() int {
	height := m.projectPaneHeight() - 3
	if height < 1 {
		height = 1
	}
	return height
}

func (m Model) projectListContentHeight() int {
	lines, _ := m.projectListContent()
	visible := m.projectListVisibleHeight()
	if len(lines) <= visible {
		return 0
	}
	return len(lines) - visible
}

func (m Model) projectDetailContentHeight() int {
	width := m.contentWidth()
	if m.projectSplit() {
		_, width = projectPaneWidths(width)
	}
	lines := m.projectDetailBody(width - 4)
	visible := m.projectPaneHeight() - 3
	if visible < 1 {
		visible = 1
	}
	if len(lines) <= visible {
		return 0
	}
	return len(lines) - visible
}
