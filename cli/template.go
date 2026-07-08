package cli

import (
	"fmt"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

func newTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "template",
		Short: "Print the built-in page template",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			src := airplan.BuiltinTemplate()
			if strings.HasSuffix(src, "\n") {
				_, err := fmt.Fprint(cmd.OutOrStdout(), src)
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), src)
			return err
		},
	}
}
