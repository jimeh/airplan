package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listOptions struct {
	json bool
}

func newListCmd() *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List uploads from the local manifest",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	records, warnings, err := airplan.ReadManifest("")
	if err != nil {
		return err
	}

	stderr := cmd.ErrOrStderr()
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}

	uploads := airplan.ActiveUploads(records)
	if opts.json {
		if uploads == nil {
			uploads = []airplan.ManifestRecord{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(uploads)
	}

	return printUploadTable(cmd.OutOrStdout(), uploads)
}

func printUploadTable(
	w io.Writer,
	uploads []airplan.ManifestRecord,
) error {
	if len(uploads) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "DATE\tTITLE\tSIZE\tURL"); err != nil {
		return err
	}
	for _, upload := range uploads {
		title := upload.Title
		if title == "" {
			title = "-"
		}

		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			upload.Time.UTC().Format("2006-01-02 15:04"),
			title,
			formatListBytes(upload.Bytes),
			upload.URL,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatListBytes(bytes int64) string {
	return fmt.Sprintf("%d B", bytes)
}
