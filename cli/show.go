package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type showOptions struct {
	config  string
	profile string
	json    bool
}

func newShowCmd() *cobra.Command {
	opts := &showOptions{}
	cmd := &cobra.Command{
		Use:           "show <url|key>",
		Short:         "Inspect one remote upload marker",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, opts, args[0])
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile (default: config default)")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print one JSON object instead of a detail block")
	return cmd
}

func runShow(cmd *cobra.Command, opts *showOptions, target string) error {
	client, _, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile,
	)
	if err != nil {
		return err
	}
	defer cancel()

	inspection, err := client.InspectUpload(ctx, target)
	if err != nil {
		return err
	}
	for _, warning := range inspection.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(
			showJSONFromInspection(inspection),
		)
	}
	return printInspection(cmd.OutOrStdout(), inspection)
}

type showJSONObject struct {
	Key    string `json:"key"`
	URL    string `json:"url"`
	Exists bool   `json:"exists"`
	Bytes  *int64 `json:"bytes,omitempty"`
}

type showJSONRecord struct {
	State     airplan.UploadState     `json:"state"`
	Dir       string                  `json:"dir"`
	MarkerKey string                  `json:"marker_key"`
	Objects   int                     `json:"objects"`
	Bytes     int64                   `json:"bytes"`
	Time      *time.Time              `json:"time,omitempty"`
	Format    string                  `json:"format,omitempty"`
	Version   int                     `json:"marker_version,omitempty"`
	Title     string                  `json:"title,omitempty"`
	Repo      string                  `json:"repo,omitempty"`
	Page      *showJSONObject         `json:"page,omitempty"`
	Source    *showJSONObject         `json:"source,omitempty"`
	Error     airplan.MarkerErrorCode `json:"error,omitempty"`
}

func showJSONFromInspection(in *airplan.UploadInspection) showJSONRecord {
	out := showJSONRecord{
		State: in.State, Dir: in.Dir, MarkerKey: in.MarkerKey,
		Objects: in.Objects, Bytes: in.Bytes, Error: in.Error,
	}
	if in.State != airplan.UploadInvalid {
		t := in.CreatedAt
		out.Time = &t
		out.Format = in.Format
		out.Version = in.MarkerVersion
		out.Title = in.Title
		out.Repo = in.Repo
		out.Page = showJSONFromObject(in.Page)
		out.Source = showJSONFromObject(in.Source)
	}
	return out
}

func showJSONFromObject(in *airplan.InspectedObject) *showJSONObject {
	if in == nil {
		return nil
	}
	out := &showJSONObject{Key: in.Key, URL: in.URL, Exists: in.Exists}
	if in.Exists {
		bytes := in.Bytes
		out.Bytes = &bytes
	}
	return out
}

func printInspection(w io.Writer, in *airplan.UploadInspection) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	write := func(label string, value any) error {
		_, err := fmt.Fprintf(tw, "%s\t%v\n", label, value)
		return err
	}
	for _, field := range []struct {
		label string
		value any
	}{
		{"STATE", in.State},
		{"DIRECTORY", in.Dir},
		{"MARKER", in.MarkerKey},
		{"OBJECTS", in.Objects},
		{"SIZE", formatListBytes(in.Bytes)},
	} {
		if err := write(field.label, field.value); err != nil {
			return err
		}
	}
	if in.State == airplan.UploadInvalid {
		if err := write("ERROR", in.Error); err != nil {
			return err
		}
		return tw.Flush()
	}

	if err := write("DATE", in.CreatedAt.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if err := write("FORMAT", in.Format); err != nil {
		return err
	}
	if err := write("MARKER VERSION", in.MarkerVersion); err != nil {
		return err
	}
	title := in.Title
	if title == "" {
		title = "-"
	}
	if err := write("TITLE", title); err != nil {
		return err
	}
	repo := in.Repo
	if repo == "" {
		repo = "-"
	}
	if err := write("REPOSITORY", repo); err != nil {
		return err
	}
	if err := printInspectedObject(tw, "PAGE", in.Page); err != nil {
		return err
	}
	if in.Source != nil {
		if err := printInspectedObject(tw, "SOURCE", in.Source); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printInspectedObject(
	w io.Writer, label string, object *airplan.InspectedObject,
) error {
	status := "missing"
	if object.Exists {
		status = formatListBytes(object.Bytes)
	}
	if _, err := fmt.Fprintf(w, "%s\t%s (%s)\n", label, object.Key, status); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%s URL\t%s\n", label, object.URL)
	return err
}
