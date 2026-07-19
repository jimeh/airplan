package cli

import (
	"fmt"

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
		Short:         "Fetch an object from an uploaded plan",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd, opts, args[0])
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.output, "output", "",
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
	client, _, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile,
	)
	if err != nil {
		return err
	}
	defer cancel()

	result, err := client.GetUpload(ctx, target, airplan.GetOptions{
		Source: opts.source,
	})
	if err != nil {
		return err
	}
	if opts.output == "" || opts.output == "-" {
		_, err = cmd.OutOrStdout().Write(result.Body)
		return err
	}
	if err := writeFileAtomic(opts.output, result.Body, 0o600); err != nil {
		return fmt.Errorf("write get output %s: %w", opts.output, err)
	}
	return nil
}
