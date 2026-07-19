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
	format           string
	lang             string
	slug             string
	title            string
	indexable        bool
	noExternalAssets bool
	mermaidURL       string
	repository       string
	maxSize          string
	template         string
	profile          string
	config           string
	output           string
}

func newPreviewCmd() *cobra.Command {
	opts := &previewOptions{}
	cmd := &cobra.Command{
		Use:   "preview [flags] [file]",
		Short: "Render a document locally without uploading it",
		Args:  cobra.MaximumNArgs(1),
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
		"input size limit, e.g. 10MiB, 512k, 1048576; 0 = no limit")
	f.StringVar(&opts.template, "template", "",
		"custom page template file (md and text input)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVar(&opts.output, "output", "",
		"write HTML to this path instead of stdout; - means stdout")
	return cmd
}

func runPreview(
	cmd *cobra.Command,
	args []string,
	opts *previewOptions,
) error {
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
