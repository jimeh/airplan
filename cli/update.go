package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type updateOptions struct {
	config, profile string
	title, maxSize  string
	json, open      bool
}

type updateJSONResult struct {
	jsonResult
	Revision       int    `json:"revision,omitempty"`
	LatestRevision int    `json:"latest_revision,omitempty"`
	PreviousURL    string `json:"previous_url,omitempty"`
	DiffURL        string `json:"diff_url,omitempty"`
	Unchanged      bool   `json:"unchanged"`
}

func newUpdateCmd() *cobra.Command {
	opts := &updateOptions{}
	cmd := &cobra.Command{
		Use:   "update <url|key> [markdown-file]",
		Short: "Upload a new linked Markdown revision",
		Args:  cobra.RangeArgs(1, 2), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error { return runUpdate(cmd, args, opts) },
	}
	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "", "config file path")
	f.StringVarP(&opts.profile, "profile", "p", "", "config profile name")
	f.StringVarP(&opts.title, "title", "t", "", "page title for the new revision")
	f.StringVar(&opts.maxSize, "max-size", "10MiB", "input limit; 0 = no limit")
	f.BoolVarP(&opts.json, "json", "j", false, "print one JSON result")
	f.BoolVarP(&opts.open, "open", "o", false, "open the new revision URL")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string, opts *updateOptions) error {
	client, _, ctx, cancel, err := setupTargetClient(cmd, opts.config, opts.profile, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	maxSize, err := airplan.ParseSize(opts.maxSize)
	if err != nil {
		return fmt.Errorf("--max-size: %s", strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxSize == 0 {
		maxSize = -1
	}
	in := airplan.Input{Format: "md", Title: opts.title, MaxSize: maxSize}
	if len(args) == 1 || args[1] == "-" {
		in.Reader = cmd.InOrStdin()
	} else {
		file, _, openErr := openRegularInput(args[1], "update input")
		if openErr != nil {
			return openErr
		}
		defer func() { _ = file.Close() }()
		in.Reader, in.Name = file, args[1]
	}
	result, err := client.UpdateDocument(ctx, airplan.UpdateDocumentInput{
		Target: args[0], Input: in,
	})
	if err != nil {
		if errors.Is(err, airplan.ErrInputTooLarge) {
			return fmt.Errorf("%w (raise or remove the limit with --max-size)", err)
		}
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}
	if opts.json {
		out := updateJSONResult{
			jsonResult: jsonResult{
				URL: result.URL, Key: result.Key, SourceURL: result.SourceURL,
				Bucket: result.Bucket, Bytes: result.Bytes,
				ContentType: result.ContentType,
			},
			Revision: result.Revision, LatestRevision: result.LatestRevision,
			PreviousURL: result.PreviousURL, DiffURL: result.DiffURL,
			Unchanged: result.Unchanged,
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(out); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintln(cmd.OutOrStdout(), result.URL); err != nil {
		return err
	}
	if opts.open {
		if err := openBrowser(result.URL); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: could not open browser: %s\n", err)
		}
	}
	return nil
}
