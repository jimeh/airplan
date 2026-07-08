package cli

import (
	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect airplan configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "schema",
		Short: "Print the config file JSON Schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := airplan.ConfigSchema()
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	})

	return cmd
}
