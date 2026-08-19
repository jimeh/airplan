package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

func runDocumentBundlePreview(cmd *cobra.Command, args []string, opts *previewOptions) error {
	if opts.files || len(args) != 1 || args[0] == "-" {
		return errors.New("airplan: bundle preview requires exactly one named entry file")
	}
	if opts.outputDir == "" {
		return errors.New("airplan: bundle preview requires --output-dir")
	}
	if cmd.Flags().Changed("collection-template") {
		return errors.New("airplan: --collection-template is only valid for collection previews")
	}
	maxPage, err := parsePreviewSize("max-size", opts.maxSize)
	if err != nil {
		return err
	}
	maxTotalPages, err := parsePreviewSize("max-total-page-size", opts.maxTotalPageSize)
	if err != nil {
		return err
	}
	maxAsset, err := parsePreviewSize("max-asset-size", opts.maxAssetSize)
	if err != nil {
		return err
	}
	maxAssets, err := parsePreviewSize("max-total-size", opts.maxTotalSize)
	if err != nil {
		return err
	}
	overrides := airplan.Settings{Template: opts.template, Repository: opts.repository, MermaidURL: airplan.ResolveMermaidURLOverride(opts.mermaidURL, cmd.Flags().Changed("mermaid-url"))}
	if cmd.Flags().Changed("indexable") {
		overrides.Indexable = &opts.indexable
	}
	if cmd.Flags().Changed("no-external-assets") {
		overrides.NoExternalAssets = &opts.noExternalAssets
	}
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{Path: opts.config, Profile: opts.profile, Overrides: overrides})
	if err != nil {
		return err
	}
	ctx, cancel := previewContext(cmd, cfg)
	defer cancel()
	opened, err := openDocumentBundle(args[0], opts.pages, opts.assets)
	if err != nil {
		return err
	}
	defer opened.close()
	opened.input.Entry.Format = opts.format
	opened.input.Entry.Title = opts.title
	opened.input.Entry.Lang = opts.lang
	opened.input.Slug = opts.slug
	opened.input.Title = opts.title
	opened.input.MaxPageSize = maxPage
	opened.input.MaxTotalPageSize = maxTotalPages
	opened.input.MaxAssetSize = maxAsset
	opened.input.MaxTotalSize = maxAssets
	warnings, err := airplan.MaterializeDocument(ctx, opened.input,
		airplan.DocumentRenderOptions{RenderInputOptions: airplan.RenderInputOptions{
			Indexable: cfg.Indexable, IncludeSource: !cfg.NoSource,
			TemplatePath: cfg.Template, NoExternalAssets: cfg.NoExternalAssets,
			MermaidURL: cfg.MermaidURL, Repository: cfg.Repository,
			Themes: cfg.ThemeBundle,
		}}, opts.outputDir)
	if err != nil {
		return err
	}
	printPreviewWarnings(cmd, warnings)
	return nil
}

func parsePreviewSize(name, value string) (int64, error) {
	parsed, err := airplan.ParseSize(value)
	if err != nil {
		return 0, fmt.Errorf("--%s: %s", name, strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if parsed == 0 {
		return -1, nil
	}
	return parsed, nil
}
