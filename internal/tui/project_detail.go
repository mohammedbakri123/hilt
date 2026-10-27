package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

func (m Model) projectDetailProject() (projects.Project, bool) {
	if m.projectDetailWorkspace {
		return projects.Project{}, false
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
	if !m.projectDetailWorkspace || m.projectResult == nil || m.projectDetailPath == "" {
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
		"",
		styleLabel.Render("MEMBER PROJECTS"),
	)
	if len(workspace.ProjectPaths) == 0 {
		return append(lines, styleMuted.Render("No project entries declared by this solution."))
	}
	for _, path := range workspace.ProjectPaths {
		name := path
		for _, project := range m.projectResult.Projects {
			if project.Path == path {
				name = project.Name
				break
			}
		}
		lines = append(lines, "  "+styleValue.Render(name)+"  "+styleFaint.Render(path))
	}
	return lines
}

func (m Model) handleProjectDetailKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "backspace":
		m.closeProjectDetail()
		return m, nil
	case "up", "k":
		m.scrollBy(-1)
		return m, nil
	case "down", "j":
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
		return m, m.startBuild()
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
		workspace, ok := m.projectDetailSolution()
		if !ok {
			return m, nil
		}
		for _, path := range workspace.ProjectPaths {
			for _, project := range m.projectResult.Projects {
				if project.Path == path {
					m.restoreProjectSelection(project.Path)
					m.projectDetailWorkspace = false
					return m, m.openProjectDetail(project)
				}
			}
		}
		return m, nil
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
