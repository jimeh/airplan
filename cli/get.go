package cli

import (
	"fmt"
	"io"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type getOptions struct {
	config  string
	profile string
	output  string
	source  bool
}

func newGetCmd() *cobra.Command {
	opts := &getOptions{}
	cmd := &cobra.Command{
		Use:           "get <url|key>",
		Short:         "Fetch an object from an upload",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, opts, args[0])
		},
	}
	f := cmd.Flags()
	// SPEC.md §6 defines -o as the get --output shorthand.
	f.StringVarP(&opts.output, "output", "o", "",
		"write bytes to this path instead of stdout; - means stdout")
	f.BoolVar(&opts.source, "source", false,
		"fetch the marker-declared source instead of the page")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile (default: config default)")
	return cmd
}

func runGet(cmd *cobra.Command, opts *getOptions, target string) error {
	client, _, ctx, cancel, err := setupTargetClient(
		cmd, opts.config, opts.profile, target,
	)
	if err != nil {
		return err
	}
	defer cancel()

	if opts.output == "" || opts.output == "-" {
		_, err = client.GetUploadTo(ctx, target, airplan.GetOptions{
			Source: opts.source,
		}, cmd.OutOrStdout())
		return err
	}
	if err := writeFileAtomicWith(opts.output, 0o600, func(w io.Writer) error {
		_, streamErr := client.GetUploadTo(ctx, target, airplan.GetOptions{
			Source: opts.source,
		}, w)
		return streamErr
	}); err != nil {
		return fmt.Errorf("write get output %s: %w", opts.output, err)
	}
	return nil
}
