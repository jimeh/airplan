package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listOptions struct {
	config  string
	profile string
	json    bool
	remote  bool
}

func newListCmd() *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List uploads from the local manifest",
		Long: "List uploads from the local manifest, or with --remote, " +
			"from a live bucket listing using the selected config profile.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path for --remote (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile for --remote (default: config default)")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")
	f.BoolVar(&opts.remote, "remote", false,
		"list uploads from a live bucket listing instead of the manifest")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	if opts.remote {
		return runRemoteList(cmd, opts)
	}

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

func runRemoteList(cmd *cobra.Command, opts *listOptions) error {
	client, cfg, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	defer cancel()

	uploads, err := client.ListRemote(ctx)
	if err != nil {
		return err
	}

	records := remoteListRecords(ctx, cmd, cfg, client, uploads)
	if opts.json {
		if records == nil {
			records = []airplan.ManifestRecord{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(records)
	}

	return printUploadTable(cmd.OutOrStdout(), records)
}

func remoteListRecords(
	ctx context.Context,
	cmd *cobra.Command,
	cfg *airplan.Config,
	client *airplan.Client,
	uploads []airplan.RemoteUpload,
) []airplan.ManifestRecord {
	records := make([]airplan.ManifestRecord, 0, len(uploads))
	for _, upload := range uploads {
		title, err := client.RemoteTitle(ctx, upload.PageKey)
		if err != nil {
			title = "-"
			fmt.Fprintf(cmd.ErrOrStderr(),
				"airplan: warning: title unavailable for %s: %s\n",
				upload.PageKey, err)
		}
		url, _ := airplan.PublicURL(cfg, upload.PageKey)
		records = append(records, airplan.ManifestRecord{
			Type:   "upload",
			Time:   upload.LastModified.UTC(),
			Key:    upload.PageKey,
			URL:    url,
			Bucket: cfg.Bucket,
			Title:  title,
			Bytes:  upload.Bytes,
		})
	}
	return records
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
