package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
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

	title := styleTitle.Render("HILT") + " " + styleMuted.Render("v"+version)
	right := styleMuted.Render(platformLabel + " · " + distro + " · " + m.dotnetSummary())
	gap := width - lipgloss.Width(title) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := fitLine(title+strings.Repeat(" ", gap)+right, width)
	rule := styleMuted.Render(strings.Repeat("─", width))
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
	if len(m.inventory.SDKs) == 0 {
		return ".NET not detected"
	}
	versions := m.inventory.SDKVersions()
	return ".NET SDK " + versions[0]
}

func (m Model) renderSidebar(height int) string {
	lines := []string{styleMuted.Render("SECTIONS"), ""}
	for index, screen := range screens {
		label := fmt.Sprintf("%d %s", index+1, screen.short)
		if Screen(index) == m.screen {
			lines = append(lines, styleSelected.Render("▸ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	lines = append(lines, "", styleMuted.Render("? help  q quit"))

	window := windowLines(lines, sidebarWidth-1, height, 0)
	for index, line := range window {
		window[index] = fitLine(line, sidebarWidth-1) + styleMuted.Render("│")
	}
	return strings.Join(window, "\n")
}

func (m Model) renderFooter(width int) string {
	keys := " ↑/↓ navigate · 1-8 jump · r refresh · d doctor · ? help · q quit"
	if m.screen == ScreenDiagnostics {
		keys = " ↑/↓ navigate · r re-run · pgup/pgdn scroll · d doctor · ? help · q quit"
	}
	if m.screen == ScreenSDKs {
		keys = " ↑/↓ navigate · r refresh · pgup/pgdn scroll · ? help · q quit"
	}
	return fitLine(styleMuted.Render(keys), width)
}

func (m Model) renderHelp(width, height int) string {
	content := []string{
		styleTitle.Render("Keyboard shortcuts"),
		"",
		"↑/↓  j/k      previous / next section",
		"1-8           jump to a section",
		"tab           cycle sections",
		"enter         open selected section",
		"r             refresh inventory",
		"d             run diagnostics",
		"pgup/pgdn     scroll content",
		"ctrl+u/ctrl+d page scrolling",
		"?  esc        close help",
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
	lines = append(lines,
		kv("Hilt version", m.backend.Version()),
		kv("Platform", info.PlatformLabel()),
		kv("Distribution", info.DisplayName()),
	)

	if manager := m.backend.PackageManager(); manager != nil {
		lines = append(lines, kv("Package manager", manager.Name))
	} else {
		lines = append(lines, kv("Package manager", "not detected"))
	}

	if path, ok := m.backend.Tool("git"); ok {
		lines = append(lines, kv("Git", path))
	} else {
		lines = append(lines, kv("Git", styleWarning.Render("not found")))
	}

	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("DOTNET"))
	switch {
	case m.loadingInventory && m.inventory == nil:
		lines = append(lines, m.spinner.View()+" scanning installations...")
		return lines
	case m.inventoryErr != nil:
		lines = append(lines, styleError.Render("Inventory failed: "+m.inventoryErr.Error()))
		return lines
	case m.inventory == nil:
		lines = append(lines, "Inventory not loaded.")
		return lines
	}

	inventory := m.inventory
	if inventory.HasUsableHost() || len(inventory.SDKs) > 0 {
		lines = append(lines, kv(".NET installed", styleOK.Render("yes")))
	} else {
		lines = append(lines, kv(".NET installed", styleWarning.Render("not detected")))
	}
	lines = append(lines,
		kv("SDKs", fmt.Sprintf("%d (%s)", len(inventory.SDKs), joinOr(inventory.SDKVersions(), "-"))),
		kv("Runtimes", fmt.Sprintf("%d", len(inventory.Runtimes))),
		kv("Installation roots", fmt.Sprintf("%d", len(inventory.Hosts))),
		kv("Resolved dotnet", valueOr(inventory.ResolvedDotnet(), "not found")),
		kv("Managed root", m.backend.Config().ManagedRoot),
	)

	if !inventory.HasUsableHost() && len(inventory.SDKs) == 0 {
		lines = append(lines, "", styleWarning.Render(
			"No .NET SDK detected. Hilt remains fully usable; managed SDK installation arrives in Phase 2."))
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

func (m Model) sdkLines(width int) []string {
	lines := sectionHeader("SDKs & Runtimes", width)
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

	lines = append(lines, "", styleLabel.Render("SDKS"))
	if len(inventory.SDKs) == 0 {
		lines = append(lines, "  none detected. SDK installation is planned for Phase 2.")
	}
	for _, sdk := range inventory.SDKs {
		state := ""
		if !sdk.Usable {
			state = "  " + styleWarning.Render("unusable")
		}
		lines = append(lines, fmt.Sprintf("  %-12s %s  [%s]%s", sdk.Version, sdk.Path, sdk.EffectiveProvenance(), state))
	}

	lines = append(lines, "", styleLabel.Render("RUNTIMES"))
	if len(inventory.Runtimes) == 0 {
		lines = append(lines, "  none detected")
	}
	grouped := dotnet.GroupRuntimes(inventory.Runtimes)
	frameworks := make([]string, 0, len(grouped))
	for framework := range grouped {
		frameworks = append(frameworks, framework)
	}
	sortStrings(frameworks)
	for _, framework := range frameworks {
		lines = append(lines, fmt.Sprintf("  %-32s %s", framework, strings.Join(grouped[framework], ", ")))
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
		styleMuted.Render("Settings editing is read-only in Phase 1. Edit the configuration file directly."),
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
	return dedupeStrings(warnings)
}

func placeholderLines(title string, phase int, description string, width int) []string {
	lines := sectionHeader(title, width)
	lines = append(lines,
		styleWarning.Render("Not implemented in Phase 1."),
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
	width := 22
	padding := width - len(label)
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
