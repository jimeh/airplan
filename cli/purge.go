package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
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
	// Local manifest semantics (SPEC.md §9): every resolved connection
	// profile scopes manifest records. In --remote mode the profile only
	// selects the connection/key_prefix because buckets do not carry
	// manifest profile metadata.
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile (local purge is scoped to the active profile)")
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
	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	candidates, err := purgeCandidates(
		airplan.ActiveUploads(records), opts, cfg.Profile,
		olderThan, time.Now())
	if err != nil {
		return err
	}

	// Only records for the connected bucket are purgeable
	// (SPEC.md §9). With no bucket configured (e.g. a config-free
	// --dry-run) there is nothing to scope against, so the filter is
	// skipped — actual deletion would still fail config validation.
	kept := candidates[:0]
	otherBuckets := 0
	otherPrefixes := 0
	for _, rec := range candidates {
		if cfg.Bucket != "" && rec.Bucket != cfg.Bucket {
			otherBuckets++
			continue
		}
		if cfg.Bucket != "" &&
			!airplan.KeyMatchesPrefix(rec.Key, cfg.KeyPrefix) {
			otherPrefixes++
			continue
		}
		kept = append(kept, rec)
	}
	candidates = kept
	if otherBuckets > 0 {
		fmt.Fprintf(stderr,
			"airplan: note: skipped %d upload(s) recorded for other buckets\n",
			otherBuckets)
	}
	if otherPrefixes > 0 {
		fmt.Fprintf(stderr,
			"airplan: note: skipped %d upload(s) recorded for other key prefixes\n",
			otherPrefixes)
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

	ctx, cancel := timeoutContext(cmd.Context(), cfg)
	defer cancel()

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
	upload   airplan.RemoteUpload
	record   airplan.ManifestRecord
	warnings []string
}

func runRemotePurge(
	cmd *cobra.Command,
	opts *purgeOptions,
	olderThan time.Duration,
) error {
	stderr := cmd.ErrOrStderr()

	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}

	// The listing phase gets its own timeout budget; the delete phase
	// gets a fresh one after the confirmation prompt, so user think
	// time never eats into either (SPEC.md §6).
	listCtx, cancelList := timeoutContext(cmd.Context(), cfg)
	defer cancelList()

	client, err := airplan.New(listCtx, cfg)
	if err != nil {
		return err
	}

	uploads, err := client.ListRemote(listCtx)
	if err != nil {
		return err
	}
	candidates, invalid, err := remotePurgeCandidates(
		listCtx, uploads, cfg, client, opts, olderThan, time.Now())
	if err != nil {
		return err
	}
	if invalid > 0 {
		fmt.Fprintf(stderr,
			"airplan: note: skipped %d invalid remote marker(s)\n", invalid)
	}
	printRemotePurgeWarnings(stderr, candidates)

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

	ctx, cancel := timeoutContext(cmd.Context(), cfg)
	defer cancel()

	var purged, failed int
	for _, cand := range candidates {
		if _, err := client.DeleteUpload(ctx, cand.upload.MarkerKey); err != nil {
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
	activeProfile string,
	olderThan time.Duration,
	now time.Time,
) ([]airplan.ManifestRecord, error) {
	// --all only satisfies the at-least-one-filter requirement; any
	// filters given alongside it still apply (SPEC.md §9) — otherwise
	// "purge --all --profile work" would delete everyone's uploads.
	var out []airplan.ManifestRecord
	cutoff := now.Add(-olderThan)
	for _, rec := range uploads {
		if rec.Profile != activeProfile {
			continue
		}
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
		out = append(out, rec)
	}
	return out, nil
}

func remotePurgeCandidates(
	ctx context.Context,
	uploads []airplan.RemoteUpload,
	cfg *airplan.Config,
	client *airplan.Client,
	opts *purgeOptions,
	olderThan time.Duration,
	now time.Time,
) ([]remotePurgeCandidate, int, error) {
	if opts.slug != "" {
		if _, err := path.Match(opts.slug, ""); err != nil {
			return nil, 0, fmt.Errorf("--slug: %w", err)
		}
	}

	inspections := inspectRemoteCandidates(ctx, client, uploads)
	var out []remotePurgeCandidate
	invalid := 0
	cutoff := now.Add(-olderThan)
	for _, result := range inspections {
		if result.err != nil {
			return nil, invalid, result.err
		}
		inspection := result.inspection
		if inspection.State == airplan.UploadInvalid {
			invalid++
			continue
		}
		if opts.olderThan != "" && !inspection.CreatedAt.Before(cutoff) {
			continue
		}
		if opts.slug != "" {
			ok, _ := path.Match(opts.slug, uploadSlug(inspection.Page.Key))
			if !ok {
				continue
			}
		}

		pageBytes := int64(0)
		if inspection.Page.Exists {
			pageBytes = inspection.Page.Bytes
		}
		out = append(out, remotePurgeCandidate{
			upload: result.upload,
			warnings: append([]string(nil),
				inspection.Warnings...),
			record: airplan.ManifestRecord{
				Type:          "upload",
				Time:          inspection.CreatedAt.UTC(),
				Key:           inspection.Page.Key,
				URL:           inspection.Page.URL,
				Bucket:        cfg.Bucket,
				Title:         inspection.Title,
				Bytes:         pageBytes,
				MarkerVersion: airplan.MarkerVersion,
			},
		})
	}
	return out, invalid, nil
}

func printRemotePurgeWarnings(
	w io.Writer, candidates []remotePurgeCandidate,
) {
	counts := map[string]int{}
	for _, candidate := range candidates {
		for _, warning := range candidate.warnings {
			counts[warning]++
		}
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		for _, warning := range candidate.warnings {
			if seen[warning] {
				continue
			}
			seen[warning] = true
			if counts[warning] > 1 {
				fmt.Fprintf(w, "airplan: warning: %s (%d uploads)\n",
					warning, counts[warning])
			} else {
				fmt.Fprintf(w, "airplan: warning: %s\n", warning)
			}
		}
	}
}

type remoteInspectionResult struct {
	index      int
	upload     airplan.RemoteUpload
	inspection *airplan.UploadInspection
	err        error
}

func inspectRemoteCandidates(
	ctx context.Context,
	client *airplan.Client,
	uploads []airplan.RemoteUpload,
) []remoteInspectionResult {
	const workers = 8
	type job struct {
		index  int
		upload airplan.RemoteUpload
	}

	jobs := make(chan job)
	results := make(chan remoteInspectionResult, len(uploads))
	workerCount := min(workers, len(uploads))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				inspection, err := client.InspectUpload(ctx, job.upload.MarkerKey)
				results <- remoteInspectionResult{
					index: job.index, upload: job.upload,
					inspection: inspection, err: err,
				}
			}
		}()
	}
	go func() {
		for index, upload := range uploads {
			jobs <- job{index: index, upload: upload}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	out := make([]remoteInspectionResult, 0, len(uploads))
	for result := range results {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].index < out[j].index })
	return out
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
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return false, errors.New(
			"airplan: confirmation input closed; rerun with --yes",
		)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
