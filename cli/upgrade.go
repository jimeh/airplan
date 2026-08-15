package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type upgradeOptions struct {
	config, profile   string
	template          string
	check, force      bool
	all, allProfiles  bool
	dryRun, yes, json bool
	concurrency       int
}

func newUpgradeCmd() *cobra.Command {
	opts := &upgradeOptions{}
	cmd := &cobra.Command{
		Use: "upgrade [url|key]", Short: "Upgrade rendered Markdown uploads",
		Args: cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error { return runUpgrade(cmd, args, opts) },
	}
	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "", "config file path")
	f.StringVarP(&opts.profile, "profile", "p", "", "config profile name")
	f.StringVar(&opts.template, "template", "", "custom page template file")
	f.BoolVar(&opts.check, "check", false, "classify without changing storage")
	f.BoolVar(&opts.force, "force", false, "re-render even when already current")
	f.BoolVar(&opts.all, "all", false, "upgrade all eligible active manifest records")
	f.BoolVar(&opts.allProfiles, "all-profiles", false, "include every named configuration profile")
	f.BoolVar(&opts.dryRun, "dry-run", false, "preview all classifications without writing")
	f.BoolVar(&opts.yes, "yes", false, "skip the bulk confirmation prompt")
	f.BoolVarP(&opts.json, "json", "j", false, "print one JSON result")
	f.IntVar(&opts.concurrency, "concurrency", 4, "maximum concurrent upgrades (1-32)")
	return cmd
}

func runUpgrade(cmd *cobra.Command, args []string, opts *upgradeOptions) error {
	if opts.allProfiles && cmd.Flags().Changed("profile") {
		return errors.New("--all-profiles cannot be combined with --profile")
	}
	if opts.allProfiles && !opts.all {
		return errors.New("--all-profiles requires --all")
	}
	if opts.all {
		if len(args) != 0 {
			return errors.New("--all does not accept a target")
		}
		return runBulkUpgrade(cmd, opts)
	}
	if len(args) != 1 {
		return errors.New("upgrade requires one target or --all")
	}
	if opts.dryRun || opts.yes || opts.allProfiles {
		return errors.New("--dry-run, --yes, and --all-profiles require --all")
	}
	client, cfg, ctx, cancel, err := setupTargetClient(cmd, opts.config, opts.profile, args[0])
	if err != nil {
		return err
	}
	defer cancel()
	if cmd.Flags().Changed("template") {
		if cfg.EffectiveBackend() == airplan.BackendAirplan {
			return errors.New("--template is controlled by the Airplan server")
		}
		cfg.Template = opts.template
		client, err = airplan.New(ctx, cfg)
		if err != nil {
			return err
		}
	}
	plan, err := client.PlanUpgradeDocument(ctx, args[0], airplan.UpgradeDocumentOptions{Force: opts.force})
	if err != nil {
		return err
	}
	if opts.check {
		return writeUpgradeJSONOrLine(cmd, opts.json, plan, fmt.Sprintf("%s\t%s\t%s", plan.State, plan.Reason, plan.URL))
	}
	if plan.State == airplan.UpgradeStateCurrent {
		return writeUpgradeJSONOrLine(cmd, opts.json, plan, plan.URL)
	}
	if plan.State != airplan.UpgradeStateUpgradeable {
		return fmt.Errorf("airplan: document is %s: %s", plan.State, plan.Reason)
	}
	result, err := client.UpgradeDocument(ctx, *plan)
	if err != nil {
		return err
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Result.URL)
	return err
}

