package cli

import (
	"github.com/spf13/cobra"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

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
