package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/install"
	"github.com/mohammedbakri123/hilt/internal/platform"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

// Backend is the application surface used by the CLI.
type Backend interface {
	Version() string
	Platform() platform.Info
	Paths() platform.Paths
	Config() config.Config
	ConfigWarnings() []string
	State() state.State
	RecordProjectOpen(projectPath, workspacePath string) error
	PackageManager() *platform.PackageManager
	Inventory(ctx context.Context) (dotnet.Inventory, error)
	Resolve(ctx context.Context, inventory dotnet.Inventory) (dotnet.Selection, error)
	PlanSDKInstall(ctx context.Context, version string, allowPrerelease bool) (install.Plan, error)
	InstallSDK(ctx context.Context, plan install.Plan, progress io.Writer) (install.Result, error)
	RemoveSDK(ctx context.Context, version string) (install.Removal, error)
	CheckSDKUpdates(ctx context.Context) ([]install.Update, error)
	AvailableSDKs(ctx context.Context, allowPrerelease bool) ([]install.AvailableSDK, error)
	UpdateSDK(ctx context.Context, version string, removeOld bool, progress io.Writer) (install.UpdateResult, error)
	PlanRuntimeInstall(ctx context.Context, version string, framework install.FrameworkFlavor, allowPrerelease bool) (install.RuntimePlan, error)
	InstallRuntime(ctx context.Context, plan install.RuntimePlan, progress io.Writer) (install.RuntimeResult, error)
	RemoveRuntime(ctx context.Context, version string) (install.Removal, error)
	CheckRuntimeUpdates(ctx context.Context) ([]install.Update, error)
	ScanProjects(ctx context.Context, root string, includeHidden bool) (projects.Result, error)
	ListDirectories(ctx context.Context, path string, includeHidden bool) ([]string, error)
	SetDefaultProjectDir(path string) error
	InspectProject(ctx context.Context, path string) (projects.Inspection, error)
	BuildProject(ctx context.Context, request execution.BuildRequest, stdout, stderr io.Writer) (execution.Result, error)
	RecordBuildTarget(target state.BuildTarget) error
	Diagnose(ctx context.Context) diagnostics.Report
	Tool(name string) (string, bool)
}

// Options injects process dependencies so every command can be tested.
type Options struct {
	Backend    Backend
	Commit     string
	Date       string
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	IsTerminal func() bool
	RunTUI     func(Backend) error
}

