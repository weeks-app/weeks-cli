package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/harness"
	"github.com/weeks-app/weeks-cli/internal/output"
	"github.com/weeks-app/weeks-cli/skills"
)

// NewSetupCmd builds `weeks setup`.
func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install the weeks agent skill into an agent harness",
		Long: "Write the embedded skill where an agent harness will find it.\n\n" +
			"A CLI without a skill is one the agent rediscovers every session.",
	}
	cmd.AddCommand(newSetupClaudeCmd())
	return cmd
}

func newSetupClaudeCmd() *cobra.Command {
	var force bool

	return withForceFlag(&cobra.Command{
		Use:   "claude",
		Short: "Install the skill for Claude Code",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)

			dir := harness.SkillDir()
			if dir == "" {
				return output.ErrUsage("cannot locate a home directory to install into")
			}

			target := filepath.Join(dir, "SKILL.md")
			existed := fileExists(target)
			if existed && !force {
				return &output.Error{
					Code:    output.CodeUsage,
					Message: target + " already exists",
					Hint:    "Pass --force to overwrite it with this binary's version.",
				}
			}

			data, err := skills.FS.ReadFile(SkillPath)
			if err != nil {
				return fmt.Errorf("reading the embedded skill: %w", err)
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", target, err)
			}

			verb := "Installed"
			if existed {
				verb = "Updated"
			}

			opts := []output.ResponseOption{
				output.WithSummary(fmt.Sprintf("%s the weeks skill at %s.", verb, target)),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "verify", Cmd: "weeks doctor --json", Description: "Confirm the harness can see it",
				}),
			}
			if !harness.DetectClaude() {
				opts = append(opts, output.WithNotice(
					"Claude Code does not appear to be installed here; the skill is in place for when it is."))
			}

			return app.Out.OK(map[string]any{
				"harness": "claude",
				"path":    target,
				"updated": existed,
			}, opts...)
		},
	}, &force)
}

func withForceFlag(cmd *cobra.Command, force *bool) *cobra.Command {
	cmd.Flags().BoolVar(force, "force", false, "Overwrite an existing skill file")
	return cmd
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
