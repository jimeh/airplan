package cli

import (
	"context"
	"errors"
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
	if err := applyManifestSelection(cmd, cfg); err != nil {
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

	manifestPath, _ := selectedManifestPath(cmd)
	records, _, err := airplan.ReadManifest(manifestPath)
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

func selectedManifestPath(cmd *cobra.Command) (string, bool) {
	if cmd != nil {
		if flag := cmd.Flag("manifest"); flag != nil && flag.Changed {
			return flag.Value.String(), true
		}
	}
	return os.Getenv("AIRPLAN_MANIFEST"), false
}

func applyManifestSelection(cmd *cobra.Command, cfg *airplan.Config) error {
	path, explicit := selectedManifestPath(cmd)
	if cfg.EffectiveBackend() == airplan.BackendAirplan {
		if explicit {
			return errors.New(
				"--manifest cannot be used with the airplan backend; " +
					"configure it on airplan serve",
			)
		}
		return nil
	}
	if path != "" {
		cfg.ManifestPath = path
	}
	return nil
}

func validatePersistentOptions(cmd *cobra.Command, _ []string) error {
	_, explicit := selectedManifestPath(cmd)
	if !explicit {
		return nil
	}
	switch cmd.Name() {
	case "airplan", "list", "show", "get", "delete", "purge", "sync",
		"serve", "mcp":
		return nil
	default:
		return fmt.Errorf("--manifest does not apply to %s", cmd.CommandPath())
	}
}

func setupTargetClient(
	cmd *cobra.Command, path, flagProfile, target string,
) (*airplan.Client, *airplan.Config, context.Context,
	context.CancelFunc, error,
) {
	profile, inferred := inferManifestProfile(cmd, target, flagProfile)
	if inferred {
		// Manifest profile inference is a local S3 convenience. An airplan
		// profile would turn local history into an implicit HTTP redirect,
		// so fall back to ordinary profile resolution instead.
		inferredCfg, loadErr := airplan.LoadConfig(airplan.ConfigOptions{
			Path:    path,
			Profile: profile,
		})
		if loadErr == nil &&
			inferredCfg.EffectiveBackend() != airplan.BackendS3 {
			profile = flagProfile
			inferred = false
		}
	}
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
