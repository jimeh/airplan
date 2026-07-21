package cli

import (
	"errors"
	"fmt"
	"io"

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
		Short:         "Delete an upload",
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
	client, _, ctx, cancel, err := setupTargetClient(
		cmd, opts.config, opts.profile, urlOrKey,
	)
	if err != nil {
		return err
	}
	defer cancel()

	res, err := client.DeleteUpload(ctx, urlOrKey)
	if err != nil {
		var mismatch *airplan.ManifestProfileMismatchError
		if errors.As(err, &mismatch) {
			printDeleteProfileMismatch(stderr, mismatch)
		}
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	fmt.Fprintf(stderr, "deleted %d objects (key %s)\n",
		len(res.Keys), res.PageKey)
	return nil
}

func printDeleteProfileMismatch(
	stderr io.Writer,
	mismatch *airplan.ManifestProfileMismatchError,
) {
	if mismatch.Recorded == "" {
		fmt.Fprintf(stderr,
			"airplan: warning: upload was recorded with root-level config, but the active profile is %q; retry with --config or AIRPLAN_CONFIG pointing to a config that resolves root-level settings\n",
			mismatch.Active)
		return
	}
	fmt.Fprintf(stderr,
		"airplan: warning: upload was recorded with profile %q, but the active profile is %q; retry with --profile %s or AIRPLAN_PROFILE=%s\n",
		mismatch.Recorded, mismatch.Active,
		mismatch.Recorded, mismatch.Recorded)
}