func (o Options) withDefaults() Options {
	if o.In == nil {
		o.In = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Err == nil {
		o.Err = os.Stderr
	}
	if o.IsTerminal == nil {
		o.IsTerminal = defaultIsTerminal
	}
	return o
}

func defaultIsTerminal() bool {
	for _, file := range []*os.File{os.Stdin, os.Stdout} {
		info, err := file.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit code %d", e.code)
	}
	return e.err.Error()
}

func usageError(message string) error {
	return &exitError{code: 2, err: errors.New(message)}
}

func failure(err error) error {
	if err == nil {
		err = errors.New("operation failed")
	}
	return &exitError{code: 1, err: err}
}

// Run executes a command line and returns the process exit code.
func Run(ctx context.Context, args []string, options Options) int {
	options = options.withDefaults()
	if ctx == nil {
		ctx = context.Background()
	}

	root := newRoot(options)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if ctx.Err() != nil {
		fmt.Fprintln(options.Err, "hilt: cancelled")
		return 130
	}

	var exit *exitError
	if errors.As(err, &exit) {
		if exit.code == 130 {
			fmt.Fprintln(options.Err, "hilt: cancelled")
			return 130
		}
		fmt.Fprintf(options.Err, "hilt: %s\n", exit.Error())
		if exit.code == 2 {
			fmt.Fprintln(options.Err, "Run 'hilt --help' for usage.")
		}
		return exit.code
	}

	fmt.Fprintf(options.Err, "hilt: %s\n", err)
	fmt.Fprintln(options.Err, "Run 'hilt --help' for usage.")
	return 2
}

func newRoot(options Options) *cobra.Command {
	root := &cobra.Command{
		Use:           "hilt",
		Short:         "Manage .NET SDKs, projects, packages, and templates",
		Long:          "Hilt is a terminal UI and CLI for managing the .NET ecosystem.",
		Version:       versionLabel(options),
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !options.IsTerminal() {
				return usageError("the interactive TUI requires a terminal; run 'hilt --help' for commands")
			}
			if options.RunTUI == nil {
				return failure(errors.New("the TUI is not available in this build"))
			}
			if err := options.RunTUI(options.Backend); err != nil {
				return failure(err)
			}
			return nil
		},
	}
	root.SetVersionTemplate("hilt {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetOut(options.Out)
	root.SetErr(options.Err)
	root.SetFlagErrorFunc(func(command *cobra.Command, err error) error {
		return usageError(err.Error())
	})

	root.AddCommand(
		newVersionCommand(options),
		newDoctorCommand(options),
		newSDKCommand(options),
		newRuntimeCommand(options),
		newProjectsCommand(options),
	)
	return root
}

func versionLabel(options Options) string {
	version := options.Backend.Version()
	if options.Commit != "" && options.Commit != "unknown" && options.Commit != version {
		return version + " (" + options.Commit + ")"
	}
	return version
}

func newVersionCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "version",
		Short: "Print Hilt version and platform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := options.Backend.Platform()
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int           `json:"schemaVersion"`
					Version       string        `json:"version"`
					Commit        string        `json:"commit,omitempty"`
					Date          string        `json:"date,omitempty"`
					Platform      platform.Info `json:"platform"`
				}{
					SchemaVersion: schemaVersion,
					Version:       options.Backend.Version(),
					Commit:        options.Commit,
					Date:          options.Date,
					Platform:      info,
				})
			}
			fmt.Fprintf(options.Out, "hilt %s\n", versionLabel(options))
			fmt.Fprintf(options.Out, "platform %s · %s\n", info.PlatformLabel(), info.DisplayName())
			if options.Commit != "" {
				fmt.Fprintf(options.Out, "commit %s\n", options.Commit)
			}
			if options.Date != "" {
				fmt.Fprintf(options.Out, "built %s\n", options.Date)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newDoctorCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Check the host, configuration, and .NET installations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			report := options.Backend.Diagnose(cmd.Context())
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if asJSON {
				normalizeReport(&report)
				if err := writeJSON(options.Out, struct {
					SchemaVersion int `json:"schemaVersion"`
					diagnostics.Report
				}{SchemaVersion: schemaVersion, Report: report}); err != nil {
					return failure(err)
				}
			} else {
				printDoctor(options.Out, options.color(), report)
			}
			if report.Summary.Errors > 0 {
				return &exitError{code: 1, err: fmt.Errorf("doctor found %d error(s)", report.Summary.Errors)}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newSDKCommand(options Options) *cobra.Command {
	command := &cobra.Command{
		Use:   "sdk",
		Short: "Manage .NET SDKs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("specify a subcommand: hilt sdk list")
		},
	}
	command.AddCommand(newInventoryListCommand(options, inventorySDKs, "list", "List installed .NET SDKs"))
	command.AddCommand(newSDKWhichCommand(options))
	command.AddCommand(newSDKAvailableCommand(options))
	command.AddCommand(newSDKInstallCommand(options))
	command.AddCommand(newSDKRemoveCommand(options))
	command.AddCommand(newSDKOutdatedCommand(options))
	command.AddCommand(newSDKUpdateCommand(options))
	return command
}

func newProjectsCommand(options Options) *cobra.Command {
	command := &cobra.Command{
		Use:   "projects",
		Short: "Discover and inspect .NET projects and solutions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("specify a subcommand: hilt projects list")
		},
	}
	command.AddCommand(newProjectsListCommand(options))
	command.AddCommand(newProjectsInspectCommand(options))
	return command
}

