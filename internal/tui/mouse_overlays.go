package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mohammedbakri123/hilt/internal/projects"
)

func (m Model) overlaySize() (int, int) {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func (m Model) handleOverlayMouseClick(mouse tea.Mouse) (bool, tea.Model, tea.Cmd) {
	if m.showHelp {
		return true, m, nil
	}
	if m.quitConfirm {
		model, command := m.handleQuitConfirmMouse(mouse)
		return true, model, command
	}
	if m.rootPicker.open {
		model, command := m.handleProjectRootPickerMouse(mouse)
		return true, model, command
	}
	if m.screen == ScreenProjects && m.projectFiltering {
		model, command := m.handleProjectFinderMouseClick(mouse)
		return true, model, command
	}
	if m.createWizard.open {
		model, command := m.handleProjectCreateMouse(mouse)
		return true, model, command
	}
	if m.projectActionMenu {
		model, command := m.handleProjectActionMouse(mouse)
		return true, model, command
	}
	if m.buildRunDialog.open {
		model, command := m.handleBuildRunDialogMouse(mouse)
		return true, model, command
	}
	if m.projectActionPrompt.open {
		model, command := m.handleProjectActionPromptMouse(mouse)
		return true, model, command
	}
	if m.prompt != promptNone || m.confirm != confirmNone {
		model, command := m.handlePromptConfirmMouse(mouse)
		return true, model, command
	}
	return false, m, nil
}

func (m Model) handleDrawerMouseClick(mouse tea.Mouse) (bool, Model) {
	if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseRight {
		return false, m
	}
	start := headerLineCount + m.bodyHeight()
	if mouse.Y < start || mouse.Y >= start+m.drawerHeight() {
		return false, m
	}
	if mouse.Button == tea.MouseRight {
		m.drawer.open = false
		return true, m
	}
	if mouse.Y == start+1 {
		m.drawer.follow = !m.drawer.follow
		if m.drawer.follow {
			m.followDrawer()
		}
	}
	return true, m
}

func (m Model) handleProjectActionMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.projectActionMenu = false
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderProjectActionMenu(width)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	actions := m.projectActionsForCurrent()
	for index, action := range actions {
		if strings.Contains(line, action.label) {
			m.projectActionCursor = index
			m.hoveredProjectAction = index
			m.projectActionMenu = false
			return m, m.startProjectAction()
		}
	}
	return m, nil
}

func (m Model) handleBuildRunDialogMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.buildRunDialog.open = false
		m.setStatus("Build / Run cancelled.", false)
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderBuildRunDialog(width)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ START ]") {
		return m, m.startConfiguredBuildRun()
	}
	if strings.Contains(line, "[ CANCEL ]") {
		m.buildRunDialog.open = false
		m.setStatus("Build / Run cancelled.", false)
		return m, nil
	}
	fields := []struct {
		field buildRunField
		label string
	}{
		{buildRunFieldMode, "Mode"},
		{buildRunFieldConfiguration, "Configuration"},
		{buildRunFieldTarget, "Target"},
		{buildRunFieldFramework, "Framework"},
		{buildRunFieldArguments, "Arguments"},
		{buildRunFieldEnvironment, "Environment"},
	}
	for _, field := range fields {
		if strings.Contains(line, fmt.Sprintf("%-14s", field.label)) {
			m.buildRunDialog.field = field.field
			return m, nil
		}
	}
	if m.buildRunDialog.field == buildRunFieldTarget {
		for index, target := range m.buildRunDialog.targets {
			if strings.Contains(line, target.Name) {
				m.buildRunDialog.target = index
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) handleProjectActionPromptMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.projectActionPrompt.open = false
		m.setStatus("Action cancelled.", false)
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderProjectActionPrompt(width)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ RUN ]") {
		return m, m.submitProjectActionPrompt()
	}
	if strings.Contains(line, "[ CANCEL ]") {
		m.projectActionPrompt.open = false
		m.setStatus("Action cancelled.", false)
		return m, nil
	}
	if strings.Contains(line, "Name") {
		m.projectActionPrompt.field = 0
	} else if strings.Contains(line, "Version") {
		m.projectActionPrompt.field = 1
	}
	return m, nil
}

