package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/install"
)

// View renders the whole application.
func (m Model) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	var content string
	switch {
	case width < minTerminalWidth || height < minTerminalRows:
		content = renderTooSmall(width, height)
	case m.showHelp:
		content = m.renderHelp(width, height)
	default:
		content = m.render(width, height)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Hilt"
	return view
}

func (m Model) render(width, height int) string {
	header := m.renderHeader(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	innerWidth := m.contentWidth()
	lines := m.screenLines(innerWidth)
	offset := m.scrollOffset()
	mainLines := windowLines(lines, innerWidth, bodyHeight, offset)

	inset := contentInset
	if m.contentWidth() == 10 {
		inset = 0
	}
	mainWidth := width
	prefix := strings.Repeat(" ", inset)
	if m.showSidebar() {
		mainWidth = width - sidebarWidth
	}
	prefixed := make([]string, 0, len(mainLines))
	for _, line := range mainLines {
		prefixed = append(prefixed, fitLine(prefix+line, mainWidth))
	}
	main := strings.Join(prefixed, "\n")

	if !m.showSidebar() {
		return lipgloss.JoinVertical(lipgloss.Left, header, main, footer)
	}

	sidebar := m.renderSidebar(bodyHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) renderHeader(width int) string {
	version := "dev"
	platformLabel := "unknown"
	distro := ""
	if m.backend != nil {
		version = m.backend.Version()
		info := m.backend.Platform()
		platformLabel = info.PlatformLabel()
		distro = info.DisplayName()
	}

	title := styleBrand.Render("HILT") + " " + styleMuted.Render("v"+version) +
		styleFaint.Render(" / ") + styleTitle.Render(screens[m.screen].title)
	right := styleMuted.Render(m.dotnetSummary())
	if width >= 90 {
		right = styleMuted.Render(platformLabel + "  ·  " + distro + "  ·  " + m.dotnetSummary())
	}
	if width < 64 {
		right = ""
	}
	gap := width - lipgloss.Width(title) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := fitLine(title+strings.Repeat(" ", gap)+right, width)
	rule := styleBorder.Render(strings.Repeat("─", width))
	return line + "\n" + fitLine(rule, width)
}

func (m Model) dotnetSummary() string {
	if m.loadingInventory && m.inventory == nil {
		return "scanning .NET"
	}
	if m.inventoryErr != nil {
		return ".NET check failed"
	}
	if m.inventory == nil {
		return ".NET unknown"
	}
	if m.selection != nil && m.selection.SDKVersion != "" {
		return ".NET SDK " + m.selection.SDKVersion
	}
	if len(m.inventory.SDKs) == 0 {
		return ".NET not detected"
	}
	versions := m.inventory.SDKVersions()
	return ".NET SDK " + versions[0]
}

func (m Model) renderSidebar(height int) string {
	lines := []string{styleLabel.Render("WORKSPACE"), ""}
	for index, screen := range screens {
		number := styleFaint.Render(fmt.Sprintf("%d", index+1))
		if Screen(index) == m.screen {
			lines = append(lines, styleAccent.Render("▌ ")+number+" "+styleSelected.Render(screen.short))
		} else {
			lines = append(lines, "  "+number+" "+screen.short)
		}
	}

	window := windowLines(lines, sidebarWidth-1, height, 0)
	for index, line := range window {
		window[index] = fitLine(line, sidebarWidth-1) + styleBorder.Render("│")
	}
	return strings.Join(window, "\n")
}

type keyHint struct {
	key   string
	label string
}

func (m Model) renderFooter(width int) string {
	var hints []keyHint
	switch {
	case m.showHelp:
		hints = []keyHint{{"esc", "close help"}, {"q", "quit"}}
	case m.operationRunning:
		hints = []keyHint{{"esc", "cancel operation"}}
	case m.loadingUpdates:
		hints = []keyHint{{"esc", "cancel update check"}}
	case m.prompt != promptNone:
		hints = []keyHint{{"enter", "resolve"}, {"←/→", "framework"}, {"esc", "cancel"}}
	case m.confirm != confirmNone:
		hints = []keyHint{{"enter/y", "confirm"}, {"esc/n", "cancel"}}
	case m.screen == ScreenSDKs:
		hints = []keyHint{{"tab", "switch list"}, {"↑/↓", "select"}, {"i", "install"}, {"x", "remove"}, {"u", "updates"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	case m.screen == ScreenDiagnostics:
		hints = []keyHint{{"↑/↓", "sections"}, {"pgup/pgdn", "scroll"}, {"r", "re-run"}, {"d", "doctor"}, {"?", "help"}, {"q", "quit"}}
	default:
		hints = []keyHint{{"↑/↓", "sections"}, {"1-8", "jump"}, {"r", "refresh"}, {"d", "doctor"}, {"?", "help"}, {"q", "quit"}}
	}

	if width < 56 && len(hints) > 3 {
		hints = []keyHint{{"↑/↓", "navigate"}, {"?", "help"}, {"q", "quit"}}
	}
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, styleKey.Render(hint.key)+" "+styleMuted.Render(hint.label))
	}
	return fitLine(" "+strings.Join(parts, styleFaint.Render("  ·  ")), width)
}

func (m Model) renderHelp(width, height int) string {
	content := []string{
		styleTitle.Render("Keyboard shortcuts"),
		"",
		"↑/↓  j/k      previous / next section",
		"1-8           jump to a section",
		"tab           switch SDK/runtime list, else cycle sections",
		"enter         open selected section / confirm prompt",
		"i             install the focused product",
		"x             remove the selected owned item",
		"u             check updates / apply the selected update",
		"esc           close prompt or cancel an operation",
		"r             refresh inventory",
		"d             run diagnostics",
		"pgup/pgdn     scroll content",
		"ctrl+u/ctrl+d page scrolling",
		"?             close help",
		"q  ctrl+c     quit Hilt",
	}
	box := styleHelpBox.Render(strings.Join(content, "\n"))
	body := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	return body
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

func (m Model) screenLines(width int) []string {
	switch m.screen {
	case ScreenDashboard:
		return m.dashboardLines(width)
	case ScreenSDKs:
		return m.sdkLines(width)
	case ScreenDiagnostics:
		return m.diagnosticLines(width)
	case ScreenSettings:
		return m.settingsLines(width)
	case ScreenProjects:
		return placeholderLines("Projects & Solutions", 3,
			"Discover solutions and projects, inspect metadata, and build, run, or test them.", width)
	case ScreenPackages:
		return placeholderLines("NuGet Packages", 4,
			"List, search, add, update, and remove NuGet packages for a selected project.", width)
	case ScreenCreate:
		return placeholderLines("Create Project", 5,
			"Generate projects from installed templates with validated names and destinations.", width)
	case ScreenTemplates:
		return placeholderLines("Templates & Architectures", 5,
			"Create Clean Architecture and Vertical Slice solutions with configurable references.", width)
	default:
		return []string{"Unknown screen"}
	}
}

func (m Model) dashboardLines(width int) []string {
	lines := sectionHeader("Dashboard", width)

	if m.backend == nil {
		return append(lines, "No backend configured.")
	}
	info := m.backend.Platform()

	system := []string{
		kvSized("Hilt version", m.backend.Version(), 14),
		kvSized("Platform", info.PlatformLabel(), 14),
		kvSized("Distribution", info.DisplayName(), 14),
	}
	if manager := m.backend.PackageManager(); manager != nil {
		system = append(system, kvSized("Package manager", manager.Name, 14))
	} else {
		system = append(system, kvSized("Package manager", styleMuted.Render("not detected"), 14))
	}
	if path, ok := m.backend.Tool("git"); ok {
		system = append(system, kvSized("Git", path, 14))
	} else {
		system = append(system, kvSized("Git", styleWarning.Render("not found"), 14))
	}

	toolchain := []string{}
	switch {
	case m.loadingInventory && m.inventory == nil:
		toolchain = append(toolchain, m.spinner.View()+" scanning installations...")
	case m.inventoryErr != nil:
		toolchain = append(toolchain, styleError.Render("Inventory failed: "+m.inventoryErr.Error()))
	case m.inventory == nil:
		toolchain = append(toolchain, "Inventory not loaded.")
	default:
		inventory := m.inventory
		if inventory.HasUsableHost() || len(inventory.SDKs) > 0 {
			toolchain = append(toolchain, kvSized(".NET installed", styleOK.Render("yes"), 14))
		} else {
			toolchain = append(toolchain, kvSized(".NET installed", styleWarning.Render("not detected"), 14))
		}
		toolchain = append(toolchain,
			kvSized("SDKs", fmt.Sprintf("%d  %s", len(inventory.SDKs), styleMuted.Render("("+joinOr(inventory.SDKVersions(), "none")+")")), 14),
			kvSized("Runtimes", fmt.Sprintf("%d  %s", len(inventory.Runtimes), styleMuted.Render(fmt.Sprintf("(%d root)", len(inventory.Hosts)))), 14),
			kvSized("Resolved dotnet", valueOr(inventory.ResolvedDotnet(), "not found"), 14),
			kvSized("Managed root", m.backend.Config().ManagedRoot, 14),
		)
		if m.selection != nil {
			toolchain = append(toolchain,
				kvSized("Selected SDK", fmt.Sprintf("%s  %s", styleValue.Render(m.selection.SDKVersion), styleMuted.Render("("+string(m.selection.EffectiveProvenance())+")")), 14),
			)
		} else if m.selectionErr != nil {
			toolchain = append(toolchain, kvSized("Selected SDK", styleWarning.Render("unavailable"), 14))
		}
	}

	lines = append(lines, renderPanels([]panelSpec{
		{title: "SYSTEM", content: system},
		{title: ".NET TOOLCHAIN", content: toolchain},
	}, width)...)

	if m.inventory != nil && !m.inventory.HasUsableHost() && len(m.inventory.SDKs) == 0 {
		lines = append(lines, "")
		lines = append(lines, styledWrapped("No .NET SDK detected. Hilt remains fully usable; press i or run hilt sdk install.", width, styleWarning)...)
	}

	warnings := m.warnings()
	if len(warnings) > 0 {
		lines = append(lines, "")
		content := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			for index, line := range wrap(warning, width-6) {
				if index == 0 {
					line = "! " + line
				} else {
					line = "  " + line
				}
				content = append(content, styleWarning.Render(line))
			}
		}
		lines = append(lines, panelBox("WARNINGS", content, width)...)
	}
	return lines
}

type panelSpec struct {
	title   string
	content []string
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

func styledWrapped(text string, width int, style lipgloss.Style) []string {
	lines := make([]string, 0, 1)
	for _, line := range wrap(text, width) {
		lines = append(lines, style.Render(line))
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

func (m Model) sdkLines(width int) []string {
	lines := sectionHeader("SDKs & Runtimes", width)
	lines = append(lines, m.managementLines(width)...)

	if m.loadingInventory && m.inventory == nil {
		return append(lines, m.spinner.View()+" scanning installations...")
	}
	if m.inventoryErr != nil {
		return append(lines, styleError.Render("Inventory failed: "+m.inventoryErr.Error()))
	}
	if m.inventory == nil {
		return append(lines, "Inventory not loaded. Press r to refresh.")
	}

	inventory := m.inventory
	lines = append(lines, fmt.Sprintf("%d host(s) · %d SDK(s) · %d runtime(s)",
		len(inventory.Hosts), len(inventory.SDKs), len(inventory.Runtimes)))

	lines = append(lines, "", styleLabel.Render("SELECTED TOOLCHAIN"))
	switch {
	case m.selection != nil:
		lines = append(lines,
			fmt.Sprintf("  SDK %s (%s)", m.selection.SDKVersion, m.selection.EffectiveProvenance()),
			"  root "+m.selection.Root,
		)
		if m.selection.GlobalJSONPath != "" {
			lines = append(lines, "  global.json "+m.selection.GlobalJSONPath)
		}
		if m.selection.Rationale != "" {
			lines = append(lines, "  "+styleMuted.Render(m.selection.Rationale))
		}
	case m.selectionErr != nil:
		lines = append(lines, "  "+styleWarning.Render(m.selectionErr.Error()))
	default:
		lines = append(lines, "  not resolved")
	}

	lines = append(lines, "", styleLabel.Render("HOSTS"))
	if len(inventory.Hosts) == 0 {
		lines = append(lines, "  none detected")
	}
	for _, host := range inventory.Hosts {
		line := fmt.Sprintf("  %s  [%s] %s", host.Root, host.Source, host.EffectiveProvenance())
		if host.Architecture != "" {
			line += "  arch=" + host.Architecture
		}
		lines = append(lines, line)
		if host.ProbeError != "" {
			lines = append(lines, "    "+styleWarning.Render("probe: "+host.ProbeError))
		}
	}

	sdkLabel := "SDKS"
	if m.sdkFocus == focusSDKs {
		sdkLabel += "  " + styleMuted.Render("(focused)")
	}
	lines = append(lines, "", styleLabel.Render(sdkLabel))
	if len(inventory.SDKs) == 0 {
		lines = append(lines, "  none detected. Press i to install a managed SDK.")
	}
	updateByVersion := make(map[string]install.Update, len(m.updates))
	for _, update := range m.updates {
		updateByVersion[update.Version] = update
	}
	for index, sdk := range inventory.SDKs {
		state := ""
		if !sdk.Usable {
			state = "  " + styleWarning.Render("unusable")
		}
		marker := " "
		if index == m.sdkCursor && m.sdkFocus == focusSDKs {
			marker = ">"
		}
		if m.selection != nil && sdk.Root == m.selection.Root && sdk.Version == m.selection.SDKVersion {
			marker = "*"
		}
		line := fmt.Sprintf("  %s %-12s %s  [%s]%s", marker, sdk.Version, sdk.Path, sdk.EffectiveProvenance(), state)
		if update, ok := updateByVersion[sdk.Version]; ok && update.UpdateAvailable {
			line += "  " + styleAccent.Render("→ "+update.LatestVersion+" available")
		}
		lines = append(lines, line)
	}

	runtimeLabel := "RUNTIMES"
	if m.sdkFocus == focusRuntimes {
		runtimeLabel += "  " + styleMuted.Render("(focused)")
	}
	lines = append(lines, "", styleLabel.Render(runtimeLabel))
	runtimes := m.orderedRuntimes()
	if len(runtimes) == 0 {
		lines = append(lines, "  none detected. Press tab then i to install a managed runtime.")
	}
	runtimeUpdates := make(map[string]install.Update, len(m.runtimeUpdates))
	for _, update := range m.runtimeUpdates {
		runtimeUpdates[update.Version] = update
	}
	for index, runtime := range runtimes {
		marker := " "
		if index == m.runtimeCursor && m.sdkFocus == focusRuntimes {
			marker = ">"
		}
		line := fmt.Sprintf("  %s %-32s %-12s [%s]", marker, runtime.Framework, runtime.Version, runtime.EffectiveProvenance())
		if update, ok := runtimeUpdates[runtime.Version]; ok && update.UpdateAvailable {
			line += "  " + styleAccent.Render("→ "+update.LatestVersion+" available")
		}
		lines = append(lines, line)
	}

	warnings := m.warnings()
	if len(warnings) > 0 {
		lines = append(lines, "", styleLabel.Render("WARNINGS"))
		for _, warning := range warnings {
			lines = append(lines, wrap(warning, width-2)...)
		}
	}
	return lines
}

func (m Model) managementLines(width int) []string {
	var lines []string
	if m.prompt != promptNone {
		label := "INSTALL SDK"
		if m.prompt == promptInstallRuntime {
			label = "INSTALL RUNTIME"
		}
		lines = append(lines, "", styleAccent.Render(label))
		lines = append(lines, "  version  "+m.promptInput+styleMuted.Render("_"))
		if m.prompt == promptInstallRuntime {
			lines = append(lines, fmt.Sprintf("  framework %s  %s", m.promptFramework,
				styleMuted.Render("(left/right to change)")))
		}
		lines = append(lines, "  "+styleMuted.Render("enter to resolve · esc to cancel"))
		return lines
	}
	if m.confirm != confirmNone {
		lines = append(lines, "", styleAccent.Render("CONFIRM"))
		for _, summary := range m.confirmSummary {
			lines = append(lines, "  "+summary)
		}
		lines = append(lines, "  "+styleMuted.Render("enter/y confirm · esc/n cancel"))
		return lines
	}
	if m.operationRunning {
		lines = append(lines, "", m.spinner.View()+" "+m.operationLabel)
		if m.operationCancelling {
			lines = append(lines, "  "+styleWarning.Render("cancelling..."))
		}
		start := 0
		if len(m.progressLines) > 8 {
			start = len(m.progressLines) - 8
		}
		for _, progress := range m.progressLines[start:] {
			lines = append(lines, "  "+styleMuted.Render(progress))
		}
		return lines
	}
	if m.loadingUpdates {
		lines = append(lines, "", m.spinner.View()+" checking managed updates...")
		return lines
	}
	if m.statusMessage != "" {
		style := styleOK
		if m.statusIsError {
			style = styleError
		}
		lines = append(lines, "")
		for _, wrapped := range wrap(m.statusMessage, width-2) {
			lines = append(lines, style.Render(wrapped))
		}
	}
	return lines
}

func (m Model) diagnosticLines(width int) []string {
	lines := sectionHeader("Diagnostics", width)
	if m.loadingDoctor && m.report == nil {
		return append(lines, m.spinner.View()+" running checks...")
	}
	if m.report == nil {
		return append(lines, "No diagnostics report yet. Press r to run hilt doctor.")
	}

	report := m.report
	lines = append(lines, fmt.Sprintf("%d ok · %d info · %d warning · %d error",
		report.Summary.OK, report.Summary.Info, report.Summary.Warnings, report.Summary.Errors))
	if m.loadingDoctor {
		lines = append(lines, m.spinner.View()+" re-running checks...")
	}

	lines = append(lines, "")
	for _, finding := range report.SortedFindings() {
		lines = append(lines, severityBadge(finding.Severity)+" "+styleTitle.Render(finding.Title))
		if finding.Summary != "" {
			lines = append(lines, "  "+finding.Summary)
		}
		for _, detail := range strings.Split(finding.Detail, "\n") {
			if strings.TrimSpace(detail) == "" {
				continue
			}
			for _, wrapped := range wrap(detail, width-4) {
				lines = append(lines, "  "+styleMuted.Render(wrapped))
			}
		}
		if finding.Remediation != "" {
			for _, wrapped := range wrap("fix: "+finding.Remediation, width-4) {
				lines = append(lines, "  "+styleAccent.Render(wrapped))
			}
		}
		lines = append(lines, "")
	}
	return lines
}

func (m Model) settingsLines(width int) []string {
	lines := sectionHeader("Settings", width)
	if m.backend == nil {
		return append(lines, "No backend configured.")
	}
	paths := m.backend.Paths()
	config := m.backend.Config()
	lines = append(lines,
		kv("Configuration", paths.ConfigFile()),
		kv("State", paths.StateFile()),
		kv("Cache", paths.CacheDir),
		kv("Data", paths.DataDir),
		kv("Managed root", config.ManagedRoot),
		"",
		kv("Default project dir", valueOr(config.DefaultProjectDir, "not set")),
		kv("Editor", valueOr(config.Editor, "not set")),
		kv("Theme", valueOr(config.UI.Theme, "auto")),
		kv("Vim keys", fmt.Sprintf("%t", config.UI.VimKeys)),
		kv("Scan exclusions", strings.Join(config.ScanExclusions, ", ")),
		"",
		styleMuted.Render("Settings editing is read-only. Edit the configuration file directly."),
	)
	if len(m.backend.ConfigWarnings()) > 0 {
		lines = append(lines, "", styleLabel.Render("CONFIGURATION WARNINGS"))
		for _, warning := range m.backend.ConfigWarnings() {
			lines = append(lines, wrap(warning, width-2)...)
		}
	}
	return lines
}

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

func placeholderLines(title string, phase int, description string, width int) []string {
	lines := sectionHeader(title, width)
	lines = append(lines,
		styleWarning.Render("Not implemented."),
		"",
		description,
		"",
		fmt.Sprintf("Planned for Phase %d. This screen does not modify files or run commands yet.", phase),
	)
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

func severityBadge(severity diagnostics.Severity) string {
	switch severity {
	case diagnostics.SeverityError:
		return styleError.Render("[ERROR]")
	case diagnostics.SeverityWarning:
		return styleWarning.Render("[WARN] ")
	case diagnostics.SeverityInfo:
		return styleInfo.Render("[INFO] ")
	default:
		return styleOK.Render("[OK]   ")
	}
}

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
