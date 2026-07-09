package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newPurgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "purge",
		Short:         "Not implemented yet",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
