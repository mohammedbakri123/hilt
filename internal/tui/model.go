package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

// Backend is the data and managed-operation surface the TUI needs.
type Backend interface {
	Version() string
	Platform() platform.Info
	Paths() platform.Paths
	Config() config.Config
	ConfigWarnings() []string
	State() state.State
	RecordProjectOpen(projectPath, workspacePath string) error
	PinExternalProject(path string) error
	UnpinExternalProject(path string) error
	PackageManager() *platform.PackageManager
	Inventory(ctx context.Context) (dotnet.Inventory, error)
	Resolve(ctx context.Context, inventory dotnet.Inventory) (dotnet.Selection, error)
	Diagnose(ctx context.Context) diagnostics.Report
	Tool(name string) (string, bool)

	PlanSDKInstall(ctx context.Context, version string, allowPrerelease bool) (install.Plan, error)
	InstallSDK(ctx context.Context, plan install.Plan, progress io.Writer) (install.Result, error)
	RemoveSDK(ctx context.Context, version string) (install.Removal, error)
	UpdateSDK(ctx context.Context, version string, removeOld bool, progress io.Writer) (install.UpdateResult, error)
	CheckSDKUpdates(ctx context.Context) ([]install.Update, error)
	AvailableSDKs(ctx context.Context, allowPrerelease bool) ([]install.AvailableSDK, error)

	PlanRuntimeInstall(ctx context.Context, version string, framework install.FrameworkFlavor, allowPrerelease bool) (install.RuntimePlan, error)
	InstallRuntime(ctx context.Context, plan install.RuntimePlan, progress io.Writer) (install.RuntimeResult, error)
	RemoveRuntime(ctx context.Context, version string) (install.Removal, error)
	CheckRuntimeUpdates(ctx context.Context) ([]install.Update, error)
	ScanProjects(ctx context.Context, root string, includeHidden bool) (projects.Result, error)
	ListDirectories(ctx context.Context, path string, includeHidden bool) ([]string, error)
	SetDefaultProjectDir(path string) error
	BuildProject(ctx context.Context, request execution.BuildRequest, stdout, stderr io.Writer) (execution.Result, error)
	RecordBuildTarget(target state.BuildTarget) error
	CreateProjects(ctx context.Context, plan projects.CreatePlan, stdout, stderr io.Writer) error
	DeleteProject(path string) error
	ProjectTree(ctx context.Context, path string) ([]projects.TreeEntry, error)
	RunProjectCommand(ctx context.Context, request execution.CommandRequest, stdout, stderr io.Writer) (execution.Result, error)
}

// Screen identifies a main section.
type Screen int

const (
	ScreenHome Screen = iota
	ScreenProjects
	ScreenSDKs
	ScreenDiagnostics
	ScreenSettings
)

type screenInfo struct {
	title string
	short string
}

