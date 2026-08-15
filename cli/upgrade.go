package cli

import (
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
	client, cfg, ctx, cancel, err := setupClient(cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	defer cancel()
	if opts.allProfiles && cfg.EffectiveBackend() != airplan.BackendS3 {
		return errors.New("--all-profiles is available only with the s3 backend")
	}
	plan, err := client.PlanBulkUpgrade(ctx, airplan.BulkUpgradeOptions{Concurrency: opts.concurrency})
	if err != nil {
		return err
	}
	clients := map[string]*airplan.Client{cfg.Profile: client}
	profileConfigs := map[string]*airplan.Config{cfg.Profile: cfg}
	if opts.allProfiles {
		profiles, profileErr := airplan.ListConfigProfiles(airplan.ConfigProfilesOptions{Path: opts.config})
		if profileErr != nil {
			return profileErr
		}
		seen := map[string]struct{}{}
		for _, item := range plan.Items {
			seen[item.Bucket+"\x00"+item.MarkerKey] = struct{}{}
		}
		for _, profile := range profiles.Profiles {
			if _, ok := clients[profile.Name]; ok {
				continue
			}
			profileCfg, loadErr := airplan.LoadConfig(airplan.ConfigOptions{Path: opts.config, Profile: profile.Name})
			if loadErr != nil {
				return fmt.Errorf("airplan: load profile %q: %w", profile.Name, loadErr)
			}
			profileCfg.ProducerVersion = buildVersion()
			profileCfg.ManifestPath = cfg.ManifestPath
			profileClient, newErr := airplan.New(ctx, profileCfg)
			if newErr != nil {
				return newErr
			}
			clients[profile.Name] = profileClient
			profileConfigs[profile.Name] = profileCfg
			profilePlan, planErr := profileClient.PlanBulkUpgrade(ctx, airplan.BulkUpgradeOptions{Concurrency: opts.concurrency})
			if planErr != nil {
				return planErr
			}
			for _, item := range profilePlan.Items {
				identity := item.Bucket + "\x00" + item.MarkerKey
				if _, ok := seen[identity]; ok {
					continue
				}
				seen[identity] = struct{}{}
				plan.Items = append(plan.Items, item)
				plan.Counts[item.State]++
			}
			plan.Warnings = append(plan.Warnings, profilePlan.Warnings...)
		}
		allRecords, listErr := client.ListManifest(ctx, airplan.ListManifestOptions{
			Scope: airplan.ManifestScopeAll,
		})
		if listErr != nil {
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
	if opts.json && (opts.dryRun || len(plan.Items) == 0) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
	}
	if !opts.json {
		printUpgradePlan(cmd.OutOrStdout(), plan)
	}
	if opts.dryRun || len(plan.Items) == 0 {
		return nil
	}
	upgradeable := make([]airplan.UpgradeDocumentPlan, 0)
	for _, item := range plan.Items {
		if item.State == airplan.UpgradeStateUpgradeable {
			upgradeable = append(upgradeable, item)
		}
	}
	if len(upgradeable) == 0 {
		return nil
	}
	if !opts.yes {
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
			return nil
		}
	}
	var result *airplan.BulkUpgradeResult
	if !opts.allProfiles {
		result, err = client.ExecuteBulkUpgrade(ctx, airplan.BulkUpgradeRequest{Items: upgradeable, Concurrency: opts.concurrency})
	} else {
		result = executeAllProfileUpgrades(ctx, clients, upgradeable, opts.concurrency)
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
	var answer string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &answer); err != nil {
		if errors.Is(err, io.EOF) {
			return false, errors.New("airplan: confirmation input closed; rerun with --yes")
		}
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
