package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	colorAccent = lipgloss.Color("39")
	colorGreen  = lipgloss.Color("42")
	colorYellow = lipgloss.Color("214")
	colorRed    = lipgloss.Color("196")
	colorMuted  = lipgloss.Color("244")

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleLabel    = lipgloss.NewStyle().Foreground(colorMuted)
	styleValue    = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleOK       = lipgloss.NewStyle().Foreground(colorGreen)
	styleInfo     = lipgloss.NewStyle().Foreground(colorAccent)
	styleWarning  = lipgloss.NewStyle().Foreground(colorYellow)
	styleError    = lipgloss.NewStyle().Foreground(colorRed)
	styleHelpBox  = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
)
