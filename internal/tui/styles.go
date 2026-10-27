package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	colorAccent = lipgloss.Color("45")
	colorGreen  = lipgloss.Color("42")
	colorYellow = lipgloss.Color("214")
	colorRed    = lipgloss.Color("196")
	colorMuted  = lipgloss.Color("244")
	colorFaint  = lipgloss.Color("243")
	colorBorder = lipgloss.Color("238")
	colorText   = lipgloss.Color("252")

	styleBrand = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("16")).
			Background(colorAccent).
			Padding(0, 1)
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleFaint    = lipgloss.NewStyle().Foreground(colorFaint)
	styleLabel    = lipgloss.NewStyle().Foreground(colorMuted)
	styleValue    = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleHover    = lipgloss.NewStyle().Foreground(colorAccent).Underline(true)
	styleOK       = lipgloss.NewStyle().Foreground(colorGreen)
	styleInfo     = lipgloss.NewStyle().Foreground(colorAccent)
	styleWarning  = lipgloss.NewStyle().Foreground(colorYellow)
	styleError    = lipgloss.NewStyle().Foreground(colorRed)
	styleKey      = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	styleBorder   = lipgloss.NewStyle().Foreground(colorBorder)
	stylePanel    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)
	styleCallout = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)
	styleHelpBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)
)
