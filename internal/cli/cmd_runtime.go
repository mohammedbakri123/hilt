package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mohammedbakri123/hilt/internal/install"
)

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
