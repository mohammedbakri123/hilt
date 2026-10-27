package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/install"
)

type dotnetSDK = dotnet.SDK
type dotnetRuntime = dotnet.Runtime

const (
	dotnetProvenanceOwned   = dotnet.ProvenanceOwned
	dotnetProvenanceManaged = dotnet.ProvenanceManaged
)

type removeTarget struct {
	product install.ProductKind
	version string
	root    string
}

type focusArea int

const (
	focusSDKs focusArea = iota
	focusRuntimes
)

type promptKind int

const (
	promptNone promptKind = iota
	promptInstallSDK
	promptInstallRuntime
	promptImportPath
)

type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmInstallSDK
	confirmInstallRuntime
	confirmRemoveSDK
	confirmRemoveRuntime
	confirmUpdateSDK
	confirmUpdateRuntime
	confirmCreateProjects
	confirmDeleteProject
)

type operationKind string

const (
	opInstallSDK       operationKind = "install-sdk"
	opInstallRuntime   operationKind = "install-runtime"
	opRemoveSDK        operationKind = "remove-sdk"
	opRemoveRuntime    operationKind = "remove-runtime"
	opUpdateSDK        operationKind = "update-sdk"
	opUpdateRuntime    operationKind = "update-runtime"
	opCreateProjects   operationKind = "create-projects"
	opProjectCommand   operationKind = "project-command"
	updateCheckPeriod                = 45 * time.Second
	availableSDKPeriod               = 180 * time.Second
)

type sdkPlanMsg struct {
	plan install.Plan
	err  error
}

type runtimePlanMsg struct {
	plan install.RuntimePlan
	err  error
}

type operationDoneMsg struct {
	kind     operationKind
	command  string
	result   execution.Result
	sdk      install.Result
	runtime  install.RuntimeResult
	removal  install.Removal
	update   install.UpdateResult
	version  string
	upToDate bool
	err      error
}

type updatesMsg struct {
	sdk     []install.Update
	runtime []install.Update
	err     error
}

type availableSDKsMsg struct {
	request int
	sdks    []install.AvailableSDK
	err     error
}

type progressMsg struct {
	line        string
	downloaded  int64
	total       int64
	hasProgress bool
}

// progressWriter forwards operation progress lines to the TUI event loop.
type progressWriter struct {
	ch chan<- tea.Msg
}

func (w progressWriter) ReportProgress(downloaded, total int64) {
	if total < 0 {
		total = 0
	}
	w.ch <- progressMsg{downloaded: downloaded, total: total, hasProgress: true}
}

func (w progressWriter) Write(data []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.ch <- progressMsg{line: line}
	}
	return len(data), nil
}

func waitForProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-ch
		if !ok {
			return nil
		}
		return message
	}
}

func (m Model) planSDKCmd(ctx context.Context, version string) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		plan, err := backend.PlanSDKInstall(ctx, version, false)
		return sdkPlanMsg{plan: plan, err: err}
	}
}

func (m Model) planRuntimeCmd(ctx context.Context, version string, framework install.FrameworkFlavor) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		plan, err := backend.PlanRuntimeInstall(ctx, version, framework, false)
		return runtimePlanMsg{plan: plan, err: err}
	}
}

func (m Model) installSDKCmd(ctx context.Context, plan install.Plan, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		result, err := backend.InstallSDK(ctx, plan, progressWriter{ch})
		return operationDoneMsg{kind: opInstallSDK, sdk: result, version: plan.Version, err: err}
	}
}

func (m Model) installRuntimeCmd(ctx context.Context, plan install.RuntimePlan, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		result, err := backend.InstallRuntime(ctx, plan, progressWriter{ch})
		return operationDoneMsg{kind: opInstallRuntime, runtime: result, version: plan.Version, err: err}
	}
}

func (m Model) removeSDKCmd(ctx context.Context, version string) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		removal, err := backend.RemoveSDK(ctx, version)
		return operationDoneMsg{kind: opRemoveSDK, removal: removal, version: version, err: err}
	}
}

