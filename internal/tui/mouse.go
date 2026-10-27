package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const headerLineCount = 2

func (m Model) handleMouseClick(message tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := message.Mouse()
	if m.showHelp || m.quitConfirm || m.rootPicker.open || m.prompt != promptNone || m.confirm != confirmNone || m.createWizard.open || m.projectActionMenu || m.busy() {
		return m, nil
	}

	if m.showSidebar() && mouse.X >= 0 && mouse.X < sidebarWidth {
		bodyLine := mouse.Y - headerLineCount
		const sidebarNavigationStart = 2
		index := bodyLine - sidebarNavigationStart
		if index >= 0 && index < len(screens) {
			return m, m.navigate(Screen(index))
		}
		return m, nil
	}

	if m.screen != ScreenProjects || mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseRight {
		return m, nil
	}
	return m.handleProjectMouseClick(mouse)
}

func (m Model) handleMouseWheel(message tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := message.Mouse()
	delta := 3
	if mouse.Button == tea.MouseWheelUp {
		delta = -delta
	} else if mouse.Button != tea.MouseWheelDown {
		return m, nil
	}

	if m.rootPicker.open {
		m.movePicker(delta)
		return m, nil
	}
	if m.showHelp || m.quitConfirm || m.prompt != promptNone || m.confirm != confirmNone || m.createWizard.open || m.projectActionMenu {
		return m, nil
	}
	if m.drawer.open {
		m.scrollDrawerBy(delta)
		return m, nil
	}
	if m.screen == ScreenProjects && m.projectSplit() {
		left, right, inside := m.projectPaneAt(mouse.X, mouse.Y)
		if inside {
			if right {
				m.projectFocus = projectFocusDetail
			} else if left {
				m.projectFocus = projectFocusList
			}
			m.scrollFocusedBy(delta)
			return m, nil
		}
	}
	m.scrollBy(delta)
	return m, nil
}

func (m Model) handleProjectMouseClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if m.projectSplit() {
		left, right, inside := m.projectPaneAt(mouse.X, mouse.Y)
		if !inside {
			return m, nil
		}
		if right {
			m.projectFocus = projectFocusDetail
			if tab, ok := m.projectTabAt(mouse.X, mouse.Y); ok {
				m.projectTab = tab
				m.projectDetailScroll = 0
				return m, m.startProjectTreeScan()
			}
			return m, nil
		}
		if !left {
			return m, nil
		}
		line := mouse.Y - m.projectPanelTop() - 2 + m.projectListScroll
		if projectIndex, ok := m.projectListProjectAt(line); ok && projectIndex >= 0 {
			m.projectCursor = projectIndex
			m.projectDetailScroll = 0
			m.projectFocus = projectFocusList
			if mouse.Button == tea.MouseRight {
				m.openProjectActionMenu()
				return m, nil
			}
			return m, m.startProjectTreeScan()
		}
		if workspaceIndex, ok := m.projectListProjectAt(line); ok && workspaceIndex < 0 && m.projectResult != nil {
			index := -workspaceIndex - 1
			if index >= 0 && index < len(m.projectResult.Workspaces) {
				if m.projectExpanded == nil {
					m.projectExpanded = make(map[string]bool)
				}
				workspace := m.projectResult.Workspaces[index]
				m.projectExpanded[workspace.Path] = !m.workspaceExpanded(workspace.Path)
			}
		}
		return m, nil
	}

	line := mouse.Y - headerLineCount + m.projectScroll
	lines := m.projectLines(m.contentWidth())
	for index, project := range m.filteredProjects() {
		if line < 0 || line >= len(lines) {
			break
		}
		plain := ansi.Strip(lines[line])
		if strings.Contains(plain, project.Name) && strings.Contains(plain, project.Path) {
			m.projectCursor = index
			m.projectDetailScroll = 0
			if mouse.Button == tea.MouseRight {
				m.openProjectActionMenu()
			}
			return m, m.startProjectTreeScan()
		}
	}
	return m, nil
}

func (m Model) projectPanelTop() int {
	return headerLineCount + 4 - m.projectScroll
}

func (m Model) projectPaneAt(x, y int) (left, right, inside bool) {
	if !m.projectSplit() {
		return false, false, false
	}
	contentX := 0
	if m.showSidebar() {
		contentX = sidebarWidth
	}
	contentX += contentInset
	listWidth, _ := projectPaneWidths(m.contentWidth())
	top := m.projectPanelTop()
	height := m.projectPaneHeight()
	if y < top || y >= top+height {
		return false, false, false
	}
	if x >= contentX && x < contentX+listWidth {
		return true, false, true
	}
	rightX := contentX + listWidth + 1
	if x >= rightX && x < rightX+(m.contentWidth()-listWidth-1) {
		return false, true, true
	}
	return false, false, false
}

func (m Model) projectTabAt(x, y int) (projectTab, bool) {
	left, right, inside := m.projectPaneAt(x, y)
	if left || !right || !inside || y != m.projectPanelTop()+2 {
		return 0, false
	}
	listWidth, _ := projectPaneWidths(m.contentWidth())
	contentX := contentInset
	if m.showSidebar() {
		contentX += sidebarWidth
	}
	rightX := contentX + listWidth + 1
	relative := x - (rightX + 2) - 2 // panel padding plus tab-bar indent
	if relative < 0 {
		return 0, false
	}
	for index, tab := range projectTabs {
		label := tab.label
		if projectTab(index) == m.projectTab {
			label = "[" + label + "]"
		}
		width := ansi.StringWidth(label)
		if relative >= 0 && relative < width {
			return projectTab(index), true
		}
		relative -= width + 2
	}
	return 0, false
}
