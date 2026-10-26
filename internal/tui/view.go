package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/projects"
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
	status := m.renderStatusLine(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(status) - lipgloss.Height(footer)
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

	var content string
	if !m.showSidebar() {
		content = lipgloss.JoinVertical(lipgloss.Left, header, main, status, footer)
	} else {
		sidebar := m.renderSidebar(bodyHeight)
		body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
		content = lipgloss.JoinVertical(lipgloss.Left, header, body, status, footer)
	}
	if m.quitConfirm {
		content = overlayCentered(content, m.renderQuitDialog(width), width, height)
	}
	return content
}

func (m Model) renderQuitDialog(width int) string {
	boxWidth := 56
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 12 {
		boxWidth = 12
	}
	content := []string{
		"An operation is still running:",
		styleValue.Render(m.busyLabel()),
		"",
		"Quitting interrupts it. Press esc to stay; cancel the operation first if you want to stop it.",
		"",
		styleKey.Render("y / enter") + "  quit anyway",
		styleKey.Render("esc / n") + "     stay",
	}
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
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

func (m Model) renderStatusLine(width int) string {
	if m.statusMessage == "" {
		return fitLine("", width)
	}
	badge := styleOK.Render("[ok]")
	style := styleOK
	if m.statusIsError {
		badge = styleError.Render("[err]")
		style = styleError
	}
	return fitLine(" "+badge+" "+style.Render(m.statusMessage), width)
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

func (m Model) footerContext() keyContext {
	switch {
	case m.quitConfirm:
		return ctxQuitConfirm
	case m.showHelp:
		return ctxHelp
	case m.operationRunning:
		return ctxOperation
	case m.loadingUpdates:
		return ctxUpdates
	case m.prompt == promptInstallSDK:
		if m.promptFocusInput {
			return ctxPromptSDKInput
		}
		return ctxPromptSDKList
	case m.prompt != promptNone:
		return ctxPromptRuntime
	case m.confirm != confirmNone:
		return ctxConfirm
	case m.screen == ScreenDashboard:
		return ctxDashboard
	case m.screen == ScreenSDKs:
		return ctxSDKs
	case m.screen == ScreenDiagnostics:
		return ctxDiagnostics
	case m.screen == ScreenProjects:
		if m.projectsLoading {
			return ctxProjectsLoading
		}
		return ctxProjects
	}
	return ctxSettings
}

func (m Model) renderFooter(width int) string {
	hints := footerBindings(m.footerContext(), width)
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, styleKey.Render(hint.Key)+" "+styleMuted.Render(hint.Label))
	}
	return fitLine(" "+strings.Join(parts, "  "), width)
}

