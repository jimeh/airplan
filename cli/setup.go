package cli

import (
	"context"
	"fmt"

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
