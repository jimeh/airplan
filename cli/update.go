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
	config, profile  string
	title, maxSize   string
	maxTotalPageSize string
	maxAssetSize     string
	maxTotalSize     string
	pages            []string
	assets           []string
	entrypoint       string
	json, open       bool
}

type updateJSONResult struct {
	jsonResult
	Pages          []airplan.PageResult  `json:"pages,omitempty"`
	Assets         []airplan.AssetResult `json:"assets,omitempty"`
	Revision       int                   `json:"revision,omitempty"`
	LatestRevision int                   `json:"latest_revision,omitempty"`
	PreviousURL    string                `json:"previous_url,omitempty"`
	DiffURL        string                `json:"diff_url,omitempty"`
	Unchanged      bool                  `json:"unchanged"`
}

func newUpdateCmd() *cobra.Command {
	opts := &updateOptions{}
	cmd := &cobra.Command{
		Use:     "new-revision <url|key> [file ...]",
		Aliases: []string{"update"},
		Short:   "Upload a new linked Markdown revision",
		Args:    cobra.MinimumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error { return runUpdate(cmd, args, opts) },
	}
	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "", "config file path")
	f.StringVarP(&opts.profile, "profile", "p", "", "config profile name")
	f.StringVarP(&opts.title, "title", "t", "", "page title for the new revision")
	f.StringVar(&opts.maxSize, "max-size", "10MiB", "input limit; 0 = no limit")
	f.StringVar(&opts.maxTotalPageSize, "max-total-page-size", "100MiB", "managed-source total limit; 0 = no limit")
	f.StringVar(&opts.maxAssetSize, "max-asset-size", "1GiB", "per-asset limit; 0 = no limit")
	f.StringVar(&opts.maxTotalSize, "max-total-size", "2GiB", "asset total limit; 0 = no limit")
	f.StringArrayVar(&opts.pages, "page", nil, "managed page file (repeatable; overrides inferred role)")
	f.StringArrayVar(&opts.assets, "asset", nil, "supporting asset file (repeatable; overrides inferred role)")
	f.StringVar(&opts.entrypoint, "entrypoint", "", "document entry file")
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
	maxTotalPages, err := parsePreviewSize("max-total-page-size", opts.maxTotalPageSize)
	if err != nil {
		return err
	}
	maxAsset, err := parsePreviewSize("max-asset-size", opts.maxAssetSize)
	if err != nil {
		return err
	}
	maxTotal, err := parsePreviewSize("max-total-size", opts.maxTotalSize)
	if err != nil {
		return err
	}
	document := airplan.DocumentInput{
		Title: opts.title, MaxPageSize: maxSize,
		MaxTotalPageSize: maxTotalPages, MaxAssetSize: maxAsset,
		MaxTotalSize: maxTotal,
	}
	stdin := len(args) == 1 || (len(args) == 2 && args[1] == "-")
	if stdin && opts.entrypoint == "" && len(opts.pages) == 0 && len(opts.assets) == 0 {
		document.Entry = airplan.PageInput{
			Reader: cmd.InOrStdin(), Format: "md",
			Title: opts.title,
		}
	} else {
		plan, planErr := airplan.PlanLocalPaths(airplan.LocalPathPlanOptions{
			Paths: args[1:], Entrypoint: opts.entrypoint,
			PagePaths: opts.pages, AssetPaths: opts.assets,
			ForceDocument: true, EntryFormat: "md",
		})
		if planErr != nil {
			return planErr
		}
		opened, openErr := openDocumentBundle(
			plan.Entrypoint, plan.PagePaths, plan.AssetPaths,
		)
		if openErr != nil {
			return openErr
		}
		defer opened.close()
		document = opened.input
		document.Entry.Format = "md"
		document.Entry.Title = opts.title
		document.Title = opts.title
		document.MaxPageSize = maxSize
		document.MaxTotalPageSize = maxTotalPages
		document.MaxAssetSize = maxAsset
		document.MaxTotalSize = maxTotal
	}
	result, err := client.CreateDocumentRevision(ctx,
		airplan.CreateDocumentRevisionInput{Target: args[0], Document: document})
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
			Pages: result.Pages, Assets: result.Assets,
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
