package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/execution"
)

func (m *Model) openProjectActionMenu() {
	if _, ok := m.selectedProject(); !ok {
		m.setStatus("Select a project before opening actions.", true)
		return
	}
	m.projectActionMenu = true
	m.projectActionCursor = 0
}

func (m Model) handleProjectActionKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "m":
		m.projectActionMenu = false
		return m, nil
	case "up", "k":
		m.projectActionCursor = (m.projectActionCursor - 1 + len(projectActions)) % len(projectActions)
		return m, nil
	case "down", "j":
		m.projectActionCursor = (m.projectActionCursor + 1) % len(projectActions)
		return m, nil
	case "enter":
		m.projectActionMenu = false
		return m, m.startProjectAction()
	}
	return m, nil
}

func (m *Model) startProjectAction() tea.Cmd {
	project, ok := m.selectedProject()
	if !ok {
		m.setStatus("Select a project before running an action.", true)
		return nil
	}
	if m.selection == nil || m.selection.DotnetPath == "" {
		m.setStatus("No usable .NET SDK was selected.", true)
		return nil
	}
	action := projectActions[m.projectActionCursor]
	args := append([]string{}, action.args...)
	args = append(args, project.Path, "--nologo")
	request := execution.CommandRequest{
		DotnetPath:       m.selection.DotnetPath,
		ProjectPath:      project.Path,
		WorkingDirectory: filepath.Dir(project.Path),
		Arguments:        args,
		Environment:      m.selection.Environment(nil),
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.startOperation(fmt.Sprintf("%s %s", action.label, project.Name), cancel)
	ch := make(chan tea.Msg, 256)
	m.progressCh = ch
	return tea.Batch(m.projectCommandCmd(ctx, action.label, request, ch), waitForProgress(ch))
}

func (m Model) projectCommandCmd(ctx context.Context, label string, request execution.CommandRequest, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		result, err := backend.RunProjectCommand(ctx, request, progressWriter{ch}, progressWriter{ch})
		return operationDoneMsg{kind: opProjectCommand, command: label, result: result, err: err}
	}
}

func (m Model) renderProjectActionMenu(width int) string {
	boxWidth := 42
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	content := []string{styleMuted.Render("Actions for the selected project"), ""}
	for index, action := range projectActions {
		marker := "  "
		if index == m.projectActionCursor {
			marker = styleAccent.Render("> ")
		}
		content = append(content, marker+styleValue.Render(action.label))
	}
	content = append(content, "", styleFaint.Render("↑/↓ select · enter run · esc close"))
	return styleCallout.Width(boxWidth).Render(joinLines(content))
}

func joinLines(lines []string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
