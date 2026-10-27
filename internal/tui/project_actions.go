package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/projects"
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
	m.hoveredProjectAction = -1
}

func (m Model) handleProjectActionKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "m":
		m.projectActionMenu = false
		m.hoveredProjectAction = -1
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
		m.hoveredProjectAction = -1
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
	if action.kind != projectActionCommand {
		if promptKind, ok := projectActionPromptFor(action.kind); ok {
			m.openProjectActionPrompt(promptKind)
			return nil
		}
	}
	args := projectActionArguments(action, targetPath)
	if action.kind == projectActionCommand && len(action.args) > 0 && action.args[0] == "run" && !action.workspace {
		project, ok := m.selectedProject()
		if !ok {
			m.setStatus("Select a project before running it.", true)
			return nil
		}
		workingDirectory := project.Directory
		if workingDirectory == "" {
			workingDirectory = filepath.Dir(project.Path)
		}
		runArgs, err := execution.RunArgs(targetPath, "Debug", "", nil)
		if err != nil {
			m.setStatus("Could not prepare the run command: "+err.Error(), true)
			return nil
		}
		return m.startRunRequest(project, execution.CommandRequest{
			DotnetPath:       m.selection.DotnetPath,
			ProjectPath:      targetPath,
			WorkingDirectory: workingDirectory,
			Arguments:        runArgs,
			Environment:      m.selection.Environment(nil),
		})
	}
	refreshProjects := action.kind == projectActionPackageAdd || action.kind == projectActionPackageRemove || action.kind == projectActionPackageUpdate
	if projectActionNeedsConfirmation(action.kind) {
		m.queueProjectActionConfirmation(action.label, name, targetPath, args, refreshProjects)
		return nil
	}
	return m.startProjectCommand(action.label, name, targetPath, args, refreshProjects)
}

func (m *Model) queueProjectActionConfirmation(label, name, targetPath string, args []string, refreshProjects bool) {
	m.pendingProjectAction = &pendingProjectAction{
		label:           label,
		name:            name,
		targetPath:      targetPath,
		args:            append([]string(nil), args...),
		refreshProjects: refreshProjects,
	}
	m.confirm = confirmProjectAction
	m.confirmSummary = []string{
		fmt.Sprintf("%s for %s", label, name),
		"This changes project state and may update files or the database.",
		"dotnet " + strings.Join(args, " "),
	}
}

func projectActionNeedsConfirmation(kind projectActionKind) bool {
	switch kind {
	case projectActionPackageRemove, projectActionPackageUpdate, projectActionEFUpdate, projectActionEFRemove:
		return true
	default:
		return false
	}
}

func (m *Model) startProjectCommand(label, name, targetPath string, args []string, refreshProjects bool) tea.Cmd {
	request := execution.CommandRequest{
		DotnetPath:       m.selection.DotnetPath,
		ProjectPath:      targetPath,
		WorkingDirectory: filepath.Dir(targetPath),
		Arguments:        args,
		Environment:      m.selection.Environment(nil),
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.startOperation(fmt.Sprintf("%s %s", label, name), cancel)
	ch := make(chan tea.Msg, 256)
	m.progressCh = ch
	return tea.Batch(m.projectCommandCmd(ctx, label, request, refreshProjects, ch), waitForProgress(ch))
}

func projectActionArguments(action projectAction, targetPath string) []string {
	switch action.kind {
	case projectActionPackageList:
		return []string{"list", targetPath, "package"}
	case projectActionPackageRestore:
		return []string{"restore", targetPath, "--nologo"}
	case projectActionEFList:
		return []string{"ef", "migrations", "list", "--project", targetPath}
	case projectActionEFUpdate:
		return []string{"ef", "database", "update", "--project", targetPath}
	case projectActionEFRemove:
		return []string{"ef", "migrations", "remove", "--project", targetPath}
	}
	args := append([]string{}, action.args...)
	return append(args, targetPath, "--nologo")
}

func projectActionPromptFor(kind projectActionKind) (projectActionPromptKind, bool) {
	switch kind {
	case projectActionPackageAdd:
		return projectActionPromptPackageAdd, true
	case projectActionPackageRemove:
		return projectActionPromptPackageRemove, true
	case projectActionPackageUpdate:
		return projectActionPromptPackageUpdate, true
	case projectActionEFAdd:
		return projectActionPromptEFAdd, true
	default:
		return projectActionPromptNone, false
	}
}

func (m Model) projectActionsForCurrent() []projectAction {
	if m.projectDetailWorkspace {
		return []projectAction{
			{label: "Build solution", args: []string{"build"}, workspace: true},
			{label: "Test solution", args: []string{"test"}, workspace: true},
		}
	}
	actions := append([]projectAction(nil), projectActions...)
	if project, ok := m.selectedProject(); ok && projectHasEFCore(project) {
		actions = append(actions,
			projectAction{label: "List migrations", kind: projectActionEFList},
			projectAction{label: "Add migration", kind: projectActionEFAdd},
			projectAction{label: "Update database", kind: projectActionEFUpdate},
			projectAction{label: "Remove migration", kind: projectActionEFRemove},
		)
	}
	return actions
}

func (m Model) projectCommandCmd(ctx context.Context, label string, request execution.CommandRequest, refreshProjects bool, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		result, err := backend.RunProjectCommand(ctx, request, progressWriter{ch}, progressWriter{ch})
		return operationDoneMsg{kind: opProjectCommand, command: label, result: result, refreshProjects: refreshProjects, err: err}
	}
}

