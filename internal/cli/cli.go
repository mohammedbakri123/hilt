package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mohammedbakri123/hilt/internal/config"
	"github.com/mohammedbakri123/hilt/internal/diagnostics"
	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/platform"
)

// Backend is the application surface used by the CLI.
type Backend interface {
	Version() string
	Platform() platform.Info
	Paths() platform.Paths
	Config() config.Config
	ConfigWarnings() []string
	PackageManager() *platform.PackageManager
	Inventory(ctx context.Context) (dotnet.Inventory, error)
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
	)
	return root
}

func versionLabel(options Options) string {
	version := options.Backend.Version()
	if options.Commit != "" && options.Commit != "unknown" {
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

			if asJSON {
				normalizeInventory(&inventory)
				if err := writeJSON(options.Out, struct {
					SchemaVersion int `json:"schemaVersion"`
					dotnet.Inventory
				}{SchemaVersion: schemaVersion, Inventory: inventory}); err != nil {
					return failure(err)
				}
				return nil
			}

			if kind == inventorySDKs {
				printSDKs(options.Out, inventory)
			} else {
				printRuntimes(options.Out, inventory)
			}
			for _, warning := range inventory.Warnings {
				fmt.Fprintln(options.Err, "warning:", warning)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "write machine-readable JSON")
	return command
}
