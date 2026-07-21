package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type previewOptions struct {
	format             string
	lang               string
	slug               string
	title              string
	indexable          bool
	noExternalAssets   bool
	mermaidURL         string
	repository         string
	maxSize            string
	template           string
	collectionTemplate string
	files              bool
	maxTotalSize       string
	profile            string
	config             string
	output             string
}

func newPreviewCmd() *cobra.Command {
	opts := &previewOptions{}
	cmd := &cobra.Command{
		Use:   "preview [flags] [file ...]",
		Short: "Render a document locally without uploading it",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreview(cmd, args, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "",
		"input format: md, html, or txt (default: auto-detect)")
	f.StringVar(&opts.lang, "lang", "",
		"highlight language for text input (default: from filename)")
	f.StringVarP(&opts.slug, "slug", "s", "",
		"page slug (default: from filename)")
	f.StringVarP(&opts.title, "title", "t", "",
		"page title (default: from content)")
	f.BoolVar(&opts.indexable, "indexable", false,
		"omit the noindex robots meta tag")
	f.BoolVar(&opts.noExternalAssets, "no-external-assets", false,
		"disable airplan-managed external assets in rendered pages")
	f.StringVar(&opts.mermaidURL, "mermaid-url", "",
		"Mermaid ECMAScript module URL")
	f.StringVar(&opts.repository, "repo", "",
		"repository context: auto, none, or URL (default: auto)")
	f.StringVar(&opts.maxSize, "max-size", "10MiB",
		"per-input limit (10MiB documents, 1GiB collections); 0 = no limit")
	f.StringVar(&opts.template, "template", "",
		"custom page template file (md and text input)")
	f.StringVar(&opts.collectionTemplate, "collection-template", "",
		"custom collection overview template file")
	f.BoolVar(&opts.files, "files", false,
		"render named inputs as a collection overview")
	f.StringVar(&opts.maxTotalSize, "max-total-size", "2GiB",
		"collection total size limit; 0 = no limit")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	// SPEC.md §6 defines -o as the preview --output shorthand.
	f.StringVarP(&opts.output, "output", "o", "",
		"write HTML to this path instead of stdout; - means stdout")
	return cmd
}

func runPreview(
	cmd *cobra.Command,
	args []string,
	opts *previewOptions,
) error {
	if opts.files || len(args) > 1 {
		return runCollectionPreview(cmd, args, opts)
	}
	for _, name := range []string{"collection-template", "max-total-size"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is only valid for collection previews", name)
		}
	}
	maxSize, err := airplan.ParseSize(opts.maxSize)
	if err != nil {
		return fmt.Errorf("--max-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxSize == 0 {
		maxSize = -1
	}

	overrides := airplan.Settings{
		Template:   opts.template,
		Repository: opts.repository,
		MermaidURL: airplan.ResolveMermaidURLOverride(
			opts.mermaidURL,
			cmd.Flags().Changed("mermaid-url"),
		),
	}
	if cmd.Flags().Changed("indexable") {
		overrides.Indexable = &opts.indexable
	}
	if cmd.Flags().Changed("no-external-assets") {
		overrides.NoExternalAssets = &opts.noExternalAssets
	}
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:      opts.config,
		Profile:   opts.profile,
		Overrides: overrides,
	})
	if err != nil {
		return err
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}
	ctx, cancel := timeoutContext(cmd.Context(), cfg)
	defer cancel()

	in := airplan.Input{
		Format:  opts.format,
		Lang:    opts.lang,
		Slug:    opts.slug,
		Title:   opts.title,
		MaxSize: maxSize,
	}
	if len(args) == 0 || args[0] == "-" {
		in.Reader = cmd.InOrStdin()
	} else {
		if opts.output != "" && opts.output != "-" {
			same, compareErr := samePreviewPath(args[0], opts.output)
			if compareErr != nil {
				return compareErr
			}
			if same {
				return errors.New(
					"--output must not overwrite the preview input",
				)
			}
		}
		file, openErr := os.Open(args[0])
		if openErr != nil {
			return openErr
		}
		defer func() { _ = file.Close() }()
		in.Reader = file
		in.Name = args[0]
	}
	doc, err := airplan.RenderInput(ctx, in,
		airplan.RenderInputOptions{
			Indexable:        cfg.Indexable,
			TemplatePath:     cfg.Template,
			NoExternalAssets: cfg.NoExternalAssets,
			MermaidURL:       cfg.MermaidURL,
			Repository:       cfg.Repository,
		})
	if err != nil {
		if errors.Is(err, airplan.ErrInputTooLarge) {
			return fmt.Errorf(
				"%w (raise or remove the limit with --max-size)", err,
			)
		}
		return err
	}
	for _, warning := range doc.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}

	if opts.output == "" || opts.output == "-" {
		_, err = cmd.OutOrStdout().Write(doc.HTML)
		return err
	}
	if err := writeFileAtomic(opts.output, doc.HTML, 0o644); err != nil {
		return fmt.Errorf("write preview %s: %w", opts.output, err)
	}
	return nil
}