func runBulkUpgrade(cmd *cobra.Command, opts *upgradeOptions) error {
	if opts.force || opts.check {
		return errors.New("--force and --check apply only to one target")
	}
	if cmd.Flags().Changed("template") {
		return errors.New("--template applies only to one target")
	}
	if opts.concurrency < 1 || opts.concurrency > 32 {
		return errors.New("--concurrency must be between 1 and 32")
	}
	client, cfg, clients, profileConfigs, profileOrder, err := prepareBulkUpgradeClients(cmd, opts)
	if err != nil {
		return err
	}
	planCtx, planCancel := timeoutContext(cmd.Context(), cfg)
	plan := &airplan.BulkUpgradePlan{
		Items: []airplan.UpgradeDocumentPlan{}, Counts: map[airplan.UpgradeState]int{},
	}
	seen := map[string]struct{}{}
	for _, profile := range profileOrder {
		profilePlan, planErr := clients[profile].PlanBulkUpgrade(planCtx,
			airplan.BulkUpgradeOptions{Concurrency: opts.concurrency})
		if planErr != nil {
			planCancel()
			return planErr
		}
		for _, item := range profilePlan.Items {
			identity := item.Bucket + "\x00" + item.MarkerKey
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			plan.Items = append(plan.Items, item)
			plan.Counts[item.State]++
		}
		plan.Warnings = append(plan.Warnings, profilePlan.Warnings...)
	}
	if opts.allProfiles {
		allRecords, listErr := client.ListManifest(planCtx, airplan.ListManifestOptions{
			Scope: airplan.ManifestScopeAll,
		})
		if listErr != nil {
			planCancel()
			return listErr
		}
		for _, record := range allRecords.Records {
			markerKey := record.MarkerKey
			if markerKey == "" || record.MarkerVersion == 0 {
				continue
			}
			identity := record.Bucket + "\x00" + markerKey
			if _, ok := seen[identity]; ok {
				continue
			}
			profileCfg := profileConfigs[record.Profile]
			reason := "referenced profile is missing from current configuration"
			if profileCfg != nil {
				if record.Bucket != profileCfg.Bucket ||
					!airplan.KeyMatchesPrefix(markerKey, profileCfg.KeyPrefix) {
					reason = "recorded bucket or key prefix differs from current profile configuration"
				} else {
					continue
				}
			}
			seen[identity] = struct{}{}
			item := airplan.UpgradeDocumentPlan{
				Target: markerKey, Profile: record.Profile, Bucket: record.Bucket,
				State: airplan.UpgradeStateInvalid, Reason: reason,
				TargetMarkerVersion:      airplan.MarkerVersion,
				TargetRendererGeneration: airplan.RendererGeneration,
			}
			plan.Items = append(plan.Items, item)
			plan.Counts[item.State]++
		}
		plan.Warnings = append(plan.Warnings, allRecords.Warnings...)
	}
	planCancel()
	if opts.json && opts.dryRun {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}
	if !opts.json {
		printUpgradePlan(cmd.ErrOrStderr(), plan)
	}
	if opts.dryRun {
		return nil
	}
	upgradeable := make([]airplan.UpgradeDocumentPlan, 0)
	for _, item := range plan.Items {
		if item.State == airplan.UpgradeStateUpgradeable {
			upgradeable = append(upgradeable, item)
		}
	}
	if len(upgradeable) > 0 && !opts.yes {
		if file, ok := cmd.InOrStdin().(*os.File); ok {
			if info, statErr := file.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice == 0 {
				return errors.New("airplan: non-interactive bulk upgrade requires --yes")
			}
		}
		confirmed, confirmErr := confirmUpgrade(cmd, len(upgradeable))
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintln(cmd.ErrOrStderr(), "aborted")
			if opts.json {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(
					&airplan.BulkUpgradeResult{
						Items: []airplan.BulkUpgradeItemResult{},
					},
				)
			}
			return nil
		}
	}
	result := &airplan.BulkUpgradeResult{Items: []airplan.BulkUpgradeItemResult{}}
	if len(upgradeable) > 0 {
		execCtx, execCancel := timeoutContext(cmd.Context(), cfg)
		defer execCancel()
		if !opts.allProfiles {
			result, err = client.ExecuteBulkUpgrade(execCtx, airplan.BulkUpgradeRequest{Items: upgradeable, Concurrency: opts.concurrency})
		} else {
			result = executeAllProfileUpgrades(execCtx, clients, upgradeable, opts.concurrency)
		}
	}
	if result == nil {
		if err != nil {
			return err
		}
		return errors.New("airplan: bulk upgrade returned no result")
	}
	if opts.json {
		if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(result); encodeErr != nil {
			return encodeErr
		}
		if err != nil {
			return err
		}
		if result.Failed > 0 {
			return errors.New("airplan: one or more upgrades failed")
		}
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "upgraded %d uploads (%d failed)\n", result.Upgraded, result.Failed)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return errors.New("airplan: one or more upgrades failed")
	}
	return nil
}

