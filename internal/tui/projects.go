package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

const (
	projectsScanTimeout  = 30 * time.Second
	projectTreeTimeout   = 15 * time.Second
	projectSplitMinWidth = 120
)

type projectFocus int

const (
	projectFocusList projectFocus = iota
	projectFocusDetail
)

type projectTab int

const (
	projectTabOverview projectTab = iota
	projectTabPackages
	projectTabTree
	projectTabData
	projectTabOutput
)

var projectTabs = []struct {
	title string
	label string
}{
	{title: "OVERVIEW", label: "Overview"},
	{title: "PACKAGES", label: "Packages"},
	{title: "FILES / TREE", label: "Files"},
	{title: "EF CORE", label: "EF"},
	{title: "OUTPUT", label: "Output"},
}

var projectTabCycle = []projectTab{
	projectTabOverview,
	projectTabPackages,
	projectTabData,
	projectTabOutput,
}

func (m *Model) moveProjectTab(delta int) {
	index := 0
	for candidate, tab := range projectTabCycle {
		if tab == m.projectTab {
			index = candidate
			break
		}
	}
	if m.projectTab == projectTabTree {
		if delta > 0 {
			m.projectTab = projectTabData
		} else {
			m.projectTab = projectTabPackages
		}
	} else {
		m.projectTab = projectTabCycle[(index+delta+len(projectTabCycle))%len(projectTabCycle)]
	}
	m.projectDetailScroll = 0
}

func (m *Model) startProjectTreeScan() tea.Cmd {
	if m.projectTab != projectTabTree && m.projectTab != projectTabData {
		return nil
	}
	project, ok := m.selectedProject()
	if !ok || m.backend == nil {
		return nil
	}
	if m.projectTreePath == project.Path && m.projectTree != nil && m.projectTreeErr == nil {
		return nil
	}
	if m.projectTreeCancel != nil {
		m.projectTreeCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), projectTreeTimeout)
	m.projectTreeCancel = cancel
	m.projectTreeRequest++
	request := m.projectTreeRequest
	path := project.Path
	m.projectTreePath = path
	m.projectTree = nil
	m.projectTreeErr = nil
	m.projectTreeLoading = true
	backend := m.backend
	return func() tea.Msg {
		entries, err := backend.ProjectTree(ctx, path)
		return projectTreeMsg{request: request, path: path, entries: entries, err: err}
	}
}

func (m Model) handleProjectTreeMsg(message projectTreeMsg) (tea.Model, tea.Cmd) {
	if message.request != m.projectTreeRequest || message.path != m.projectTreePath {
		return m, nil
	}
	if m.projectTreeCancel != nil {
		m.projectTreeCancel()
		m.projectTreeCancel = nil
	}
	m.projectTreeLoading = false
	m.projectTreeErr = message.err
	if message.err == nil {
		m.projectTree = message.entries
	}
	return m, nil
}

func (m *Model) startProjectsScan() tea.Cmd {
	if m.backend == nil {
		return nil
	}
	m.stopProjectScan()
	ctx, cancel := context.WithTimeout(context.Background(), projectsScanTimeout)
	m.projectCancel = cancel
	m.projectRequest++
	request := m.projectRequest
	m.projectsLoading = true
	m.projectsErr = nil
	backend := m.backend
	// The normal root is the configured Hilt workspace (apps/ by default). Any
	// other path is explicit: either the user chose it with the root picker or
	// pinned it with the import action. No filesystem-wide discovery is used.
	roots := append([]string{m.activeProjectScanRoot()}, backend.State().PinnedExternalProjects...)
	return func() tea.Msg {
		var combined projects.Result
		for index, root := range roots {
			result, err := backend.ScanProjects(ctx, root, false)
			if err != nil {
				if index == 0 {
					return projectsMsg{request: request, err: err}
				}
				combined.Warnings = append(combined.Warnings, "pinned source "+root+": "+err.Error())
				continue
			}
			if index == 0 {
				combined = result
				continue
			}
			combined = mergeProjectResults(combined, result)
		}
		return projectsMsg{request: request, result: combined}
	}
}

