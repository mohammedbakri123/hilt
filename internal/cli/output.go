package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
)

const schemaVersion = 1

func (o Options) color() bool {
	return o.IsTerminal() && os.Getenv("NO_COLOR") == ""
}

func writeJSON(writer io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err = fmt.Fprintln(writer, string(data))
	return err
}

func normalizeInventory(inventory *dotnet.Inventory) {
	if inventory.Hosts == nil {
		inventory.Hosts = []dotnet.Host{}
	}
	if inventory.SDKs == nil {
		inventory.SDKs = []dotnet.SDK{}
	}
	if inventory.Runtimes == nil {
		inventory.Runtimes = []dotnet.Runtime{}
	}
	if inventory.Warnings == nil {
		inventory.Warnings = []string{}
	}
}

func normalizeReport(report *diagnostics.Report) {
	if report.Findings == nil {
		report.Findings = []diagnostics.Finding{}
	}
}

func printDoctor(writer io.Writer, color bool, report diagnostics.Report) {
	fmt.Fprintf(writer, "Hilt %s doctor\n", report.HiltVersion)
	fmt.Fprintf(writer, "Platform: %s · %s\n", report.Platform.PlatformLabel(), report.Platform.DisplayName())
	fmt.Fprintf(writer, "Summary: %d ok · %d info · %d warning · %d error\n\n",
		report.Summary.OK, report.Summary.Info, report.Summary.Warnings, report.Summary.Errors)

	for _, finding := range report.SortedFindings() {
		fmt.Fprintf(writer, "%s %s\n", badge(finding.Severity, color), finding.Title)
		if finding.Summary != "" {
			fmt.Fprintf(writer, "  %s\n", finding.Summary)
		}
		for _, line := range strings.Split(finding.Detail, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			fmt.Fprintf(writer, "  %s\n", line)
		}
		if finding.Remediation != "" {
			fmt.Fprintf(writer, "  fix: %s\n", finding.Remediation)
		}
	}
}

func badge(severity diagnostics.Severity, color bool) string {
	label := "[OK]"
	code := "32"
	switch severity {
	case diagnostics.SeverityError:
		label = "[ERROR]"
		code = "31"
	case diagnostics.SeverityWarning:
		label = "[WARN]"
		code = "33"
	case diagnostics.SeverityInfo:
		label = "[INFO]"
		code = "36"
	}
	if !color {
		return label
	}
	return "\x1b[" + code + "m" + label + "\x1b[0m"
}

func printSDKs(writer io.Writer, inventory dotnet.Inventory, selection *dotnet.Selection) {
	if len(inventory.SDKs) == 0 {
		fmt.Fprintln(writer, "No .NET SDKs detected.")
		return
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "VERSION\tSOURCE\tOWNERSHIP\tSELECTED\tUSABLE\tPATH")
	for _, sdk := range inventory.SDKs {
		usable := "yes"
		if !sdk.Usable {
			usable = "no"
		}
		selected := ""
		if selection != nil && sdk.Root == selection.Root && sdk.Version == selection.SDKVersion {
			selected = "*"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
			sdk.Version, sourceName(inventory, sdk.Root, sdk.Source), sdk.EffectiveProvenance(), selected, usable, sdk.Path)
	}
	_ = table.Flush()
}

func printRuntimes(writer io.Writer, inventory dotnet.Inventory) {
	if len(inventory.Runtimes) == 0 {
		fmt.Fprintln(writer, "No .NET runtimes detected.")
		return
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "FRAMEWORK\tVERSION\tSOURCE\tOWNERSHIP\tPATH")
	for _, runtime := range inventory.Runtimes {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			runtime.Framework, runtime.Version, sourceName(inventory, runtime.Root, runtime.Source),
			runtime.EffectiveProvenance(), runtime.Path)
	}
	_ = table.Flush()
}

func printSelection(writer io.Writer, selection dotnet.Selection) {
	fmt.Fprintln(writer, "Selected .NET toolchain")
	fmt.Fprintf(writer, "  SDK         %s\n", selection.SDKVersion)
	fmt.Fprintf(writer, "  Root        %s\n", selection.Root)
	fmt.Fprintf(writer, "  dotnet      %s\n", selection.DotnetPath)
	fmt.Fprintf(writer, "  Source      %s\n", valueOr(string(selection.Source), "unknown"))
	fmt.Fprintf(writer, "  Ownership   %s\n", selection.EffectiveProvenance())
	if selection.GlobalJSONPath != "" {
		fmt.Fprintf(writer, "  global.json %s\n", selection.GlobalJSONPath)
	}
	fmt.Fprintf(writer, "  Reason      %s\n", selection.Rationale)
	if len(selection.Attempts) > 0 {
		fmt.Fprintln(writer, "  Attempts")
		for _, attempt := range selection.Attempts {
			line := fmt.Sprintf("    %s (%s)", attempt.Root, attempt.Source)
			switch {
			case attempt.Error != "":
				line += ": " + attempt.Error
			case attempt.Version != "":
				line += ": " + attempt.Version
			}
			fmt.Fprintln(writer, line)
		}
	}
}

func sourceName(inventory dotnet.Inventory, root string, source dotnet.Source) string {
	if source != "" {
		return string(source)
	}
	for _, host := range inventory.Hosts {
		if host.Root == root && host.Source != "" {
			return string(host.Source)
		}
	}
	return "unknown"
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
