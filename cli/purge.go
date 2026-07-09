package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type purgeOptions struct {
	config    string
	olderThan string
	slug      string
	profile   string
	all       bool
	dryRun    bool
	yes       bool
}

func newPurgeCmd() *cobra.Command {
	opts := &purgeOptions{}

	cmd := &cobra.Command{
		Use:           "purge",
		Short:         "Delete uploads from the local manifest",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPurge(cmd, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVar(&opts.olderThan, "older-than", "",
		"filter: uploads older than this age, e.g. 30d, 2w, 36h")
	f.StringVar(&opts.slug, "slug", "",
		"filter: glob pattern matched against the page slug")
	// Unified semantics (SPEC.md §9): as on every command, --profile
	// selects the connection profile — and on purge it additionally
	// filters to uploads recorded with that profile, so purging a
	// profile's uploads always uses that profile's credentials.
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile; also filters to uploads made with it")
	f.BoolVar(&opts.all, "all", false,
		"delete all active uploads in the local manifest")
	f.BoolVar(&opts.dryRun, "dry-run", false,
		"preview matching uploads without deleting")
	f.BoolVar(&opts.yes, "yes", false,
		"skip the confirmation prompt")

	return cmd
}

func runPurge(cmd *cobra.Command, opts *purgeOptions) error {
	stderr := cmd.ErrOrStderr()

	if !opts.all && opts.olderThan == "" && opts.slug == "" &&
		opts.profile == "" {
		return errors.New(
			"purge requires at least one filter or explicit --all")
	}

	var olderThan time.Duration
	if opts.olderThan != "" {
		var err error
		olderThan, err = airplan.ParseAge(opts.olderThan)
		if err != nil {
			return fmt.Errorf("--older-than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	candidates, err := purgeCandidates(
		airplan.ActiveUploads(records), opts, olderThan, time.Now())
	if err != nil {
		return err
	}

	if opts.dryRun {
		printPurgeCandidates(stderr, candidates)
		return nil
	}

	if len(candidates) == 0 {
		fmt.Fprintln(stderr, "purged 0 uploads (0 failed)")
		return nil
	}

	if !opts.yes {
		printPurgeCandidates(stderr, candidates)
		ok, err := confirmPurge(cmd, len(candidates))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stderr, "aborted")
			return nil
		}
	}

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

	var purged, failed int
	for _, rec := range candidates {
		if _, err := client.DeleteUpload(ctx, rec.Key); err != nil {
			failed++
			fmt.Fprintf(stderr, "airplan: error: delete %s: %s\n",
				purgeTarget(rec), err)
			continue
		}
		purged++
	}

	fmt.Fprintf(stderr, "purged %d uploads (%d failed)\n", purged, failed)
	if failed > 0 {
		return fmt.Errorf("airplan: purge failed: %d upload(s) failed", failed)
	}
	return nil
}

func purgeCandidates(
	uploads []airplan.ManifestRecord,
	opts *purgeOptions,
	olderThan time.Duration,
	now time.Time,
) ([]airplan.ManifestRecord, error) {
	if opts.all {
		return uploads, nil
	}

	var out []airplan.ManifestRecord
	cutoff := now.Add(-olderThan)
	for _, rec := range uploads {
		if opts.olderThan != "" && !rec.Time.Before(cutoff) {
			continue
		}
		if opts.slug != "" {
			ok, err := path.Match(opts.slug, uploadSlug(rec.Key))
			if err != nil {
				return nil, fmt.Errorf("--slug: %w", err)
			}
			if !ok {
				continue
			}
		}
		if opts.profile != "" && rec.Profile != opts.profile {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func uploadSlug(key string) string {
	base := path.Base(key)
	return strings.TrimSuffix(base, path.Ext(base))
}

func printPurgeCandidates(w io.Writer, records []airplan.ManifestRecord) {
	for _, rec := range records {
		fmt.Fprintf(w, "%s\n", describePurgeRecord(rec))
	}
}

func describePurgeRecord(rec airplan.ManifestRecord) string {
	if rec.Title == "" {
		return purgeTarget(rec)
	}
	return purgeTarget(rec) + " - " + rec.Title
}

func purgeTarget(rec airplan.ManifestRecord) string {
	if rec.URL != "" {
		return rec.URL
	}
	return rec.Key
}

func confirmPurge(cmd *cobra.Command, count int) (bool, error) {
	stderr := cmd.ErrOrStderr()
	fmt.Fprintf(stderr, "Delete %d uploads? [y/N] ", count)

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
