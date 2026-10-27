package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) homeLines(width int) []string {
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

	projectRoot := m.backend.Config().DefaultProjectDir
	if projectRoot == "" {
		projectRoot = m.backend.Paths().ProjectsRoot
	}
	if m.projectResult != nil && m.projectResult.Root != "" {
		projectRoot = m.projectResult.Root
	}
	if projectRoot == "" {
		projectRoot = "not configured"
	}
	projectContent := []string{kvSized("Projects root", projectRoot, 14)}
	currentState := m.backend.State()
	if currentState.ActiveProject != "" {
		projectContent = append(projectContent, kvSized("Active project", currentState.ActiveProject, 14))
	}
	if len(currentState.RecentProjects) > 0 {
		projectContent = append(projectContent, kvSized("Recent projects", fmt.Sprintf("%d", len(currentState.RecentProjects)), 14))
		for index, recent := range currentState.RecentProjects {
			if index == 3 {
				break
			}
			projectContent = append(projectContent, "  "+styleMuted.Render(filepath.Base(recent.Path)))
		}
	}
	switch {
	case m.projectsLoading:
		projectContent = append(projectContent, m.spinner.View()+" scanning projects...")
	case m.projectsErr != nil:
		projectContent = append(projectContent, styleError.Render("Scan failed: "+m.projectsErr.Error()))
	case m.projectResult != nil:
		projectContent = append(projectContent, kvSized("Last scan", fmt.Sprintf("%d projects  ·  %d workspaces", len(m.projectResult.Projects), len(m.projectResult.Workspaces)), 14))
		if project, ok := m.selectedProject(); ok {
			projectContent = append(projectContent, kvSized("Selected project", project.Name, 14))
		}
	default:
		projectContent = append(projectContent, kvSized("Next step", "Press 2 to scan projects", 14))
	}
	lines = append(lines, "")
	lines = append(lines, panelBox("PROJECTS", projectContent, width)...)

	actions := []string{
		styleAccent.Render("2") + "  " + styleValue.Render("Projects") + "  " + styleMuted.Render("scan, inspect, build, or run"),
		styleAccent.Render("3") + "  " + styleValue.Render("SDKs") + "  " + styleMuted.Render("install and manage toolchains"),
		styleAccent.Render("4") + "  " + styleValue.Render("Diagnostics") + "  " + styleMuted.Render("check the local environment"),
		styleFaint.Render("click a sidebar item or press its number"),
	}
	lines = append(lines, "")
	lines = append(lines, panelBox("QUICK ACTIONS", actions, width)...)

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

func (m Model) renderFooter(width int) string {
	hints := footerBindings(m.footerContext(), width)
	parts := make([]string, 0, len(hints))
	separator := "  "
	if width < 120 {
		separator = " "
	}
	for _, hint := range hints {
		parts = append(parts, styleKey.Render(hint.Key)+" "+styleMuted.Render(hint.Label))
	}
	return fitLine(" "+strings.Join(parts, separator), width)
}

func (m Model) renderHelp(width, height int) string {
	content := []string{styleTitle.Render("Keyboard shortcuts")}
	sections := []struct {
		title   string
		context keyContext
	}{
		{title: "NAVIGATION", context: ctxHelpNavigation},
		{title: "MANAGED .NET", context: ctxHelpManaged},
		{title: "VIEW", context: ctxHelpView},
	}
	for _, section := range sections {
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