func (m Model) handleProjectCreateMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.closeProjectCreateWizard()
		m.setStatus("Project creation cancelled.", false)
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderProjectCreateWizard(width)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ CONTINUE ]") {
		return m.handleProjectCreateKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	if strings.Contains(line, "[ CANCEL ]") {
		m.closeProjectCreateWizard()
		m.setStatus("Project creation cancelled.", false)
		return m, nil
	}
	if m.createWizard.stage == createStageTemplate {
		for index, template := range projects.Templates() {
			if strings.Contains(line, template.Name) {
				m.createWizard.templateCursor = index
				return m, nil
			}
		}
	}
	if m.createWizard.stage == createStageSolution {
		modes := []string{"Independent project", "Create new solution", "Add to existing solution"}
		for index, mode := range modes {
			if strings.Contains(line, mode) {
				m.createWizard.solutionMode = index
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) handlePromptConfirmMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if m.confirm != confirmNone && m.screen != ScreenSDKs {
		if mouse.Button == tea.MouseRight {
			m.clearConfirm()
			m.setStatus("Cancelled.", false)
			return m, nil
		}
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		width, height := m.overlaySize()
		box := m.renderConfirmDialog(width)
		bounds := centeredOverlayBounds(box, width, height)
		line := overlayLineAt(box, bounds, mouse.Y)
		if strings.Contains(line, "[ CONFIRM ]") {
			return m, m.confirmAction()
		}
		if strings.Contains(line, "[ CANCEL ]") {
			m.clearConfirm()
			m.setStatus("Cancelled.", false)
		}
		return m, nil
	}
	if m.prompt != promptNone && mouse.Button == tea.MouseRight {
		m.clearPrompt()
		m.setStatus("Cancelled.", false)
	}
	return m, nil
}

func (m Model) handleQuitConfirmMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.quitConfirm = false
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderQuitDialog(width)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ QUIT ]") {
		if m.buildCancel != nil {
			m.buildCancel()
		}
		return m, tea.Quit
	}
	if strings.Contains(line, "[ STAY ]") {
		m.quitConfirm = false
	}
	return m, nil
}

func (m Model) handleProjectRootPickerMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button == tea.MouseRight {
		m.closeProjectRootPicker()
		return m, nil
	}
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.overlaySize()
	box := m.renderProjectRootPicker(width, height)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ CHOOSE ]") {
		return m, m.chooseCurrentPickerRoot()
	}
	if strings.Contains(line, "[ CANCEL ]") {
		m.closeProjectRootPicker()
		return m, nil
	}
	if strings.Contains(line, "[ OPEN ]") {
		rows := m.pickerRows()
		if m.rootPicker.cursor >= 0 && m.rootPicker.cursor < len(rows) {
			return m, m.navigatePicker(rows[m.rootPicker.cursor].path)
		}
		return m, nil
	}
	for index, row := range m.pickerRows() {
		if strings.Contains(line, row.label) {
			m.rootPicker.cursor = index
			m.clampPickerScroll()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) handleProjectFinderButtons(mouse tea.Mouse) (Model, tea.Cmd, bool) {
	width, height := m.overlaySize()
	box := m.renderProjectFinder(width, height)
	bounds := centeredOverlayBounds(box, width, height)
	line := overlayLineAt(box, bounds, mouse.Y)
	if strings.Contains(line, "[ OPEN ]") {
		return m, m.openProjectFromFinder(), true
	}
	if strings.Contains(line, "[ PARENT ]") {
		parent := filepath.Dir(filepath.Clean(m.projectFinderPath))
		if parent != filepath.Clean(m.projectFinderPath) {
			return m, m.navigateProjectFinder(parent), true
		}
		return m, nil, true
	}
	if strings.Contains(line, "[ CLOSE ]") {
		m.projectFiltering = false
		m.projectFilter = ""
		m.projectFinderEntriesCache = nil
		return m, nil, true
	}
	return m, nil, false
}
