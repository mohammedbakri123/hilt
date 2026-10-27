package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/execution"
)

func (m *Model) openProjectActionMenu() {
	if m.projectDetailWorkspace {
		if _, ok := m.projectDetailSolution(); !ok {
			m.setStatus("Select a solution before opening actions.", true)
			return
		}
	} else if _, ok := m.selectedProject(); !ok {
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
		actions := m.projectActionsForCurrent()
		m.projectActionCursor = (m.projectActionCursor - 1 + len(actions)) % len(actions)
		return m, nil
	case "down", "j":
		actions := m.projectActionsForCurrent()
		m.projectActionCursor = (m.projectActionCursor + 1) % len(actions)
		return m, nil
	case "enter":
		m.projectActionMenu = false
		return m, m.startProjectAction()
	}
	return m, nil
}

func (m *Model) startProjectAction() tea.Cmd {
	actions := m.projectActionsForCurrent()
	if len(actions) == 0 {
		m.setStatus("Select a project or solution before running an action.", true)
		return nil
	}
	if m.selection == nil || m.selection.DotnetPath == "" {
		m.setStatus("No usable .NET SDK was selected.", true)
		return nil
	}
	action := actions[clampIndex(m.projectActionCursor, len(actions))]
	name := ""
	targetPath := ""
	if m.projectDetailWorkspace {
		workspace, ok := m.projectDetailSolution()
		if !ok {
			m.setStatus("Select a solution before running an action.", true)
			return nil
		}
		name = workspace.Name
		targetPath = workspace.Path
	} else {
		project, ok := m.selectedProject()
		if !ok {
			m.setStatus("Select a project before running an action.", true)
			return nil
		}
		name = project.Name
		targetPath = project.Path
		if action.workspace {
			targetPath = m.workspaceFor(project.Path)
			if targetPath == "" {
				m.setStatus("The selected project is not part of a solution.", true)
				return nil
			}
		}
	}
	args := append([]string{}, action.args...)
	args = append(args, targetPath, "--nologo")
	request := execution.CommandRequest{
		DotnetPath:       m.selection.DotnetPath,
		ProjectPath:      targetPath,
		WorkingDirectory: filepath.Dir(targetPath),
		Arguments:        args,
		Environment:      m.selection.Environment(nil),
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.startOperation(fmt.Sprintf("%s %s", action.label, name), cancel)
	ch := make(chan tea.Msg, 256)
	m.progressCh = ch
	return tea.Batch(m.projectCommandCmd(ctx, action.label, request, ch), waitForProgress(ch))
}

func (m Model) projectActionsForCurrent() []projectAction {
	if m.projectDetailWorkspace {
		return []projectAction{
			{label: "Build solution", args: []string{"build"}, workspace: true},
			{label: "Test solution", args: []string{"test"}, workspace: true},
		}
	}
	return projectActions
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
	target := "project"
	if m.projectDetailWorkspace {
		target = "solution"
	}
	content := []string{styleMuted.Render("Actions for the selected " + target), ""}
	for index, action := range m.projectActionsForCurrent() {
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