func mergeProjectResults(left, right projects.Result) projects.Result {
	seenProjects := make(map[string]bool, len(left.Projects))
	for _, project := range left.Projects {
		seenProjects[project.Path] = true
	}
	for _, project := range right.Projects {
		if !seenProjects[project.Path] {
			left.Projects = append(left.Projects, project)
			seenProjects[project.Path] = true
		}
	}
	seenWorkspaces := make(map[string]bool, len(left.Workspaces))
	for _, workspace := range left.Workspaces {
		seenWorkspaces[workspace.Path] = true
	}
	for _, workspace := range right.Workspaces {
		if !seenWorkspaces[workspace.Path] {
			left.Workspaces = append(left.Workspaces, workspace)
			seenWorkspaces[workspace.Path] = true
		}
	}
	left.Warnings = append(left.Warnings, right.Warnings...)
	return left
}

func (m *Model) stopProjectScan() {
	if m.projectCancel != nil {
		m.projectCancel()
		m.projectCancel = nil
	}
}

func (m *Model) cancelProjects() {
	if !m.projectsLoading {
		return
	}
	m.stopProjectScan()
	m.projectsLoading = false
	m.setStatus("Project scan cancelled.", false)
}

func (m Model) handleProjectsMsg(message projectsMsg) (tea.Model, tea.Cmd) {
	if message.request != m.projectRequest {
		return m, nil
	}
	m.stopProjectScan()
	m.projectsLoading = false
	if message.err != nil {
		if errors.Is(message.err, context.Canceled) {
			m.setStatus("Project scan cancelled.", false)
			return m, nil
		}
		m.projectsErr = message.err
		m.setStatus("Project scan failed.", true)
		return m, nil
	}
	previous := ""
	if m.projectResult != nil {
		if selected, ok := m.selectedProject(); ok {
			previous = selected.Path
		}
	}
	if previous == "" && m.backend != nil {
		previous = m.backend.State().ActiveProject
	}
	result := message.result
	m.projectResult = &result
	m.projectsErr = nil
	m.projectsLoaded = true
	m.restoreProjectSelection(previous)
	if m.projectFiltering {
		m.projectFinderCursor = clampIndex(m.projectFinderCursor, len(m.projectFinderEntries()))
	}
	m.ensureProjectSelectionVisible()
	m.clampScroll()
	return m, nil
}

func (m *Model) moveProject(delta int) {
	projectsList := m.filteredProjects()
	if len(projectsList) == 0 {
		return
	}
	m.projectCursor = clampIndex(m.projectCursor+delta, len(projectsList))
	m.projectDetailScroll = 0
	m.ensureProjectSelectionVisible()
}

func (m *Model) beginDeleteProject() {
	project, ok := m.selectedProject()
	if !ok {
		m.setStatus("Select a project before deleting it.", true)
		return
	}
	m.clearConfirm()
	m.pendingDeleteProject = &project
	m.confirm = confirmDeleteProject
	m.confirmSummary = []string{
		"Delete project file " + project.Path,
		"The source directory and its files will be preserved.",
		"If a solution references this project, it will need to be cleaned up separately.",
	}
}

