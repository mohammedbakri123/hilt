package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/install"
)

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
	for _, line := range wrap("Settings editing is read-only. Press p in Projects to change the scan root, or edit the configuration file directly.", width-6) {
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
	case confirmCreateProjects:
		return "CREATE PROJECT"
	case confirmDeleteProject:
		return "DELETE PROJECT FILE"
	default:
		return "ACTION"
	}
}

func listLabel(label string, focused bool) string {
	if !focused {
		return styleLabel.Render(label)
	}
	return styleSelected.Render(label) + "  " + styleAccent.Render("(focused)")
}
