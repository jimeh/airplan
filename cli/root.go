// Package cli implements the airplan command-line interface. It
// contains no business logic: it parses flags, calls the core airplan
// package, and formats output per the contract in SPEC.md §1 — final
// URL on stdout, everything else on stderr, non-zero exit on failure.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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
	format    string
	slug      string
	title     string
	noSource  bool
	indexable bool
	maxSize   string
	config    string

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
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "",
		"input format: md, html, or txt (default: auto-detect)")
	f.StringVarP(&opts.slug, "slug", "s", "",
		"filename portion of the URL (default: from filename)")
	f.StringVarP(&opts.title, "title", "t", "",
		"page title (default: from content)")
	f.BoolVar(&opts.noSource, "no-source", false,
		"don't upload the original source alongside the page")
	f.BoolVar(&opts.indexable, "indexable", false,
		"omit the noindex robots meta tag")
	f.StringVar(&opts.maxSize, "max-size", "10MB",
		"input size limit, e.g. 10MB, 512k, 1048576; 0 = no limit")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")

	f.StringVar(&opts.endpoint, "endpoint", "", "S3 endpoint URL")
	f.StringVar(&opts.bucket, "bucket", "", "bucket name")
	f.StringVar(&opts.region, "region", "", "region (default: auto)")
	f.StringVar(&opts.publicBaseURL, "public-base-url", "",
		"base URL public links are assembled from")
	f.StringVar(&opts.keyPrefix, "key-prefix", "",
		"prefix prepended to object keys")

	return cmd
}

// uploadTimeout bounds one whole invocation so a stalled endpoint
// fails with a clear error instead of hanging the CLI (and any agent
// harness driving it) indefinitely. Generous: plan documents are
// small, so healthy uploads finish in seconds.
const uploadTimeout = 2 * time.Minute

// run executes the upload pipeline for the root command, honoring the
// output contract of SPEC.md §1: the final URL is the only thing on
// stdout; warnings and errors go to stderr.
func run(cmd *cobra.Command, args []string, opts *rootOptions) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), uploadTimeout)
	defer cancel()
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
		Overrides: flagOverrides(cmd, opts),
	})
	if err != nil {
		return err
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
	fmt.Fprintln(cmd.OutOrStdout(), res.URL)
	return nil
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
	}
	f := cmd.Flags()
	if f.Changed("no-source") {
		ov.NoSource = &opts.noSource
	}
	if f.Changed("indexable") {
		ov.Indexable = &opts.indexable
	}
	return ov
}