func (m Model) removeRuntimeCmd(ctx context.Context, version string) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		removal, err := backend.RemoveRuntime(ctx, version)
		return operationDoneMsg{kind: opRemoveRuntime, removal: removal, version: version, err: err}
	}
}

func (m Model) updateSDKCmd(ctx context.Context, update install.Update, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		result, err := backend.UpdateSDK(ctx, update.Version, false, progressWriter{ch})
		return operationDoneMsg{kind: opUpdateSDK, update: result, version: update.Version, err: err}
	}
}

func (m Model) updateRuntimeCmd(ctx context.Context, update install.Update, framework install.FrameworkFlavor, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		plan, err := backend.PlanRuntimeInstall(ctx, update.LatestVersion, framework, false)
		if err != nil {
			return operationDoneMsg{kind: opUpdateRuntime, version: update.Version, err: err}
		}
		result, err := backend.InstallRuntime(ctx, plan, progressWriter{ch})
		return operationDoneMsg{kind: opUpdateRuntime, runtime: result, version: update.LatestVersion, err: err}
	}
}

func (m Model) availableSDKsCmd(ctx context.Context, request int) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		sdks, err := backend.AvailableSDKs(ctx, false)
		return availableSDKsMsg{request: request, sdks: sdks, err: err}
	}
}

func (m Model) updatesCmd(ctx context.Context) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		sdkUpdates, sdkErr := backend.CheckSDKUpdates(ctx)
		runtimeUpdates, runtimeErr := backend.CheckRuntimeUpdates(ctx)
		return updatesMsg{
			sdk:     sdkUpdates,
			runtime: runtimeUpdates,
			err:     errors.Join(sdkErr, runtimeErr),
		}
	}
}

func (m *Model) startOperation(label string, cancel context.CancelFunc) {
	m.operationRunning = true
	m.operationCancelling = false
	m.operationLabel = label
	m.operationCancel = cancel
	m.operationErr = nil
	m.progressLines = nil
	m.progressDone = 0
	m.progressTotal = 0
	m.statusMessage = ""
	m.statusIsError = false
}

func (m *Model) finishOperation() {
	if m.operationCancel != nil {
		m.operationCancel()
	}
	m.operationRunning = false
	m.operationCancelling = false
	m.operationCancel = nil
	m.progressCh = nil
	m.progressDone = 0
	m.progressTotal = 0
	m.operationErr = nil
}

func (m *Model) clearPrompt() {
	m.cancelAvailableSDKs()
	m.prompt = promptNone
	m.promptInput = ""
	m.promptFocusInput = false
	m.promptFramework = install.FrameworkAll
}

func (m *Model) clearConfirm() {
	m.confirm = confirmNone
	m.confirmSummary = nil
	m.pendingPlan = nil
	m.pendingRuntimePlan = nil
	m.pendingRemove = nil
	m.pendingUpdate = nil
	m.pendingFramework = install.FrameworkAll
	m.pendingCreatePlan = nil
	m.pendingDeleteProject = nil
}

func (m *Model) setStatus(message string, isError bool) {
	m.statusMessage = message
	m.statusIsError = isError
}

func (m *Model) startSDKPrompt() tea.Cmd {
	m.screen = ScreenSDKs
	m.cursor = int(ScreenSDKs)
	m.sdkFocus = focusSDKs
	m.prompt = promptInstallSDK
	m.promptInput = ""
	m.promptFocusInput = false
	m.promptFramework = install.FrameworkAll
	m.clearConfirm()
	m.setStatus("", false)
	return m.loadAvailableSDKs()
}

func (m *Model) loadAvailableSDKs() tea.Cmd {
	m.cancelAvailableSDKs()
	m.availableRequest++
	request := m.availableRequest
	m.availableSDKs = nil
	m.availableCursor = 0
	m.availableErr = nil
	m.availableLoading = true
	ctx, cancel := context.WithTimeout(context.Background(), availableSDKPeriod)
	m.availableCancel = cancel
	return m.availableSDKsCmd(ctx, request)
}

