package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listOptions struct {
	config      string
	profile     string
	columns     string
	json        bool
	remote      bool
	wide        bool
	reverse     bool
	allProfiles bool
	newerThan   string
	olderThan   string
	kind        string
	slug        string
	limit       int
}

func newListCmd() *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List uploads from the local manifest",
		Long: "List uploads from the local manifest, or with --remote, " +
			"from a live bucket listing using the selected config profile.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.config, "config", "",
		"config file path for --remote (default: XDG config dir)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"filter local history, or select config profile for --remote")
	f.BoolVarP(&opts.allProfiles, "all-profiles", "A", false,
		"list local S3 manifest history across every recorded profile")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")
	// SPEC.md §6 defines -r as the list --remote shorthand.
	f.BoolVarP(&opts.remote, "remote", "r", false,
		"list uploads from a live bucket listing instead of the manifest")
	f.StringVar(&opts.columns, "columns", "",
		"table columns (comma list, or additive +name/-name modifiers)")
	f.BoolVar(&opts.wide, "wide", false,
		"show every table column valid for the selected list mode")
	f.BoolVar(&opts.reverse, "reverse", false,
		"print newest uploads first")
	f.StringVar(&opts.newerThan, "newer-than", "",
		"filter: uploads at or after an absolute time or age")
	f.StringVar(&opts.olderThan, "older-than", "",
		"filter: uploads before an absolute time or age")
	f.IntVar(&opts.limit, "limit", 0,
		"keep the N most recent matching uploads")
	f.StringVar(&opts.kind, "kind", "",
		"filter: upload kind (document or collection)")
	f.StringVar(&opts.slug, "slug", "",
		"filter: glob pattern matched against document slugs")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	if err := validateListPresentationOptions(cmd.Flags().Changed, opts); err != nil {
		return err
	}
	filters, err := parseListFilters(cmd, opts, time.Now())
	if err != nil {
		return err
	}
	if opts.remote {
		return runRemoteList(cmd, opts, filters)
	}

	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		// Preserve config-free local history when the default config cannot
		// select one of several profiles, and preserve the historical use of
		// --profile as a local-manifest filter. Explicit config/backend
		// selectors remain authoritative and surface their errors.
		if !allowsConfigFreeLocalList(cmd) {
			return err
		}
		cfg = &airplan.Config{
			Backend: airplan.BackendS3, Profile: opts.profile,
		}
		if err := applyManifestSelection(cmd, cfg); err != nil {
			return err
		}
	}
	if opts.allProfiles && cfg.EffectiveBackend() == airplan.BackendAirplan {
		return errors.New("--all-profiles cannot be used with the airplan backend")
	}
	var profile *string
	if opts.allProfiles {
		profile = nil
	} else if cmd.Flags().Changed("profile") {
		profile = &opts.profile
	} else if os.Getenv("AIRPLAN_PROFILE") != "" {
		profile = &cfg.Profile
	}
	if cfg.EffectiveBackend() == airplan.BackendAirplan {
		ctx, cancel := timeoutContext(cmd.Context(), cfg)
		defer cancel()
		client, err := airplan.New(ctx, cfg)
		if err != nil {
			return err
		}
		listed, err := client.ListManifest(ctx,
			airplan.ListManifestOptions{Scope: airplan.ManifestScopeService})
		if err != nil {
			return err
		}
		return outputManifestList(
			cmd, listed.Records, listed.Warnings, opts, filters,
		)
	}
	listed, err := airplan.ListManifestHistory(cfg.ManifestPath, profile)
	if err != nil {
		return err
	}
	return outputManifestList(
		cmd, listed.Records, listed.Warnings, opts, filters,
	)
}

func allowsConfigFreeLocalList(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("config") {
		return false
	}
	if _, explicit := selectedManifestPath(cmd); explicit {
		return false
	}
	for _, name := range []string{
		"AIRPLAN_CONFIG", "AIRPLAN_BACKEND", "AIRPLAN_API_URL",
		"AIRPLAN_API_TOKEN", "AIRPLAN_PROFILE",
	} {
		if os.Getenv(name) != "" {
			return false
		}
	}
	return true
}

