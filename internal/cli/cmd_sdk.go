package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/mohammedbakri123/hilt/internal/dotnet"
	"github.com/mohammedbakri123/hilt/internal/install"
)

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