func (m *Model) startRuntimePrompt() {
	m.cancelAvailableSDKs()
	m.prompt = promptInstallRuntime
	m.promptInput = ""
	m.promptFocusInput = true
	m.promptFramework = install.FrameworkAll
	m.clearConfirm()
	m.setStatus("", false)
}

func (m *Model) cancelAvailableSDKs() {
	if m.availableCancel != nil {
		m.availableCancel()
		m.availableCancel = nil
	}
	m.availableLoading = false
	m.availableRequest++
}

func (m Model) downloadableSDKs() []install.AvailableSDK {
	if len(m.availableSDKs) == 0 {
		return nil
	}
	installed := make(map[string]bool)
	if m.inventory != nil {
		for _, sdk := range m.inventory.SDKs {
			installed[sdk.Version] = true
		}
	}
	available := make([]install.AvailableSDK, 0, len(m.availableSDKs))
	for _, sdk := range m.availableSDKs {
		if installed[sdk.Version] {
			continue
		}
		available = append(available, sdk)
	}
	return available
}

func (m Model) selectedAvailableSDK() (install.AvailableSDK, bool) {
	available := m.downloadableSDKs()
	if len(available) == 0 || m.availableCursor < 0 || m.availableCursor >= len(available) {
		return install.AvailableSDK{}, false
	}
	return available[m.availableCursor], true
}

func (m *Model) clampAvailableCursor() {
	available := m.downloadableSDKs()
	if len(available) == 0 {
		m.availableCursor = 0
		return
	}
	if m.availableCursor < 0 {
		m.availableCursor = 0
	}
	if m.availableCursor >= len(available) {
		m.availableCursor = len(available) - 1
	}
}

func (m *Model) moveAvailable(delta int) {
	m.clampAvailableCursor()
	available := m.downloadableSDKs()
	if len(available) == 0 {
		return
	}
	m.availableCursor += delta
	m.clampAvailableCursor()
}

func (m *Model) submitPrompt() tea.Cmd {
	version := strings.TrimSpace(m.promptInput)
	if version == "" {
		m.setStatus("Enter a version, channel, or alias (latest, lts).", true)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	if m.prompt == promptInstallRuntime {
		framework := m.promptFramework
		m.clearPrompt()
		m.startOperation(fmt.Sprintf("Resolving runtime %s (%s)", version, framework), cancel)
		return m.planRuntimeCmd(ctx, version, framework)
	}
	m.clearPrompt()
	m.startOperation("Resolving SDK "+version, cancel)
	return m.planSDKCmd(ctx, version)
}

func (m *Model) beginRemove() {
	if m.inventory == nil {
		m.setStatus("Inventory is not loaded yet.", true)
		return
	}
	m.clearConfirm()
	m.setStatus("", false)
	if m.sdkFocus == focusRuntimes {
		runtime, ok := m.selectedRuntime()
		if !ok {
			m.setStatus("No runtime is selected.", true)
			return
		}
		switch runtime.EffectiveProvenance() {
		case dotnetProvenanceOwned:
			m.pendingRemove = &removeTarget{product: install.ProductRuntime, version: runtime.Version, root: runtime.Root}
			m.confirm = confirmRemoveRuntime
			m.confirmSummary = []string{
				fmt.Sprintf("Remove managed runtime %s", runtime.Version),
				"root " + runtime.Root,
				"Only Hilt-managed files are removed.",
			}
		case dotnetProvenanceManaged:
			m.setStatus(fmt.Sprintf("Runtime %s is under the managed root but is not recorded in Hilt's install manifest; refusing to remove it.", runtime.Version), true)
		case dotnet.ProvenanceSystem:
			m.setStatus(fmt.Sprintf("Runtime %s is provided by the system at %s; Hilt only removes managed runtimes.", runtime.Version, runtime.Root), true)
		default:
			m.setStatus(fmt.Sprintf("Runtime %s is provided by an external installation at %s; Hilt only removes managed runtimes.", runtime.Version, runtime.Root), true)
		}
		return
	}
	sdk, ok := m.selectedSDK()
	if !ok {
		m.setStatus("No SDK is selected.", true)
		return
	}
	switch sdk.EffectiveProvenance() {
	case dotnetProvenanceOwned:
		m.pendingRemove = &removeTarget{product: install.ProductSDK, version: sdk.Version, root: sdk.Root}
		m.confirm = confirmRemoveSDK
		m.confirmSummary = []string{
			fmt.Sprintf("Remove managed SDK %s", sdk.Version),
			"root " + sdk.Root,
			"Only Hilt-managed files are removed.",
		}
	case dotnetProvenanceManaged:
		m.setStatus(fmt.Sprintf("SDK %s is under the managed root but is not recorded in Hilt's install manifest; refusing to remove it.", sdk.Version), true)
	case dotnet.ProvenanceSystem:
		m.setStatus(fmt.Sprintf("SDK %s is provided by the system at %s; Hilt only removes managed SDKs.", sdk.Version, sdk.Root), true)
	default:
		m.setStatus(fmt.Sprintf("SDK %s is provided by an external installation at %s; Hilt only removes managed SDKs.", sdk.Version, sdk.Root), true)
	}
}

func (m *Model) beginUpdateCheck() tea.Cmd {
	if m.loadingUpdates {
		return nil
	}
	if m.inventory == nil {
		m.setStatus("Inventory is not loaded yet.", true)
		return nil
	}
	m.clearPrompt()
	m.clearConfirm()
	m.loadingUpdates = true
	m.updatesErr = nil
	m.setStatus("Checking for managed updates...", false)
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckPeriod)
	m.updateCancel = cancel
	return m.updatesCmd(ctx)
}

