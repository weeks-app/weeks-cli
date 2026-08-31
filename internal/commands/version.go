package commands

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/output"
	"github.com/weeks-app/weeks-cli/internal/version"
)

// NewVersionCmd builds `weeks version`.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, commit, and build of this binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			return app.Out.OK(map[string]any{
				"version": version.Version,
				"commit":  version.Commit,
				"date":    version.Date,
				"go":      runtime.Version(),
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			}, output.WithSummary("weeks "+version.Version))
		},
	}
}