func projectHasEFCore(project projects.Project) bool {
	for _, packageReference := range project.PackageReferences {
		if projects.IsEntityFrameworkCorePackage(packageReference.Name) {
			return true
		}
	}
	return false
}

func (m Model) renderProjectActionMenu(width int) string {
	boxWidth := 68
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	target := "project"
	if m.projectDetailWorkspace {
		target = "solution"
	}
	content := []string{styleMuted.Render("Actions for the selected " + target), styleFaint.Render("Choose an action; destructive changes ask for confirmation."), ""}
	group := ""
	for index, action := range m.projectActionsForCurrent() {
		currentGroup := projectActionGroup(action)
		if currentGroup != group {
			if group != "" {
				content = append(content, "")
			}
			content = append(content, styleLabel.Render(currentGroup))
			group = currentGroup
		}
		marker := "  "
		if index == m.projectActionCursor || index == m.hoveredProjectAction {
			marker = styleAccent.Render("> ")
		}
		line := marker + styleValue.Render(action.label)
		if description := projectActionDescription(action); description != "" {
			line += "  " + styleMuted.Render(description)
		}
		content = append(content, line)
	}
	selected := m.projectActionsForCurrent()
	if len(selected) > 0 {
		action := selected[clampIndex(m.projectActionCursor, len(selected))]
		content = append(content, "", styleFaint.Render("Selected: "+projectActionDescription(action)))
	}
	content = append(content, "", styleAccent.Render("click an action")+"  "+styleFaint.Render("↑/↓ select · enter run · esc close"))
	return styleCallout.Width(boxWidth).Render(joinLines(content))
}

func projectActionGroup(action projectAction) string {
	if action.kind >= projectActionEFList {
		return "EF CORE"
	}
	if action.kind >= projectActionPackageList {
		return "PACKAGES"
	}
	return "BUILD & RUN"
}

func projectActionDescription(action projectAction) string {
	switch action.kind {
	case projectActionPackageList:
		return "inspect references"
	case projectActionPackageAdd:
		return "add NuGet reference"
	case projectActionPackageRemove:
		return "remove NuGet reference"
	case projectActionPackageUpdate:
		return "change NuGet version"
	case projectActionPackageRestore:
		return "restore dependencies"
	case projectActionEFList:
		return "show migrations"
	case projectActionEFAdd:
		return "create migration"
	case projectActionEFUpdate:
		return "apply database changes"
	case projectActionEFRemove:
		return "remove last migration"
	}
	if action.workspace {
		return "whole solution"
	}
	if len(action.args) > 0 && action.args[0] == "run" {
		return "start app server"
	}
	return "dotnet " + strings.Join(action.args, " ")
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