func outputManifestList(
	cmd *cobra.Command, uploads []airplan.ManifestRecord,
	warnings []string, opts *listOptions, filters listFilters,
) error {
	stderr := cmd.ErrOrStderr()
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}
	uploads = selectManifestList(uploads, filters)
	if opts.reverse {
		uploads = reverseManifestRecords(uploads)
	}
	if opts.json {
		if uploads == nil {
			uploads = []airplan.ManifestRecord{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(uploads)
	}

	rows := localListRows(uploads)
	columns, err := resolveListColumns(
		listModeLocal, opts.columns, cmd.Flags().Changed("columns"), opts.wide, rows,
	)
	if err != nil {
		return err
	}
	return printListTable(cmd.OutOrStdout(), rows, columns)
}

func runRemoteList(
	cmd *cobra.Command, opts *listOptions, filters listFilters,
) error {
	client, cfg, ctx, cancel, err := setupClient(
		cmd, opts.config, opts.profile)
	if err != nil {
		return err
	}
	defer cancel()

	uploads, err := client.ListRemote(ctx)
	if err != nil {
		return err
	}
	uploads = selectRemoteList(uploads, filters)
	if opts.reverse {
		uploads = reverseRemoteUploads(uploads)
	}
	if cfg.EffectiveBackend() == airplan.BackendS3 && cfg.PublicBaseURL == "" {
		for _, upload := range uploads {
			if upload.URL != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n",
					airplan.PublicURLFallbackWarning)
				break
			}
		}
	}

	if opts.json {
		if uploads == nil {
			uploads = []airplan.RemoteUpload{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(
			remoteListJSONRecords(uploads),
		)
	}

	rows := remoteListRows(uploads)
	columns, err := resolveListColumns(
		listModeRemote, opts.columns, cmd.Flags().Changed("columns"), opts.wide, rows,
	)
	if err != nil {
		return err
	}
	return printListTable(cmd.OutOrStdout(), rows, columns)
}

type remoteListJSONRecord struct {
	Time      time.Time          `json:"time"`
	Dir       string             `json:"dir"`
	MarkerKey string             `json:"marker_key"`
	Objects   int                `json:"objects"`
	Bytes     int64              `json:"bytes"`
	Slug      string             `json:"slug,omitempty"`
	Key       string             `json:"key,omitempty"`
	URL       string             `json:"url,omitempty"`
	Kind      airplan.UploadKind `json:"kind,omitempty"`
	Conflict  bool               `json:"conflict,omitempty"`
}

func remoteListJSONRecords(
	uploads []airplan.RemoteUpload,
) []remoteListJSONRecord {
	records := make([]remoteListJSONRecord, 0, len(uploads))
	for _, upload := range uploads {
		records = append(records, remoteListJSONRecord{
			Time:      upload.LastModified.UTC(),
			Dir:       upload.Dir,
			MarkerKey: upload.MarkerKey,
			Objects:   upload.Objects,
			Bytes:     upload.Bytes,
			Slug:      upload.Slug,
			Key:       upload.Key,
			URL:       upload.URL,
			Kind:      upload.Kind,
			Conflict:  upload.Conflict,
		})
	}
	return records
}

func reverseManifestRecords(records []airplan.ManifestRecord) []airplan.ManifestRecord {
	reversed := slices.Clone(records)
	slices.Reverse(reversed)
	return reversed
}

func reverseRemoteUploads(uploads []airplan.RemoteUpload) []airplan.RemoteUpload {
	reversed := slices.Clone(uploads)
	slices.Reverse(reversed)
	return reversed
}

func formatListBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	size := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	for i, unit := range units {
		size /= 1024
		rounded := math.Round(size*10) / 10
		if rounded < 1024 || i == len(units)-1 {
			value := strings.TrimSuffix(fmt.Sprintf("%.1f", rounded), ".0")
			return value + " " + unit
		}
	}

	return fmt.Sprintf("%d B", bytes)
}
