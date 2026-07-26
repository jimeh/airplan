package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listOptions struct {
	config    string
	profile   string
	json      bool
	remote    bool
	columns   string
	wide      bool
	reverse   bool
	newerThan string
	olderThan string
	limit     int
	kind      string
	slug      string
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
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a JSON array instead of a table")
	// SPEC.md §6 defines -r as the list --remote shorthand.
	f.BoolVarP(&opts.remote, "remote", "r", false,
		"list uploads from a live bucket listing instead of the manifest")
	// Column flags are long-only: -c would be confused with --config.
	f.StringVar(&opts.columns, "columns", "",
		"table columns: absolute set (date,title,url) or +/- adjustments "+
			"against the mode default (+dir,-title)")
	f.BoolVar(&opts.wide, "wide", false,
		"show every table column valid for the selected mode")
	f.BoolVar(&opts.reverse, "reverse", false,
		"print newest uploads first")
	f.StringVar(&opts.newerThan, "newer-than", "",
		"filter: uploads at or after this age or date "+
			"(7d, 2026-07-01, 2026-07-01T09:00:00Z)")
	f.StringVar(&opts.olderThan, "older-than", "",
		"filter: uploads before this age or date (30d, 2026-01-01)")
	f.IntVar(&opts.limit, "limit", 0,
		"keep only the N most recent matches "+
			"(still printed oldest first unless --reverse)")
	f.StringVar(&opts.kind, "kind", "",
		"filter: document or collection")
	f.StringVar(&opts.slug, "slug", "",
		"filter: glob matched against document slugs; "+
			"collections are excluded")

	return cmd
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	if err := validateListPresentationFlags(cmd, opts); err != nil {
		return err
	}
	filter, err := listFilterFromFlags(cmd, opts)
	if err != nil {
		return err
	}
	if opts.remote {
		return runRemoteList(cmd, opts, filter)
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
	var profile *string
	if cmd.Flags().Changed("profile") {
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
			cmd, filter.manifest(listed.Records), listed.Warnings, opts,
		)
	}
	listed, err := airplan.ListManifestHistory(cfg.ManifestPath, profile)
	if err != nil {
		return err
	}
	return outputManifestList(
		cmd, filter.manifest(listed.Records), listed.Warnings, opts,
	)
}

// listSelection applies the resolved list filters (SPEC.md §9). An
// explicit --limit 0 selects nothing, while the flag default means no
// limit.
type listSelection struct {
	filter    airplan.ListFilter
	limitZero bool
}

func (s listSelection) manifest(
	records []airplan.ManifestRecord,
) []airplan.ManifestRecord {
	if s.limitZero {
		return []airplan.ManifestRecord{}
	}
	return s.filter.FilterManifest(records)
}

func (s listSelection) remote(
	uploads []airplan.RemoteUpload,
) []airplan.RemoteUpload {
	if s.limitZero {
		return []airplan.RemoteUpload{}
	}
	return s.filter.FilterRemote(uploads)
}

