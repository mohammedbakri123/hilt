package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

const projectFinderMaxRows = 8

type projectFinderEntry struct {
	name        string
	path        string
	directory   bool
	project     projects.Project
	hasProject  bool
	workspace   projects.Workspace
	hasSolution bool
}

type projectFinderMsg struct {
	request     int
	path        string
	directories []string
	result      projects.Result
	err         error
}

func (m Model) projectFinderVisibleRows(height int) int {
	rows := height - 18
	if rows > projectFinderMaxRows {
		rows = projectFinderMaxRows
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m Model) projectFinderEntries() []projectFinderEntry {
	if m.projectResult == nil {
		return nil
	}
	entries := m.projectFinderEntriesCache
	if entries == nil {
		entries = m.projectFinderEntriesForPath(m.projectFinderPath)
	}
	query := strings.TrimSpace(strings.ToLower(m.projectFilter))
	if query == "" {
		return entries
	}
	filtered := make([]projectFinderEntry, 0, len(entries))
	for _, entry := range entries {
		haystack := strings.ToLower(strings.Join([]string{entry.name, entry.path}, " "))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m Model) projectFinderEntriesForPath(path string) []projectFinderEntry {
	if m.projectResult == nil || path == "" {
		return nil
	}
	return projectFinderEntriesForResult(path, *m.projectResult)
}

func projectFinderEntriesForResult(path string, scanResult projects.Result) []projectFinderEntry {
	if path == "" {
		return nil
	}
	root := filepath.Clean(path)
	entries := make(map[string]projectFinderEntry)
	addDirectory := func(directory string) {
		directory = filepath.Clean(directory)
		relative, err := filepath.Rel(root, directory)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return
		}
		first := strings.Split(relative, string(filepath.Separator))[0]
		child := filepath.Join(root, first)
		entries[child] = projectFinderEntry{name: first, path: child, directory: true}
	}
	for _, workspace := range scanResult.Workspaces {
		addDirectory(workspace.Directory)
		if filepath.Clean(workspace.Directory) == root {
			entry := projectFinderEntry{name: workspace.Name, path: workspace.Path, workspace: workspace, hasSolution: true}
			entries[workspace.Path] = entry
		}
	}
	for _, project := range scanResult.Projects {
		addDirectory(project.Directory)
		if filepath.Clean(project.Directory) == root {
			entry := projectFinderEntry{name: project.Name, path: project.Path, project: project, hasProject: true}
			entries[project.Path] = entry
		}
	}
	result := make([]projectFinderEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].directory != result[j].directory {
			return result[i].directory
		}
		return strings.ToLower(result[i].name) < strings.ToLower(result[j].name)
	})
	return result
}

func (m Model) projectFinderLayout(width, height int) (boxWidth, top, left, rowsTop, visibleRows int) {
	boxWidth = width - 8
	if boxWidth > 100 {
		boxWidth = 100
	}
	if boxWidth < 32 {
		boxWidth = 32
	}
	visibleRows = m.projectFinderVisibleRows(height)
	entries := m.projectFinderEntries()
	displayRows := len(entries)
	if displayRows == 0 {
		displayRows = 1
	}
	if displayRows > visibleRows {
		displayRows = visibleRows
	}
	boxHeight := displayRows + 10
	top = (height - boxHeight) / 2
	if top < 0 {
		top = 0
	}
	left = (width - boxWidth) / 2
	if left < 0 {
		left = 0
	}
	rowsTop = top + 7
	return boxWidth, top, left, rowsTop, visibleRows
}