func (m *Model) beginUpdate() tea.Cmd {
	if m.inventory == nil {
		m.setStatus("Inventory is not loaded yet.", true)
		return nil
	}
	m.setStatus("", false)
	if m.sdkFocus == focusRuntimes {
		runtime, ok := m.selectedRuntime()
		if !ok {
			m.setStatus("No runtime is selected.", true)
			return nil
		}
		update, found := updateFor(m.runtimeUpdates, runtime.Version)
		if !found {
			m.setStatus(fmt.Sprintf("No update is available for runtime %s.", runtime.Version), false)
			return nil
		}
		framework := install.FrameworkAll
		if runtime.Framework == install.RuntimeFrameworkNetCore {
			framework = install.FrameworkNetCore
		}
		m.pendingUpdate = &update
		m.pendingFramework = framework
		m.confirm = confirmUpdateRuntime
		m.confirmSummary = []string{
			fmt.Sprintf("Update runtime %s to %s", update.Version, update.LatestVersion),
			"Installs the newest stable version in the same channel.",
			"The old runtime is retained.",
		}
		return nil
	}
	sdk, ok := m.selectedSDK()
	if !ok {
		m.setStatus("No SDK is selected.", true)
		return nil
	}
	update, found := updateFor(m.updates, sdk.Version)
	if !found {
		m.setStatus(fmt.Sprintf("No update is available for SDK %s.", sdk.Version), false)
		return nil
	}
	m.pendingUpdate = &update
	m.confirm = confirmUpdateSDK
	m.confirmSummary = []string{
		fmt.Sprintf("Update SDK %s to %s", update.Version, update.LatestVersion),
		"Installs the newest stable version in the same channel.",
		"The old SDK is retained; remove it later with x if desired.",
	}
	return nil
}

func updateFor(updates []install.Update, version string) (install.Update, bool) {
	for _, update := range updates {
		if update.Version == version && update.UpdateAvailable {
			return update, true
		}
	}
	return install.Update{}, false
}

