package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

// Backend is the read-only surface the TUI needs.
type Backend interface {
	Version() string
	Platform() platform.Info
	Paths() platform.Paths
	Config() config.Config
	ConfigWarnings() []string
	PackageManager() *platform.PackageManager
	Inventory(ctx context.Context) (dotnet.Inventory, error)
	Resolve(ctx context.Context, inventory dotnet.Inventory) (dotnet.Selection, error)
	Diagnose(ctx context.Context) diagnostics.Report
	Tool(name string) (string, bool)
}

// Screen identifies a main section.
type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenSDKs
	ScreenProjects
	ScreenPackages
	ScreenCreate
	ScreenTemplates
	ScreenDiagnostics
	ScreenSettings
)

type screenInfo struct {
	title string
	short string
}

var screens = []screenInfo{
	{"Dashboard", "Dashboard"},
	{"SDKs & Runtimes", "SDKs"},
	{"Projects & Solutions", "Projects"},
	{"NuGet Packages", "NuGet"},
	{"Create Project", "Create"},
	{"Templates & Architectures", "Templates"},
	{"Diagnostics", "Diagnostics"},
	{"Settings", "Settings"},
}

const (
	sidebarWidth     = 22
	contentInset     = 2
	minTerminalWidth = 40
	minTerminalRows  = 8
)

type inventoryMsg struct {
	inventory    dotnet.Inventory
	selection    *dotnet.Selection
	selectionErr error
	err          error
}

type doctorMsg struct {
	report diagnostics.Report
}

// Model is the root Bubble Tea model.
type Model struct {
	backend Backend

	width  int
	height int

	screen Screen
	cursor int

	loadingInventory bool
	inventory        *dotnet.Inventory
	inventoryErr     error
	selection        *dotnet.Selection
	selectionErr     error

	loadingDoctor bool
	report        *diagnostics.Report

	sdkScroll  int
	diagScroll int

	showHelp bool

	spinner spinner.Model
}

// Now builds the root model.
func Now(backend Backend) Model {
	return Model{
		backend:          backend,
		screen:           ScreenDashboard,
		cursor:           int(ScreenDashboard),
		width:            80,
		height:           24,
		loadingInventory: true,
		spinner:          spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

// Run starts the full-screen program.
func Run(backend Backend) error {
	program := tea.NewProgram(Now(backend))
	_, err := program.Run()
	return err
}

// Init loads the inventory and starts the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.inventoryCmd(), m.spinner.Tick)
}

func (m Model) inventoryCmd() tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		inventory, err := backend.Inventory(ctx)
		if err != nil {
			return inventoryMsg{inventory: inventory, err: err}
		}
		selection, selectionErr := backend.Resolve(ctx, inventory)
		var selected *dotnet.Selection
		if selectionErr == nil {
			selected = &selection
		}
		return inventoryMsg{inventory: inventory, selection: selected, selectionErr: selectionErr}
	}
}

func (m Model) doctorCmd() tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return doctorMsg{report: backend.Diagnose(ctx)}
	}
}

// Update handles messages without blocking on I/O.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		return m, nil

	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		return m, command

	case inventoryMsg:
		m.loadingInventory = false
		if message.err != nil {
			m.inventoryErr = message.err
			m.selection = nil
			m.selectionErr = nil
		} else {
			m.inventoryErr = nil
			inventory := message.inventory
			m.inventory = &inventory
			m.selection = message.selection
			m.selectionErr = message.selectionErr
		}
		m.clampScroll()
		return m, nil

	case doctorMsg:
		m.loadingDoctor = false
		report := message.report
		m.report = &report
		m.clampScroll()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(message)
	}

	return m, nil
}

func (m Model) handleKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.String()

	if m.showHelp {
		switch key {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc", "?":
			m.showHelp = false
		}
		return m, nil
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "r":
		m.loadingInventory = true
		commands := []tea.Cmd{m.inventoryCmd(), m.spinner.Tick}
		if m.screen == ScreenDiagnostics {
			m.loadingDoctor = true
			commands = append(commands, m.doctorCmd())
		}
		return m, tea.Batch(commands...)
	case "d":
		command := m.navigate(ScreenDiagnostics)
		return m, command
	case "1", "2", "3", "4", "5", "6", "7", "8":
		index := int(key[0] - '1')
		command := m.navigate(Screen(index))
		return m, command
	case "tab", "right", "l":
		command := m.navigate(Screen((int(m.screen) + 1) % len(screens)))
		return m, command
	case "shift+tab", "left", "h":
		command := m.navigate(Screen((int(m.screen) - 1 + len(screens)) % len(screens)))
		return m, command
	case "up", "k":
		command := m.navigate(Screen((int(m.screen) - 1 + len(screens)) % len(screens)))
		return m, command
	case "down", "j":
		command := m.navigate(Screen((int(m.screen) + 1) % len(screens)))
		return m, command
	case "enter":
		command := m.navigate(Screen(m.cursor))
		return m, command
	case "pgup", "ctrl+u":
		m.scrollBy(-8)
		return m, nil
	case "pgdown", "ctrl+d":
		m.scrollBy(8)
		return m, nil
	case "home":
		m.setScroll(0)
		return m, nil
	case "end":
		m.setScroll(m.maxScroll())
		return m, nil
	}

	return m, nil
}

func (m *Model) navigate(target Screen) tea.Cmd {
	if target < 0 || int(target) >= len(screens) {
		return nil
	}
	m.screen = target
	m.cursor = int(target)

	if target == ScreenDiagnostics && m.report == nil && !m.loadingDoctor {
		m.loadingDoctor = true
		return m.doctorCmd()
	}
	return nil
}

func (m *Model) scrollBy(delta int) {
	m.setScroll(m.scrollOffset() + delta)
}

func (m Model) scrollOffset() int {
	if m.screen == ScreenDiagnostics {
		return m.diagScroll
	}
	return m.sdkScroll
}

func (m *Model) setScroll(value int) {
	max := m.maxScroll()
	if value < 0 {
		value = 0
	}
	if value > max {
		value = max
	}
	if m.screen == ScreenDiagnostics {
		m.diagScroll = value
	} else {
		m.sdkScroll = value
	}
}

func (m *Model) clampScroll() {
	m.setScroll(m.scrollOffset())
}

func (m Model) maxScroll() int {
	lines := m.screenLines(m.contentWidth())
	visible := m.bodyHeight()
	if len(lines) <= visible {
		return 0
	}
	return len(lines) - visible
}

func (m Model) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	if m.showSidebar() {
		width -= sidebarWidth
	}
	width -= contentInset
	if width < 10 {
		width = 10
	}
	return width
}

func (m Model) showSidebar() bool {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return width >= 64
}

func (m Model) bodyHeight() int {
	height := m.height
	if height <= 0 {
		height = 24
	}
	body := height - 3
	if body < 3 {
		body = 3
	}
	return body
}
