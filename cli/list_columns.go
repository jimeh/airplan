package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jimeh/airplan/airplan"
)

// listMode selects the column vocabulary of a listing (SPEC.md §9). Local
// listings describe manifest records; remote listings describe marker
// directories discovered by a live bucket listing.
type listMode int

const (
	listModeLocal listMode = iota
	listModeRemote
)

// listColumnRole is a column's availability within one listing mode.
type listColumnRole int

const (
	// listColumnUnavailable means the mode has no value for the column, so
	// requesting it is an error rather than a blank column.
	listColumnUnavailable listColumnRole = iota
	// listColumnWide selects the column only under --wide or an explicit
	// --columns request.
	listColumnWide
	// listColumnAuto additionally selects the column when its automatic rule
	// fires for the printed result set.
	listColumnAuto
	// listColumnDefault always selects the column.
	listColumnDefault
)

// listColumn is one column of the shared local/remote list registry.
type listColumn struct {
	// name is the --columns identifier.
	name string
	// header is the table header cell.
	header string
	// local and remote are the column's role in each listing mode.
	local  listColumnRole
	remote listColumnRole
}

// role returns the column's availability in mode.
func (c listColumn) role(mode listMode) listColumnRole {
	if mode == listModeRemote {
		return c.remote
	}
	return c.local
}

// listColumnRegistry is the canonical column order shared by local and remote
// listings (SPEC.md §9). Selected columns always print in this order,
// regardless of the order they were requested in.
var listColumnRegistry = []listColumn{
	{
		name: "date", header: "DATE",
		local: listColumnDefault, remote: listColumnDefault,
	},
	{
		name: "profile", header: "PROFILE",
		local: listColumnAuto, remote: listColumnUnavailable,
	},
	{
		name: "state", header: "STATE",
		local: listColumnAuto, remote: listColumnUnavailable,
	},
	{
		name: "protected", header: "PROTECTED",
		local: listColumnAuto, remote: listColumnAuto,
	},
	{
		name: "kind", header: "KIND",
		local: listColumnDefault, remote: listColumnDefault,
	},
	{
		name: "title", header: "TITLE",
		local: listColumnDefault, remote: listColumnUnavailable,
	},
	{
		name: "slug", header: "SLUG",
		local: listColumnWide, remote: listColumnDefault,
	},
	{
		name: "objects", header: "OBJECTS",
		local: listColumnDefault, remote: listColumnDefault,
	},
	{
		name: "size", header: "SIZE",
		local: listColumnDefault, remote: listColumnDefault,
	},
	{
		name: "page-size", header: "PAGE SIZE",
		local: listColumnWide, remote: listColumnUnavailable,
	},
	{
		name: "dir", header: "DIRECTORY",
		local: listColumnAuto, remote: listColumnAuto,
	},
	{
		name: "format", header: "FORMAT",
		local: listColumnWide, remote: listColumnUnavailable,
	},
	{
		name: "repo", header: "REPO",
		local: listColumnWide, remote: listColumnUnavailable,
	},
	{
		name: "bucket", header: "BUCKET",
		local: listColumnWide, remote: listColumnUnavailable,
	},
	{
		name: "url", header: "URL",
		local: listColumnDefault, remote: listColumnDefault,
	},
}

func lookupListColumn(name string) (listColumn, bool) {
	for _, column := range listColumnRegistry {
		if column.name == name {
			return column, true
		}
	}
	return listColumn{}, false
}

// listColumnNames returns the columns valid in mode, in canonical order.
func listColumnNames(mode listMode) []string {
	names := make([]string, 0, len(listColumnRegistry))
	for _, column := range listColumnRegistry {
		if column.role(mode) != listColumnUnavailable {
			names = append(names, column.name)
		}
	}
	return names
}

// listColumnRequest is the user's table column selection.
type listColumnRequest struct {
	// spec is the raw --columns value, meaningful only when changed is true.
	spec string
	// changed reports whether --columns was given.
	changed bool
	// wide requests every column valid for the listing mode.
	wide bool
}