func (m Model) externalSourceFor(project projects.Project) string {
	if m.backend == nil {
		return ""
	}
	workspace := m.workspaceFor(project.Path)
	best := ""
	bestLength := -1
	for _, source := range m.backend.State().PinnedExternalProjects {
		if source == project.Path || (workspace != "" && source == workspace) {
			return source
		}
		relative, err := filepath.Rel(source, project.Path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		if len(source) > bestLength {
			best = source
			bestLength = len(source)
		}
	}
	return best
}

func (m *Model) unpinSelectedExternalProject() tea.Cmd {
	project, ok := m.selectedProject()
	if !ok {
		m.setStatus("Select an external project before removing its source.", true)
		return nil
	}
	source := m.externalSourceFor(project)
	if source == "" {
		m.setStatus("The selected project is not an imported source.", true)
		return nil
	}
	backend := m.backend
	return func() tea.Msg {
		if backend == nil {
			return projectUnpinMsg{path: source, err: errors.New("no backend configured")}
		}
		return projectUnpinMsg{path: source, err: backend.UnpinExternalProject(source)}
	}
}

func (m *Model) toggleSelectedProjectWorkspace() {
	project, ok := m.selectedProject()
	if !ok || m.projectResult == nil {
		m.setStatus("Select a project before changing the tree.", false)
		return
	}
	workspacePath := m.workspaceFor(project.Path)
	if workspacePath == "" {
		m.setStatus("The selected project is independent.", false)
		return
	}
	if m.projectExpanded == nil {
		m.projectExpanded = make(map[string]bool)
	}
	expanded := m.workspaceExpanded(workspacePath)
	m.projectExpanded[workspacePath] = !expanded
	state := "collapsed"
	if !expanded {
		state = "expanded"
	}
	m.setStatus(fmt.Sprintf("Workspace %s %s.", filepath.Base(workspacePath), state), false)
}

func (m Model) workspaceExpanded(path string) bool {
	if m.projectExpanded == nil {
		return true
	}
	expanded, exists := m.projectExpanded[path]
	if !exists {
		return true
	}
	return expanded
}

func (m Model) filteredProjects() []projects.Project {
	if m.projectResult == nil {
		return nil
	}
	query := strings.TrimSpace(strings.ToLower(m.projectFilter))
	if query == "" {
		return m.projectResult.Projects
	}
	filtered := make([]projects.Project, 0, len(m.projectResult.Projects))
	for _, project := range m.projectResult.Projects {
		haystack := strings.ToLower(strings.Join([]string{
			project.Name,
			project.Path,
			project.Directory,
			project.SDK,
			project.OutputType,
			strings.Join(project.TargetFrameworks, " "),
		}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, project)
		}
	}
	return filtered
}

func (m *Model) setProjectFilter(filter string) {
	previous := ""
	if selected, ok := m.selectedProject(); ok {
		previous = selected.Path
	}
	m.projectFilter = filter
	m.clampProjectSelection()
	m.restoreProjectSelection(previous)
	m.projectListScroll = 0
	m.projectDetailScroll = 0
	m.ensureProjectSelectionVisible()
}

func (m Model) handleProjectFilterKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		origin := m.projectFinderOrigin
		m.projectFiltering = false
		m.projectFilter = ""
		m.restoreProjectSelection(origin)
		m.projectFinderCursor = 0
		m.projectFinderOrigin = ""
		m.projectFinderEntriesCache = nil
		m.projectFinderLoading = false
		return m, nil
	case "enter", "tab":
		return m, m.openProjectFromFinder()
	case "up", "k":
		m.projectFinderCursor = clampIndex(m.projectFinderCursor-1, len(m.projectFinderEntries()))
		return m, nil
	case "down", "j":
		m.projectFinderCursor = clampIndex(m.projectFinderCursor+1, len(m.projectFinderEntries()))
		return m, nil
	case "pgup":
		m.projectFinderCursor = clampIndex(m.projectFinderCursor-6, len(m.projectFinderEntries()))
		return m, nil
	case "pgdown":
		m.projectFinderCursor = clampIndex(m.projectFinderCursor+6, len(m.projectFinderEntries()))
		return m, nil
	case "backspace":
		runes := []rune(m.projectFilter)
		if len(runes) > 0 {
			m.projectFilter = string(runes[:len(runes)-1])
			m.projectFinderCursor = 0
			return m, nil
		}
		parent := filepath.Dir(filepath.Clean(m.projectFinderPath))
		if parent != filepath.Clean(m.projectFinderPath) {
			return m, m.navigateProjectFinder(parent)
		}
		return m, nil
	case "ctrl+u":
		m.projectFilter = ""
		m.projectFinderCursor = 0
		return m, nil
	}
	if message.Text != "" && !message.Mod.Contains(tea.ModCtrl) && !message.Mod.Contains(tea.ModAlt) {
		m.projectFilter += message.Text
		m.projectFinderCursor = 0
	}
	return m, nil
}

