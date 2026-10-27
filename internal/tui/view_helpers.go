package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

// ── Layout helpers ────────────────────────────────────────────────────────────

func windowLines(lines []string, width, height, offset int) []string {
	if height < 0 {
		height = 0
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	result := make([]string, 0, height)
	for index := 0; index < height; index++ {
		source := offset + index
		line := ""
		if source < len(lines) {
			line = lines[source]
		}
		result = append(result, fitLine(line, width))
	}
	return result
}

func fitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "…")
	if padding := width - lipgloss.Width(line); padding > 0 {
		line += strings.Repeat(" ", padding)
	}
	return line
}

func wrap(text string, width int) []string {
	if width < 10 {
		width = 10
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func styledWrapped(text string, width int, style lipgloss.Style) []string {
	lines := make([]string, 0, 1)
	for _, line := range wrap(text, width) {
		lines = append(lines, style.Render(line))
	}
	return lines
}

func sectionHeader(title string, width int) []string {
	ruleWidth := width
	if ruleWidth > 48 {
		ruleWidth = 48
	}
	if ruleWidth < 4 {
		ruleWidth = 4
	}
	return []string{
		styleTitle.Render(title),
		styleMuted.Render(strings.Repeat("─", ruleWidth)),
		"",
	}
}

func kv(label, value string) string {
	return kvSized(label, value, 22)
}

func kvSized(label, value string, labelWidth int) string {
	padding := labelWidth - lipgloss.Width(label)
	if padding < 1 {
		padding = 1
	}
	return styleLabel.Render(label) + strings.Repeat(" ", padding) + value
}

func joinOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// ── Panel primitives ──────────────────────────────────────────────────────────

type panelSpec struct {
	title   string
	content []string
}

type overlayBounds struct {
	left   int
	top    int
	width  int
	height int
}

func centeredOverlayBounds(box string, width, height int) overlayBounds {
	boxWidth := lipgloss.Width(box)
	boxHeight := len(strings.Split(box, "\n"))
	left := (width - boxWidth) / 2
	top := (height - boxHeight) / 2
	if left < 0 {
		left = 0
	}
	if top < 0 {
		top = 0
	}
	return overlayBounds{left: left, top: top, width: boxWidth, height: boxHeight}
}

func overlayLineAt(box string, bounds overlayBounds, y int) string {
	line := y - bounds.top
	lines := strings.Split(box, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return ansi.Strip(lines[line])
}

func renderPanels(specs []panelSpec, width int) []string {
	if len(specs) == 0 {
		return nil
	}
	if len(specs) == 2 && width >= 84 {
		gap := 1
		panelWidth := (width - gap) / 2
		height := 0
		for _, spec := range specs {
			if len(spec.content) > height {
				height = len(spec.content)
			}
		}
		panels := make([][]string, len(specs))
		for index, spec := range specs {
			content := append([]string(nil), spec.content...)
			for len(content) < height {
				content = append(content, "")
			}
			panels[index] = panelBox(spec.title, content, panelWidth)
		}
		lines := make([]string, 0, len(panels[0]))
		for index := range panels[0] {
			line := panels[0][index] + strings.Repeat(" ", gap) + panels[1][index]
			lines = append(lines, line)
		}
		return lines
	}
	var lines []string
	for index, spec := range specs {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, panelBox(spec.title, spec.content, width)...)
	}
	return lines
}

func panelBox(title string, content []string, width int) []string {
	inner := width - 4
	if inner < 8 {
		inner = 8
	}
	body := []string{styleLabel.Render(title)}
	for _, line := range content {
		body = append(body, fitLine(line, inner))
	}
	box := stylePanel.Width(width).Render(strings.Join(body, "\n"))
	return strings.Split(box, "\n")
}

func calloutBox(title string, content []string, width int) []string {
	inner := width - 4
	if inner < 8 {
		inner = 8
	}
	body := []string{styleTitle.Render(title)}
	for _, line := range content {
		body = append(body, fitLine(line, inner))
	}
	box := styleCallout.Width(width).Render(strings.Join(body, "\n"))
	return strings.Split(box, "\n")
}

func renderTooSmall(width, height int) string {
	lines := []string{
		styleTitle.Render("HILT"),
		"",
		fmt.Sprintf("Terminal too small (%dx%d).", width, height),
		fmt.Sprintf("Hilt needs at least %dx%d.", minTerminalWidth, minTerminalRows),
		"",
		"Resize the terminal, then press q to quit.",
	}
	for index, line := range lines {
		lines[index] = fitLine(line, width)
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func overlayCentered(base, box string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	boxLines := strings.Split(box, "\n")
	left := (width - lipgloss.Width(box)) / 2
	if left < 0 {
		left = 0
	}
	top := (height - len(boxLines)) / 2
	if top < 0 {
		top = 0
	}
	for index, line := range boxLines {
		target := top + index
		if target >= len(baseLines) {
			break
		}
		baseLines[target] = overlayLine(baseLines[target], line, left)
	}
	return strings.Join(baseLines, "\n")
}

func overlayLine(base, overlay string, left int) string {
	before := ansi.Truncate(base, left, "")
	after := ansi.TruncateLeft(base, left+lipgloss.Width(overlay), "")
	return before + overlay + after
}

// ── Provenance helpers ────────────────────────────────────────────────────────

func provenanceText(provenance dotnet.Provenance) string {
	switch provenance {
	case dotnet.ProvenanceManaged:
		return styleOK.Render(string(provenance))
	case dotnet.ProvenanceExternal:
		return styleWarning.Render(string(provenance))
	default:
		return styleMuted.Render(string(provenance))
	}
}

func provenanceCell(provenance dotnet.Provenance) string {
	label := string(provenance)
	style := styleMuted
	switch provenance {
	case dotnet.ProvenanceManaged:
		style = styleOK
	case dotnet.ProvenanceExternal:
		style = styleWarning
	}
	return style.Render(fmt.Sprintf("%-10s", label))
}

// ── Warnings ──────────────────────────────────────────────────────────────────

func (m Model) warnings() []string {
	if m.backend == nil {
		return nil
	}
	warnings := append([]string(nil), m.backend.ConfigWarnings()...)
	if m.inventoryErr != nil {
		warnings = append(warnings, m.inventoryErr.Error())
	}
	if m.inventory != nil {
		warnings = append(warnings, m.inventory.Warnings...)
	}
	if m.selectionErr != nil {
		warnings = append(warnings, m.selectionErr.Error())
	}
	return dedupeStrings(warnings)
}
