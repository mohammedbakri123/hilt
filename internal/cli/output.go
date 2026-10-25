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

func printSDKs(writer io.Writer, inventory dotnet.Inventory) {
	if len(inventory.SDKs) == 0 {
		fmt.Fprintln(writer, "No .NET SDKs detected.")
		return
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "VERSION\tSOURCE\tUSABLE\tPATH")
	for _, sdk := range inventory.SDKs {
		usable := "yes"
		if !sdk.Usable {
			usable = "no"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", sdk.Version, sdk.EffectiveProvenance(), usable, sdk.Path)
	}
	_ = table.Flush()
}

func printRuntimes(writer io.Writer, inventory dotnet.Inventory) {
	if len(inventory.Runtimes) == 0 {
		fmt.Fprintln(writer, "No .NET runtimes detected.")
		return
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "FRAMEWORK\tVERSION\tSOURCE\tPATH")
	for _, runtime := range inventory.Runtimes {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n",
			runtime.Framework, runtime.Version, runtime.EffectiveProvenance(), runtime.Path)
	}
	_ = table.Flush()
}