// validate rejects unusable selections before any manifest read or bucket
// listing happens, so a mistyped column costs no request: bad flag
// combinations, unknown or wrong-mode names, and a selection that strips every
// column the mode can print. A selection that empties only because an
// automatic column did not fire is caught once the rows are known, since that
// depends on them.
//
// Column selection is a table concern, so it cannot be combined with --json.
func (r listColumnRequest) validate(mode listMode, jsonOutput bool) error {
	if jsonOutput {
		if r.wide {
			return errors.New("--wide cannot be combined with --json")
		}
		if r.changed {
			return errors.New("--columns cannot be combined with --json")
		}
	}
	if r.wide && r.changed {
		return errors.New("--wide cannot be combined with --columns")
	}
	if !r.changed {
		return nil
	}
	adjustments, additive, err := parseListColumnSpec(mode, r.spec)
	if err != nil {
		return err
	}
	if additive && removesEveryListColumn(mode, adjustments) {
		return errors.New("--columns left no columns to print")
	}
	return nil
}

// removesEveryListColumn reports whether adjustments strip every column the
// mode could select on its own, automatic ones included. That is the only
// empty selection knowable before the result set is.
func removesEveryListColumn(
	mode listMode, adjustments []listColumnAdjustment,
) bool {
	removed := make(map[string]bool, len(adjustments))
	for _, adjustment := range adjustments {
		if !adjustment.remove {
			return false
		}
		removed[adjustment.name] = true
	}
	for _, column := range listColumnRegistry {
		switch column.role(mode) {
		case listColumnDefault, listColumnAuto:
			if !removed[column.name] {
				return false
			}
		}
	}
	return true
}

// selectListColumns resolves the columns to print. auto holds the names of the
// auto columns whose rules fired for the result set being printed.
func selectListColumns(
	mode listMode, req listColumnRequest, auto map[string]bool,
) ([]listColumn, error) {
	if req.wide {
		return filterListColumnsByName(mode, func(listColumn) bool {
			return true
		}), nil
	}
	if !req.changed {
		return defaultListColumns(mode, auto), nil
	}
	return requestedListColumns(mode, req.spec, auto)
}

// defaultListColumns returns the mode's default columns plus every auto column
// whose rule fired.
func defaultListColumns(mode listMode, auto map[string]bool) []listColumn {
	return filterListColumnsByName(mode, func(c listColumn) bool {
		switch c.role(mode) {
		case listColumnDefault:
			return true
		case listColumnAuto:
			return auto[c.name]
		default:
			return false
		}
	})
}

// filterListColumnsByName returns the mode's available columns that keep
// accepts, in canonical order.
func filterListColumnsByName(
	mode listMode, keep func(listColumn) bool,
) []listColumn {
	columns := make([]listColumn, 0, len(listColumnRegistry))
	for _, column := range listColumnRegistry {
		if column.role(mode) == listColumnUnavailable {
			continue
		}
		if keep(column) {
			columns = append(columns, column)
		}
	}
	return columns
}

// listColumnAdjustment is one parsed --columns entry.
type listColumnAdjustment struct {
	name string
	// remove marks the "-name" form; every other entry selects its column.
	remove bool
}

// requestedListColumns resolves an explicit --columns value. The absolute form
// (date,title,url) replaces the default set; the additive form (+dir,-title)
// adjusts it. The two forms do not mix.
func requestedListColumns(
	mode listMode, spec string, auto map[string]bool,
) ([]listColumn, error) {
	adjustments, additive, err := parseListColumnSpec(mode, spec)
	if err != nil {
		return nil, err
	}

	selected := map[string]bool{}
	if additive {
		for _, column := range defaultListColumns(mode, auto) {
			selected[column.name] = true
		}
	}
	for _, adjustment := range adjustments {
		selected[adjustment.name] = !adjustment.remove
	}

	columns := filterListColumnsByName(mode, func(c listColumn) bool {
		return selected[c.name]
	})
	if len(columns) == 0 {
		return nil, errors.New("--columns left no columns to print")
	}
	return columns, nil
}

func parseListColumnSpec(
	mode listMode, spec string,
) ([]listColumnAdjustment, bool, error) {
	absolute := false
	additive := false
	adjustments := make([]listColumnAdjustment, 0, len(listColumnRegistry))
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		adjustment := listColumnAdjustment{}
		switch {
		case strings.HasPrefix(entry, "+"):
			additive = true
			entry = strings.TrimSpace(strings.TrimPrefix(entry, "+"))
		case strings.HasPrefix(entry, "-"):
			adjustment.remove = true
			additive = true
			entry = strings.TrimSpace(strings.TrimPrefix(entry, "-"))
		default:
			absolute = true
		}
		if entry == "" {
			return nil, false, errors.New(
				"--columns requires at least one column name",
			)
		}
		if err := checkListColumnName(mode, entry); err != nil {
			return nil, false, err
		}
		adjustment.name = entry
		adjustments = append(adjustments, adjustment)
	}
	if absolute && additive {
		return nil, false, errors.New(
			"--columns cannot mix an absolute column set with " +
				"+/- adjustments",
		)
	}
	return adjustments, additive, nil
}

