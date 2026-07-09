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
	remote    bool
	all       bool
	dryRun    bool
	yes       bool
}

func newPurgeCmd() *cobra.Command {
	opts := &purgeOptions{}

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Delete uploads from the local manifest",
		Long: "Delete uploads from the local manifest, or with --remote, " +
			"from a live bucket listing. In remote mode, --profile " +
			"selects the connection profile and key_prefix only; it " +
			"does not filter records because buckets do not store " +
			"manifest profile metadata.",
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
	// Local manifest semantics (SPEC.md §9): --profile selects the
	// connection profile and additionally filters to uploads recorded
	// with that profile. In --remote mode it only selects the
	// connection profile/key_prefix because buckets do not carry
	// manifest profile metadata.
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile; locally also filters to uploads made with it")
	f.BoolVar(&opts.remote, "remote", false,
		"purge uploads from a live bucket listing instead of the manifest")
	f.BoolVar(&opts.all, "all", false,
		"delete all active uploads in the selected source")
	f.BoolVar(&opts.dryRun, "dry-run", false,
		"preview matching uploads without deleting")
	f.BoolVar(&opts.yes, "yes", false,
		"skip the confirmation prompt")

	return cmd
}

func runPurge(cmd *cobra.Command, opts *purgeOptions) error {
	stderr := cmd.ErrOrStderr()

	hasFilter := opts.all || opts.olderThan != "" || opts.slug != "" ||
		(!opts.remote && opts.profile != "")
	if !hasFilter {
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

	if opts.remote {
		return runRemotePurge(cmd, opts, olderThan)
	}

	// Config is loaded before candidate selection so records for
	// other buckets can be excluded (SPEC.md §9) — but the timeout
	// context is still created only after the confirmation prompt,
	// so it can't expire while the user reads it.
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:    opts.config,
		Profile: opts.profile,
	})
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
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

	// Only records for the connected bucket are purgeable
	// (SPEC.md §9). With no bucket configured (e.g. a config-free
	// --dry-run) there is nothing to scope against, so the filter is
	// skipped — actual deletion would still fail config validation.
	if cfg.Bucket != "" {
		kept := candidates[:0]
		skipped := 0
		for _, rec := range candidates {
			if rec.Bucket != "" && rec.Bucket != cfg.Bucket {
				skipped++
				continue
			}
			kept = append(kept, rec)
		}
		candidates = kept
		if skipped > 0 {
			fmt.Fprintf(stderr,
				"airplan: note: skipped %d upload(s) recorded for "+
					"other buckets\n", skipped)
		}
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

	ctx := cmd.Context()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
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

type remotePurgeCandidate struct {
	upload airplan.RemoteUpload
	record airplan.ManifestRecord
}

func runRemotePurge(
	cmd *cobra.Command,
	opts *purgeOptions,
	olderThan time.Duration,
) error {
	stderr := cmd.ErrOrStderr()

	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:    opts.config,
		Profile: opts.profile,
	})
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	ctx := cmd.Context()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	client, err := airplan.New(ctx, cfg)
	if err != nil {
		return err
	}

	uploads, err := client.ListRemote(ctx)
	if err != nil {
		return err
	}
	candidates, err := remotePurgeCandidates(
		uploads, cfg, opts, olderThan, time.Now())
	if err != nil {
		return err
	}

	records := remotePurgeRecords(candidates)
	if opts.dryRun {
		printPurgeCandidates(stderr, records)
		return nil
	}

	if len(candidates) == 0 {
		fmt.Fprintln(stderr, "purged 0 uploads (0 failed)")
		return nil
	}

	if !opts.yes {
		printPurgeCandidates(stderr, records)
		ok, err := confirmPurge(cmd, len(candidates))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(stderr, "aborted")
			return nil
		}
	}

	var purged, failed int
	for _, cand := range candidates {
		if _, err := client.DeleteUpload(ctx, cand.upload.Dir); err != nil {
			failed++
			fmt.Fprintf(stderr, "airplan: error: delete %s: %s\n",
				purgeTarget(cand.record), err)
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

func remotePurgeCandidates(
	uploads []airplan.RemoteUpload,
	cfg *airplan.Config,
	opts *purgeOptions,
	olderThan time.Duration,
	now time.Time,
) ([]remotePurgeCandidate, error) {
	var out []remotePurgeCandidate
	cutoff := now.Add(-olderThan)
	for _, upload := range uploads {
		if !opts.all {
			if opts.olderThan != "" && !upload.LastModified.Before(cutoff) {
				continue
			}
			if opts.slug != "" {
				ok, err := path.Match(opts.slug, uploadSlug(upload.PageKey))
				if err != nil {
					return nil, fmt.Errorf("--slug: %w", err)
				}
				if !ok {
					continue
				}
			}
		}

		url, _ := airplan.PublicURL(cfg, upload.PageKey)
		out = append(out, remotePurgeCandidate{
			upload: upload,
			record: airplan.ManifestRecord{
				Type:   "upload",
				Time:   upload.LastModified.UTC(),
				Key:    upload.PageKey,
				URL:    url,
				Bucket: cfg.Bucket,
				Bytes:  upload.Bytes,
			},
		})
	}
	return out, nil
}

func remotePurgeRecords(
	candidates []remotePurgeCandidate,
) []airplan.ManifestRecord {
	records := make([]airplan.ManifestRecord, 0, len(candidates))
	for _, cand := range candidates {
		records = append(records, cand.record)
	}
	return records
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
