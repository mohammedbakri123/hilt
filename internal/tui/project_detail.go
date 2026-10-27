package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

func (m Model) projectDetailProject() (projects.Project, bool) {
	if m.projectDetailWorkspace {
		return projects.Project{}, false
	}
	if m.projectDetailProjectLoaded && m.projectDetailProjectValue.Path == m.projectDetailPath {
		return m.projectDetailProjectValue, true
	}
	if m.projectResult == nil || m.projectDetailPath == "" {
		return projects.Project{}, false
	}
	for _, project := range m.projectResult.Projects {
		if project.Path == m.projectDetailPath {
			return project, true
		}
	}
	return projects.Project{}, false
}

func (m Model) projectDetailSolution() (projects.Workspace, bool) {
	if !m.projectDetailWorkspace {
		return projects.Workspace{}, false
	}
	if m.projectDetailWorkspaceLoaded && m.projectDetailWorkspaceValue.Path == m.projectDetailPath {
		return m.projectDetailWorkspaceValue, true
	}
	if m.projectResult == nil || m.projectDetailPath == "" {
		return projects.Workspace{}, false
	}
	for _, workspace := range m.projectResult.Workspaces {
		if workspace.Path == m.projectDetailPath {
			return workspace, true
		}
	}
	return projects.Workspace{}, false
}

func (m Model) projectDetailLines(width int) []string {
	if m.projectDetailWorkspace {
		return m.solutionDetailLines(sectionHeader("Solution Detail", width))
	}
	lines := sectionHeader("Project Detail", width)
	project, ok := m.projectDetailProject()
	if !ok {
		return append(lines, styleMuted.Render("Project is no longer present in the latest scan."), "", styleKey.Render("esc")+" back to projects")
	}

	badges := []string{}
	if project.TestProject {
		badges = append(badges, styleOK.Render("[test]"))
	}
	if m.projectIsExternal(project.Path) {
		badges = append(badges, styleWarning.Render("[external]"))
	}
	lines = append(lines,
		styleMuted.Render("Projects & Solutions  /  "+project.Name),
		"",
		styleTitle.Render(project.Name)+"  "+styleMuted.Render("["+string(project.Kind)+"]")+"  "+strings.Join(badges, " "),
		styleFaint.Render(project.Path),
		styleKey.Render("b")+" quick build   "+styleKey.Render("B")+" build / run   "+styleKey.Render("m")+" actions   "+styleKey.Render("o")+" output   "+styleKey.Render("t")+" files",
		"",
		m.projectTabBar(),
		"",
	)
	lines = append(lines, m.projectTabContent(project, width-2)...)
	return projectWarningContent(lines, m.projectResult.Warnings, width)
}

func (m Model) solutionDetailLines(lines []string) []string {
	workspace, ok := m.projectDetailSolution()
	if !ok {
		return append(lines, styleMuted.Render("Solution is no longer present in the latest scan."), "", styleKey.Render("esc")+" back to projects")
	}
	lines = append(lines,
		styleMuted.Render("Projects & Solutions  /  "+workspace.Name),
		"",
		styleTitle.Render(workspace.Name)+"  "+styleMuted.Render("["+string(workspace.Kind)+"]"),
		styleFaint.Render(workspace.Path),
		styleKey.Render("b")+" build solution   "+styleKey.Render("m")+" actions   "+styleKey.Render("enter")+" open member   "+styleKey.Render("esc")+" back",
		"",
		styleLabel.Render(fmt.Sprintf("MEMBER PROJECTS · %d", len(workspace.ProjectPaths))),
	)
	if len(workspace.ProjectPaths) == 0 {
		return append(lines, styleMuted.Render("No project entries declared by this solution."))
	}
	for index, path := range workspace.ProjectPaths {
		name := path
		status := styleWarning.Render("[missing]")
		for _, project := range m.projectResult.Projects {
			if project.Path == path {
				name = project.Name
				status = styleOK.Render("[ready]")
				break
			}
		}
		marker := "  "
		if index == m.solutionMemberCursor {
			marker = styleAccent.Render("> ")
		}
		lines = append(lines, marker+status+" "+styleValue.Render(name)+"  "+styleFaint.Render(path))
	}
	lines = append(lines, "", styleFaint.Render("↑/↓ select member · enter open selected member"))
	return lines
}

