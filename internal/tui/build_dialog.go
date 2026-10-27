package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/projects"
)

// openBuildRunDialog prepares a session-only execution form. Nothing typed in
// the form is persisted; the existing build target state remains limited to
// the last successfully started build.
func (m *Model) openBuildRunDialog() {
	if m.backend == nil {
		m.setStatus("No backend configured.", true)
		return
	}
	if m.projectsLoading {
		m.setStatus("Wait for the project scan to finish before opening Build / Run.", true)
		return
	}
	targets := m.buildRunTargets()
	if len(targets) == 0 {
		m.setStatus("Scan a project or solution before opening Build / Run.", true)
		return
	}

	selectedPath := ""
	if m.projectDetailWorkspace {
		if project, present, ready := m.selectedSolutionMember(); present && ready {
			selectedPath = project.Path
		} else if workspace, ok := m.projectDetailSolution(); ok {
			selectedPath = workspace.Path
		}
	} else if project, ok := m.selectedProject(); ok {
		selectedPath = project.Path
	}
	target := 0
	for index, candidate := range targets {
		if candidate.Path == selectedPath {
			target = index
			break
		}
	}

	m.buildRunDialog = buildRunDialog{
		open:          true,
		field:         buildRunFieldMode,
		mode:          buildRunBuild,
		configuration: "Debug",
		targets:       targets,
		target:        target,
	}
	m.setStatus("Configure a build or run, then press enter.", false)
}

func (m Model) buildRunTargets() []buildRunTarget {
	if m.projectResult == nil {
		return nil
	}
	projectsByPath := make(map[string]projects.Project, len(m.projectResult.Projects))
	for _, project := range m.projectResult.Projects {
		projectsByPath[project.Path] = project
	}
	targets := make([]buildRunTarget, 0, len(m.projectResult.Workspaces)+len(m.projectResult.Projects))
	for _, workspace := range m.projectResult.Workspaces {
		frameworks := []string{}
		missing := 0
		for _, path := range workspace.ProjectPaths {
			project, ok := projectsByPath[path]
			if !ok {
				missing++
				continue
			}
			frameworks = appendUnique(frameworks, project.TargetFrameworks...)
		}
		targets = append(targets, buildRunTarget{
			Name:         workspace.Name,
			Path:         workspace.Path,
			Directory:    workspace.Directory,
			Workspace:    true,
			MissingCount: missing,
			Frameworks:   frameworks,
		})
	}
	for _, project := range m.projectResult.Projects {
		targets = append(targets, buildRunTarget{
			Name:       project.Name,
			Path:       project.Path,
			Directory:  project.Directory,
			Frameworks: append([]string(nil), project.TargetFrameworks...),
		})
	}
	return targets
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func (m *Model) handleBuildRunDialogKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	dialog := &m.buildRunDialog
	switch message.String() {
	case "esc", "ctrl+c":
		dialog.open = false
		dialog.errorMessage = ""
		m.setStatus("Build / Run cancelled.", false)
		return *m, nil
	case "tab":
		dialog.field = (dialog.field + 1) % 6
		return *m, nil
	case "shift+tab":
		dialog.field = (dialog.field + 5) % 6
		return *m, nil
	case "up", "k":
		m.adjustBuildRunField(-1)
		return *m, nil
	case "down", "j":
		m.adjustBuildRunField(1)
		return *m, nil
	case "left", "h":
		if dialog.field != buildRunFieldArguments && dialog.field != buildRunFieldEnvironment {
			m.adjustBuildRunField(-1)
		}
		return *m, nil
	case "right", "l":
		if dialog.field != buildRunFieldArguments && dialog.field != buildRunFieldEnvironment {
			m.adjustBuildRunField(1)
		}
		return *m, nil
	case "enter":
		return *m, m.startConfiguredBuildRun()
	case "backspace", "ctrl+h":
		m.deleteBuildRunText()
		return *m, nil
	case "ctrl+u":
		m.clearBuildRunText()
		return *m, nil
	}
	if message.Text != "" && message.Mod&tea.ModCtrl == 0 && message.Mod&tea.ModAlt == 0 {
		switch dialog.field {
		case buildRunFieldArguments:
			dialog.arguments += message.Text
		case buildRunFieldEnvironment:
			dialog.environment += message.Text
		}
		dialog.errorMessage = ""
	}
	return *m, nil
}