func (m Model) renderProjectFinder(width, height int) string {
	boxWidth, _, _, _, visibleRows := m.projectFinderLayout(width, height)
	entries := m.projectFinderEntries()
	innerWidth := boxWidth - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	filter := m.projectFilter
	filterLine := styleLabel.Render("filter") + "  "
	if filter == "" {
		filterLine += styleFaint.Render("type to filter this folder")
	} else {
		filterLine += styleValue.Render(filter)
	}
	filterLine += styleAccent.Render("▌")
	content := []string{
		styleTitle.Render("OPEN PROJECT OR SOLUTION"),
		styleMuted.Render("Browse folders and choose a .NET project or solution."),
		fitLine(styleLabel.Render("path")+"  "+pickerPath(m.projectFinderPath, innerWidth-8), innerWidth),
		fitLine(filterLine, innerWidth),
		finderSummary(m, len(entries)),
		"",
	}
	if len(entries) == 0 {
		content = append(content, styleMuted.Render("No folders or supported project files here."))
	} else {
		start := 0
		if m.projectFinderCursor >= visibleRows {
			start = m.projectFinderCursor - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(entries) {
			end = len(entries)
		}
		for index := start; index < end; index++ {
			entry := entries[index]
			marker := "  "
			nameStyle := styleValue
			kind := ""
			suffix := ""
			if entry.directory {
				marker = "▸ "
				kind = "folder"
			} else if entry.hasSolution {
				kind = string(entry.workspace.Kind)
			} else if entry.hasProject {
				kind = string(entry.project.Kind)
				if entry.project.TestProject {
					suffix = " [test]"
				}
			}
			if index == m.projectFinderCursor {
				marker = "▌ "
				nameStyle = styleSelected
			}
			row := marker + nameStyle.Render(entry.name) + suffix
			if kind != "" {
				row += "  " + styleMuted.Render("["+kind+"]")
			}
			row += "  " + styleFaint.Render(pickerPath(entry.path, innerWidth-20))
			content = append(content, fitLine(row, innerWidth))
		}
	}
	content = append(content, "", styleKey.Render("enter")+" open   "+styleKey.Render("backspace")+" parent   "+styleKey.Render("esc")+" close")
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
}

func (m Model) projectFinderIndexAt(x, y, width, height int) (int, bool) {
	boxWidth, _, left, rowsTop, visibleRows := m.projectFinderLayout(width, height)
	if x < left || x >= left+boxWidth {
		return 0, false
	}
	entries := m.projectFinderEntries()
	if len(entries) == 0 || y < rowsTop {
		return 0, false
	}
	index := y - rowsTop
	start := 0
	if m.projectFinderCursor >= visibleRows {
		start = m.projectFinderCursor - visibleRows + 1
	}
	index += start
	if index < 0 || index >= len(entries) || index >= start+visibleRows {
		return 0, false
	}
	return index, true
}

func (m *Model) startProjectFinder() tea.Cmd {
	m.lastProjectClickPath = ""
	m.lastProjectClickAt = time.Time{}
	m.projectFinderOrigin = ""
	if selected, ok := m.selectedProject(); ok {
		m.projectFinderOrigin = selected.Path
	}
	m.projectFiltering = true
	m.projectFinderPath = m.projectWorkspaceRoot()
	if m.projectFinderPath == "" && m.projectResult != nil {
		m.projectFinderPath = m.projectResult.Root
	}
	m.setProjectFilter("")
	m.projectFinderCursor = 0
	m.projectFinderScroll = 0
	m.projectFinderEntriesCache = nil
	m.projectFinderLoading = true
	m.projectFinderError = nil
	m.projectFinderRequest++
	m.setStatus("", false)
	return m.projectFinderCmd()
}

func (m Model) projectFinderCmd() tea.Cmd {
	backend := m.backend
	path := m.projectFinderPath
	request := m.projectFinderRequest
	return func() tea.Msg {
		if backend == nil {
			return projectFinderMsg{request: request, path: path, err: fmt.Errorf("no backend configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), projectsScanTimeout)
		defer cancel()
		directories, err := backend.ListDirectories(ctx, path, false)
		if err != nil {
			return projectFinderMsg{request: request, path: path, err: err}
		}
		result, err := backend.ScanProjects(ctx, path, false)
		return projectFinderMsg{request: request, path: path, directories: directories, result: result, err: err}
	}
}

func (m Model) handleProjectFinderMsg(message projectFinderMsg) Model {
	if !m.projectFiltering || message.request != m.projectFinderRequest || message.path != m.projectFinderPath {
		return m
	}
	m.projectFinderLoading = false
	if message.err != nil {
		m.projectFinderError = message.err
		return m
	}
	m.projectFinderError = nil
	entries := projectFinderEntriesForResult(m.projectFinderPath, message.result)
	for _, directory := range message.directories {
		path := filepath.Clean(directory)
		found := false
		for _, entry := range entries {
			if entry.path == path {
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, projectFinderEntry{name: filepath.Base(path), path: path, directory: true})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].directory != entries[j].directory {
			return entries[i].directory
		}
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
	m.projectFinderEntriesCache = entries
	m.projectFinderCursor = clampIndex(m.projectFinderCursor, len(m.projectFinderEntries()))
	return m
}

func (m *Model) navigateProjectFinder(path string) tea.Cmd {
	m.projectFinderPath = filepath.Clean(path)
	m.projectFilter = ""
	m.projectFinderEntriesCache = nil
	m.projectFinderCursor = 0
	m.projectFinderScroll = 0
	m.projectFinderLoading = true
	m.projectFinderError = nil
	m.projectFinderRequest++
	return m.projectFinderCmd()
}

func (m *Model) openProjectFromFinder() tea.Cmd {
	entries := m.projectFinderEntries()
	if len(entries) == 0 {
		m.setStatus("This folder has no supported project or solution files.", true)
		return nil
	}
	entry := entries[clampIndex(m.projectFinderCursor, len(entries))]
	if entry.directory {
		return m.navigateProjectFinder(entry.path)
	}
	m.projectFiltering = false
	m.projectFilter = ""
	m.projectFinderEntriesCache = nil
	m.projectFinderError = nil
	m.projectFinderLoading = false
	m.projectFinderOrigin = ""
	if entry.hasProject {
		m.restoreProjectSelection(entry.project.Path)
		return m.openProjectDetail(entry.project)
	}
	if entry.hasSolution {
		return m.openWorkspaceDetail(entry.workspace)
	}
	return nil
}

func (m Model) handleProjectFinderMouseClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	index, ok := m.projectFinderIndexAt(mouse.X, mouse.Y, width, height)
	if !ok {
		return m, nil
	}
	entries := m.projectFinderEntries()
	entry := entries[index]
	m.projectFinderCursor = index
	return m, m.projectFinderClickCommand(entry)
}

func (m *Model) projectFinderClickCommand(entry projectFinderEntry) tea.Cmd {
	now := time.Now()
	if m.lastProjectClickPath == entry.path && !m.lastProjectClickAt.IsZero() && now.Sub(m.lastProjectClickAt) <= 450*time.Millisecond {
		m.lastProjectClickPath = ""
		m.lastProjectClickAt = time.Time{}
		if entry.directory {
			return m.navigateProjectFinder(entry.path)
		}
		return m.openProjectFromFinder()
	}
	m.lastProjectClickPath = entry.path
	m.lastProjectClickAt = now
	return nil
}

func itemWord(count int) string {
	if count == 1 {
		return "item"
	}
	return "items"
}

func finderSummary(m Model, count int) string {
	if m.projectFinderLoading {
		return styleMuted.Render(fmt.Sprintf("%d %s  ·  %s reading folder...", count, itemWord(count), m.spinner.View()))
	}
	if m.projectFinderError != nil {
		return styleError.Render("Could not read folder: " + m.projectFinderError.Error())
	}
	return styleMuted.Render(fmt.Sprintf("%d %s", count, itemWord(count)))
}
