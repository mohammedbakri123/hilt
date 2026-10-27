package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	pickerMaxWidth        = 92
	pickerMaxHeight       = 22
	pickerDirectoryPeriod = 15 * time.Second
)

type projectRootPicker struct {
	open    bool
	hidden  bool
	path    string
	entries []string
	cursor  int
	scroll  int
	loading bool
	saving  bool
	loadErr error
	saveErr error
	request int
}

type pickerDirectoriesMsg struct {
	request int
	path    string
	entries []string
	err     error
}

type projectRootSavedMsg struct {
	path string
	err  error
}

type pickerEntry struct {
	label  string
	path   string
	parent bool
}

func (m Model) projectScanRoot() string {
	if m.backend == nil {
		return ""
	}
	root := m.backend.Config().DefaultProjectDir
	if root == "" {
		root = m.backend.Paths().ProjectsRoot
	}
	if root == "" {
		root = m.backend.Paths().HomeDir
	}
	return root
}

// projectWorkspaceRoot is the Hilt-owned workspace used for normal project
// creation and the default Projects scan. External sources are opt-in through
// the import action; they never replace this workspace.
func (m Model) projectWorkspaceRoot() string {
	if m.backend == nil {
		return ""
	}
	root := m.backend.Paths().ProjectsRoot
	if root == "" {
		root = m.backend.Config().DefaultProjectDir
	}
	if root == "" {
		root = m.backend.Paths().HomeDir
	}
	return root
}

func (m Model) activeProjectScanRoot() string {
	if m.projectRootOverride != "" {
		return m.projectRootOverride
	}
	return m.projectWorkspaceRoot()
}

func (m *Model) openProjectRootPicker() tea.Cmd {
	if m.backend == nil {
		return nil
	}
	root := m.projectScanRoot()
	if root == "" {
		root = string(filepath.Separator)
	}
	m.rootPicker = projectRootPicker{
		open:    true,
		path:    root,
		loading: true,
		request: m.rootPicker.request + 1,
	}
	return m.pickerDirectoryCmd()
}

func (m *Model) closeProjectRootPicker() {
	m.rootPicker.request++
	m.rootPicker.open = false
	m.rootPicker.entries = nil
	m.rootPicker.loading = false
	m.rootPicker.saving = false
	m.rootPicker.cursor = 0
	m.rootPicker.scroll = 0
	m.rootPicker.loadErr = nil
	m.rootPicker.saveErr = nil
}

func (m *Model) navigatePicker(path string) tea.Cmd {
	m.rootPicker.request++
	m.rootPicker.path = path
	m.rootPicker.entries = nil
	m.rootPicker.cursor = 0
	m.rootPicker.scroll = 0
	m.rootPicker.loading = true
	m.rootPicker.saving = false
	m.rootPicker.loadErr = nil
	m.rootPicker.saveErr = nil
	return m.pickerDirectoryCmd()
}