func (m Model) projectIsExternal(path string) bool {
	if m.backend == nil {
		return false
	}
	root := m.backend.Paths().ProjectsRoot
	if root == "" {
		root = m.backend.Config().DefaultProjectDir
	}
	if root == "" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)
}

func (m Model) projectSplit() bool {
	// The Projects screen is intentionally a focused picker. Detail content
	// lives on its own page so wide terminals do not turn the picker into a
	// crowded split dashboard.
	return false
}

func (m *Model) toggleProjectFocus() {
	if !m.projectSplit() {
		m.projectFocus = projectFocusList
		return
	}
	if m.projectFocus == projectFocusList {
		m.projectFocus = projectFocusDetail
		return
	}
	m.projectFocus = projectFocusList
}

func (m Model) focusLabel() string {
	if m.projectFocus == projectFocusDetail {
		return "project detail"
	}
	return "project list"
}

func (m *Model) ensureProjectSelectionVisible() {
	if !m.projectSplit() {
		return
	}
	_, selectedLine := m.projectListContent()
	visible := m.projectListVisibleHeight()
	if visible <= 0 {
		return
	}
	if selectedLine < m.projectListScroll {
		m.projectListScroll = selectedLine
	}
	if selectedLine >= m.projectListScroll+visible {
		m.projectListScroll = selectedLine - visible + 1
	}
	m.projectListScroll = clampIndex(m.projectListScroll, m.projectListContentHeight())
}

func (m *Model) scrollFocusedBy(delta int) {
	if m.screen == ScreenProjects && m.projectSplit() {
		m.setFocusedScroll(m.focusedScroll() + delta)
		return
	}
	m.setScroll(m.scrollOffset() + delta)
}

func (m *Model) setFocusedScroll(value int) {
	if m.screen == ScreenProjects && m.projectSplit() {
		maximum := m.maxFocusedScroll()
		if value < 0 {
			value = 0
		}
		if value > maximum {
			value = maximum
		}
		if m.projectFocus == projectFocusDetail {
			m.projectDetailScroll = value
		} else {
			m.projectListScroll = value
		}
		return
	}
	m.setScroll(value)
}

func (m Model) focusedScroll() int {
	if m.screen != ScreenProjects || !m.projectSplit() {
		return m.scrollOffset()
	}
	if m.projectFocus == projectFocusDetail {
		return m.projectDetailScroll
	}
	return m.projectListScroll
}

func (m Model) maxFocusedScroll() int {
	if m.screen != ScreenProjects || !m.projectSplit() {
		return m.maxScroll()
	}
	if m.projectFocus == projectFocusDetail {
		return m.projectDetailContentHeight()
	}
	return m.projectListContentHeight()
}

func (m Model) openProjectCmd(project projects.Project) tea.Cmd {
	workspace := m.workspaceFor(project.Path)
	backend := m.backend
	pinExternal := m.projectIsExternal(project.Path)
	return func() tea.Msg {
		if backend == nil {
			return projectOpenedMsg{project: project, err: errors.New("no backend configured")}
		}
		if pinExternal {
			if err := backend.PinExternalProject(project.Path); err != nil {
				return projectOpenedMsg{project: project, err: err}
			}
		}
		return projectOpenedMsg{
			project: project,
			err:     backend.RecordProjectOpen(project.Path, workspace),
		}
	}
}

func (m *Model) openProjectDetail(project projects.Project) tea.Cmd {
	m.projectDetailOpen = true
	m.projectDetailPath = project.Path
	m.projectDetailWorkspace = false
	m.projectDetailProjectValue = project
	m.projectDetailProjectLoaded = true
	m.projectDetailWorkspaceLoaded = false
	m.projectDetailScroll = 0
	m.projectListScroll = 0
	m.projectFocus = projectFocusDetail
	return m.openProjectCmd(project)
}

