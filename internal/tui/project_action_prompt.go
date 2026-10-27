package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) openProjectActionPrompt(kind projectActionPromptKind) {
	m.projectActionPrompt = projectActionPrompt{open: true, kind: kind}
	m.setStatus("Enter the action details, then press enter.", false)
}

func (m Model) projectActionPromptTitle() string {
	switch m.projectActionPrompt.kind {
	case projectActionPromptPackageAdd:
		return "ADD PACKAGE"
	case projectActionPromptPackageRemove:
		return "REMOVE PACKAGE"
	case projectActionPromptPackageUpdate:
		return "UPDATE PACKAGE"
	case projectActionPromptEFAdd:
		return "ADD MIGRATION"
	default:
		return "PROJECT ACTION"
	}
}

func (m *Model) handleProjectActionPromptKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	prompt := &m.projectActionPrompt
	switch message.String() {
	case "esc", "ctrl+c":
		prompt.open = false
		prompt.errorMessage = ""
		m.setStatus("Action cancelled.", false)
		return *m, nil
	case "tab", "down", "right":
		if prompt.hasVersion() {
			prompt.field = (prompt.field + 1) % 2
		}
		return *m, nil
	case "shift+tab", "up", "left":
		if prompt.hasVersion() {
			prompt.field = (prompt.field + 1) % 2
		}
		return *m, nil
	case "enter":
		return *m, m.submitProjectActionPrompt()
	case "backspace", "ctrl+h":
		m.deleteProjectActionPromptText()
		return *m, nil
	case "ctrl+u":
		if prompt.field == 0 {
			prompt.name = ""
		} else {
			prompt.version = ""
		}
		return *m, nil
	}
	if message.Text != "" && message.Mod&tea.ModCtrl == 0 && message.Mod&tea.ModAlt == 0 {
		if prompt.field == 0 {
			prompt.name += message.Text
		} else {
			prompt.version += message.Text
		}
		prompt.errorMessage = ""
	}
	return *m, nil
}

func (p projectActionPrompt) hasVersion() bool {
	return p.kind == projectActionPromptPackageAdd || p.kind == projectActionPromptPackageUpdate
}

func (m *Model) deleteProjectActionPromptText() {
	value := &m.projectActionPrompt.name
	if m.projectActionPrompt.field == 1 && m.projectActionPrompt.hasVersion() {
		value = &m.projectActionPrompt.version
	}
	runes := []rune(*value)
	if len(runes) > 0 {
		*value = string(runes[:len(runes)-1])
	}
}

func (m *Model) submitProjectActionPrompt() tea.Cmd {
	name := strings.TrimSpace(m.projectActionPrompt.name)
	if name == "" {
		m.projectActionPrompt.errorMessage = "A package or migration name is required."
		return nil
	}
	project, ok := m.selectedProject()
	if !ok {
		m.projectActionPrompt.errorMessage = "Select a project before running this action."
		return nil
	}
	args := []string{}
	label := m.projectActionPromptTitle()
	refresh := false
	switch m.projectActionPrompt.kind {
	case projectActionPromptPackageAdd:
		args = []string{"add", project.Path, "package", name}
		if version := strings.TrimSpace(m.projectActionPrompt.version); version != "" {
			args = append(args, "--version", version)
		}
		refresh = true
	case projectActionPromptPackageRemove:
		args = []string{"remove", project.Path, "package", name}
		refresh = true
	case projectActionPromptPackageUpdate:
		args = []string{"add", project.Path, "package", name}
		if version := strings.TrimSpace(m.projectActionPrompt.version); version != "" {
			args = append(args, "--version", version)
		}
		refresh = true
	case projectActionPromptEFAdd:
		if !projectHasEFCore(project) {
			m.projectActionPrompt.errorMessage = "EF Core is not detected in this project."
			return nil
		}
		args = []string{"ef", "migrations", "add", name, "--project", project.Path}
	}
	m.projectActionPrompt.open = false
	return m.startProjectCommand(label, project.Name, project.Path, args, refresh)
}

func (m Model) renderProjectActionPrompt(width int) string {
	boxWidth := width - 4
	if boxWidth > 72 {
		boxWidth = 72
	}
	if boxWidth < 32 {
		boxWidth = 32
	}
	name := valueOr(m.projectActionPrompt.name, "type here")
	if m.projectActionPrompt.field == 0 && m.projectActionPrompt.name == "" {
		name = styleFaint.Render(name)
	}
	content := []string{
		styleMuted.Render("Values are used only for this command."),
		"",
		m.projectActionPromptRow("Name", name, 0),
	}
	if m.projectActionPrompt.hasVersion() {
		version := valueOr(m.projectActionPrompt.version, "latest")
		if m.projectActionPrompt.field == 1 && m.projectActionPrompt.version == "" {
			version = styleFaint.Render(version)
		}
		content = append(content, m.projectActionPromptRow("Version", version, 1))
	}
	if m.projectActionPrompt.errorMessage != "" {
		content = append(content, "", styleError.Render(m.projectActionPrompt.errorMessage))
	}
	content = append(content, "", styleFaint.Render("tab fields · enter run · esc close"))
	return styleCallout.Width(boxWidth).Render(joinLines(append([]string{styleTitle.Render(m.projectActionPromptTitle())}, content...)))
}

func (m Model) projectActionPromptRow(label, value string, field int) string {
	marker := "  "
	if m.projectActionPrompt.field == field {
		marker = styleAccent.Render("> ")
	}
	return marker + styleLabel.Render(fmt.Sprintf("%-10s", label)) + value
}