func (m *Model) adjustBuildRunField(delta int) {
	dialog := &m.buildRunDialog
	switch dialog.field {
	case buildRunFieldMode:
		dialog.mode = buildRunMode((int(dialog.mode) + delta + 2) % 2)
	case buildRunFieldConfiguration:
		if dialog.configuration == "Debug" {
			dialog.configuration = "Release"
		} else {
			dialog.configuration = "Debug"
		}
	case buildRunFieldTarget:
		if len(dialog.targets) > 0 {
			dialog.target = (dialog.target + delta + len(dialog.targets)) % len(dialog.targets)
			dialog.framework = 0
		}
	case buildRunFieldFramework:
		count := len(m.currentBuildRunTarget().Frameworks) + 1
		dialog.framework = (dialog.framework + delta + count) % count
	}
	dialog.errorMessage = ""
}

func (m Model) currentBuildRunTarget() buildRunTarget {
	if m.buildRunDialog.target < 0 || m.buildRunDialog.target >= len(m.buildRunDialog.targets) {
		return buildRunTarget{}
	}
	return m.buildRunDialog.targets[m.buildRunDialog.target]
}

func (m Model) currentBuildRunFramework() string {
	target := m.currentBuildRunTarget()
	index := m.buildRunDialog.framework - 1
	if index < 0 || index >= len(target.Frameworks) {
		return ""
	}
	return target.Frameworks[index]
}

func (m *Model) deleteBuildRunText() {
	dialog := &m.buildRunDialog
	var value *string
	switch dialog.field {
	case buildRunFieldArguments:
		value = &dialog.arguments
	case buildRunFieldEnvironment:
		value = &dialog.environment
	}
	if value != nil {
		runes := []rune(*value)
		if len(runes) > 0 {
			*value = string(runes[:len(runes)-1])
		}
	}
}

func (m *Model) clearBuildRunText() {
	switch m.buildRunDialog.field {
	case buildRunFieldArguments:
		m.buildRunDialog.arguments = ""
	case buildRunFieldEnvironment:
		m.buildRunDialog.environment = ""
	}
}

func (m *Model) startConfiguredBuildRun() tea.Cmd {
	target := m.currentBuildRunTarget()
	if target.Path == "" {
		m.buildRunDialog.errorMessage = "Choose a project or solution target."
		return nil
	}
	if target.MissingCount > 0 {
		m.buildRunDialog.errorMessage = fmt.Sprintf("%s has %d missing project member(s). Fix the solution before running it.", target.Name, target.MissingCount)
		return nil
	}
	if m.selection == nil || m.selection.DotnetPath == "" {
		m.buildRunDialog.errorMessage = "No usable .NET SDK is selected."
		return nil
	}
	arguments, err := parseBuildRunArguments(m.buildRunDialog.arguments)
	if err != nil {
		m.buildRunDialog.errorMessage = "Arguments: " + err.Error()
		return nil
	}
	environment, err := parseBuildRunEnvironment(m.buildRunDialog.environment)
	if err != nil {
		m.buildRunDialog.errorMessage = "Environment: " + err.Error()
		return nil
	}
	environment = mergeEnvironment(m.selection.Environment(nil), environment)
	project := projects.Project{
		Path:             target.Path,
		Name:             target.Name,
		Directory:        target.Directory,
		Kind:             projects.KindProject,
		TargetFrameworks: append([]string(nil), target.Frameworks...),
	}
	if target.Workspace {
		project.Kind = projects.KindSolution
	}
	if project.Directory == "" {
		project.Directory = filepath.Dir(project.Path)
	}

	m.buildRunDialog.open = false
	m.buildRunDialog.errorMessage = ""
	framework := m.currentBuildRunFramework()
	if m.buildRunDialog.mode == buildRunBuild {
		return m.startBuildRequest(project, execution.BuildRequest{
			DotnetPath:       m.selection.DotnetPath,
			ProjectPath:      target.Path,
			WorkingDirectory: project.Directory,
			Configuration:    m.buildRunDialog.configuration,
			Framework:        framework,
			Arguments:        arguments,
			Environment:      environment,
		})
	}
	runArguments, err := execution.RunArgs(target.Path, m.buildRunDialog.configuration, framework, arguments)
	if err != nil {
		m.buildRunDialog.open = true
		m.buildRunDialog.errorMessage = err.Error()
		return nil
	}
	return m.startRunRequest(project, execution.CommandRequest{
		DotnetPath:       m.selection.DotnetPath,
		ProjectPath:      target.Path,
		WorkingDirectory: project.Directory,
		Arguments:        runArguments,
		Environment:      environment,
	})
}

