package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type purgeOptions struct {
	config           string
	olderThan        string
	slug             string
	profile          string
	remote           bool
	all              bool
	dryRun           bool
	yes              bool
	concurrency      int
	includeVersioned bool
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
		"filter: uploads before this age or date, e.g. 30d, 2w, 2026-07-01")
	f.StringVar(&opts.slug, "slug", "",
		"filter: glob pattern matched against the page slug")
	// Local manifest semantics (SPEC.md §9): every resolved connection
	// profile scopes manifest records. In --remote mode the profile only
	// selects the connection/key_prefix because buckets do not carry
	// manifest profile metadata.
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile (local purge is scoped to the active profile)")
	// SPEC.md §6 defines -r as the purge --remote shorthand.
	f.BoolVarP(&opts.remote, "remote", "r", false,
		"purge uploads from a live bucket listing instead of the manifest")
	f.BoolVar(&opts.all, "all", false,
		"delete all active uploads in the selected source")
	f.BoolVar(&opts.dryRun, "dry-run", false,
		"preview matching uploads without deleting")
	f.BoolVar(&opts.yes, "yes", false,
		"skip the confirmation prompt")
	f.IntVar(&opts.concurrency, "concurrency",
		airplan.DefaultRemoteConcurrency,
		"maximum concurrent remote marker inspections (1-64)")
	f.BoolVar(&opts.includeVersioned, "include-versioned", false,
		"include linked document revisions in purge candidates")

	return cmd
}

func runPurge(cmd *cobra.Command, opts *purgeOptions) error {
	stderr := cmd.ErrOrStderr()
	if cmd.Flags().Changed("profile") && strings.TrimSpace(opts.profile) == "" {
		return errors.New("--profile requires a non-empty profile name")
	}
	if cmd.Flags().Changed("concurrency") && !opts.remote {
		return errors.New("--concurrency requires --remote")
	}
	if opts.remote {
		if err := validateCLIConcurrency(opts.concurrency); err != nil {
			return fmt.Errorf("--concurrency: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		if !opts.all && !cmd.Flags().Changed("older-than") && opts.slug == "" {
			return errors.New(
				"purge requires at least one filter or explicit --all")
		}
	}

	// Purge shares the listing time parser (SPEC.md §9), so an age and an
	// absolute date select the same boundary here and in list --older-than.
	var createdBefore time.Time
	if cmd.Flags().Changed("older-than") {
		now := time.Now()
		var err error
		createdBefore, err = airplan.ParseTimeFilter(opts.olderThan, now)
		if err != nil {
			return flagError("--older-than", err)
		}
		// PurgePlanOptions reads a zero CreatedBefore as "no age filter", so a
		// year-one boundary would widen the selection to everything instead
		// of the nothing it selects in list --older-than. Refuse it rather
		// than diverge on a destructive command.
		if createdBefore.IsZero() {
			return errors.New(
				"--older-than: that boundary selects nothing; " +
					"no upload is older than year one")
		}
		if !createdBefore.Before(now) {
			message := "--older-than: must select a time in the past"
			if !opts.all {
				message += "; use --all to purge everything"
			}
			return errors.New(message)
		}
	}

	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	profileOnly := !opts.remote &&
		cfg.EffectiveBackend() == airplan.BackendS3 &&
		cmd.Flags().Changed("profile") && !cmd.Flags().Changed("older-than") &&
		opts.slug == "" && !opts.all
	hasFilter := opts.all || cmd.Flags().Changed("older-than") || opts.slug != "" ||
		profileOnly
	if !hasFilter {
		return errors.New(
			"purge requires at least one filter or explicit --all")
	}

	client, err := airplan.New(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	source := airplan.UploadSourceManifest
	if opts.remote {
		source = airplan.UploadSourceStorage
	}
	planCtx := cmd.Context()
	planCancel := func() {}
	if opts.remote || cfg.EffectiveBackend() == airplan.BackendAirplan {
		planCtx, planCancel = timeoutContext(planCtx, cfg)
	}
	defer planCancel()
	plan, err := client.PlanPurge(planCtx, airplan.PurgePlanOptions{
		Source: source, CreatedBefore: createdBefore,
		Slug: opts.slug, All: opts.all || profileOnly,
		Concurrency:      opts.concurrency,
		IncludeVersioned: opts.includeVersioned,
	})
	if err != nil {
		return err
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}
	if plan.Invalid > 0 {
		fmt.Fprintf(stderr,
			"airplan: note: skipped %d invalid remote marker(s)\n",
			plan.Invalid)
	}
	candidates := make([]airplan.ManifestRecord, 0, len(plan.Candidates))
	warningCandidates := make([]remotePurgeCandidate, 0,
		len(plan.Candidates)+len(plan.Protected))
	for _, candidate := range plan.Candidates {
		candidates = append(candidates, candidate.Record)
		warningCandidates = append(warningCandidates, remotePurgeCandidate{
			warnings: candidate.Warnings,
		})
	}
	for _, item := range plan.Protected {
		warningCandidates = append(warningCandidates, remotePurgeCandidate{
			warnings: item.Warnings,
		})
	}
	printRemotePurgeWarnings(stderr, warningCandidates)
	// Protected uploads are excluded from the candidate list up front so
	// dry-run and confirmation both reflect what purge will actually touch
	// (SPEC.md §9). Skips are notes, never failures.
	protected := len(plan.Protected)
	if len(plan.Versioned) > 0 {
		fmt.Fprintf(stderr, "airplan: note: skipped %d linked revision(s); use --include-versioned to include them\n", len(plan.Versioned))
	}
	for _, item := range plan.Protected {
		printProtectedSkip(stderr, item.Record)
	}

	if opts.dryRun {
		printPurgeCandidates(stderr, candidates)
		return nil
	}

	if len(candidates) == 0 {
		fmt.Fprintf(stderr, "purged 0 uploads (%d protected, 0 failed)\n",
			protected)
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
	ids := make([]string, 0, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		ids = append(ids, candidate.UploadID)
	}
	result, purgeErr := client.Purge(ctx, airplan.PurgeRequest{
		UploadIDs: ids, IncludeVersioned: opts.includeVersioned,
	})
	if result == nil {
		return purgeErr
	}
	purged := 0
	failed := 0
	for index, item := range result.Items {
		if item.Versioned {
			fmt.Fprintf(stderr, "airplan: note: skipped linked revision %s; use --include-versioned to include it\n", item.UploadID)
			continue
		}
		if item.Protected {
			// A delete-time skip catches protection set after planning; it
			// is not a failure and does not change the exit status.
			protected++
			if index < len(candidates) {
				printProtectedSkip(stderr, candidates[index])
			}
			continue
		}
		if item.Error == "" {
			purged++
			continue
		}
		failed++
		target := item.UploadID
		if index < len(candidates) {
			target = purgeTarget(candidates[index])
		}
		fmt.Fprintf(stderr, "airplan: error: delete %s: %s\n",
			target, item.Error)
	}

	fmt.Fprintf(stderr, "purged %d uploads (%d protected, %d failed)\n",
		purged, protected, failed)
	return purgeErr
}

func printProtectedSkip(w io.Writer, rec airplan.ManifestRecord) {
	fmt.Fprintf(w, "airplan: note: skipping protected upload %s\n",
		purgeTarget(rec))
}

type remotePurgeCandidate struct {
	warnings []string
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