func listFilterFromFlags(
	cmd *cobra.Command, opts *listOptions,
) (listSelection, error) {
	selection := listSelection{filter: airplan.ListFilter{
		Kind:  airplan.UploadKind(opts.kind),
		Slug:  opts.slug,
		Limit: opts.limit,
	}}
	now := time.Now()
	if opts.newerThan != "" {
		when, err := airplan.ParseTimeFilter(opts.newerThan, now)
		if err != nil {
			return selection, fmt.Errorf("--newer-than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		selection.filter.NewerThan = when
	}
	if opts.olderThan != "" {
		when, err := airplan.ParseTimeFilter(opts.olderThan, now)
		if err != nil {
			return selection, fmt.Errorf("--older-than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		selection.filter.OlderThan = when
	}
	selection.limitZero = cmd.Flags().Changed("limit") && opts.limit == 0
	if err := selection.filter.Validate(); err != nil {
		return selection, errors.New(
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	return selection, nil
}

// validateListPresentationFlags rejects invalid table-presentation flag
// combinations before any manifest or network access (SPEC.md §9).
func validateListPresentationFlags(
	cmd *cobra.Command, opts *listOptions,
) error {
	columnsSet := cmd.Flags().Changed("columns")
	if opts.json && columnsSet {
		return errors.New("--columns cannot be combined with --json")
	}
	if opts.json && opts.wide {
		return errors.New("--wide cannot be combined with --json")
	}
	if opts.wide && columnsSet {
		return errors.New("--columns cannot be combined with --wide")
	}
	// Validate column names early; auto rules do not affect validity.
	_, err := resolveListColumns(
		opts.remote, opts.columns, opts.wide, listAutoInfo{},
	)
	return err
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
	warnings []string, opts *listOptions,
) error {
	stderr := cmd.ErrOrStderr()
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", warning)
	}
	if opts.reverse {
		slices.Reverse(uploads)
	}
	if opts.json {
		if uploads == nil {
			uploads = []airplan.ManifestRecord{}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(uploads)
	}

	cols, err := resolveListColumns(
		false, opts.columns, opts.wide, manifestAutoInfo(uploads),
	)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return errors.New("--columns removes every column")
	}
	rows := make([][]string, 0, len(uploads))
	for _, rec := range uploads {
		row := make([]string, 0, len(cols))
		for _, col := range cols {
			row = append(row, col.localValue(rec))
		}
		rows = append(rows, row)
	}
	return printListTable(cmd.OutOrStdout(), cols, rows)
}

func runRemoteList(
	cmd *cobra.Command, opts *listOptions, filter listSelection,
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
	uploads = filter.remote(uploads)
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

	cols, err := resolveListColumns(
		true, opts.columns, opts.wide, remoteAutoInfo(uploads),
	)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return errors.New("--columns removes every column")
	}
	rows := make([][]string, 0, len(uploads))
	for _, upload := range uploads {
		row := make([]string, 0, len(cols))
		for _, col := range cols {
			row = append(row, col.remoteValue(upload))
		}
		rows = append(rows, row)
	}
	return printListTable(cmd.OutOrStdout(), cols, rows)
}

// columnMode describes how one column participates in a list mode
// (SPEC.md §9): unavailable, --wide only, automatic (default only when
// its rule carries information), or always in the default set.
type columnMode int

const (
	columnUnavailable columnMode = iota
	columnWide
	columnAuto
	columnDefault
)

// listAutoInfo carries the result-set facts that activate automatic
// columns (SPEC.md §9).
type listAutoInfo struct {
	// multiProfile is true when records span more than one profile.
	multiProfile bool
	// hasLegacy is true when at least one row is a legacy upload that
	// delete reconciliation and purge cannot touch.
	hasLegacy bool
	// missingURL is true when some row has no inferable URL, leaving
	// the directory as the only handle for show/delete.
	missingURL bool
}

type listColumnSpec struct {
	name   string
	header string
	local  columnMode
	remote columnMode
	// auto reports whether this automatic column carries information
	// for the current result set. Nil for non-auto columns.
	auto        func(listAutoInfo) bool
	localValue  func(airplan.ManifestRecord) string
	remoteValue func(airplan.RemoteUpload) string
}

func (s listColumnSpec) mode(remote bool) columnMode {
	if remote {
		return s.remote
	}
	return s.local
}

func (s listColumnSpec) inDefault(remote bool, auto listAutoInfo) bool {
	switch s.mode(remote) {
	case columnDefault:
		return true
	case columnAuto:
		return s.auto(auto)
	default:
		return false
	}
}

// listColumns is the shared column registry for local and remote list
// (SPEC.md §9). Slice order is the canonical table order; url is last.
func listColumns() []listColumnSpec {
	return []listColumnSpec{
		{
			name: "date", header: "DATE",
			local: columnDefault, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				return formatListDate(rec.Time)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return formatListDate(upload.LastModified)
			},
		},
		{
			name: "kind", header: "KIND",
			local: columnDefault, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Kind)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				if upload.Conflict {
					return "conflict"
				}
				return orDash(string(upload.Kind))
			},
		},
		{
			name: "title", header: "TITLE",
			local: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Title)
			},
		},
		{
			name: "objects", header: "OBJECTS",
			local: columnDefault, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				// Declared counts are absent on records that predate
				// them (SPEC.md §9); render "-" without warning.
				if rec.Objects <= 0 {
					return "-"
				}
				return strconv.Itoa(rec.Objects)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return strconv.Itoa(upload.Objects)
			},
		},
		{
			name: "size", header: "SIZE",
			local: columnDefault, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				// The whole-upload declared size; page bytes stay on the
				// page-size column (SPEC.md §9).
				if rec.TotalBytes <= 0 {
					return "-"
				}
				return formatListBytes(rec.TotalBytes)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return formatListBytes(upload.Bytes)
			},
		},
		{
			name: "slug", header: "SLUG",
			local: columnWide, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Slug)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return orDash(upload.Slug)
			},
		},
		{
			name: "profile", header: "PROFILE",
			local: columnAuto,
			auto:  func(auto listAutoInfo) bool { return auto.multiProfile },
			localValue: func(rec airplan.ManifestRecord) string {
				if rec.Profile == "" {
					return "<root>"
				}
				return rec.Profile
			},
		},
		{
			name: "state", header: "STATE",
			local: columnAuto,
			auto:  func(auto listAutoInfo) bool { return auto.hasLegacy },
			localValue: func(rec airplan.ManifestRecord) string {
				if airplan.IsSupportedMarkerVersion(rec.MarkerVersion) {
					return "managed"
				}
				return "legacy"
			},
		},
		{
			name: "dir", header: "DIRECTORY",
			local: columnAuto, remote: columnAuto,
			auto: func(auto listAutoInfo) bool { return auto.missingURL },
			localValue: func(rec airplan.ManifestRecord) string {
				dir := path.Base(path.Dir(rec.Key))
				if dir == "." || dir == "/" {
					return "-"
				}
				return dir
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return orDash(upload.Dir)
			},
		},
		{
			name: "page-size", header: "PAGE-SIZE",
			local: columnWide,
			localValue: func(rec airplan.ManifestRecord) string {
				return formatListBytes(rec.Bytes)
			},
		},
		{
			name: "format", header: "FORMAT",
			local: columnWide,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Format)
			},
		},
		{
			name: "repo", header: "REPO",
			local: columnWide,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Repo)
			},
		},
		{
			name: "bucket", header: "BUCKET",
			local: columnWide,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.Bucket)
			},
		},
		{
			name: "url", header: "URL",
			local: columnDefault, remote: columnDefault,
			localValue: func(rec airplan.ManifestRecord) string {
				return orDash(rec.URL)
			},
			remoteValue: func(upload airplan.RemoteUpload) string {
				return orDash(upload.URL)
			},
		},
	}
}

