package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listOptions struct {
	config      string
	profile     string
	json        bool
	remote      bool
	columns     string
	wide        bool
	reverse     bool
	newerThan   string
	olderThan   string
	limit       int
	kind        string
	slug        string
	protected   bool
	noProtected bool
	allProfiles bool
}

// columnRequest returns the table column selection given on the command line.
func (o *listOptions) columnRequest(cmd *cobra.Command) listColumnRequest {
	return listColumnRequest{
		spec:    o.columns,
		changed: cmd.Flags().Changed("columns"),
		wide:    o.wide,
	}
}

// listFilter resolves the selection flags into a library filter (SPEC.md §9).
// Times are interpreted relative to now, so ages and local dates resolve once
// per invocation.
func (o *listOptions) listFilter(
	cmd *cobra.Command, now time.Time,
) (airplan.ListFilter, error) {
	if o.protected && o.noProtected {
		return airplan.ListFilter{}, errors.New(
			"--protected cannot be combined with --no-protected")
	}
	filter := airplan.ListFilter{
		Kind: airplan.UploadKind(o.kind), Slug: o.slug,
	}
	if o.protected || o.noProtected {
		protected := o.protected
		filter.Protected = &protected
	}
	if cmd.Flags().Changed("newer-than") {
		when, err := airplan.ParseTimeFilter(o.newerThan, now)
		if err != nil {
			return filter, flagError("--newer-than", err)
		}
		filter.NewerThan = &when
	}
	if cmd.Flags().Changed("older-than") {
		when, err := airplan.ParseTimeFilter(o.olderThan, now)
		if err != nil {
			return filter, flagError("--older-than", err)
		}
		filter.OlderThan = &when
	}
	if cmd.Flags().Changed("limit") {
		limit := o.limit
		filter.Limit = &limit
	}

	// Validating one field at a time keeps the library's rules authoritative
	// while still naming the flag that carried the bad value.
	if err := (airplan.ListFilter{Kind: filter.Kind}).Validate(); err != nil {
		return filter, flagError("--kind", err)
	}
	if cmd.Flags().Changed("kind") && strings.TrimSpace(o.kind) == "" {
		return filter, errors.New("--kind requires a non-empty value")
	}
	if err := (airplan.ListFilter{Slug: filter.Slug}).Validate(); err != nil {
		return filter, flagError("--slug", err)
	}
	if cmd.Flags().Changed("slug") && o.slug == "" {
		return filter, errors.New("--slug requires a non-empty value")
	}
	if err := (airplan.ListFilter{Limit: filter.Limit}).Validate(); err != nil {
		return filter, fmt.Errorf("--limit %s",
			strings.TrimPrefix(err.Error(), "airplan: limit "))
	}
	return filter, filter.Validate()
}

// flagError restates a library error as a flag-scoped command-line error.
func flagError(flag string, err error) error {
	return fmt.Errorf("%s: %s", flag,
		strings.TrimPrefix(err.Error(), "airplan: "))
}

func newListCmd() *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List uploads from the local manifest",
		Long: "List uploads from the local manifest, or with --remote, " +
			"from a live bucket listing using the selected config profile.\n\n" +
			"Local listing uses the resolved config profile by default. " +
			"--profile NAME selects another profile, --profile= selects " +
			"root-level history, and --all-profiles lists every profile.",
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
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")
	// SPEC.md §6 defines -r as the list --remote shorthand.
	f.BoolVarP(&opts.remote, "remote", "r", false,
		"list uploads from a live bucket listing instead of the manifest")
	// Column selection is long-only: -c already means --config (SPEC.md §6).
	f.StringVar(&opts.columns, "columns", "",
		"table columns: an absolute set (date,title,url) or adjustments "+
			"to the default set (+dir,-title)")
	f.BoolVar(&opts.wide, "wide", false,
		"show every table column available for this listing")
	f.BoolVar(&opts.reverse, "reverse", false,
		"print newest uploads first")
	// Selection flags (SPEC.md §9) apply to the table and to --json alike.
	f.StringVar(&opts.newerThan, "newer-than", "",
		"filter: uploads at or after an age or date, e.g. 7d or 2026-07-01")
	f.StringVar(&opts.olderThan, "older-than", "",
		"filter: uploads before an age or date, e.g. 30d or 2026-07-01")
	f.IntVar(&opts.limit, "limit", 0,
		"filter: keep only the N most recent matches, still printed "+
			"oldest first")
	f.StringVar(&opts.kind, "kind", "",
		"filter: document or collection")
	f.StringVar(&opts.slug, "slug", "",
		"filter: glob matched against document slugs; collections never match")
	f.BoolVar(&opts.protected, "protected", false,
		"filter: only purge-protected uploads")
	f.BoolVar(&opts.noProtected, "no-protected", false,
		"filter: only uploads without purge protection")
	f.BoolVarP(&opts.allProfiles, "all-profiles", "A", false,
		"list every recorded profile")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	mode := listModeLocal
	if opts.remote {
		mode = listModeRemote
	}
	if err := opts.columnRequest(cmd).validate(mode, opts.json); err != nil {
		return err
	}
	if opts.allProfiles {
		if cmd.Flags().Changed("profile") {
			return errors.New(
				"--all-profiles cannot be combined with --profile")
		}
		// Remote listing is scoped to the selected profile's key_prefix by
		// storage layout, not by choice (SPEC.md §9).
		if opts.remote {
			return errors.New(
				"--all-profiles cannot be combined with --remote")
		}
	}
	filter, err := opts.listFilter(cmd, time.Now())
	if err != nil {
		return err
	}
	if opts.remote {
		return runRemoteList(cmd, opts, filter)
	}

	cfg, err := loadCommandConfig(cmd, opts.config, opts.profile)
	if err != nil {
		// --all-profiles needs no active connection profile, so it can preserve
		// config-free local history. Ordinary list resolves one profile and
		// surfaces ambiguity instead. Explicit config/backend selectors remain
		// authoritative and surface their errors.
		if !opts.allProfiles || !allowsConfigFreeAllProfilesList(cmd) {
			return err
		}
		cfg = &airplan.Config{
			Backend: airplan.BackendS3, Profile: opts.profile,
		}
		if err := applyManifestSelection(cmd, cfg); err != nil {
			return err
		}
	}
	var profile *string
	switch {
	case opts.allProfiles:
		profile = nil
	case cmd.Flags().Changed("profile"):
		profile = &opts.profile
	default:
		profile = &cfg.Profile
	}
	if cfg.EffectiveBackend() == airplan.BackendAirplan {
		if opts.allProfiles {
			return errors.New(
				"--all-profiles cannot be used with the airplan backend")
		}
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
			cmd, opts, filter, listed.Records, listed.Warnings,
		)
	}
	listed, err := airplan.ListManifestHistory(cfg.ManifestPath, profile)
	if err != nil {
		return err
	}
	return outputManifestList(
		cmd, opts, filter, listed.Records, listed.Warnings,
	)
}

