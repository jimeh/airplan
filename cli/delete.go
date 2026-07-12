package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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
	profile, inferred, err := deleteProfile(urlOrKey, opts.profile)
	if err != nil {
		return err
	}

	client, _, ctx, cancel, err := setupClient(cmd, opts.config, profile)
	if err != nil {
		if inferred {
			return fmt.Errorf(
				"airplan: upload was recorded with profile %q, but it could not be selected: %s",
				profile, strings.TrimPrefix(err.Error(), "airplan: "),
			)
		}
		return err
	}
	defer cancel()
	if inferred {
		fmt.Fprintf(stderr,
			"airplan: note: using profile %q recorded in the local manifest\n",
			profile)
	}

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

func deleteProfile(target, flagProfile string) (string, bool, error) {
	if flagProfile != "" || os.Getenv("AIRPLAN_PROFILE") != "" {
		return flagProfile, false, nil
	}

	records, _, err := airplan.ReadManifest("")
	if err != nil {
		// Profile inference is a convenience. Remote marker validation still
		// provides the authority for deletion when history is unavailable.
		return flagProfile, false, nil
	}
	profiles := make(map[string]struct{})
	for _, rec := range airplan.MatchingManifestUploads(records, target) {
		if rec.MarkerVersion == airplan.MarkerVersion && rec.Profile != "" {
			profiles[rec.Profile] = struct{}{}
		}
	}
	if len(profiles) == 0 {
		return flagProfile, false, nil
	}
	names := make([]string, 0, len(profiles))
	for profile := range profiles {
		names = append(names, profile)
	}
	sort.Strings(names)
	if len(names) > 1 {
		return "", false, fmt.Errorf(
			"airplan: local manifest records for this upload name multiple profiles (%s); select one with --profile or AIRPLAN_PROFILE",
			strings.Join(names, ", "),
		)
	}
	return names[0], true, nil
}

func printDeleteProfileMismatch(
	stderr io.Writer,
	mismatch *airplan.ManifestProfileMismatchError,
) {
	if mismatch.Recorded == "" {
		fmt.Fprintf(stderr,
			"airplan: warning: upload was recorded with root-level config, but the active profile is %q; omit --profile, unset AIRPLAN_PROFILE, and retry\n",
			mismatch.Active)
		return
	}
	fmt.Fprintf(stderr,
		"airplan: warning: upload was recorded with profile %q, but the active profile is %q; retry with --profile %s or AIRPLAN_PROFILE=%s\n",
		mismatch.Recorded, mismatch.Active,
		mismatch.Recorded, mismatch.Recorded)
}