func (m Model) pickerDirectoryCmd() tea.Cmd {
	backend := m.backend
	path := m.rootPicker.path
	hidden := m.rootPicker.hidden
	request := m.rootPicker.request
	return func() tea.Msg {
		if backend == nil {
			return pickerDirectoriesMsg{request: request, path: path, err: fmt.Errorf("no backend configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), pickerDirectoryPeriod)
		defer cancel()
		entries, err := backend.ListDirectories(ctx, path, hidden)
		return pickerDirectoriesMsg{request: request, path: path, entries: entries, err: err}
	}
}

func (m Model) saveProjectRootCmd() tea.Cmd {
	backend := m.backend
	path := m.rootPicker.path
	return func() tea.Msg {
		if backend == nil {
			return projectRootSavedMsg{path: path, err: fmt.Errorf("no backend configured")}
		}
		if err := backend.SetDefaultProjectDir(path); err != nil {
			return projectRootSavedMsg{path: path, err: err}
		}
		saved := path
		if current := backend.Config().DefaultProjectDir; current != "" {
			saved = current
		}
		return projectRootSavedMsg{path: saved}
	}
}

func (m Model) handlePickerDirectories(message pickerDirectoriesMsg) Model {
	if !m.rootPicker.open || message.request != m.rootPicker.request {
		return m
	}
	m.rootPicker.loading = false
	if message.err != nil {
		m.rootPicker.loadErr = message.err
		m.rootPicker.entries = nil
		m.rootPicker.cursor = 0
		m.rootPicker.scroll = 0
		return m
	}
	m.rootPicker.loadErr = nil
	m.rootPicker.entries = append([]string(nil), message.entries...)
	rows := m.pickerRows()
	m.rootPicker.cursor = 0
	if len(rows) > 0 && rows[0].parent && len(rows) > 1 {
		m.rootPicker.cursor = 1
	}
	m.rootPicker.scroll = 0
	m.clampPickerScroll()
	return m
}

func (m Model) handleProjectRootSaved(message projectRootSavedMsg) (tea.Model, tea.Cmd) {
	if !m.rootPicker.open {
		return m, nil
	}
	m.rootPicker.saving = false
	if message.err != nil {
		m.rootPicker.saveErr = message.err
		m.setStatus("Could not change the project root.", true)
		return m, nil
	}
	m.closeProjectRootPicker()
	m.projectRootOverride = message.path
	m.projectFilter = ""
	m.projectFiltering = false
	m.projectCursor = 0
	m.projectListScroll = 0
	m.projectDetailScroll = 0
	m.setStatus("Project root: "+message.path+".", false)
	return m, m.startProjectsScan()
}

func (m *Model) pickerRows() []pickerEntry {
	path := filepath.Clean(m.rootPicker.path)
	if path == "" || path == "." {
		return nil
	}
	rows := make([]pickerEntry, 0, len(m.rootPicker.entries)+1)
	if parent := filepath.Dir(path); parent != path {
		rows = append(rows, pickerEntry{label: "..", path: parent, parent: true})
	}
	for _, entry := range m.rootPicker.entries {
		name := filepath.Base(entry)
		rows = append(rows, pickerEntry{label: name + "/", path: entry})
	}
	return rows
}

func (m Model) pickerVisibleRows() int {
	height := m.height
	if height <= 0 {
		height = 24
	}
	if height < 18 {
		return 1
	}
	boxHeight := pickerMaxHeight
	if height-4 < boxHeight {
		boxHeight = height - 4
	}
	rows := boxHeight - 10
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) clampPickerScroll() {
	visible := m.pickerVisibleRows()
	rows := len(m.pickerRows())
	if m.rootPicker.cursor < 0 {
		m.rootPicker.cursor = 0
	}
	if rows == 0 {
		m.rootPicker.cursor = 0
		m.rootPicker.scroll = 0
		return
	}
	if m.rootPicker.cursor >= rows {
		m.rootPicker.cursor = rows - 1
	}
	if m.rootPicker.scroll < 0 {
		m.rootPicker.scroll = 0
	}
	if m.rootPicker.cursor < m.rootPicker.scroll {
		m.rootPicker.scroll = m.rootPicker.cursor
	}
	if m.rootPicker.cursor >= m.rootPicker.scroll+visible {
		m.rootPicker.scroll = m.rootPicker.cursor - visible + 1
	}
	maxScroll := rows - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.rootPicker.scroll > maxScroll {
		m.rootPicker.scroll = maxScroll
	}
}

func (m *Model) movePicker(delta int) {
	if len(m.pickerRows()) == 0 {
		return
	}
	m.rootPicker.cursor += delta
	m.clampPickerScroll()
}

func (m *Model) togglePickerHidden() tea.Cmd {
	m.rootPicker.hidden = !m.rootPicker.hidden
	return m.navigatePicker(m.rootPicker.path)
}

func (m *Model) chooseCurrentPickerRoot() tea.Cmd {
	if m.rootPicker.loading || m.rootPicker.saving {
		return nil
	}
	if m.rootPicker.loadErr != nil {
		m.setStatus("Cannot choose a folder that could not be read.", true)
		return nil
	}
	m.rootPicker.saving = true
	m.rootPicker.saveErr = nil
	return m.saveProjectRootCmd()
}

func (m Model) handleProjectRootPickerKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.String()
	picker := &m.rootPicker
	if picker.saving {
		return m, nil
	}
	switch key {
	case "esc":
		m.closeProjectRootPicker()
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.movePicker(-1)
	case "down", "j":
		m.movePicker(1)
	case "pgup":
		m.movePicker(-m.pickerVisibleRows())
	case "pgdown":
		m.movePicker(m.pickerVisibleRows())
	case "left", "backspace", "h":
		parent := filepath.Dir(filepath.Clean(picker.path))
		if parent != filepath.Clean(picker.path) {
			return m, m.navigatePicker(parent)
		}
	case "enter", "right", "l":
		rows := m.pickerRows()
		if picker.cursor >= 0 && picker.cursor < len(rows) {
			return m, m.navigatePicker(rows[picker.cursor].path)
		}
	case "s":
		return m, m.chooseCurrentPickerRoot()
	case ".":
		return m, m.togglePickerHidden()
	}
	return m, nil
}

func (m Model) renderProjectRootPicker(width, height int) string {
	picker := m.rootPicker
	boxWidth := pickerMaxWidth
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 20 {
		boxWidth = width - 2
	}
	inner := boxWidth - 4
	if inner < 10 {
		inner = 10
	}

	pathLine := styleLabel.Render("CURRENT") + " " + pickerPath(picker.path, inner-8)
	content := []string{
		fitLine(styleTitle.Render("CHOOSE PROJECT ROOT"), inner),
		fitLine(styleMuted.Render("Scan this folder for .NET projects and solutions."), inner),
		"",
		fitLine(pathLine, inner),
		"",
	}

	switch {
	case picker.saving:
		content = append(content, fitLine(m.spinner.View()+" saving project root...", inner))
	case picker.loading:
		content = append(content, fitLine(m.spinner.View()+" reading folders...", inner))
	case picker.saveErr != nil:
		content = append(content, fitLine(styleError.Render("Could not save: "+picker.saveErr.Error()), inner))
	case picker.loadErr != nil:
		content = append(content, fitLine(styleError.Render("Cannot read this folder: "+picker.loadErr.Error()), inner))
	default:
		label := "DIRECTORIES"
		if picker.hidden {
			label += "  " + styleMuted.Render("(hidden shown)")
		}
		content = append(content, fitLine(styleLabel.Render(label), inner))
		rows := m.pickerRows()
		visible := m.pickerVisibleRows()
		start := picker.scroll
		if start > len(rows) {
			start = len(rows)
		}
		if picker.cursor < start {
			start = picker.cursor
		}
		if picker.cursor >= start+visible {
			start = picker.cursor - visible + 1
		}
		if start < 0 {
			start = 0
		}
		end := start + visible
		if end > len(rows) {
			end = len(rows)
		}
		if len(rows) == 0 {
			content = append(content, "  "+styleMuted.Render("no subdirectories"))
		}
		for index := start; index < end; index++ {
			row := rows[index]
			line := "  " + row.label
			if row.parent {
				line = "  " + styleMuted.Render(row.label)
			}
			if index == picker.cursor {
				line = styleAccent.Render("▌ ") + styleSelected.Render(row.label)
			}
			content = append(content, fitLine(line, inner))
		}
	}

	hint := "[ OPEN ]  [ CHOOSE ]  [ CANCEL ]"
	if picker.saving {
		hint = "saving the project root..."
	}
	content = append(content, "", fitLine(styleFaint.Render(hint), inner))
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
}

func pickerPath(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= width {
		return path
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(path)
	result := make([]rune, 0, width)
	for index := len(runes) - 1; index >= 0; index-- {
		candidate := append([]rune{runes[index]}, result...)
		if lipgloss.Width("…"+string(candidate)) > width {
			break
		}
		result = candidate
	}
	return "…" + string(result)
}