func allowsConfigFreeAllProfilesList(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("config") {
		return false
	}
	if _, explicit := selectedManifestPath(cmd); explicit {
		return false
	}
	selectors := []string{
		"AIRPLAN_CONFIG", "AIRPLAN_BACKEND", "AIRPLAN_API_URL",
		"AIRPLAN_API_TOKEN",
	}
	for _, name := range selectors {
		if os.Getenv(name) != "" {
			return false
		}
	}
	return true
}

func outputManifestList(
	cmd *cobra.Command, opts *listOptions, filter airplan.ListFilter,
	uploads []airplan.ManifestRecord, warnings []string,
) error {
	stderr := cmd.ErrOrStderr()
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}
	uploads = filter.FilterManifestRecords(uploads)
	if opts.reverse {
		slices.Reverse(uploads)
	}
	if opts.json {
		if uploads == nil {
			uploads = []airplan.ManifestRecord{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(uploads)
	}

	columns, err := selectListColumns(
		listModeLocal, opts.columnRequest(cmd), autoLocalListColumns(uploads),
	)
	if err != nil {
		return err
	}
	return printUploadTable(cmd.OutOrStdout(), uploads, columns)
}

func runRemoteList(
	cmd *cobra.Command, opts *listOptions, filter airplan.ListFilter,
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
	uploads = filter.FilterRemoteUploads(uploads)
	if cfg.EffectiveBackend() == airplan.BackendS3 && cfg.PublicBaseURL == "" {
		for _, upload := range uploads {
			if upload.URL != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n",
					airplan.PublicURLFallbackWarning)
				break
			}
		}
	}
	if opts.reverse {
		slices.Reverse(uploads)
	}

	if opts.json {
		if uploads == nil {
			uploads = []airplan.RemoteUpload{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(
			remoteListJSONRecords(uploads),
		)
	}

	columns, err := selectListColumns(
		listModeRemote, opts.columnRequest(cmd),
		autoRemoteListColumns(uploads),
	)
	if err != nil {
		return err
	}
	return printRemoteUploadTable(cmd.OutOrStdout(), uploads, columns)
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
	Protected bool               `json:"protected,omitempty"`
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
			Protected: upload.Protected,
		})
	}
	return records
}

func printRemoteUploadTable(
	w io.Writer, uploads []airplan.RemoteUpload, columns []listColumn,
) error {
	rows := make([][]string, 0, len(uploads))
	for _, upload := range uploads {
		row := make([]string, 0, len(columns))
		for _, column := range columns {
			row = append(row, remoteListCell(column.name, upload))
		}
		rows = append(rows, row)
	}
	return printListTable(w, columns, rows)
}

func printUploadTable(
	w io.Writer, uploads []airplan.ManifestRecord, columns []listColumn,
) error {
	rows := make([][]string, 0, len(uploads))
	for _, upload := range uploads {
		row := make([]string, 0, len(columns))
		for _, column := range columns {
			row = append(row, localListCell(column.name, upload))
		}
		rows = append(rows, row)
	}
	return printListTable(w, columns, rows)
}

// printListTable writes an aligned table, or nothing at all when there are no
// rows, so an empty listing keeps stdout empty (SPEC.md §1).
func printListTable(
	w io.Writer, columns []listColumn, rows [][]string,
) error {
	if len(rows) == 0 {
		return nil
	}

	headers := make([]string, 0, len(columns))
	for _, column := range columns {
		headers = append(headers, column.header)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
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