func (m Model) renderHelp(width, height int) string {
	content := []string{styleTitle.Render("Keyboard shortcuts"), ""}
	sections := []struct {
		title   string
		context keyContext
	}{
		{title: "NAVIGATION", context: ctxHelpNavigation},
		{title: "MANAGED .NET", context: ctxHelpManaged},
		{title: "VIEW", context: ctxHelpView},
	}
	for index, section := range sections {
		if index > 0 {
			content = append(content, "")
		}
		content = append(content, styleLabel.Render(section.title))
		for _, binding := range bindingsFor(section.context) {
			help := binding.Help
			if help == "" {
				help = binding.Label
			}
			content = append(content, helpLine(binding.Key, help))
		}
	}
	box := styleHelpBox.Render(strings.Join(content, "\n"))
	if lipgloss.Width(box) > width || lipgloss.Height(box) > height {
		box = styleHelpBox.Padding(0, 1).Render(strings.Join(content, "\n"))
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func helpLine(keys, description string) string {
	return "  " + styleKey.Render(fmt.Sprintf("%-12s", keys)) + " " + styleMuted.Render(description)
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
		return m.projectLines(width)
	default:
		return []string{"Unknown screen"}
	}
}

func (m Model) dashboardLines(width int) []string {
	lines := sectionHeader("Home", width)

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

func (m Model) projectLines(width int) []string {
	lines := sectionHeader("Projects & Solutions", width)
	if m.backend == nil {
		return append(lines, "No backend configured.")
	}
	if m.projectsLoading && m.projectResult == nil {
		return append(lines, m.spinner.View()+" scanning projects...")
	}
	if m.projectsErr != nil {
		for _, line := range styledWrapped("Project scan failed: "+m.projectsErr.Error(), width-2, styleError) {
			lines = append(lines, line)
		}
	}
	if m.projectResult == nil {
		return append(lines, "Projects not loaded. Press r to scan.")
	}

	result := m.projectResult
	summary := fmt.Sprintf("  %d workspaces  ·  %d projects", len(result.Workspaces), len(result.Projects))
	if m.projectsLoading {
		summary += "  " + styleMuted.Render(m.spinner.View()+" rescanning...")
	}
	lines = append(lines, styleMuted.Render(summary))

	lines = append(lines, "", styleLabel.Render("WORKSPACES"))
	if len(result.Workspaces) == 0 {
		lines = append(lines, "  "+styleMuted.Render("none found"))
	}
	for _, workspace := range result.Workspaces {
		line := "  " + styleValue.Render(workspace.Name) + "  " +
			styleMuted.Render("["+string(workspace.Kind)+"] "+workspace.Path)
		line += styleFaint.Render(fmt.Sprintf("  (%d projects)", len(workspace.ProjectPaths)))
		lines = append(lines, line)
	}

	lines = append(lines, "", styleLabel.Render("PROJECTS"))
	if len(result.Projects) == 0 {
		lines = append(lines, "  "+styleMuted.Render("No .csproj files found."))
	} else {
		lines = append(lines, "    "+styleFaint.Render(fmt.Sprintf("%-22s %-28s %s", "NAME", "FRAMEWORKS", "PATH")))
	}
	for index, project := range result.Projects {
		marker := " "
		name := fmt.Sprintf("%-22s", project.Name)
		if index == m.projectCursor {
			marker = styleAccent.Render(">")
			name = styleValue.Render(name)
		}
		frameworks := valueOr(strings.Join(project.TargetFrameworks, ", "), "none")
		line := "  " + marker + " " + name + " " + styleMuted.Render(fmt.Sprintf("%-28s", frameworks))
		if project.TestProject {
			line += " " + styleOK.Render("[test]")
		}
		line += " " + styleFaint.Render(project.Path)
		lines = append(lines, line)
	}

	if project, ok := m.selectedProject(); ok {
		lines = append(lines, "")
		lines = append(lines, projectDetailPanel(project, width)...)
	}

	if len(result.Warnings) > 0 {
		lines = append(lines, "", styleLabel.Render("WARNINGS"))
		for _, warning := range result.Warnings {
			for _, line := range wrap(warning, width-2) {
				lines = append(lines, styleWarning.Render("  ! "+line))
			}
		}
	}
	return lines
}

func projectDetailPanel(project projects.Project, width int) []string {
	content := []string{
		kvSized("Path", project.Path, 12),
		kvSized("Frameworks", valueOr(strings.Join(project.TargetFrameworks, ", "), "none"), 12),
		kvSized("SDK", valueOr(project.SDK, "unknown"), 12),
		kvSized("Output", valueOr(project.OutputType, "default"), 12),
	}
	if project.TestProject {
		content = append(content, kvSized("Test project", styleOK.Render("yes"), 12))
	}
	if len(project.ProjectReferences) > 0 {
		content = append(content, kvSized("References", fmt.Sprintf("%d", len(project.ProjectReferences)), 12))
		for _, reference := range project.ProjectReferences {
			for _, line := range wrap("  "+reference, width-6) {
				content = append(content, styleMuted.Render(line))
			}
		}
	}
	return panelBox("SELECTED PROJECT", content, width)
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
	lines = append(lines, styleMuted.Render(fmt.Sprintf("  %d host  ·  %d SDKs  ·  %d runtimes",
		len(inventory.Hosts), len(inventory.SDKs), len(inventory.Runtimes))))

	lines = append(lines, "")
	if m.bodyHeight() >= 26 {
		toolchain := []string{}
		switch {
		case m.selection != nil:
			toolchain = append(toolchain,
				kvSized("SDK", fmt.Sprintf("%s  %s", styleValue.Render(m.selection.SDKVersion), provenanceText(m.selection.EffectiveProvenance())), 11),
				kvSized("Root", m.selection.Root, 11),
			)
			if m.selection.GlobalJSONPath != "" {
				toolchain = append(toolchain, kvSized("global.json", m.selection.GlobalJSONPath, 11))
			}
			if m.selection.Rationale != "" {
				for _, line := range wrap(m.selection.Rationale, width-6) {
					toolchain = append(toolchain, styleMuted.Render(line))
				}
			}
		case m.selectionErr != nil:
			for _, line := range styledWrapped(m.selectionErr.Error(), width-6, styleWarning) {
				toolchain = append(toolchain, line)
			}
		default:
			toolchain = append(toolchain, styleMuted.Render("not resolved"))
		}
		lines = append(lines, panelBox("SELECTED TOOLCHAIN", toolchain, width)...)
	} else {
		switch {
		case m.selection != nil:
			lines = append(lines, "  "+styleLabel.Render("TOOLCHAIN")+"  "+styleValue.Render(m.selection.SDKVersion)+"  "+provenanceText(m.selection.EffectiveProvenance()))
			lines = append(lines, "  "+styleLabel.Render("ROOT")+"       "+styleMuted.Render(m.selection.Root))
			if m.selection.GlobalJSONPath != "" {
				lines = append(lines, "  "+styleLabel.Render("GLOBAL")+"     "+styleMuted.Render(m.selection.GlobalJSONPath))
			}
		case m.selectionErr != nil:
			lines = append(lines, styledWrapped(m.selectionErr.Error(), width-4, styleWarning)...)
		default:
			lines = append(lines, "  "+styleMuted.Render("not resolved"))
		}
	}

	lines = append(lines, "", styleLabel.Render("HOSTS"))
	if len(inventory.Hosts) == 0 {
		lines = append(lines, "  "+styleMuted.Render("none detected"))
	}
	for _, host := range inventory.Hosts {
		line := "  " + styleValue.Render(host.Root) + "  " + styleMuted.Render(fmt.Sprintf("[%s] %s", host.Source, host.EffectiveProvenance()))
		if host.Architecture != "" {
			line += styleFaint.Render("  arch=" + host.Architecture)
		}
		lines = append(lines, line)
		if host.ProbeError != "" {
			lines = append(lines, "    "+styleWarning.Render("probe: "+host.ProbeError))
		}
	}

	lines = append(lines, "", listLabel("SDKS", m.sdkFocus == focusSDKs))
	if len(inventory.SDKs) == 0 {
		lines = append(lines, "  "+styleMuted.Render("None detected.")+" Press i to install a managed SDK.")
	} else {
		lines = append(lines, "    "+styleFaint.Render(fmt.Sprintf("%-12s %-10s %s", "VERSION", "SOURCE", "LOCATION")))
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
			marker = styleAccent.Render(">")
		}
		if m.selection != nil && sdk.Root == m.selection.Root && sdk.Version == m.selection.SDKVersion {
			marker = styleOK.Render("*")
		}
		version := fmt.Sprintf("%-12s", sdk.Version)
		if index == m.sdkCursor && m.sdkFocus == focusSDKs {
			version = styleValue.Render(version)
		}
		line := "  " + marker + " " + version + " " + provenanceCell(sdk.EffectiveProvenance()) + " " + styleMuted.Render(sdk.Path) + state
		if update, ok := updateByVersion[sdk.Version]; ok && update.UpdateAvailable {
			line += "  " + styleAccent.Render("→ "+update.LatestVersion+" available")
		}
		lines = append(lines, line)
	}

	lines = append(lines, "", listLabel("RUNTIMES", m.sdkFocus == focusRuntimes))
	runtimes := m.orderedRuntimes()
	if len(runtimes) == 0 {
		lines = append(lines, "  "+styleMuted.Render("None detected.")+" Press tab then i to install a managed runtime.")
	} else {
		lines = append(lines, "    "+styleFaint.Render(fmt.Sprintf("%-24s %-12s %s", "FRAMEWORK", "VERSION", "SOURCE")))
	}
	runtimeUpdates := make(map[string]install.Update, len(m.runtimeUpdates))
	for _, update := range m.runtimeUpdates {
		runtimeUpdates[update.Version] = update
	}
	for index, runtime := range runtimes {
		marker := " "
		if index == m.runtimeCursor && m.sdkFocus == focusRuntimes {
			marker = styleAccent.Render(">")
		}
		version := fmt.Sprintf("%-12s", runtime.Version)
		if index == m.runtimeCursor && m.sdkFocus == focusRuntimes {
			version = styleValue.Render(version)
		}
		line := "  " + marker + " " + fmt.Sprintf("%-24s", runtime.Framework) + " " + version + " " + provenanceCell(runtime.EffectiveProvenance())
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
		title := "INSTALL SDK"
		if m.prompt == promptInstallRuntime {
			title = "INSTALL RUNTIME"
		}
		input := m.promptInput
		if input == "" {
			input = styleFaint.Render("latest, lts, or exact version")
		}
		label := styleLabel.Render("version")
		cursor := ""
		if m.prompt != promptInstallSDK || m.promptFocusInput {
			cursor = styleAccent.Render("▌")
		}
		if m.prompt == promptInstallSDK && m.promptFocusInput {
			label = styleAccent.Render("version")
		}
		content := []string{label + "  " + input + cursor}
		if m.prompt == promptInstallSDK {
			content = append(content, m.availableSDKPromptLines(width-4)...)
		}
		if m.prompt == promptInstallRuntime {
			content = append(content, styleMuted.Render("framework "+string(m.promptFramework))+"   "+styleFaint.Render("←/→ change"))
		}
		lines = append(lines, "")
		lines = append(lines, calloutBox(title, content, width)...)
		return lines
	}
	if m.confirm != confirmNone {
		content := make([]string, 0, len(m.confirmSummary)+1)
		for _, summary := range m.confirmSummary {
			for index, line := range wrap(summary, width-8) {
				if index == 0 {
					content = append(content, styleAccent.Render("• ")+line)
				} else {
					content = append(content, "  "+line)
				}
			}
		}
		lines = append(lines, "")
		lines = append(lines, calloutBox("CONFIRM "+confirmTitle(m.confirm), content, width)...)
		return lines
	}
	if m.operationRunning {
		content := []string{m.spinner.View() + " " + styleValue.Render(m.operationLabel)}
		if m.operationCancelling {
			content = append(content, "  "+styleWarning.Render("Cancelling..."))
		}
		if m.progressTotal > 0 || m.progressDone > 0 {
			content = append(content, downloadProgressLine(m.progressDone, m.progressTotal, width-4))
		}
		start := 0
		if len(m.progressLines) > 8 {
			start = len(m.progressLines) - 8
		}
		for _, progress := range m.progressLines[start:] {
			content = append(content, "  "+styleMuted.Render(progress))
		}
		lines = append(lines, "")
		lines = append(lines, calloutBox("IN PROGRESS", content, width)...)
		return lines
	}
	if m.loadingUpdates {
		return append(lines, "", m.spinner.View()+" checking managed updates...")
	}
	return lines
}

func downloadProgressLine(downloaded, total int64, width int) string {
	if total <= 0 {
		return styleMuted.Render(humanSize(downloaded) + " downloaded")
	}
	if downloaded > total {
		downloaded = total
	}
	percent := int(downloaded * 100 / total)
	label := fmt.Sprintf("%3d%%  %s / %s", percent, humanSize(downloaded), humanSize(total))
	barWidth := width - lipgloss.Width(label) - 3
	if barWidth < 4 {
		return styleMuted.Render(label)
	}
	filled := int(int64(barWidth) * downloaded / total)
	bar := styleOK.Render(strings.Repeat("█", filled)) + styleFaint.Render(strings.Repeat("░", barWidth-filled))
	return "[" + bar + "] " + styleMuted.Render(label)
}

func humanSize(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := int64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func (m Model) availableSDKPromptLines(width int) []string {
	if m.availableLoading {
		return []string{styleMuted.Render(m.spinner.View() + " loading available SDKs...")}
	}
	if m.availableErr != nil {
		lines := []string{styleError.Render("Could not load available SDKs.")}
		for _, line := range wrap(m.availableErr.Error(), width) {
			lines = append(lines, styleMuted.Render(line))
		}
		return lines
	}

	available := m.downloadableSDKs()
	if len(available) == 0 {
		message := "No downloadable SDKs were found for this platform."
		if len(m.availableSDKs) > 0 {
			message = "Every downloadable SDK for this platform is already installed."
		}
		return []string{"", styleMuted.Render(message)}
	}

	const visible = 6
	start := 0
	if m.availableCursor >= visible {
		start = m.availableCursor - visible + 1
	}
	end := start + visible
	if end > len(available) {
		end = len(available)
	}
	suffix := styleFaint.Render("  ·  installed hidden")
	lines := []string{
		"",
		styleLabel.Render(fmt.Sprintf("AVAILABLE  %d", len(available))) + suffix,
	}
	for index := start; index < end; index++ {
		sdk := available[index]
		marker := "  "
		version := styleValue.Render(fmt.Sprintf("%-14s", sdk.Version))
		if index == m.availableCursor && !m.promptFocusInput {
			marker = styleAccent.Render("> ")
			version = styleSelected.Render(fmt.Sprintf("%-14s", sdk.Version))
		}
		row := marker + version + " " + styleMuted.Render(fmt.Sprintf("%-5s", sdk.Channel)) + " " + styleMuted.Render(fmt.Sprintf("%-4s", sdk.ReleaseType))
		if sdk.Latest {
			row += " " + styleOK.Render("latest")
		}
		lines = append(lines, row)
	}
	if end < len(available) || start > 0 {
		lines = append(lines, styleFaint.Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(available))))
	}
	if m.promptFocusInput {
		lines = append(lines, styleFaint.Render("tab select from list · enter install"))
	} else {
		lines = append(lines, styleFaint.Render("↑/↓ select · enter install · tab type"))
	}
	return lines
}