func runCollectionPreview(cmd *cobra.Command, args []string, opts *previewOptions) error {
	if len(args) == 0 {
		return errors.New("--files requires one or more named files")
	}
	if len(args) > airplan.MaxCollectionFiles {
		return fmt.Errorf("collection has %d files; maximum is %d",
			len(args), airplan.MaxCollectionFiles)
	}
	for _, name := range args {
		if name == "-" {
			return errors.New("--files requires one or more named files")
		}
	}
	for _, name := range []string{"format", "lang", "slug", "template", "no-external-assets", "mermaid-url"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is only valid for document previews", name)
		}
	}
	maxSize, err := airplan.ParseSize(opts.maxSize)
	if err != nil {
		return fmt.Errorf("--max-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if !cmd.Flags().Changed("max-size") {
		maxSize = airplan.DefaultMaxCollectionFileSize
	}
	if maxSize == 0 {
		maxSize = -1
	}
	maxTotal, err := airplan.ParseSize(opts.maxTotalSize)
	if err != nil {
		return fmt.Errorf("--max-total-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxTotal == 0 {
		maxTotal = -1
	}
	overrides := airplan.Settings{CollectionTemplate: opts.collectionTemplate, Repository: opts.repository}
	if cmd.Flags().Changed("indexable") {
		overrides.Indexable = &opts.indexable
	}
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{Path: opts.config, Profile: opts.profile, Overrides: overrides})
	if err != nil {
		return err
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
	}
	ctx, cancel := timeoutContext(cmd.Context(), cfg)
	defer cancel()
	for _, path := range args {
		if opts.output != "" && opts.output != "-" {
			same, e := samePreviewPath(path, opts.output)
			if e != nil {
				return e
			}
			if same {
				return errors.New("--output must not overwrite a preview input")
			}
		}
	}
	inputs, closers, err := openCollectionInputs(args)
	if err != nil {
		return err
	}
	defer func() {
		for _, file := range closers {
			_ = file.Close()
		}
	}()
	body, _, err := airplan.RenderCollection(ctx, airplan.FilesInput{Files: inputs, Title: opts.title, MaxSize: maxSize, MaxTotalSize: maxTotal}, airplan.CollectionRenderOptions{Indexable: cfg.Indexable, TemplatePath: cfg.CollectionTemplate, Repository: cfg.Repository})
	if err != nil {
		return err
	}
	if opts.output == "" || opts.output == "-" {
		_, err = cmd.OutOrStdout().Write(body)
		return err
	}
	if err = writeFileAtomic(opts.output, body, 0o644); err != nil {
		return fmt.Errorf("write preview %s: %w", opts.output, err)
	}
	return nil
}

func samePreviewPath(input, output string) (bool, error) {
	inputPath, err := filepath.Abs(input)
	if err != nil {
		return false, fmt.Errorf("resolve preview input %s: %w", input, err)
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return false, fmt.Errorf("resolve preview output %s: %w", output, err)
	}
	if filepath.Clean(inputPath) == filepath.Clean(outputPath) {
		return true, nil
	}

	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return false, fmt.Errorf("stat preview input %s: %w", input, err)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat preview output %s: %w", output, err)
	}
	return os.SameFile(inputInfo, outputInfo), nil
}
