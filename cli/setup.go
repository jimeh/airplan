package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

// loadCommandConfig loads configuration for a subcommand, printing
// load warnings to the command's stderr — the sequence every
// history/cleanup command shares.
func loadCommandConfig(
	cmd *cobra.Command, path, profile string,
) (*airplan.Config, error) {
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:    path,
		Profile: profile,
	})
	if err != nil {
		return nil, err
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", w)
	}
	return cfg, nil
}

// setupClient is the whole common sequence — load config, print
// warnings, apply the timeout, construct the client — for commands
// whose timeout starts immediately. The purge flows compose
// loadCommandConfig and timeoutContext directly instead: their
// timeout placement is deliberately different (after the
// confirmation prompt, and per-phase for remote purge).
func setupClient(
	cmd *cobra.Command, path, profile string,
) (*airplan.Client, *airplan.Config, context.Context,
	context.CancelFunc, error,
) {
	cfg, err := loadCommandConfig(cmd, path, profile)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	ctx, cancel := timeoutContext(cmd.Context(), cfg)
	client, err := airplan.New(ctx, cfg)
	if err != nil {
		cancel()
		return nil, nil, nil, nil, err
	}
	return client, cfg, ctx, cancel, nil
}

// inferManifestProfile selects a uniquely matching marker-managed history
// profile when no flag or environment selector is active.
func inferManifestProfile(
	cmd *cobra.Command, target, flagProfile string,
) (string, bool) {
	if cmd.Flags().Changed("profile") || os.Getenv("AIRPLAN_PROFILE") != "" {
		return flagProfile, false
	}

	records, _, err := airplan.ReadManifest("")
	if err != nil {
		return flagProfile, false
	}
	var matches []airplan.ManifestRecord
	for _, rec := range airplan.MatchingManifestUploads(records, target) {
		if airplan.IsSupportedMarkerVersion(rec.MarkerVersion) {
			matches = append(matches, rec)
		}
	}
	if len(matches) != 1 || matches[0].Profile == "" {
		return flagProfile, false
	}
	return matches[0].Profile, true
}

func setupTargetClient(
	cmd *cobra.Command, path, flagProfile, target string,
) (*airplan.Client, *airplan.Config, context.Context,
	context.CancelFunc, error,
) {
	profile, inferred := inferManifestProfile(cmd, target, flagProfile)
	client, cfg, ctx, cancel, err := setupClient(cmd, path, profile)
	if err != nil {
		if inferred {
			return nil, nil, nil, nil, fmt.Errorf(
				"airplan: upload was recorded with profile %q, but it could not be selected: %s",
				profile, strings.TrimPrefix(err.Error(), "airplan: "),
			)
		}
		return nil, nil, nil, nil, err
	}
	if inferred {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"airplan: note: using profile %q recorded in the local manifest\n",
			profile)
	}
	return client, cfg, ctx, cancel, nil
}

// timeoutContext applies cfg.Timeout when set (SPEC.md §6). Callers
// choose where in their flow to call it — notably after interactive
// confirmation prompts, so the budget is never spent on user think
// time.
func timeoutContext(
	ctx context.Context, cfg *airplan.Config,
) (context.Context, context.CancelFunc) {
	if cfg.Timeout > 0 {
		return context.WithTimeout(ctx, cfg.Timeout)
	}
	return ctx, func() {}
}
