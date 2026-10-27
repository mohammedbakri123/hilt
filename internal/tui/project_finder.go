package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const projectFinderMaxRows = 8

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

func (m Model) projectFinderLayout(width, height int) (boxWidth, top, left, rowsTop, visibleRows int) {
	boxWidth = width - 8
	if boxWidth > 100 {
		boxWidth = 100
	}
	if boxWidth < 32 {
		boxWidth = 32
	}
	visibleRows = m.projectFinderVisibleRows(height)
	results := m.filteredProjects()
	displayRows := len(results)
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
	results := m.filteredProjects()
	innerWidth := boxWidth - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	query := m.projectFilter
	input := styleLabel.Render("find") + "  "
	if query == "" {
		input += styleFaint.Render("type a name, path, framework, or SDK")
	} else {
		input += styleValue.Render(query)
	}
	input += styleAccent.Render("▌")

	content := []string{
		styleTitle.Render("FIND & OPEN PROJECT"),
		styleMuted.Render("Search the projects already discovered in your workspace."),
		"",
		fitLine(input, innerWidth),
		styleMuted.Render(fmt.Sprintf("%d match%s", len(results), pluralSuffix(len(results)))),
		"",
	}
	if len(results) == 0 {
		content = append(content, styleMuted.Render("No matches. Try a project name, path, framework, or SDK."))
	} else {
		start := 0
		if m.projectFinderCursor >= visibleRows {
			start = m.projectFinderCursor - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(results) {
			end = len(results)
		}
		for index := start; index < end; index++ {
			project := results[index]
			marker := "  "
			nameStyle := styleValue
			if index == m.projectFinderCursor {
				marker = "▌ "
				nameStyle = styleSelected
			}
			kind := string(project.Kind)
			if project.TestProject {
				kind = "test"
			}
			row := marker + nameStyle.Render(project.Name) + "  " + styleMuted.Render("["+kind+"]")
			row += "  " + styleFaint.Render(pickerPath(project.Path, innerWidth-20))
			content = append(content, fitLine(row, innerWidth))
		}
	}
	content = append(content, "", styleKey.Render("↑/↓")+" select   "+styleKey.Render("enter")+" open   "+styleKey.Render("esc")+" close")
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "es"
}

func (m Model) projectFinderIndexAt(x, y, width, height int) (int, bool) {
	_, _, left, rowsTop, visibleRows := m.projectFinderLayout(width, height)
	if x < left || x >= left+lipgloss.Width(m.renderProjectFinder(width, height)) {
		return 0, false
	}
	results := m.filteredProjects()
	if len(results) == 0 || y < rowsTop {
		return 0, false
	}
	index := y - rowsTop
	start := 0
	if m.projectFinderCursor >= visibleRows {
		start = m.projectFinderCursor - visibleRows + 1
	}
	index += start
	if index < 0 || index >= len(results) || index >= start+visibleRows {
		return 0, false
	}
	return index, true
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
	results := m.filteredProjects()
	m.projectFinderCursor = index
	m.projectCursor = index
	return m, m.projectClickCommand(results[index])
}
