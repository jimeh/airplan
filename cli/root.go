// Package cli implements the airplan command-line interface. It
// contains no business logic: it parses flags, calls the core airplan
// package, and formats upload output per SPEC.md §1 — final URL on
// stdout, everything else on stderr, non-zero exit on failure. Local
// preview output is defined separately in SPEC.md §6.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

// version is stamped by the release build via ldflags.
var version = "dev"

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		msg := err.Error()
		// Core library errors already carry the "airplan:" prefix,
		// stdlib style; only bare errors (cobra's own, wrapping) need
		// it added.
		if !strings.HasPrefix(msg, "airplan:") {
			msg = "airplan: " + msg
		}
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}
	return 0
}

// rootOptions holds the root command's flag values.
type rootOptions struct {
	format           string
	lang             string
	slug             string
	title            string
	noSource         bool
	indexable        bool
	noExternalAssets bool
	mermaidURL       string
	maxSize          string
	template         string
	timeout          string
	json             bool
	open             bool
	profile          string
	config           string

	// Connection overrides for one-off use (SPEC.md §6).
	endpoint      string
	bucket        string
	region        string
	publicBaseURL string
	keyPrefix     string
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:   "airplan [flags] [file]",
		Short: "Upload a plan document and print its shareable URL",
		Long: "airplan uploads AI/LLM agent plan files (markdown, " +
			"HTML, or plain text) to S3-compatible object storage " +
			"under a randomized, unguessable URL path and prints the " +
			"resulting URL.",
		Args:          cobra.MaximumNArgs(1),
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "",
		"input format: md, html, or txt (default: auto-detect)")
	f.StringVar(&opts.lang, "lang", "",
		"highlight language for text input (default: from filename)")
	f.StringVarP(&opts.slug, "slug", "s", "",
		"filename portion of the URL (default: from filename)")
	f.StringVarP(&opts.title, "title", "t", "",
		"page title (default: from content)")
	f.BoolVar(&opts.noSource, "no-source", false,
		"don't upload the original source alongside the page")
	f.BoolVar(&opts.indexable, "indexable", false,
		"omit the noindex robots meta tag")
	f.BoolVar(&opts.noExternalAssets, "no-external-assets", false,
		"disable airplan-managed external assets in rendered pages")
	f.StringVar(&opts.mermaidURL, "mermaid-url", "",
		"Mermaid ECMAScript module URL")
	f.StringVar(&opts.maxSize, "max-size", "10MiB",
		"input size limit, e.g. 10MiB, 512k, 1048576; 0 = no limit")
	f.StringVar(&opts.timeout, "timeout", "",
		"operation timeout, e.g. 30s, 1m30s; 0 = none (default 30s)")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a single JSON object instead of the URL")
	f.BoolVarP(&opts.open, "open", "o", false,
		"open the resulting URL in the default browser")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")

	f.StringVar(&opts.endpoint, "endpoint", "", "S3 endpoint URL")
	f.StringVar(&opts.bucket, "bucket", "", "bucket name")
	f.StringVar(&opts.region, "region", "", "region (default: auto)")
	f.StringVar(&opts.publicBaseURL, "public-base-url", "",
		"base URL public links are assembled from")
	f.StringVar(&opts.template, "template", "",
		"custom page template file (md and text input)")
	f.StringVar(&opts.keyPrefix, "key-prefix", "",
		"prefix prepended to object keys")

	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newTemplateCmd())
	cmd.AddCommand(newPreviewCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newPurgeCmd())
	return cmd
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

func resolveVersion(
	stamped string,
	info *debug.BuildInfo,
	ok bool,
) string {
	if stamped != "dev" {
		return stamped
	}
	if !ok || info == nil || info.Main.Version == "" ||
		info.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

// run executes the upload pipeline for the root command, honoring the
// output contract of SPEC.md §1: the final URL is the only thing on
// stdout; warnings and errors go to stderr.
func run(cmd *cobra.Command, args []string, opts *rootOptions) error {
	stderr := cmd.ErrOrStderr()

	maxSize, err := airplan.ParseSize(opts.maxSize)
	if err != nil {
		return fmt.Errorf("--max-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxSize == 0 {
		maxSize = -1 // 0 on the CLI means unlimited (SPEC.md §2)
	}

	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:      opts.config,
		Profile:   opts.profile,
		Overrides: flagOverrides(cmd, opts),
	})
	if err != nil {
		return err
	}

	// The resolved timeout bounds the upload operation after config
	// resolution (SPEC.md §6).
	ctx := cmd.Context()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	client, err := airplan.New(ctx, cfg)
	if err != nil {
		return err
	}

	in := airplan.Input{
		Format:  opts.format,
		Slug:    opts.slug,
		Title:   opts.title,
		MaxSize: maxSize,
		Lang:    opts.lang,
	}
	if len(args) == 0 || args[0] == "-" {
		in.Reader = cmd.InOrStdin()
	} else {
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		in.Reader = f
		in.Name = args[0]
	}

	res, err := client.Upload(ctx, in)
	if err != nil {
		if errors.Is(err, airplan.ErrInputTooLarge) {
			return fmt.Errorf(
				"%w (raise or remove the limit with --max-size)", err,
			)
		}
		return err
	}

	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}
	if err := printResult(cmd.OutOrStdout(), res, opts.json); err != nil {
		return err
	}
	if opts.open {
		if err := openBrowser(res.URL); err != nil {
			fmt.Fprintf(stderr,
				"airplan: warning: could not open browser: %s\n", err)
		}
	}
	return nil
}

type jsonResult struct {
	URL         string `json:"url"`
	Key         string `json:"key"`
	SourceURL   string `json:"source_url,omitempty"`
	Bucket      string `json:"bucket"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

func printResult(w io.Writer, res *airplan.Result, jsonOutput bool) error {
	if !jsonOutput {
		_, err := fmt.Fprintln(w, res.URL)
		return err
	}

	out := jsonResult{
		URL:         res.URL,
		Key:         res.Key,
		SourceURL:   res.SourceURL,
		Bucket:      res.Bucket,
		Bytes:       res.Bytes,
		ContentType: res.ContentType,
	}
	return json.NewEncoder(w).Encode(out)
}

var openBrowser = defaultOpenBrowser

func defaultOpenBrowser(url string) error {
	var name string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "linux":
		name = "xdg-open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}

	return exec.Command(name, args...).Start()
}

// flagOverrides packs explicitly-passed flags into a Settings overlay
// for LoadConfig — the top of the precedence order in SPEC.md §7,
// where it also counts toward profile-resolution completeness.
// Changed() guards let an explicit false override a config-file true
// for the bool flags; string flags use "" as "not set".
func flagOverrides(cmd *cobra.Command, opts *rootOptions) airplan.Settings {
	ov := airplan.Settings{
		Endpoint:      opts.endpoint,
		Bucket:        opts.bucket,
		Region:        opts.region,
		PublicBaseURL: opts.publicBaseURL,
		KeyPrefix:     opts.keyPrefix,
		Template:      opts.template,
		Timeout:       opts.timeout,
		MermaidURL: airplan.ResolveMermaidURLOverride(
			opts.mermaidURL,
			cmd.Flags().Changed("mermaid-url"),
		),
	}
	f := cmd.Flags()
	if f.Changed("no-source") {
		ov.NoSource = &opts.noSource
	}
	if f.Changed("indexable") {
		ov.Indexable = &opts.indexable
	}
	if f.Changed("no-external-assets") {
		ov.NoExternalAssets = &opts.noExternalAssets
	}
	return ov
}