// checkListColumnName rejects unknown names and names belonging to the other
// listing mode, so neither becomes a silently blank column.
func checkListColumnName(mode listMode, name string) error {
	column, ok := lookupListColumn(name)
	if !ok {
		return fmt.Errorf(
			"unknown list column %q; valid columns: %s",
			name, strings.Join(listColumnNames(mode), ", "),
		)
	}
	if column.role(mode) != listColumnUnavailable {
		return nil
	}
	// Every registered column has a local value, so an unavailable one is a
	// local-only column asked for in a remote listing.
	return fmt.Errorf(
		"list column %q is not available with --remote", name,
	)
}

// autoLocalListColumns reports which auto columns apply to manifest records
// (SPEC.md §9): profile when the set spans more than one profile, state when
// it holds legacy history, protected when any row is protected, and dir when
// a row has no URL to identify it by.
func autoLocalListColumns(records []airplan.ManifestRecord) map[string]bool {
	auto := map[string]bool{}
	profiles := map[string]bool{}
	for _, record := range records {
		profiles[record.Profile] = true
		if !airplan.IsSupportedMarkerVersion(record.MarkerVersion) {
			auto["state"] = true
		}
		if record.Protected {
			auto["protected"] = true
		}
		if record.URL == "" {
			auto["dir"] = true
		}
	}
	if len(profiles) > 1 {
		auto["profile"] = true
	}
	return auto
}

// autoRemoteListColumns reports which auto columns apply to a remote listing
// (SPEC.md §9). Protected applies when the LIST snapshot contains a sentinel;
// dir applies when a conflict or collection has no inferable URL.
func autoRemoteListColumns(uploads []airplan.RemoteUpload) map[string]bool {
	auto := map[string]bool{}
	for _, upload := range uploads {
		if upload.Protected {
			auto["protected"] = true
		}
		if upload.URL == "" {
			auto["dir"] = true
		}
	}
	return auto
}

// listDateLayout is the table rendering of a record or marker timestamp.
const listDateLayout = "2006-01-02 15:04"

// localListCell renders one manifest record column (SPEC.md §9).
func localListCell(name string, record airplan.ManifestRecord) string {
	switch name {
	case "date":
		return record.Time.UTC().Format(listDateLayout)
	case "profile":
		if record.Profile == "" {
			return "<root>"
		}
		return record.Profile
	case "state":
		if airplan.IsSupportedMarkerVersion(record.MarkerVersion) {
			return "managed"
		}
		return "legacy"
	case "protected":
		if record.Protected {
			return "yes"
		}
		return "no"
	case "kind":
		return listCellOrDash(string(airplan.ManifestRecordKind(record)))
	case "title":
		return listCellOrDash(record.Title)
	case "objects":
		if record.Objects == 0 {
			return "-"
		}
		return strconv.Itoa(record.Objects)
	case "size":
		// SIZE reports the marker-declared inventory. Records written before
		// airplan recorded it show a dash rather than a page-only number that
		// would mean something different (SPEC.md §9).
		if record.TotalBytes == 0 {
			return "-"
		}
		return formatListBytes(record.TotalBytes)
	case "page-size":
		return formatListBytes(record.Bytes)
	case "slug":
		return listCellOrDash(record.Slug)
	case "dir":
		return listCellOrDash(airplan.ManifestRecordDir(record))
	case "format":
		return listCellOrDash(record.Format)
	case "repo":
		return listCellOrDash(record.Repo)
	case "bucket":
		return listCellOrDash(record.Bucket)
	case "url":
		return listCellOrDash(record.URL)
	}
	return "-"
}

// remoteListCell renders one remote listing column (SPEC.md §9).
func remoteListCell(name string, upload airplan.RemoteUpload) string {
	switch name {
	case "date":
		return upload.LastModified.UTC().Format(listDateLayout)
	case "protected":
		if upload.Protected {
			return "yes"
		}
		return "no"
	case "kind":
		if upload.Conflict {
			return "conflict"
		}
		return listCellOrDash(string(upload.Kind))
	case "objects":
		return strconv.Itoa(upload.Objects)
	case "size":
		return formatListBytes(upload.Bytes)
	case "slug":
		return listCellOrDash(upload.Slug)
	case "dir":
		return listCellOrDash(upload.Dir)
	case "url":
		return listCellOrDash(upload.URL)
	}
	return "-"
}

func listCellOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
