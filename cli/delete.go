package cli

import (
	"context"
	"fmt"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type deleteOptions struct {
	config  string
	profile string
}

func newDeleteCmd() *cobra.Command {
	opts := &deleteOptions{}

	cmd := &cobra.Command{
		Use:           "delete <url|key>",
		Short:         "Delete an uploaded plan",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")

	return cmd
}

func runDelete(cmd *cobra.Command, urlOrKey string, opts *deleteOptions) error {
	stderr := cmd.ErrOrStderr()

	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:    opts.config,
		Profile: opts.profile,
	})
	if err != nil {
		return err
	}

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

	res, err := client.DeleteUpload(ctx, urlOrKey)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	fmt.Fprintf(stderr, "deleted %d objects (key %s)\n",
		len(res.Keys), res.PageKey)
	return nil
}
