package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View renders the whole application.
func (m Model) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	var content string
	switch {
	case width < minTerminalWidth || height < minTerminalRows:
		content = renderTooSmall(width, height)
	case m.showHelp:
		content = m.renderHelp(width, height)
	default:
		content = m.render(width, height)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Hilt"
	return view
}

func (m Model) render(width, height int) string {
	header := m.renderHeader(width)
	status := m.renderStatusLine(width)
	footer := m.renderFooter(width)
	bodyHeight := m.bodyHeight()

	innerWidth := m.contentWidth()
	lines := m.screenLines(innerWidth)
	offset := m.scrollOffset()
	mainLines := windowLines(lines, innerWidth, bodyHeight, offset)

	inset := contentInset
	if m.contentWidth() == 10 {
		inset = 0
	}
	mainWidth := width
	prefix := strings.Repeat(" ", inset)
	if m.showSidebar() {
		mainWidth = width - sidebarWidth
	}
	prefixed := make([]string, 0, len(mainLines))
	for _, line := range mainLines {
		prefixed = append(prefixed, fitLine(prefix+line, mainWidth))
	}
	main := strings.Join(prefixed, "\n")

	drawer := strings.Join(m.renderDrawer(width), "\n")
	var content string
	if !m.showSidebar() {
		if drawer != "" {
			content = lipgloss.JoinVertical(lipgloss.Left, header, main, drawer, status, footer)
		} else {
			content = lipgloss.JoinVertical(lipgloss.Left, header, main, status, footer)
		}
	} else {
		sidebar := m.renderSidebar(bodyHeight)
		body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
		if drawer != "" {
			content = lipgloss.JoinVertical(lipgloss.Left, header, body, drawer, status, footer)
		} else {
			content = lipgloss.JoinVertical(lipgloss.Left, header, body, status, footer)
		}
	}
	if m.quitConfirm {
		content = overlayCentered(content, m.renderQuitDialog(width), width, height)
	}
	if m.rootPicker.open {
		content = overlayCentered(content, m.renderProjectRootPicker(width, height), width, height)
	}
	return content
}

func (m Model) renderQuitDialog(width int) string {
	boxWidth := 56
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 12 {
		boxWidth = 12
	}
	content := []string{
		"An operation is still running:",
		styleValue.Render(m.busyLabel()),
		"",
		"Quitting interrupts it. Press esc to stay; cancel the operation first if you want to stop it.",
		"",
		styleKey.Render("y / enter") + "  quit anyway",
		styleKey.Render("esc / n") + "     stay",
	}
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
}

// screenLines dispatches to the screen-specific renderer.
func (m Model) screenLines(width int) []string {
	switch m.screen {
	case ScreenHome:
		return m.homeLines(width)
	case ScreenSDKs:
		return m.sdkLines(width)
	case ScreenDiagnostics:
		return m.diagnosticLines(width)
	case ScreenSettings:
		return m.settingsLines(width)
	case ScreenProjects:
		return m.projectLines(width)
	default:
		return []string{"Unknown screen"}
	}
}

func (m Model) footerContext() keyContext {
	switch {
	case m.quitConfirm:
		return ctxQuitConfirm
	case m.rootPicker.open:
		return ctxProjectRoot
	case m.showHelp:
		return ctxHelp
	case m.operationRunning:
		return ctxOperation
	case m.buildRunning:
		if m.drawer.open {
			return ctxBuildOutput
		}
		return ctxBuild
	case m.loadingUpdates:
		return ctxUpdates
	case m.prompt == promptInstallSDK:
		if m.promptFocusInput {
			return ctxPromptSDKInput
		}
		return ctxPromptSDKList
	case m.prompt != promptNone:
		return ctxPromptRuntime
	case m.confirm != confirmNone:
		return ctxConfirm
	case m.drawer.open && !m.projectFiltering:
		return ctxOutput
	case m.screen == ScreenHome:
		return ctxHome
	case m.screen == ScreenSDKs:
		return ctxSDKs
	case m.screen == ScreenDiagnostics:
		return ctxDiagnostics
	case m.screen == ScreenProjects:
		if m.projectFiltering {
			return ctxProjectFilter
		}
		if m.projectsLoading {
			return ctxProjectsLoading
		}
		if m.projectSplit() {
			return ctxProjectSplit
		}
		return ctxProjects
	}
	return ctxSettings
}