func confirmTitle(kind confirmKind) string {
	switch kind {
	case confirmInstallSDK:
		return "INSTALL SDK"
	case confirmInstallRuntime:
		return "INSTALL RUNTIME"
	case confirmRemoveSDK:
		return "REMOVE SDK"
	case confirmRemoveRuntime:
		return "REMOVE RUNTIME"
	case confirmUpdateSDK:
		return "UPDATE SDK"
	case confirmUpdateRuntime:
		return "UPDATE RUNTIME"
	default:
		return "ACTION"
	}
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

func listLabel(label string, focused bool) string {
	if !focused {
		return styleLabel.Render(label)
	}
	return styleSelected.Render(label) + "  " + styleAccent.Render("(focused)")
}

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

func (m Model) diagnosticLines(width int) []string {
	lines := sectionHeader("Diagnostics", width)
	if m.loadingDoctor && m.report == nil {
		return append(lines, m.spinner.View()+" running checks...")
	}
	if m.report == nil {
		return append(lines, "No diagnostics report yet. Press r to run hilt doctor.")
	}

	report := m.report
	lines = append(lines, fmt.Sprintf("%s %d   %s %d   %s %d   %s %d",
		styleOK.Render("ok"), report.Summary.OK,
		styleInfo.Render("info"), report.Summary.Info,
		styleWarning.Render("warn"), report.Summary.Warnings,
		styleError.Render("error"), report.Summary.Errors))
	if m.loadingDoctor {
		lines = append(lines, m.spinner.View()+" re-running checks...")
	}

	lines = append(lines, "", styleLabel.Render("FINDINGS"))
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
	pathContent := []string{
		kv("Configuration", paths.ConfigFile()),
		kv("State", paths.StateFile()),
		kv("Cache", paths.CacheDir),
		kv("Data", paths.DataDir),
		kv("Managed root", config.ManagedRoot),
	}
	lines = append(lines, panelBox("PATHS", pathContent, width)...)
	preferenceContent := []string{
		kv("Default project dir", valueOr(config.DefaultProjectDir, "not set")),
		kv("Editor", valueOr(config.Editor, "not set")),
		kv("Theme", valueOr(config.UI.Theme, "auto")),
		kv("Vim keys", fmt.Sprintf("%t", config.UI.VimKeys)),
		kv("Scan exclusions", strings.Join(config.ScanExclusions, ", ")),
	}
	for _, line := range wrap("Settings editing is read-only. Edit the configuration file directly.", width-6) {
		preferenceContent = append(preferenceContent, styleMuted.Render(line))
	}
	lines = append(lines, panelBox("PREFERENCES", preferenceContent, width)...)
	if len(m.backend.ConfigWarnings()) > 0 {
		lines = append(lines, "")
		content := make([]string, 0, len(m.backend.ConfigWarnings()))
		for _, warning := range m.backend.ConfigWarnings() {
			for index, line := range wrap(warning, width-6) {
				if index == 0 {
					line = "! " + line
				} else {
					line = "  " + line
				}
				content = append(content, styleWarning.Render(line))
			}
		}
		lines = append(lines, panelBox("CONFIGURATION WARNINGS", content, width)...)
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