func prepareBulkUpgradeClients(
	cmd *cobra.Command, opts *upgradeOptions,
) (*airplan.Client, *airplan.Config, map[string]*airplan.Client,
	map[string]*airplan.Config, []string, error,
) {
	if !opts.allProfiles {
		cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		client, err := airplan.New(cmd.Context(), cfg)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		return client, cfg,
			map[string]*airplan.Client{cfg.Profile: client},
			map[string]*airplan.Config{cfg.Profile: cfg},
			[]string{cfg.Profile}, nil
	}

	inventory, err := airplan.ListConfigProfiles(
		airplan.ConfigProfilesOptions{Path: opts.config},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	configs := make(map[string]*airplan.Config)
	order := make([]string, 0, len(inventory.Profiles))
	if len(inventory.Profiles) == 0 {
		getenv := func(name string) string {
			if name == "AIRPLAN_PROFILE" {
				return ""
			}
			return os.Getenv(name)
		}
		cfg, loadErr := airplan.LoadConfig(airplan.ConfigOptions{
			Path: opts.config, Profile: opts.profile, Getenv: getenv,
		})
		if loadErr != nil {
			return nil, nil, nil, nil, nil, loadErr
		}
		if err := applyManifestSelection(cmd, cfg); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		cfg.ProducerVersion = buildVersion()
		for _, warning := range cfg.Warnings {
			fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", warning)
		}
		configs[cfg.Profile] = cfg
		order = append(order, cfg.Profile)
	} else {
		for _, profile := range inventory.Profiles {
			cfg, loadErr := loadCommandConfig(cmd, opts.config, profile.Name)
			if loadErr != nil {
				return nil, nil, nil, nil, nil,
					fmt.Errorf("airplan: load profile %q: %w", profile.Name, loadErr)
			}
			configs[profile.Name] = cfg
			order = append(order, profile.Name)
		}
	}
	for _, profile := range order {
		if configs[profile].EffectiveBackend() != airplan.BackendS3 {
			return nil, nil, nil, nil, nil, fmt.Errorf(
				"--all-profiles requires s3 profiles; profile %q uses %s",
				profile, configs[profile].EffectiveBackend(),
			)
		}
	}
	clients := make(map[string]*airplan.Client, len(order))
	for _, profile := range order {
		client, newErr := airplan.New(cmd.Context(), configs[profile])
		if newErr != nil {
			return nil, nil, nil, nil, nil, newErr
		}
		clients[profile] = client
	}
	primary := order[0]
	return clients[primary], configs[primary], clients, configs, order, nil
}

func executeAllProfileUpgrades(
	ctx context.Context, clients map[string]*airplan.Client,
	items []airplan.UpgradeDocumentPlan, concurrency int,
) *airplan.BulkUpgradeResult {
	result := &airplan.BulkUpgradeResult{
		Items: make([]airplan.BulkUpgradeItemResult, len(items)),
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				item := items[index]
				profileClient := clients[item.Profile]
				result.Items[index].Plan = item
				if profileClient == nil {
					result.Items[index].Error = "profile is unavailable"
					continue
				}
				upgraded, err := profileClient.UpgradeDocument(ctx, item)
				if err != nil {
					result.Items[index].Error = err.Error()
				} else {
					result.Items[index].Result = upgraded
				}
			}
		}()
	}
	for index := range items {
		select {
		case jobs <- index:
		case <-ctx.Done():
			for remaining := index; remaining < len(items); remaining++ {
				result.Items[remaining] = airplan.BulkUpgradeItemResult{
					Plan: items[remaining], Error: "upgrade context expired before start",
				}
			}
			close(jobs)
			wg.Wait()
			return countAllProfileUpgradeResult(result)
		}
	}
	close(jobs)
	wg.Wait()
	return countAllProfileUpgradeResult(result)
}

func countAllProfileUpgradeResult(result *airplan.BulkUpgradeResult) *airplan.BulkUpgradeResult {
	for _, item := range result.Items {
		if item.Result != nil && item.Result.Upgraded {
			result.Upgraded++
		}
		if item.Error != "" {
			result.Failed++
		}
	}
	return result
}

func writeUpgradeJSONOrLine(cmd *cobra.Command, asJSON bool, value any, line string) error {
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
	return err
}

func printUpgradePlan(w io.Writer, plan *airplan.BulkUpgradePlan) {
	fmt.Fprintln(w, "STATE\tCURRENT\tTARGET\tURL")
	for _, item := range plan.Items {
		fmt.Fprintf(w, "%s\tmarker %d / renderer %d\tmarker %d / renderer %d\t%s\n", item.State, item.CurrentMarkerVersion, item.CurrentRendererGeneration, item.TargetMarkerVersion, item.TargetRendererGeneration, item.URL)
	}
}

func confirmUpgrade(cmd *cobra.Command, count int) (bool, error) {
	fmt.Fprintf(cmd.ErrOrStderr(), "Upgrade %d uploads? [y/N] ", count)
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
