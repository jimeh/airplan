package cli

import (
	"fmt"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type protectOptions struct {
	config  string
	profile string
	reason  string
}

func newProtectCmd() *cobra.Command {
	opts := &protectOptions{}

	cmd := &cobra.Command{
		Use:   "protect <url|key>",
		Short: "Mark an upload as purge-protected",
		Long: "Mark an upload as purge-protected so purge skips it and " +
			"delete refuses it without --force (SPEC.md §9). Protecting an " +
			"already protected upload succeeds and rewrites the sentinel.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProtect(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")
	f.StringVar(&opts.reason, "reason", "",
		"optional note stored with the protection (at most 256 characters)")

	return cmd
}

func newUnprotectCmd() *cobra.Command {
	opts := &protectOptions{}

	cmd := &cobra.Command{
		Use:   "unprotect <url|key>",
		Short: "Remove purge protection from an upload",
		Long: "Remove purge protection from an upload so purge and delete " +
			"can remove it again (SPEC.md §9). Unprotecting an unprotected " +
			"upload succeeds.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnprotect(cmd, args[0], opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")

	return cmd
}

func runProtect(cmd *cobra.Command, urlOrKey string, opts *protectOptions) error {
	client, _, ctx, cancel, err := setupTargetClient(
		cmd, opts.config, opts.profile, urlOrKey,
	)
	if err != nil {
		return err
	}
	defer cancel()

	res, err := client.ProtectUpload(ctx, urlOrKey, opts.reason)
	if err != nil {
		return err
	}
	printProtectionResult(cmd, res, "protected")
	return nil
}

func runUnprotect(
	cmd *cobra.Command, urlOrKey string, opts *protectOptions,
) error {
	client, _, ctx, cancel, err := setupTargetClient(
		cmd, opts.config, opts.profile, urlOrKey,
	)
	if err != nil {
		return err
	}
	defer cancel()

	res, err := client.UnprotectUpload(ctx, urlOrKey)
	if err != nil {
		return err
	}
	printProtectionResult(cmd, res, "unprotected")
	return nil
}

// printProtectionResult writes the one-line stderr summary; stdout stays
// empty like delete (SPEC.md §1).
func printProtectionResult(
	cmd *cobra.Command, res *airplan.ProtectionResult, verb string,
) {
	stderr := cmd.ErrOrStderr()
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}
	fmt.Fprintf(stderr, "%s upload (key %s)\n", verb, res.PageKey)
}
