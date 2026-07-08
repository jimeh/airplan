// Package cli implements the airplan command-line interface. It
// contains no business logic: it parses flags, calls the core airplan
// package, and formats output per the contract in SPEC.md §1 — final
// URL on stdout, everything else on stderr, non-zero exit on failure.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "airplan: %s\n", err)
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
		Long: "airplan uploads AI/LLM agent plan files (markdown or " +
			"HTML) to S3-compatible object storage under a randomized, " +
			"unguessable URL path and prints the resulting URL.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "",
		"input format: md or html (default: auto-detect)")
	f.StringVarP(&opts.slug, "slug", "s", "",
		"filename portion of the URL (default: from filename)")
	f.StringVarP(&opts.title, "title", "t", "",
		"page title (default: from content)")
	f.BoolVar(&opts.noSource, "no-source", false,
		"don't upload the original .md alongside the page")
	f.BoolVar(&opts.indexable, "indexable", false,
		"omit the noindex robots meta tag")
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

// run executes the upload pipeline for the root command.
func run(cmd *cobra.Command, args []string, opts *rootOptions) error {
	return errors.New("not implemented")
}
