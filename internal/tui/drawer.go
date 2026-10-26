package tui

import (
	"strings"
)

type outputDrawer struct {
	open   bool
	follow bool
	scroll int
	lines  []string
	status string
	meta   []string
}

func newOutputDrawer() outputDrawer {
	return outputDrawer{follow: true}
}

func (m Model) drawerHeight() int {
	if !m.drawer.open {
		return 0
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	height -= 4
	maximum := height - 3
	if maximum < 5 {
		return 0
	}
	target := height * 2 / 5
	if target < 6 {
		target = 6
	}
	if target > 14 {
		target = 14
	}
	if target > maximum {
		target = maximum
	}
	return target
}

func (m Model) drawerBodyHeight() int {
	height := m.drawerHeight() - 4
	if height < 1 {
		return 1
	}
	return height
}

func (m Model) drawerMaxScroll() int {
	maximum := len(m.drawer.lines) - m.drawerBodyHeight()
	if maximum < 0 {
		return 0
	}
	return maximum
}

func (m *Model) appendDrawerLine(line string) {
	m.drawer.lines = append(m.drawer.lines, line)
	const maximum = 2000
	if len(m.drawer.lines) > maximum {
		m.drawer.lines = m.drawer.lines[len(m.drawer.lines)-maximum:]
	}
	if m.drawer.follow {
		m.drawer.scroll = m.drawerMaxScroll()
	}
}

func (m *Model) scrollDrawerBy(delta int) {
	m.drawer.follow = false
	m.drawer.scroll += delta
	m.clampDrawerScroll()
	if m.drawer.scroll == m.drawerMaxScroll() {
		m.drawer.follow = true
	}
}

func (m *Model) setDrawerScroll(value int) {
	m.drawer.follow = false
	m.drawer.scroll = value
	m.clampDrawerScroll()
}

func (m *Model) clampDrawerScroll() {
	maximum := m.drawerMaxScroll()
	if m.drawer.scroll < 0 {
		m.drawer.scroll = 0
	}
	if m.drawer.scroll > maximum {
		m.drawer.scroll = maximum
	}
}

func (m *Model) followDrawer() {
	m.drawer.follow = true
	m.drawer.scroll = m.drawerMaxScroll()
}

func (m Model) drawerOffset() int {
	if m.drawer.follow {
		return m.drawerMaxScroll()
	}
	offset := m.drawer.scroll
	if offset < 0 {
		return 0
	}
	if offset > m.drawerMaxScroll() {
		return m.drawerMaxScroll()
	}
	return offset
}

func (m Model) drawerStatusLabel() string {
	if m.drawer.status != "" {
		return m.drawer.status
	}
	return "no build yet"
}

func (m Model) renderDrawer(width int) []string {
	height := m.drawerHeight()
	if height == 0 {
		return nil
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	follow := "paused"
	if m.drawer.follow {
		follow = "follow"
	}
	title := " " + styleLabel.Render("OUTPUT") +
		"  " + styleMuted.Render("["+follow+"]") +
		"  " + styleMuted.Render("· "+m.drawerStatusLabel())
	lines := []string{
		styleBorder.Render(strings.Repeat("─", width)),
		fitLine(title, width),
	}
	meta := m.drawer.meta
	if len(meta) == 0 {
		meta = []string{"No output has been captured in this session."}
	}
	metaLine := meta[0]
	if len(meta) > 1 {
		metaLine += "  ·  " + strings.Join(meta[1:], "  ·  ")
	}
	lines = append(lines, fitLine(" "+styleMuted.Render(metaLine), width))
	lines = append(lines, styleBorder.Render(strings.Repeat("─", width)))

	body := m.drawer.lines
	if len(body) == 0 {
		body = []string{"No build output yet. Select a project and press b to build."}
	}
	window := windowLines(body, inner, height-4, m.drawerOffset())
	for _, line := range window {
		lines = append(lines, " "+line+" ")
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	for index, line := range lines {
		lines[index] = fitLine(line, width)
	}
	return lines
}