func (m *Model) confirmAction() tea.Cmd {
	defer m.clearConfirm()
	switch m.confirm {
	case confirmInstallSDK:
		if m.pendingPlan == nil {
			return nil
		}
		plan := *m.pendingPlan
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation("Installing managed SDK "+plan.Version, cancel)
		ch := make(chan tea.Msg, 128)
		m.progressCh = ch
		return tea.Batch(m.installSDKCmd(ctx, plan, ch), waitForProgress(ch))
	case confirmInstallRuntime:
		if m.pendingRuntimePlan == nil {
			return nil
		}
		plan := *m.pendingRuntimePlan
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation(fmt.Sprintf("Installing managed runtime %s (%s)", plan.Version, plan.Framework), cancel)
		ch := make(chan tea.Msg, 128)
		m.progressCh = ch
		return tea.Batch(m.installRuntimeCmd(ctx, plan, ch), waitForProgress(ch))
	case confirmRemoveSDK:
		if m.pendingRemove == nil {
			return nil
		}
		target := *m.pendingRemove
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation("Removing managed SDK "+target.version, cancel)
		return m.removeSDKCmd(ctx, target.version)
	case confirmRemoveRuntime:
		if m.pendingRemove == nil {
			return nil
		}
		target := *m.pendingRemove
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation("Removing managed runtime "+target.version, cancel)
		return m.removeRuntimeCmd(ctx, target.version)
	case confirmUpdateSDK:
		if m.pendingUpdate == nil {
			return nil
		}
		update := *m.pendingUpdate
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation(fmt.Sprintf("Updating SDK %s to %s", update.Version, update.LatestVersion), cancel)
		ch := make(chan tea.Msg, 128)
		m.progressCh = ch
		return tea.Batch(m.updateSDKCmd(ctx, update, ch), waitForProgress(ch))
	case confirmUpdateRuntime:
		if m.pendingUpdate == nil {
			return nil
		}
		update := *m.pendingUpdate
		framework := m.pendingFramework
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation(fmt.Sprintf("Updating runtime %s to %s", update.Version, update.LatestVersion), cancel)
		ch := make(chan tea.Msg, 128)
		m.progressCh = ch
		return tea.Batch(m.updateRuntimeCmd(ctx, update, framework, ch), waitForProgress(ch))
	case confirmCreateProjects:
		if m.pendingCreatePlan == nil {
			return nil
		}
		plan := *m.pendingCreatePlan
		ctx, cancel := context.WithCancel(context.Background())
		m.startOperation("Creating "+plan.Template.Name, cancel)
		ch := make(chan tea.Msg, 128)
		m.progressCh = ch
		return tea.Batch(m.createProjectsCmd(ctx, plan, ch), waitForProgress(ch))
	case confirmDeleteProject:
		if m.pendingDeleteProject == nil {
			return nil
		}
		project := *m.pendingDeleteProject
		backend := m.backend
		return func() tea.Msg {
			if backend == nil {
				return projectDeleteMsg{project: project, err: errors.New("no backend configured")}
			}
			return projectDeleteMsg{project: project, err: backend.DeleteProject(project.Path)}
		}
	}
	return nil
}

func (m *Model) cancelOperation() {
	if m.buildRunning {
		m.cancelBuild()
		return
	}
	if m.operationRunning && m.operationCancel != nil {
		m.operationCancel()
		m.operationCancelling = true
		m.setStatus("Cancelling operation...", false)
	}
	if m.loadingUpdates && m.updateCancel != nil {
		m.updateCancel()
		m.updateCancel = nil
		m.loadingUpdates = false
		m.setStatus("Update check cancelled.", false)
	}
}

func (m *Model) nudgeSelection(delta int) {
	if m.inventory == nil {
		return
	}
	if m.sdkFocus == focusRuntimes {
		count := len(m.orderedRuntimes())
		if count == 0 {
			return
		}
		m.runtimeCursor = clampIndex(m.runtimeCursor+delta, count)
		return
	}
	count := len(m.inventory.SDKs)
	if count == 0 {
		return
	}
	m.sdkCursor = clampIndex(m.sdkCursor+delta, count)
}

