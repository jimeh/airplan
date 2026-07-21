package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type syncOptions struct {
	config      string
	profile     string
	concurrency int
	noPrune     bool
	dryRun      bool
	json        bool
}

func newSyncCmd() *cobra.Command {
	opts := &syncOptions{}
	cmd := &cobra.Command{
		Use:           "sync",
		Short:         "Sync the local manifest with remote uploads",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSync(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile (default: config default)")
	f.IntVar(&opts.concurrency, "concurrency",
		airplan.DefaultRemoteConcurrency,
		"maximum concurrent targeted remote requests (1-64)")
	f.BoolVar(&opts.noPrune, "no-prune", false,
		"import remote uploads without tombstoning local-only records")
	f.BoolVar(&opts.dryRun, "dry-run", false,
		"validate and display changes without writing the manifest")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print one JSON result object")
	return cmd
}

func runSync(cmd *cobra.Command, opts *syncOptions) error {
	if err := validateCLIConcurrency(opts.concurrency); err != nil {
		return fmt.Errorf("--concurrency: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	client, _, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile,
	)
	if err != nil {
		return err
	}
	defer cancel()

	result, syncErr := client.SyncManifest(ctx, airplan.SyncManifestOptions{
		Concurrency: opts.concurrency,
		Prune:       !opts.noPrune,
		DryRun:      opts.dryRun,
	})
	if result == nil {
		return syncErr
	}
	if result.Added == nil {
		result.Added = []airplan.ManifestRecord{}
	}
	if result.Tombstoned == nil {
		result.Tombstoned = []airplan.ManifestRecord{}
	}
	if result.Failures == nil {
		result.Failures = []airplan.SyncFailure{}
	}
	if opts.json {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(
			syncJSONFromResult(result),
		); err != nil {
			return err
		}
		return syncErr
	}
	stderr := cmd.ErrOrStderr()
	for _, warning := range result.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}
	for _, failure := range result.Failures {
		fmt.Fprintf(stderr, "airplan: error: %s %s: %s\n",
			failure.Operation, failure.MarkerKey, failure.Error)
	}
	verb := "synced"
	tombstoneVerb := "tombstoned"
	if opts.dryRun {
		verb = "would sync"
		tombstoneVerb = "would tombstone"
	}
	fmt.Fprintf(stderr, "%s %d uploads, %s %d\n",
		verb, len(result.Added), tombstoneVerb, len(result.Tombstoned))
	fmt.Fprintf(stderr,
		"(%d unchanged, %d incomplete, %d invalid, %d retained, %d failed)\n",
		result.Unchanged, result.Incomplete, result.Invalid,
		result.Retained, len(result.Failures))
	return syncErr
}

type syncJSONResult struct {
	Added            int                      `json:"added"`
	Tombstoned       int                      `json:"tombstoned"`
	Unchanged        int                      `json:"unchanged"`
	Incomplete       int                      `json:"incomplete"`
	Invalid          int                      `json:"invalid"`
	Retained         int                      `json:"retained"`
	Failed           int                      `json:"failed"`
	AddedRecords     []airplan.ManifestRecord `json:"added_records"`
	TombstoneRecords []airplan.ManifestRecord `json:"tombstone_records"`
	Failures         []airplan.SyncFailure    `json:"failures"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

func syncJSONFromResult(result *airplan.SyncManifestResult) syncJSONResult {
	return syncJSONResult{
		Added: len(result.Added), Tombstoned: len(result.Tombstoned),
		Unchanged: result.Unchanged, Incomplete: result.Incomplete,
		Invalid: result.Invalid, Retained: result.Retained,
		Failed: len(result.Failures), AddedRecords: result.Added,
		TombstoneRecords: result.Tombstoned, Failures: result.Failures,
		Warnings: result.Warnings,
	}
}

func validateCLIConcurrency(concurrency int) error {
	if concurrency < 1 || concurrency > airplan.MaxRemoteConcurrency {
		return fmt.Errorf("airplan: concurrency must be between 1 and %d",
			airplan.MaxRemoteConcurrency)
	}
	return nil
}
