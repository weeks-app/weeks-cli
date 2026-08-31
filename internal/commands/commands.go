package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// NewCommandsCmd builds `weeks commands`, the catalog an agent reads once
// instead of carrying a tool list in its context forever.
func NewCommandsCmd() *cobra.Command {
	var flat bool

	cmd := &cobra.Command{
		Use:   "commands",
		Short: "List every command this binary offers",
		Long: "Print the full command catalog, derived from this binary's own command tree.\n\n" +
			"This is the discovery surface: an agent calls it once, learns the whole vocabulary,\n" +
			"and spends no context carrying a tool list it may not need. Because the catalog is\n" +
			"read from the live tree rather than a hand-kept list, it cannot drift from the binary.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			catalog := Catalog(cmd.Root())

			var data any = catalog
			if flat {
				paths := make([]string, 0, len(catalog))
				for _, c := range catalog {
					paths = append(paths, c.Path)
				}
				data = paths
			}

			return app.Out.OK(data,
				output.WithSummary(fmt.Sprintf("%d commands.", len(catalog))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "learn", Cmd: "weeks skill", Description: "Print the agent skill that teaches this CLI"},
					output.Breadcrumb{Action: "help", Cmd: "weeks <command> --help --agent", Description: "Structured help for one command"},
					output.Breadcrumb{Action: "diagnose", Cmd: "weeks doctor --json", Description: "Check config, credentials, and connectivity"},
				),
			)
		},
	}

	cmd.Flags().BoolVar(&flat, "flat", false, "List command paths only, without flags or descriptions")
	return cmd
}