var screens = []screenInfo{
	{"Home", "Home"},
	{"Projects & Solutions", "Projects"},
	{"SDKs & Runtimes", "SDKs"},
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

type projectOpenedMsg struct {
	project projects.Project
	err     error
}

type projectImportMsg struct {
	path string
	err  error
}

type projectUnpinMsg struct {
	path string
	err  error
}

type projectDeleteMsg struct {
	project projects.Project
	err     error
}

type projectTreeMsg struct {
	request int
	path    string
	entries []projects.TreeEntry
	err     error
}

type createStage int

const (
	createStageTemplate createStage = iota
	createStageName
	createStageDestination
	createStageSolution
)

type projectCreateWizard struct {
	open           bool
	stage          createStage
	templateCursor int
	name           string
	destination    string
	solutionPath   string
	solutionMode   int // 0 independent, 1 new solution, 2 existing solution
	errorMessage   string
}

type projectAction struct {
	label     string
	args      []string
	workspace bool
}

var projectActions = []projectAction{
	{label: "Build", args: []string{"build"}},
	{label: "Clean", args: []string{"clean"}},
	{label: "Restore", args: []string{"restore"}},
	{label: "Test", args: []string{"test"}},
	{label: "Run", args: []string{"run"}},
	{label: "Build solution", args: []string{"build"}, workspace: true},
	{label: "Test solution", args: []string{"test"}, workspace: true},
}

type projectsMsg struct {
	request int
	result  projects.Result
	err     error
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

	showHelp    bool
	quitConfirm bool

	sdkFocus      focusArea
	sdkCursor     int
	runtimeCursor int

	prompt           promptKind
	promptInput      string
	promptFocusInput bool
	promptFramework  install.FrameworkFlavor

	availableSDKs    []install.AvailableSDK
	availableCursor  int
	availableLoading bool
	availableErr     error
	availableCancel  context.CancelFunc
	availableRequest int

	confirm        confirmKind
	confirmSummary []string

	pendingPlan          *install.Plan
	pendingRuntimePlan   *install.RuntimePlan
	pendingRemove        *removeTarget
	pendingUpdate        *install.Update
	pendingFramework     install.FrameworkFlavor
	pendingCreatePlan    *projects.CreatePlan
	pendingDeleteProject *projects.Project

	operationRunning    bool
	operationCancelling bool
	operationLabel      string
	operationCancel     context.CancelFunc
	operationErr        error
	progressCh          chan tea.Msg
	progressLines       []string
	progressDone        int64
	progressTotal       int64

	projectsLoading        bool
	projectsLoaded         bool
	projectResult          *projects.Result
	projectsErr            error
	projectCursor          int
	projectScroll          int
	projectListScroll      int
	projectDetailScroll    int
	projectFocus           projectFocus
	projectTab             projectTab
	projectExpanded        map[string]bool
	projectFilter          string
	projectFiltering       bool
	projectFinderCursor    int
	projectFinderOrigin    string
	projectDetailOpen      bool
	projectDetailPath      string
	projectDetailWorkspace bool
	projectCancel          context.CancelFunc
	projectRequest         int
	projectTreeRequest     int
	projectTreePath        string
	projectTree            []projects.TreeEntry
	projectTreeLoading     bool
	projectTreeErr         error
	projectTreeCancel      context.CancelFunc
	lastProjectClickPath   string
	lastProjectClickAt     time.Time
	createWizard           projectCreateWizard
	projectActionMenu      bool
	projectActionCursor    int

	rootPicker projectRootPicker
	// projectRootOverride is session-local and is set only after the user
	// explicitly chooses an alternate scan root. A saved legacy config value
	// cannot silently broaden the default workspace on startup.
	projectRootOverride string

	updates        []install.Update
	runtimeUpdates []install.Update
	loadingUpdates bool
	updatesLoaded  bool
	updatesErr     error
	updateCancel   context.CancelFunc

	statusMessage string
	statusIsError bool

	drawer outputDrawer

	buildRun        int
	buildRunning    bool
	buildCancelling bool
	buildCancel     context.CancelFunc
	buildCh         chan tea.Msg
	buildTarget     projects.Project
	hasBuildTarget  bool
	buildResult     *execution.Result
	buildErr        error

	spinner spinner.Model
}

// Now builds the root model.
func Now(backend Backend) Model {
	return Model{
		backend:          backend,
		screen:           ScreenHome,
		cursor:           int(ScreenHome),
		width:            80,
		height:           24,
		loadingInventory: true,
		drawer:           newOutputDrawer(),
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
		m.clampSelection()
		m.clampAvailableCursor()
		m.clampScroll()
		return m, nil

	case availableSDKsMsg:
		if message.request != m.availableRequest || m.prompt != promptInstallSDK {
			return m, nil
		}
		m.cancelAvailableSDKs()
		m.availableLoading = false
		if message.err != nil {
			if errors.Is(message.err, context.Canceled) {
				return m, nil
			}
			m.availableErr = message.err
			return m, nil
		}
		m.availableErr = nil
		m.availableSDKs = message.sdks
		m.availableCursor = 0
		m.clampAvailableCursor()
		return m, nil

	case sdkPlanMsg:
		m.finishOperation()
		if message.err != nil {
			m.operationErr = message.err
			m.setStatus(message.err.Error(), true)
			return m, nil
		}
		plan := message.plan
		m.pendingPlan = &plan
		m.confirm = confirmInstallSDK
		m.confirmSummary = sdkPlanSummary(plan)
		return m, nil

	case runtimePlanMsg:
		m.finishOperation()
		if message.err != nil {
			m.operationErr = message.err
			m.setStatus(message.err.Error(), true)
			return m, nil
		}
		plan := message.plan
		m.pendingRuntimePlan = &plan
		m.confirm = confirmInstallRuntime
		m.confirmSummary = runtimePlanSummary(plan)
		return m, nil

	case progressMsg:
		if message.hasProgress {
			m.progressDone = message.downloaded
			m.progressTotal = message.total
		} else {
			if strings.HasPrefix(message.line, "Downloading ") || strings.HasPrefix(message.line, "Retrying ") {
				m.progressDone = 0
				m.progressTotal = 0
			}
			m.progressLines = append(m.progressLines, message.line)
			if len(m.progressLines) > 200 {
				m.progressLines = m.progressLines[len(m.progressLines)-200:]
			}
		}
		if m.progressCh == nil {
			return m, nil
		}
		return m, waitForProgress(m.progressCh)

	case operationDoneMsg:
		m.finishOperation()
		if message.err != nil {
			if errors.Is(message.err, context.Canceled) {
				m.setStatus("Operation cancelled.", false)
				return m, nil
			}
			m.operationErr = message.err
			m.setStatus(message.err.Error(), true)
			return m, nil
		}
		if message.kind == opCreateProjects {
			m.setStatus("Project creation completed.", false)
			return m, m.startProjectsScan()
		}
		if message.kind == opProjectCommand {
			m.setStatus(message.command+" completed.", false)
			return m, nil
		}
		m.updates = nil
		m.runtimeUpdates = nil
		m.updatesLoaded = false
		switch message.kind {
		case opInstallSDK:
			if message.sdk.AlreadyInstalled {
				m.setStatus(fmt.Sprintf("SDK %s is already installed in the managed root.", message.sdk.Version), false)
			} else {
				m.setStatus(fmt.Sprintf("Installed and verified managed SDK %s.", message.sdk.Version), false)
			}
		case opInstallRuntime:
			if message.runtime.AlreadyInstalled {
				m.setStatus(fmt.Sprintf("Runtime %s is already installed in the managed root.", message.runtime.Version), false)
			} else {
				m.setStatus(fmt.Sprintf("Installed and verified managed runtime %s (%s).", message.runtime.Version, message.runtime.Framework), false)
			}
		case opRemoveSDK:
			m.setStatus(fmt.Sprintf("Removed managed SDK %s.", message.removal.Version), false)
		case opRemoveRuntime:
			m.setStatus(fmt.Sprintf("Removed managed runtime %s.", message.removal.Version), false)
		case opUpdateSDK:
			if !message.update.UpdateAvailable {
				m.setStatus(fmt.Sprintf("SDK %s is already up to date.", message.update.From), false)
			} else {
				status := fmt.Sprintf("Updated SDK %s to %s.", message.update.From, message.update.To)
				if message.update.OldRetained {
					status += " The old version is retained."
				}
				m.setStatus(status, false)
			}
		case opUpdateRuntime:
			m.setStatus(fmt.Sprintf("Installed newer managed runtime %s; the previous runtime is retained.", message.runtime.Version), false)
		}
		m.loadingInventory = true
		return m, tea.Batch(m.inventoryCmd(), m.spinner.Tick)

	case updatesMsg:
		m.loadingUpdates = false
		if m.updateCancel != nil {
			m.updateCancel()
			m.updateCancel = nil
		}
		if message.err != nil {
			m.updatesErr = message.err
			if errors.Is(message.err, context.Canceled) {
				m.setStatus("Update check cancelled.", false)
			} else {
				m.setStatus(message.err.Error(), true)
			}
			return m, nil
		}
		m.updates = message.sdk
		m.runtimeUpdates = message.runtime
		m.updatesLoaded = true
		available := 0
		for _, update := range append(append([]install.Update(nil), m.updates...), m.runtimeUpdates...) {
			if update.UpdateAvailable {
				available++
			}
		}
		if available == 0 {
			m.setStatus("Update check complete: everything is up to date.", false)
		} else {
			m.setStatus(fmt.Sprintf("Update check complete: %d update(s) available. Select an item and press u to apply.", available), false)
		}
		return m, nil

	case buildLineMsg:
		if message.run != m.buildRun {
			return m, nil
		}
		m.appendDrawerLine(message.text)
		if m.buildCh == nil {
			return m, nil
		}
		return m, waitForBuild(m.buildCh)

	case buildDoneMsg:
		m.finishBuild(message)
		if m.buildCh == nil {
			return m, nil
		}
		return m, waitForBuild(m.buildCh)

	case projectsMsg:
		return m.handleProjectsMsg(message)

	case pickerDirectoriesMsg:
		return m.handlePickerDirectories(message), nil

	case projectRootSavedMsg:
		return m.handleProjectRootSaved(message)

	case projectOpenedMsg:
		if message.err != nil {
			m.setStatus("Could not remember project: "+message.err.Error(), true)
			return m, nil
		}
		m.setStatus("Active project: "+message.project.Name+".", false)
		return m, nil

	case doctorMsg:
		m.loadingDoctor = false
		report := message.report
		m.report = &report
		m.clampScroll()
		return m, nil

	case projectImportMsg:
		m.clearPrompt()
		if message.err != nil {
			m.setStatus("Could not import project source: "+message.err.Error(), true)
			return m, nil
		}
		m.setStatus("Added external source: "+message.path, false)
		return m, m.startProjectsScan()

	case projectUnpinMsg:
		if message.err != nil {
			m.setStatus("Could not remove external source: "+message.err.Error(), true)
			return m, nil
		}
		m.setStatus("Removed external source: "+message.path, false)
		return m, m.startProjectsScan()

	case projectDeleteMsg:
		m.clearConfirm()
		if message.err != nil {
			m.setStatus("Could not delete project: "+message.err.Error(), true)
			return m, nil
		}
		m.setStatus("Deleted project file: "+message.project.Name, false)
		return m, m.startProjectsScan()

	case projectTreeMsg:
		return m.handleProjectTreeMsg(message)

	case tea.KeyPressMsg:
		return m.handleKey(message)

	case tea.MouseClickMsg:
		return m.handleMouseClick(message)

	case tea.MouseWheelMsg:
		return m.handleMouseWheel(message)
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

	if m.quitConfirm {
		return m.handleQuitConfirmKey(message)
	}

	if m.rootPicker.open {
		return m.handleProjectRootPickerKey(message)
	}

	if m.projectActionMenu {
		return m.handleProjectActionKey(message)
	}

	if m.busy() {
		switch key {
		case "q":
			m.quitConfirm = true
		case "esc", "ctrl+c":
			m.cancelOperation()
		case "o":
			if m.buildRunning {
				m.drawer.open = !m.drawer.open
			}
		}
		return m, nil
	}

	if m.projectsLoading && m.screen == ScreenProjects && key == "esc" {
		m.cancelProjects()
		return m, nil
	}

	if m.screen == ScreenProjects && m.projectFiltering {
		return m.handleProjectFilterKey(message)
	}
	if m.screen == ScreenProjects && m.projectDetailOpen {
		return m.handleProjectDetailKey(message)
	}

	if m.createWizard.open {
		return m.handleProjectCreateKey(message)
	}

	if m.prompt != promptNone {
		return m.handlePromptKey(message)
	}

	if m.confirm != confirmNone {
		return m.handleConfirmKey(message)
	}

	if m.screen == ScreenProjects && key == "esc" && m.projectFilter != "" {
		m.setProjectFilter("")
		m.projectFiltering = false
		return m, nil
	}

	if m.drawer.open {
		switch key {
		case "o":
			m.drawer.open = false
			return m, nil
		case "f":
			if m.drawer.follow {
				m.drawer.follow = false
				m.clampDrawerScroll()
			} else {
				m.followDrawer()
			}
			return m, nil
		case "esc":
			m.drawer.open = false
			return m, nil
		case "pgup", "ctrl+u":
			m.scrollDrawerBy(-8)
			return m, nil
		case "pgdown", "ctrl+d":
			m.scrollDrawerBy(8)
			return m, nil
		case "home":
			m.setDrawerScroll(0)
			return m, nil
		case "end":
			m.followDrawer()
			return m, nil
		}
	}

	if key == "o" {
		m.drawer.open = true
		if m.drawer.follow {
			m.followDrawer()
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
		if m.screen == ScreenProjects {
			if command := m.startProjectsScan(); command != nil {
				commands = append(commands, command)
			}
		}
		return m, tea.Batch(commands...)
	case "d":
		command := m.navigate(ScreenDiagnostics)
		return m, command
	case "esc":
		m.setStatus("", false)
		return m, nil
	case "1", "2", "3", "4", "5":
		index := int(key[0] - '1')
		command := m.navigate(Screen(index))
		return m, command
	}

	if m.screen == ScreenSDKs {
		switch key {
		case "tab", "shift+tab":
			if m.sdkFocus == focusSDKs {
				m.sdkFocus = focusRuntimes
			} else {
				m.sdkFocus = focusSDKs
			}
			return m, nil
		case "up", "k":
			m.nudgeSelection(-1)
			return m, nil
		case "down", "j":
			m.nudgeSelection(1)
			return m, nil
		case "i":
			if m.sdkFocus == focusRuntimes {
				m.startRuntimePrompt()
				return m, nil
			}
			return m, m.startSDKPrompt()
		case "x":
			m.beginRemove()
			return m, nil
		case "u":
			if m.updatesLoaded {
				return m, m.beginUpdate()
			}
			return m, m.beginUpdateCheck()
		}
	}

	if m.screen == ScreenProjects {
		if key == "/" || key == "f" {
			m.startProjectFinder()
			return m, nil
		}
		if key == "tab" || key == "shift+tab" {
			if m.projectSplit() {
				m.toggleProjectFocus()
			}
			return m, nil
		}
		switch key {
		case "m":
			m.openProjectActionMenu()
			return m, nil
		case "p":
			return m, m.openProjectRootPicker()
		case "a":
			m.prompt = promptImportPath
			m.promptInput = ""
			m.promptFocusInput = true
			m.setStatus("", false)
			return m, nil
		case "u":
			return m, m.unpinSelectedExternalProject()
		case "n":
			m.startProjectCreateWizard()
			return m, nil
		case "x":
			m.beginDeleteProject()
			return m, nil
		case "e":
			m.toggleSelectedProjectWorkspace()
			return m, nil
		case "b":
			return m, m.startBuild()
		case "t":
			project, ok := m.selectedProject()
			if !ok {
				m.setStatus("Select a project first.", false)
				return m, nil
			}
			m.projectTab = projectTabTree
			openCommand := m.openProjectDetail(project)
			return m, tea.Batch(openCommand, m.startProjectTreeScan())
		case "up", "k":
			if m.projectSplit() && m.projectFocus == projectFocusDetail {
				m.scrollFocusedBy(-1)
				return m, nil
			}
			m.moveProject(-1)
			return m, m.startProjectTreeScan()
		case "down", "j":
			if m.projectSplit() && m.projectFocus == projectFocusDetail {
				m.scrollFocusedBy(1)
				return m, nil
			}
			m.moveProject(1)
			return m, m.startProjectTreeScan()
		case "enter":
			project, ok := m.selectedProject()
			if !ok {
				m.setStatus("Select a project first.", false)
				return m, nil
			}
			return m, m.openProjectDetail(project)
		}
	}

	if m.screen == ScreenHome && key == "i" {
		return m, m.startSDKPrompt()
	}

	switch key {
	case "]":
		command := m.navigate(Screen((int(m.screen) + 1) % len(screens)))
		return m, command
	case "[":
		command := m.navigate(Screen((int(m.screen) - 1 + len(screens)) % len(screens)))
		return m, command
	case "pgup", "ctrl+u":
		m.scrollFocusedBy(-8)
		return m, nil
	case "pgdown", "ctrl+d":
		m.scrollFocusedBy(8)
		return m, nil
	case "home":
		m.setFocusedScroll(0)
		return m, nil
	case "end":
		m.setFocusedScroll(m.maxFocusedScroll())
		return m, nil
	}

	return m, nil
}

func (m Model) busy() bool {
	return m.operationRunning || m.loadingUpdates || m.buildRunning
}

func (m Model) busyLabel() string {
	if m.buildRunning {
		return "Building " + m.buildTarget.Name
	}
	if m.operationRunning {
		if m.operationLabel != "" {
			return m.operationLabel
		}
		return "An operation is running"
	}
	if m.loadingUpdates {
		return "Checking for managed updates"
	}
	return ""
}

func (m Model) handleQuitConfirmKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "y", "enter":
		if m.buildCancel != nil {
			m.buildCancel()
		}
		return m, tea.Quit
	case "esc", "n", "ctrl+c":
		m.quitConfirm = false
		return m, nil
	}
	return m, nil
}

func (m Model) handlePromptKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.String()
	if m.prompt == promptImportPath {
		switch key {
		case "esc", "ctrl+c":
			m.clearPrompt()
			m.setStatus("Import cancelled.", false)
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.promptInput)
			if path == "" {
				m.setStatus("Enter a directory, project, or solution path.", true)
				return m, nil
			}
			backend := m.backend
			return m, func() tea.Msg {
				if backend == nil {
					return projectImportMsg{path: path, err: errors.New("no backend configured")}
				}
				return projectImportMsg{path: path, err: backend.PinExternalProject(path)}
			}
		case "backspace", "ctrl+h":
			if m.promptInput != "" {
				runes := []rune(m.promptInput)
				m.promptInput = string(runes[:len(runes)-1])
			}
			return m, nil
		case "ctrl+u":
			m.promptInput = ""
			return m, nil
		}
		if message.Text != "" && message.Mod&tea.ModCtrl == 0 && message.Mod&tea.ModAlt == 0 {
			m.promptInput += message.Text
		}
		return m, nil
	}
	if m.prompt == promptInstallSDK {
		switch key {
		case "esc", "ctrl+c":
			m.clearPrompt()
			m.setStatus("Install cancelled.", false)
			return m, nil
		case "tab":
			m.promptFocusInput = !m.promptFocusInput
			return m, nil
		case "up", "k":
			if !m.promptFocusInput {
				m.moveAvailable(-1)
			}
			return m, nil
		case "down", "j":
			if !m.promptFocusInput {
				m.moveAvailable(1)
			}
			return m, nil
		case "pgup":
			if !m.promptFocusInput {
				m.moveAvailable(-6)
			}
			return m, nil
		case "pgdown":
			if !m.promptFocusInput {
				m.moveAvailable(6)
			}
			return m, nil
		case "ctrl+u":
			if m.promptFocusInput {
				m.promptInput = ""
			} else {
				m.moveAvailable(-6)
			}
			return m, nil
		case "ctrl+d":
			if !m.promptFocusInput {
				m.moveAvailable(6)
			}
			return m, nil
		case "home":
			if !m.promptFocusInput {
				m.availableCursor = 0
			}
			return m, nil
		case "end":
			if !m.promptFocusInput {
				if available := m.downloadableSDKs(); len(available) > 0 {
					m.availableCursor = len(available) - 1
				}
			}
			return m, nil
		case "enter":
			if m.promptFocusInput {
				return m, m.submitPrompt()
			}
			sdk, ok := m.selectedAvailableSDK()
			if !ok {
				m.setStatus("No downloadable SDK is selected. Press tab to type a version.", true)
				return m, nil
			}
			m.promptInput = sdk.Version
			return m, m.submitPrompt()
		case "backspace", "ctrl+h":
			if m.promptInput != "" {
				runes := []rune(m.promptInput)
				m.promptInput = string(runes[:len(runes)-1])
			}
			return m, nil
		}
		if message.Text != "" && message.Mod&tea.ModCtrl == 0 && message.Mod&tea.ModAlt == 0 {
			m.promptFocusInput = true
			m.promptInput += message.Text
		}
		return m, nil
	}

	switch key {
	case "esc", "ctrl+c":
		m.clearPrompt()
		m.setStatus("Install cancelled.", false)
		return m, nil
	case "enter":
		return m, m.submitPrompt()
	case "backspace", "ctrl+h":
		if m.promptInput != "" {
			runes := []rune(m.promptInput)
			m.promptInput = string(runes[:len(runes)-1])
		}
		return m, nil
	case "ctrl+u":
		m.promptInput = ""
		return m, nil
	case "left":
		if m.prompt == promptInstallRuntime {
			m.previousFramework()
		}
		return m, nil
	case "right", "tab":
		if m.prompt == promptInstallRuntime {
			m.nextFramework()
		}
		return m, nil
	}
	if message.Text != "" && message.Mod&tea.ModCtrl == 0 && message.Mod&tea.ModAlt == 0 {
		m.promptInput += message.Text
	}
	return m, nil
}

