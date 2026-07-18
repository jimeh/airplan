package cli

import (
	"io"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the airplan agent skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), airplan.AgentSkill())
			return err
		},
	}
}
