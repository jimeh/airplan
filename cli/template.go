package cli

import (
	"fmt"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

func newTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "template [document|collection]",
		Short: "Print a built-in page template",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := airplan.BuiltinTemplate()
			if len(args) == 1 {
				switch args[0] {
				case "document":
				case "collection":
					src = airplan.BuiltinCollectionTemplate()
				default:
					return fmt.Errorf("unknown template kind %q", args[0])
				}
			}
			if strings.HasSuffix(src, "\n") {
				_, err := fmt.Fprint(cmd.OutOrStdout(), src)
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), src)
			return err
		},
	}
}