func newProjectsListCommand(options Options) *cobra.Command {
	var asJSON bool
	var root string
	var includeHidden bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List discovered projects and solutions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			result, err := options.Backend.ScanProjects(cmd.Context(), root, includeHidden)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if result.Workspaces == nil {
				result.Workspaces = []projects.Workspace{}
			}
			if result.Projects == nil {
				result.Projects = []projects.Project{}
			}
			if result.Warnings == nil {
				result.Warnings = []string{}
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int `json:"schemaVersion"`
					projects.Result
				}{SchemaVersion: schemaVersion, Result: result})
			}
			printProjectList(options.Out, result)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	command.Flags().StringVar(&root, "path", "", "directory or project/solution file to scan (default: current directory)")
	command.Flags().BoolVar(&includeHidden, "all", false, "include hidden files and directories")
	return command
}

func newProjectsInspectCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "inspect <path>",
		Short: "Inspect one project or solution file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			inspection, err := options.Backend.InspectProject(cmd.Context(), args[0])
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if inspection.Projects == nil {
				inspection.Projects = []projects.Project{}
			}
			if inspection.Warnings == nil {
				inspection.Warnings = []string{}
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int                 `json:"schemaVersion"`
					Inspection    projects.Inspection `json:"inspection"`
				}{SchemaVersion: schemaVersion, Inspection: inspection})
			}
			printProjectInspection(options.Out, inspection)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newSDKAvailableCommand(options Options) *cobra.Command {
	var asJSON bool
	var allowPrerelease bool
	var includeInstalled bool
	command := &cobra.Command{
		Use:   "available",
		Short: "List downloadable .NET SDKs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			available, err := options.Backend.AvailableSDKs(cmd.Context(), allowPrerelease)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if !includeInstalled {
				inventory, inventoryErr := options.Backend.Inventory(cmd.Context())
				if inventoryErr != nil {
					if cmd.Context().Err() != nil {
						return &exitError{code: 130, err: inventoryErr}
					}
					return failure(inventoryErr)
				}
				available = filterInstalledSDKs(available, inventory)
			}
			if available == nil {
				available = []install.AvailableSDK{}
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int                    `json:"schemaVersion"`
					SDKs          []install.AvailableSDK `json:"sdks"`
				}{SchemaVersion: schemaVersion, SDKs: available})
			}
			printAvailableSDKs(options.Out, available)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	command.Flags().BoolVar(&allowPrerelease, "prerelease", false, "include prerelease SDKs")
	command.Flags().BoolVar(&includeInstalled, "all", false, "include versions already installed")
	return command
}

func filterInstalledSDKs(available []install.AvailableSDK, inventory dotnet.Inventory) []install.AvailableSDK {
	installed := make(map[string]bool, len(inventory.SDKs))
	for _, sdk := range inventory.SDKs {
		installed[sdk.Version] = true
	}
	filtered := make([]install.AvailableSDK, 0, len(available))
	for _, sdk := range available {
		if installed[sdk.Version] {
			continue
		}
		filtered = append(filtered, sdk)
	}
	return filtered
}

func newSDKInstallCommand(options Options) *cobra.Command {
	var asJSON bool
	var allowPrerelease bool
	command := &cobra.Command{
		Use:   "install <version>",
		Short: "Install a managed .NET SDK",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if _, err := install.ParseVersionRequest(args[0], allowPrerelease); err != nil {
				return usageError(err.Error())
			}
			plan, err := options.Backend.PlanSDKInstall(cmd.Context(), args[0], allowPrerelease)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}

			if asJSON {
				result, err := options.Backend.InstallSDK(cmd.Context(), plan, io.Discard)
				if err != nil {
					if cmd.Context().Err() != nil {
						return &exitError{code: 130, err: err}
					}
					return failure(err)
				}
				return writeJSON(options.Out, struct {
					SchemaVersion int            `json:"schemaVersion"`
					Install       install.Result `json:"install"`
				}{SchemaVersion: schemaVersion, Install: result})
			}

			printInstallPlan(options.Out, plan)
			result, err := options.Backend.InstallSDK(cmd.Context(), plan, options.Err)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			printInstallResult(options.Out, result)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	command.Flags().BoolVar(&allowPrerelease, "prerelease", false, "allow exact or channel prerelease versions")
	return command
}

func newSDKRemoveCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "remove <version>",
		Short: "Remove a managed .NET SDK",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if _, err := install.ValidateVersion(args[0]); err != nil {
				return usageError(err.Error())
			}
			removal, err := options.Backend.RemoveSDK(cmd.Context(), args[0])
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int             `json:"schemaVersion"`
					Removal       install.Removal `json:"removal"`
				}{SchemaVersion: schemaVersion, Removal: removal})
			}
			printRemoval(options.Out, removal)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newSDKOutdatedCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "outdated",
		Short: "Check managed .NET SDKs for updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			updates, err := options.Backend.CheckSDKUpdates(cmd.Context())
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if updates == nil {
				updates = []install.Update{}
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int              `json:"schemaVersion"`
					Updates       []install.Update `json:"updates"`
				}{SchemaVersion: schemaVersion, Updates: updates})
			}
			printUpdates(options.Out, updates)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newRuntimeCommand(options Options) *cobra.Command {
	command := &cobra.Command{
		Use:   "runtime",
		Short: "Manage .NET runtimes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError("specify a subcommand: hilt runtime list")
		},
	}
	command.AddCommand(newInventoryListCommand(options, inventoryRuntimes, "list", "List installed .NET runtimes"))
	command.AddCommand(newRuntimeInstallCommand(options))
	command.AddCommand(newRuntimeRemoveCommand(options))
	command.AddCommand(newRuntimeOutdatedCommand(options))
	return command
}

func newRuntimeInstallCommand(options Options) *cobra.Command {
	var asJSON bool
	var framework string
	var allowPrerelease bool
	command := &cobra.Command{
		Use:   "install <version>",
		Short: "Install a managed .NET runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if _, err := install.ParseVersionRequest(args[0], allowPrerelease); err != nil {
				return usageError(err.Error())
			}
			flavor := install.FrameworkFlavor(strings.TrimSpace(framework))
			if !flavor.Valid() {
				return usageError(fmt.Sprintf("invalid framework %q: expected all, netcore, or aspnetcore", framework))
			}
			plan, err := options.Backend.PlanRuntimeInstall(cmd.Context(), args[0], flavor, allowPrerelease)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}

			if asJSON {
				result, err := options.Backend.InstallRuntime(cmd.Context(), plan, io.Discard)
				if err != nil {
					if cmd.Context().Err() != nil {
						return &exitError{code: 130, err: err}
					}
					return failure(err)
				}
				return writeJSON(options.Out, struct {
					SchemaVersion int                   `json:"schemaVersion"`
					Install       install.RuntimeResult `json:"install"`
				}{SchemaVersion: schemaVersion, Install: result})
			}

			printRuntimeInstallPlan(options.Out, plan)
			result, err := options.Backend.InstallRuntime(cmd.Context(), plan, options.Err)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			printRuntimeInstallResult(options.Out, result)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	command.Flags().StringVar(&framework, "framework", string(install.FrameworkAll), "runtime framework flavor: all, netcore, or aspnetcore")
	command.Flags().BoolVar(&allowPrerelease, "prerelease", false, "allow exact or channel prerelease versions")
	return command
}

func newRuntimeRemoveCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "remove <version>",
		Short: "Remove a managed .NET runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if _, err := install.ValidateVersion(args[0]); err != nil {
				return usageError(err.Error())
			}
			removal, err := options.Backend.RemoveRuntime(cmd.Context(), args[0])
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int             `json:"schemaVersion"`
					Removal       install.Removal `json:"removal"`
				}{SchemaVersion: schemaVersion, Removal: removal})
			}
			printRemoval(options.Out, removal)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newRuntimeOutdatedCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "outdated",
		Short: "Check managed .NET runtimes for updates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			updates, err := options.Backend.CheckRuntimeUpdates(cmd.Context())
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if updates == nil {
				updates = []install.Update{}
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int              `json:"schemaVersion"`
					Updates       []install.Update `json:"updates"`
				}{SchemaVersion: schemaVersion, Updates: updates})
			}
			printRuntimeUpdates(options.Out, updates)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func newSDKUpdateCommand(options Options) *cobra.Command {
	var asJSON bool
	var removeOld bool
	command := &cobra.Command{
		Use:   "update <version>",
		Short: "Update a managed .NET SDK to the newest stable release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			if _, err := install.ValidateVersion(args[0]); err != nil {
				return usageError(err.Error())
			}
			result, err := options.Backend.UpdateSDK(cmd.Context(), args[0], removeOld, cmd.OutOrStdout())
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				if asJSON {
					_ = writeJSON(options.Out, struct {
						SchemaVersion int                  `json:"schemaVersion"`
						Update        install.UpdateResult `json:"update"`
					}{SchemaVersion: schemaVersion, Update: result})
				} else {
					printSDKUpdateResult(options.Out, result)
				}
				return failure(err)
			}
			if asJSON {
				return writeJSON(options.Out, struct {
					SchemaVersion int                  `json:"schemaVersion"`
					Update        install.UpdateResult `json:"update"`
				}{SchemaVersion: schemaVersion, Update: result})
			}
			printSDKUpdateResult(options.Out, result)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	command.Flags().BoolVar(&removeOld, "remove-old", false, "remove the previous SDK after a successful update")
	return command
}

func newSDKWhichCommand(options Options) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:     "which",
		Aliases: []string{"current"},
		Short:   "Show the selected .NET SDK toolchain",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			inventory, err := options.Backend.Inventory(cmd.Context())
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			selection, err := options.Backend.Resolve(cmd.Context(), inventory)
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if asJSON {
				if err := writeJSON(options.Out, struct {
					SchemaVersion int `json:"schemaVersion"`
					dotnet.Selection
				}{SchemaVersion: schemaVersion, Selection: selection}); err != nil {
					return failure(err)
				}
				return nil
			}
			printSelection(options.Out, selection)
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

type inventoryKind int

const (
	inventorySDKs inventoryKind = iota
	inventoryRuntimes
)

func newInventoryListCommand(options Options, kind inventoryKind, use, short string) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}
			inventory, err := options.Backend.Inventory(cmd.Context())
			if err != nil {
				if cmd.Context().Err() != nil {
					return &exitError{code: 130, err: err}
				}
				return failure(err)
			}
			if err := cmd.Context().Err(); err != nil {
				return &exitError{code: 130, err: err}
			}

			var selection *dotnet.Selection
			var selectionErr error
			if kind == inventorySDKs {
				resolved, err := options.Backend.Resolve(cmd.Context(), inventory)
				if err != nil {
					if cmd.Context().Err() != nil {
						return &exitError{code: 130, err: err}
					}
					selectionErr = err
				} else {
					selection = &resolved
				}
			}

			if asJSON {
				normalizeInventory(&inventory)
				payload := struct {
					SchemaVersion int `json:"schemaVersion"`
					dotnet.Inventory
					Selection      *dotnet.Selection `json:"selection,omitempty"`
					SelectionError string            `json:"selectionError,omitempty"`
				}{
					SchemaVersion: schemaVersion,
					Inventory:     inventory,
					Selection:     selection,
				}
				if selectionErr != nil {
					payload.SelectionError = selectionErr.Error()
				}
				if err := writeJSON(options.Out, payload); err != nil {
					return failure(err)
				}
				return nil
			}

			if kind == inventorySDKs {
				printSDKs(options.Out, inventory, selection)
			} else {
				printRuntimes(options.Out, inventory)
			}
			for _, warning := range inventory.Warnings {
				fmt.Fprintln(options.Err, "warning:", warning)
			}
			if warning := selectionWarning(selectionErr); warning != "" {
				fmt.Fprintln(options.Err, "warning:", warning)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}

func selectionWarning(err error) string {
	if err == nil {
		return ""
	}
	var missing *dotnet.MissingSDKError
	if errors.As(err, &missing) && missing.RequestedVersion == "" {
		return ""
	}
	return err.Error()
}