func (m Model) handleConfirmKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "n":
		m.clearConfirm()
		m.setStatus("Cancelled.", false)
		return m, nil
	case "enter", "y":
		return m, m.confirmAction()
	}
	return m, nil
}

func (m *Model) navigate(target Screen) tea.Cmd {
	if target < 0 || int(target) >= len(screens) {
		return nil
	}
	m.screen = target
	m.cursor = int(target)
	if target != ScreenProjects {
		m.projectDetailOpen = false
		m.projectDetailPath = ""
		m.projectDetailWorkspace = false
	}

	if target == ScreenDiagnostics && m.report == nil && !m.loadingDoctor {
		m.loadingDoctor = true
		return m.doctorCmd()
	}
	if target == ScreenProjects && !m.projectsLoaded && !m.projectsLoading {
		return m.startProjectsScan()
	}
	return nil
}

func (m *Model) scrollBy(delta int) {
	m.setScroll(m.scrollOffset() + delta)
}

func (m Model) scrollOffset() int {
	switch m.screen {
	case ScreenDiagnostics:
		return m.diagScroll
	case ScreenProjects:
		if m.projectDetailOpen {
			return m.projectDetailScroll
		}
		return m.projectScroll
	default:
		return m.sdkScroll
	}
}

func (m *Model) setScroll(value int) {
	max := m.maxScroll()
	if value < 0 {
		value = 0
	}
	if value > max {
		value = max
	}
	switch m.screen {
	case ScreenDiagnostics:
		m.diagScroll = value
	case ScreenProjects:
		if m.projectDetailOpen {
			m.projectDetailScroll = value
		} else {
			m.projectScroll = value
		}
	default:
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
	body := height - 4 - m.drawerHeight()
	if body < 3 {
		body = 3
	}
	return body
}