func parseBuildRunArguments(value string) ([]string, error) {
	var arguments []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			arguments = append(arguments, current.String())
			current.Reset()
		}
	}
	for _, character := range value {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			current.WriteRune(character)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	flush()
	return arguments, nil
}

func parseBuildRunEnvironment(value string) ([]string, error) {
	value = strings.ReplaceAll(value, "\n", ";")
	var result []string
	for _, raw := range strings.Split(value, ";") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		separator := strings.IndexByte(entry, '=')
		if separator <= 0 {
			return nil, fmt.Errorf("%q must use KEY=VALUE", entry)
		}
		key := strings.TrimSpace(entry[:separator])
		if key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("invalid variable name %q", key)
		}
		result = append(result, key+entry[separator:])
	}
	return result, nil
}

func mergeEnvironment(base, overrides []string) []string {
	result := append([]string(nil), base...)
	positions := make(map[string]int, len(result))
	for index, entry := range result {
		if separator := strings.IndexByte(entry, '='); separator > 0 {
			positions[entry[:separator]] = index
		}
	}
	for _, entry := range overrides {
		separator := strings.IndexByte(entry, '=')
		if separator <= 0 {
			continue
		}
		key := entry[:separator]
		if index, ok := positions[key]; ok {
			result[index] = entry
		} else {
			positions[key] = len(result)
			result = append(result, entry)
		}
	}
	return result
}

func (m Model) renderBuildRunDialog(width int) string {
	boxWidth := width - 4
	if boxWidth > 92 {
		boxWidth = 92
	}
	if boxWidth < 32 {
		boxWidth = 32
	}
	target := m.currentBuildRunTarget()
	targetLabel := "none"
	if target.Path != "" {
		targetLabel = target.Name
		if target.Workspace {
			targetLabel += "  [solution]"
		}
		if target.MissingCount > 0 {
			targetLabel += fmt.Sprintf("  [missing %d]", target.MissingCount)
		}
	}
	framework := m.currentBuildRunFramework()
	if framework == "" {
		framework = "auto"
	}
	content := []string{
		styleMuted.Render("Choose the target and execution options for this session."),
		"",
		m.buildRunDialogRow("Mode", m.buildRunDialog.mode.label(), buildRunFieldMode),
		m.buildRunDialogRow("Configuration", m.buildRunDialog.configuration, buildRunFieldConfiguration),
		m.buildRunDialogRow("Target", targetLabel, buildRunFieldTarget),
		m.buildRunDialogRow("Framework", framework, buildRunFieldFramework),
		m.buildRunDialogRow("Arguments", valueOr(m.buildRunDialog.arguments, "none"), buildRunFieldArguments),
		m.buildRunDialogRow("Environment", valueOr(m.buildRunDialog.environment, "none"), buildRunFieldEnvironment),
	}
	if m.buildRunDialog.field == buildRunFieldTarget && len(m.buildRunDialog.targets) > 0 {
		content = append(content, "", styleLabel.Render("TARGETS"))
		start := m.buildRunDialog.target - 2
		if start < 0 {
			start = 0
		}
		end := start + 5
		if end > len(m.buildRunDialog.targets) {
			end = len(m.buildRunDialog.targets)
		}
		for index := start; index < end; index++ {
			candidate := m.buildRunDialog.targets[index]
			marker := "  "
			if index == m.buildRunDialog.target {
				marker = styleAccent.Render("> ")
			}
			label := candidate.Name
			if candidate.Workspace {
				label += "  [solution]"
			}
			if candidate.MissingCount > 0 {
				label += fmt.Sprintf("  %s", styleWarning.Render(fmt.Sprintf("[missing %d]", candidate.MissingCount)))
			}
			content = append(content, marker+styleValue.Render(label))
		}
	}
	if m.buildRunDialog.errorMessage != "" {
		content = append(content, "", styleError.Render(m.buildRunDialog.errorMessage))
	}
	content = append(content, "", styleAccent.Render("[ START ]")+"  "+styleMuted.Render("click fields")+"  "+styleFaint.Render("tab/shift-tab · ↑/↓ change"), styleFaint.Render("[ CANCEL ]  esc close"))
	return styleCallout.Width(boxWidth).Render(joinLines(append([]string{styleTitle.Render("BUILD / RUN")}, content...)))
}

func (m Model) buildRunDialogRow(label, value string, field buildRunField) string {
	marker := "  "
	if m.buildRunDialog.field == field {
		marker = styleAccent.Render("> ")
	}
	return marker + styleLabel.Render(fmt.Sprintf("%-14s", label)) + styleValue.Render(value)
}