func (m Model) selectedSolutionMember() (projects.Project, bool, bool) {
	workspace, ok := m.projectDetailSolution()
	if !ok || m.solutionMemberCursor < 0 || m.solutionMemberCursor >= len(workspace.ProjectPaths) {
		return projects.Project{}, false, false
	}
	path := workspace.ProjectPaths[m.solutionMemberCursor]
	if m.projectResult == nil {
		return projects.Project{Path: path, Name: filepath.Base(path), Directory: filepath.Dir(path), Kind: projects.KindProject}, true, false
	}
	for _, project := range m.projectResult.Projects {
		if project.Path == path {
			return project, true, true
		}
	}
	return projects.Project{Path: path, Name: filepath.Base(path), Directory: filepath.Dir(path), Kind: projects.KindProject}, true, false
}

func (m Model) handleProjectDetailKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "backspace":
		m.closeProjectDetail()
		return m, nil
	case "up", "k":
		if m.projectDetailWorkspace {
			m.moveSolutionMember(-1)
			return m, nil
		}
		m.scrollBy(-1)
		return m, nil
	case "down", "j":
		if m.projectDetailWorkspace {
			m.moveSolutionMember(1)
			return m, nil
		}
		m.scrollBy(1)
		return m, nil
	case "pgup":
		m.scrollBy(-8)
		return m, nil
	case "pgdown":
		m.scrollBy(8)
		return m, nil
	case "home":
		m.setScroll(0)
		return m, nil
	case "end":
		m.setScroll(m.maxScroll())
		return m, nil
	case ",":
		m.moveProjectTab(-1)
		return m, m.startProjectTreeScan()
	case ".":
		m.moveProjectTab(1)
		return m, m.startProjectTreeScan()
	case "t":
		if m.projectDetailWorkspace {
			return m, nil
		}
		m.projectTab = projectTabTree
		m.projectDetailScroll = 0
		return m, m.startProjectTreeScan()
	case "b":
		if m.projectDetailWorkspace {
			m.projectActionCursor = 0
			return m, m.startProjectAction()
		}
		return m, m.startBuild()
	case "B":
		m.openBuildRunDialog()
		return m, nil
	case "m":
		m.openProjectActionMenu()
		return m, nil
	case "x":
		m.beginDeleteProject()
		return m, nil
	case "u":
		return m, m.unpinSelectedExternalProject()
	case "r":
		return m, m.startProjectsScan()
	case "o":
		m.drawer.open = !m.drawer.open
		if m.drawer.open && m.drawer.follow {
			m.followDrawer()
		}
		return m, nil
	case "enter":
		if !m.projectDetailWorkspace {
			return m, nil
		}
		if _, ok := m.projectDetailSolution(); !ok {
			return m, nil
		}
		project, present, ready := m.selectedSolutionMember()
		if !present {
			return m, nil
		}
		if !ready {
			m.setStatus("Selected solution member is missing: "+project.Path, true)
			return m, nil
		}
		m.restoreProjectSelection(project.Path)
		m.projectDetailWorkspace = false
		return m, m.openProjectDetail(project)
	case "?":
		m.showHelp = true
		return m, nil
	case "d":
		return m, m.navigate(ScreenDiagnostics)
	case "1", "2", "3", "4", "5":
		return m, m.navigate(Screen(int(message.String()[0] - '1')))
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) closeProjectDetail() {
	m.projectDetailOpen = false
	m.projectDetailPath = ""
	m.projectDetailWorkspace = false
	m.projectDetailProjectValue = projects.Project{}
	m.projectDetailProjectLoaded = false
	m.projectDetailWorkspaceValue = projects.Workspace{}
	m.projectDetailWorkspaceLoaded = false
	m.solutionMemberCursor = 0
	m.projectDetailScroll = 0
	m.projectFocus = projectFocusList
	m.setStatus("Back to Projects & Solutions.", false)
}

func (m Model) handleProjectDetailMouseClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if mouse.Y == headerLineCount+3-m.projectDetailScroll {
		m.closeProjectDetail()
		return m, nil
	}
	if tab, ok := m.projectDetailTabAt(mouse.X, mouse.Y); ok {
		m.projectTab = tab
		m.projectDetailScroll = 0
		return m, m.startProjectTreeScan()
	}
	return m, nil
}

func (m Model) projectDetailTabAt(x, y int) (projectTab, bool) {
	targetY := headerLineCount + 8 - m.projectDetailScroll
	if y != targetY {
		return 0, false
	}
	contentX := contentInset
	if m.showSidebar() {
		contentX += sidebarWidth
	}
	relative := x - contentX - 2
	if relative < 0 {
		return 0, false
	}
	for index, tab := range projectTabs {
		label := tab.label
		if projectTab(index) == m.projectTab {
			label = "[" + label + "]"
		}
		width := ansi.StringWidth(label)
		if relative < width {
			return projectTab(index), true
		}
		relative -= width + 2
	}
	return 0, false
}
