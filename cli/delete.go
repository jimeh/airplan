package cli

import (
	"fmt"

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

	client, _, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	defer cancel()

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