func manifestAutoInfo(uploads []airplan.ManifestRecord) listAutoInfo {
	auto := listAutoInfo{}
	profiles := make(map[string]struct{})
	for _, rec := range uploads {
		profiles[rec.Profile] = struct{}{}
		if !airplan.IsSupportedMarkerVersion(rec.MarkerVersion) {
			auto.hasLegacy = true
		}
		if rec.URL == "" {
			auto.missingURL = true
		}
	}
	auto.multiProfile = len(profiles) > 1
	return auto
}

func remoteAutoInfo(uploads []airplan.RemoteUpload) listAutoInfo {
	auto := listAutoInfo{}
	for _, upload := range uploads {
		if upload.URL == "" {
			auto.missingURL = true
		}
	}
	return auto
}

// resolveListColumns selects the table columns for one list invocation
// (SPEC.md §9): the mode default plus firing automatic columns, every
// valid column under --wide, or the --columns selection.
func resolveListColumns(
	remote bool, columnsFlag string, wide bool, auto listAutoInfo,
) ([]listColumnSpec, error) {
	specs := listColumns()
	if wide {
		out := make([]listColumnSpec, 0, len(specs))
		for _, spec := range specs {
			if spec.mode(remote) != columnUnavailable {
				out = append(out, spec)
			}
		}
		return out, nil
	}
	if columnsFlag == "" {
		var out []listColumnSpec
		for _, spec := range specs {
			if spec.inDefault(remote, auto) {
				out = append(out, spec)
			}
		}
		return out, nil
	}
	return parseColumnsFlag(specs, remote, columnsFlag, auto)
}

func parseColumnsFlag(
	specs []listColumnSpec, remote bool, flag string, auto listAutoInfo,
) ([]listColumnSpec, error) {
	entries := strings.Split(flag, ",")
	names := make([]string, 0, len(entries))
	ops := make([]byte, 0, len(entries))
	adjustments := false
	absolute := false
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		op := byte(0)
		if strings.HasPrefix(entry, "+") || strings.HasPrefix(entry, "-") {
			op = entry[0]
			entry = entry[1:]
			adjustments = true
		} else {
			absolute = true
		}
		if entry == "" {
			return nil, errors.New("--columns: empty column name")
		}
		names = append(names, entry)
		ops = append(ops, op)
	}
	if adjustments && absolute {
		return nil, errors.New(
			"--columns cannot mix an absolute column list with " +
				"+/- adjustments")
	}

	if absolute {
		seen := make(map[string]struct{}, len(names))
		out := make([]listColumnSpec, 0, len(names))
		for _, name := range names {
			index, err := listColumnIndex(specs, remote, name)
			if err != nil {
				return nil, err
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, fmt.Errorf(
					"--columns lists column %q more than once", name)
			}
			seen[name] = struct{}{}
			out = append(out, specs[index])
		}
		return out, nil
	}

	selected := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.inDefault(remote, auto) {
			selected[spec.name] = true
		}
	}
	for position, name := range names {
		if _, err := listColumnIndex(specs, remote, name); err != nil {
			return nil, err
		}
		selected[name] = ops[position] == '+'
	}
	var out []listColumnSpec
	for _, spec := range specs {
		if selected[spec.name] {
			out = append(out, spec)
		}
	}
	// An empty adjusted selection is not rejected here: automatic
	// columns may still join once the real result set is known, so the
	// zero-column check lives on the render path.
	return out, nil
}

func listColumnIndex(
	specs []listColumnSpec, remote bool, name string,
) (int, error) {
	for index, spec := range specs {
		if spec.name != name {
			continue
		}
		if spec.mode(remote) != columnUnavailable {
			return index, nil
		}
		if remote {
			return 0, fmt.Errorf(
				"column %q is only available in local list, "+
					"not with --remote", name)
		}
		return 0, fmt.Errorf(
			"column %q is only available with --remote", name)
	}
	return 0, fmt.Errorf("unknown column %q (valid columns: %s)",
		name, strings.Join(validListColumnNames(specs, remote), ", "))
}

func validListColumnNames(specs []listColumnSpec, remote bool) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.mode(remote) != columnUnavailable {
			names = append(names, spec.name)
		}
	}
	return names
}

func printListTable(
	w io.Writer, cols []listColumnSpec, rows [][]string,
) error {
	if len(rows) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	headers := make([]string, 0, len(cols))
	for _, col := range cols {
		headers = append(headers, col.header)
	}
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

func formatListDate(when time.Time) string {
	return when.UTC().Format("2006-01-02 15:04")
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
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
