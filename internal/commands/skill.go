package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/skills"
)

// SkillPath is the embedded skill's path inside the binary.
const SkillPath = "weeks/SKILL.md"

// NewSkillCmd builds `weeks skill`.
func NewSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill embedded in this binary",
		Long: "Print the SKILL.md that teaches an agent to use this CLI.\n\n" +
			"Any agent can bootstrap from this output alone. `weeks setup claude` writes the same\n" +
			"document where Claude Code will find it without being asked.",
		Args: cobra.NoArgs,
		// The skill is a document, not data: it is Markdown on stdout in every
		// format mode, so `weeks skill > SKILL.md` does what it looks like.
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := skills.FS.ReadFile(SkillPath)
			if err != nil {
				return fmt.Errorf("reading the embedded skill: %w", err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}