func (m *Model) openWorkspaceDetail(workspace projects.Workspace) tea.Cmd {
	m.adoptWorkspace(workspace)
	m.projectDetailOpen = true
	m.projectDetailPath = workspace.Path
	m.projectDetailWorkspace = true
	m.projectDetailWorkspaceValue = workspace
	m.projectDetailWorkspaceLoaded = true
	m.projectDetailProjectLoaded = false
	m.projectDetailScroll = 0
	m.projectListScroll = 0
	m.projectFocus = projectFocusDetail
	return m.openWorkspaceCmd(workspace)
}

func (m Model) openWorkspaceCmd(workspace projects.Workspace) tea.Cmd {
	backend := m.backend
	pinExternal := m.projectIsExternal(workspace.Path)
	return func() tea.Msg {
		if backend == nil {
			return workspaceOpenedMsg{workspace: workspace, err: errors.New("no backend configured")}
		}
		if pinExternal {
			if err := backend.PinExternalProject(workspace.Path); err != nil {
				return workspaceOpenedMsg{workspace: workspace, err: err}
			}
		}
		recordErr := backend.RecordWorkspaceOpen(workspace.Path)
		if recordErr != nil {
			return workspaceOpenedMsg{workspace: workspace, err: recordErr}
		}
		ctx, cancel := context.WithTimeout(context.Background(), projectsScanTimeout)
		defer cancel()
		result, scanErr := backend.ScanProjects(ctx, workspace.Path, false)
		return workspaceOpenedMsg{workspace: workspace, result: result, scanErr: scanErr}
	}
}

func (m Model) workspaceFor(projectPath string) string {
	if m.projectResult == nil {
		return ""
	}
	for _, workspace := range m.projectResult.Workspaces {
		for _, path := range workspace.ProjectPaths {
			if path == projectPath {
				return workspace.Path
			}
		}
	}
	return ""
}

func (m Model) selectedProject() (projects.Project, bool) {
	if m.projectDetailOpen && !m.projectDetailWorkspace {
		if project, ok := m.projectDetailProject(); ok {
			return project, true
		}
	}
	projectsList := m.filteredProjects()
	if len(projectsList) == 0 {
		return projects.Project{}, false
	}
	index := clampIndex(m.projectCursor, len(projectsList))
	return projectsList[index], true
}

func (m *Model) adoptProject(project projects.Project) {
	if m.projectResult == nil {
		result := projects.Result{Root: m.projectWorkspaceRoot()}
		m.projectResult = &result
	}
	for _, existing := range m.projectResult.Projects {
		if existing.Path == project.Path {
			return
		}
	}
	m.projectResult.Projects = append(m.projectResult.Projects, project)
	for _, workspace := range m.projectResult.Workspaces {
		if workspace.Path == project.Path {
			return
		}
	}
	m.projectResult.Workspaces = append(m.projectResult.Workspaces, projects.Workspace{
		Path:         project.Path,
		Name:         project.Name,
		Directory:    project.Directory,
		Kind:         projects.KindProject,
		ProjectPaths: []string{project.Path},
	})
}

func (m *Model) adoptWorkspace(workspace projects.Workspace) {
	if m.projectResult == nil {
		result := projects.Result{Root: m.projectWorkspaceRoot()}
		m.projectResult = &result
	}
	for _, existing := range m.projectResult.Workspaces {
		if existing.Path == workspace.Path {
			return
		}
	}
	m.projectResult.Workspaces = append(m.projectResult.Workspaces, workspace)
}

func (m *Model) restoreProjectSelection(path string) {
	m.clampProjectSelection()
	if path == "" || m.projectResult == nil {
		return
	}
	for index, project := range m.filteredProjects() {
		if project.Path == path {
			m.projectCursor = index
			return
		}
	}
}

func (m *Model) clampProjectSelection() {
	projectsList := m.filteredProjects()
	if len(projectsList) == 0 {
		m.projectCursor = 0
		return
	}
	m.projectCursor = clampIndex(m.projectCursor, len(projectsList))
}
