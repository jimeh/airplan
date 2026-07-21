package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

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
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List uploads from the local manifest",
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
		"filter local history, or select config profile for --remote")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")
	// SPEC.md §6 defines -r as the list --remote shorthand.
	f.BoolVarP(&opts.remote, "remote", "r", false,
		"list uploads from a live bucket listing instead of the manifest")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	if opts.remote {
		return runRemoteList(cmd, opts)
	}
	if cmd.Flags().Changed("config") {
		return fmt.Errorf("--config requires --remote")
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil {
		return err
	}

	stderr := cmd.ErrOrStderr()
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}

	uploads := airplan.ManifestUploads(records)
	if cmd.Flags().Changed("profile") {
		filtered := uploads[:0]
		for _, upload := range uploads {
			if upload.Profile == opts.profile {
				filtered = append(filtered, upload)
			}
		}
		uploads = filtered
	}
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
	if cfg.PublicBaseURL == "" {
		for _, upload := range uploads {
			if upload.URL != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n",
					airplan.PublicURLFallbackWarning)
				break
			}
		}
	}

	if opts.json {
		if uploads == nil {
			uploads = []airplan.RemoteUpload{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(
			remoteListJSONRecords(uploads),
		)
	}

	return printRemoteUploadTable(cmd.OutOrStdout(), uploads)
}

type remoteListJSONRecord struct {
	Time      time.Time          `json:"time"`
	Dir       string             `json:"dir"`
	MarkerKey string             `json:"marker_key"`
	Objects   int                `json:"objects"`
	Bytes     int64              `json:"bytes"`
	Slug      string             `json:"slug,omitempty"`
	Key       string             `json:"key,omitempty"`
	URL       string             `json:"url,omitempty"`
	Kind      airplan.UploadKind `json:"kind,omitempty"`
	Conflict  bool               `json:"conflict,omitempty"`
}

func remoteListJSONRecords(
	uploads []airplan.RemoteUpload,
) []remoteListJSONRecord {
	records := make([]remoteListJSONRecord, 0, len(uploads))
	for _, upload := range uploads {
		records = append(records, remoteListJSONRecord{
			Time:      upload.LastModified.UTC(),
			Dir:       upload.Dir,
			MarkerKey: upload.MarkerKey,
			Objects:   upload.Objects,
			Bytes:     upload.Bytes,
			Slug:      upload.Slug,
			Key:       upload.Key,
			URL:       upload.URL,
			Kind:      upload.Kind,
			Conflict:  upload.Conflict,
		})
	}
	return records
}

func printRemoteUploadTable(w io.Writer, uploads []airplan.RemoteUpload) error {
	if len(uploads) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(
		tw, "DATE\tKIND\tOBJECTS\tSIZE\tSLUG\tDIRECTORY\tURL",
	); err != nil {
		return err
	}
	for _, upload := range uploads {
		slug := upload.Slug
		if slug == "" {
			slug = "-"
		}
		url := upload.URL
		if url == "" {
			url = "-"
		}
		kind := string(upload.Kind)
		if upload.Conflict {
			kind = "conflict"
		}
		if kind == "" {
			kind = "-"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			upload.LastModified.UTC().Format("2006-01-02 15:04"),
			kind,
			upload.Objects,
			formatListBytes(upload.Bytes),
			slug,
			upload.Dir,
			url,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printUploadTable(
	w io.Writer,
	uploads []airplan.ManifestRecord,
) error {
	if len(uploads) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(
		tw, "DATE\tPROFILE\tSTATE\tTITLE\tSIZE\tURL",
	); err != nil {
		return err
	}
	for _, upload := range uploads {
		title := upload.Title
		if title == "" {
			title = "-"
		}
		profile := upload.Profile
		if profile == "" {
			profile = "<root>"
		}
		state := "legacy"
		if airplan.IsSupportedMarkerVersion(upload.MarkerVersion) {
			state = "managed"
		}

		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			upload.Time.UTC().Format("2006-01-02 15:04"),
			profile,
			state,
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
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	size := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	for i, unit := range units {
		size /= 1024
		rounded := math.Round(size*10) / 10
		if rounded < 1024 || i == len(units)-1 {
			value := strings.TrimSuffix(fmt.Sprintf("%.1f", rounded), ".0")
			return value + " " + unit
		}
	}

	return fmt.Sprintf("%d B", bytes)
}
