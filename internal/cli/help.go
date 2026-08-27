package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/commands"
)

// installAgentHelp makes `--help --agent` answer with structured JSON instead
// of prose laid out for a terminal.
//
// Help is where an agent looks first, so the flag that switches output to JSON
// switches help too — one rule across the whole surface rather than an
// exception to learn.
//
// Unlike a data command, help is emitted as a bare object rather than wrapped
// in the {ok, data, …} envelope. That is the 37signals toolkit's convention,
// and it is load-bearing: scripts/check-cli-surface.sh and the toolkit's
// rubric-check and surface-compat actions all read `.flags` and `.subcommands`
// off the top level. Wrapping it would leave weeks-cli nominally on the
// toolkit with none of its tooling working.
func installAgentHelp(root *cobra.Command, flags *rootFlags) {
	defaultHelp := root.HelpFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !flags.agent && !flags.json {
			defaultHelp(cmd, args)
			return
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(commands.Describe(cmd))
	})
}