func (m *Model) clampSelection() {
	if m.inventory == nil {
		m.sdkCursor = 0
		m.runtimeCursor = 0
		return
	}
	if len(m.inventory.SDKs) == 0 {
		m.sdkCursor = 0
	} else {
		m.sdkCursor = clampIndex(m.sdkCursor, len(m.inventory.SDKs))
	}
	if count := len(m.orderedRuntimes()); count == 0 {
		m.runtimeCursor = 0
	} else {
		m.runtimeCursor = clampIndex(m.runtimeCursor, count)
	}
}

func clampIndex(index, count int) int {
	if index < 0 {
		return 0
	}
	if index >= count {
		return count - 1
	}
	return index
}

func (m Model) selectedSDK() (dotnetSDK, bool) {
	if m.inventory == nil || len(m.inventory.SDKs) == 0 {
		return dotnetSDK{}, false
	}
	index := clampIndex(m.sdkCursor, len(m.inventory.SDKs))
	return m.inventory.SDKs[index], true
}

func (m Model) orderedRuntimes() []dotnet.Runtime {
	if m.inventory == nil {
		return nil
	}
	runtimes := append([]dotnet.Runtime(nil), m.inventory.Runtimes...)
	sort.SliceStable(runtimes, func(left, right int) bool {
		if runtimes[left].Framework != runtimes[right].Framework {
			return runtimes[left].Framework < runtimes[right].Framework
		}
		return runtimes[left].Version < runtimes[right].Version
	})
	return runtimes
}

func (m Model) selectedRuntime() (dotnetRuntime, bool) {
	runtimes := m.orderedRuntimes()
	if len(runtimes) == 0 {
		return dotnetRuntime{}, false
	}
	index := clampIndex(m.runtimeCursor, len(runtimes))
	return runtimes[index], true
}

func (m *Model) nextFramework() {
	switch m.promptFramework {
	case install.FrameworkAll:
		m.promptFramework = install.FrameworkNetCore
	case install.FrameworkNetCore:
		m.promptFramework = install.FrameworkASPNETCore
	default:
		m.promptFramework = install.FrameworkAll
	}
}

func (m *Model) previousFramework() {
	switch m.promptFramework {
	case install.FrameworkAll:
		m.promptFramework = install.FrameworkASPNETCore
	case install.FrameworkASPNETCore:
		m.promptFramework = install.FrameworkNetCore
	default:
		m.promptFramework = install.FrameworkAll
	}
}

func sdkPlanSummary(plan install.Plan) []string {
	lines := []string{fmt.Sprintf("Install managed SDK %s", plan.Version)}
	if plan.Requested != "" && plan.Requested != plan.Version {
		lines = append(lines, "requested "+plan.Requested)
	}
	lines = append(lines, "root "+plan.Root)
	if plan.AlreadyInstalled {
		lines = append(lines, "Already present in the managed root; Hilt will verify it.")
	}
	if plan.SystemCopy {
		lines = append(lines, "A system copy already exists and will not be modified.")
	}
	lines = append(lines, "Downloads and verifies the Microsoft archive before publishing.")
	return lines
}

func runtimePlanSummary(plan install.RuntimePlan) []string {
	lines := []string{fmt.Sprintf("Install managed runtime %s", plan.Version)}
	if plan.Requested != "" && plan.Requested != plan.Version {
		lines = append(lines, "requested "+plan.Requested)
	}
	lines = append(lines, "framework "+plan.Framework)
	if len(plan.Frameworks) > 0 {
		lines = append(lines, "includes "+strings.Join(plan.Frameworks, ", "))
	}
	lines = append(lines, "root "+plan.Root)
	if plan.AlreadyInstalled {
		lines = append(lines, "Already present in the managed root; Hilt will verify it.")
	}
	if plan.SystemCopy {
		lines = append(lines, "A system copy already exists and will not be modified.")
	}
	lines = append(lines, "Downloads and verifies the Microsoft archives before publishing.")
	return lines
}
