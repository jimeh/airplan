package cli

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/jimeh/airplan/airplan"
)

type listMode string

const (
	listModeLocal  listMode = "local"
	listModeRemote listMode = "remote"
)

type listColumn struct {
	name          string
	header        string
	local         bool
	remote        bool
	localDefault  bool
	remoteDefault bool
}

// listColumns is the single presentation registry for manifest and storage
// list tables. Registry order is the canonical order for defaults and --wide.
var listColumns = []listColumn{
	{name: "date", header: "DATE", local: true, remote: true, localDefault: true, remoteDefault: true},
	{name: "kind", header: "KIND", local: true, remote: true, localDefault: true, remoteDefault: true},
	{name: "title", header: "TITLE", local: true, localDefault: true},
	{name: "objects", header: "OBJECTS", local: true, remote: true, localDefault: true, remoteDefault: true},
	{name: "size", header: "SIZE", local: true, remote: true, localDefault: true, remoteDefault: true},
	{name: "slug", header: "SLUG", local: true, remote: true, remoteDefault: true},
	{name: "profile", header: "PROFILE", local: true},
	{name: "state", header: "STATE", local: true},
	{name: "dir", header: "DIR", local: true, remote: true},
	{name: "page-size", header: "PAGE-SIZE", local: true},
	{name: "format", header: "FORMAT", local: true},
	{name: "repo", header: "REPO", local: true},
	{name: "bucket", header: "BUCKET", local: true},
	{name: "url", header: "URL", local: true, remote: true, localDefault: true, remoteDefault: true},
}

type listTableRow struct {
	values  map[string]string
	profile string
	legacy  bool
	url     string
}

func validateListPresentationOptions(cmdChanged func(string) bool, opts *listOptions) error {
	if opts.allProfiles && opts.remote {
		return errors.New("--all-profiles is only valid for local list")
	}
	if opts.allProfiles && cmdChanged("profile") {
		return errors.New("--all-profiles cannot be used with --profile")
	}
	columnsChanged := cmdChanged("columns")
	if opts.wide && columnsChanged {
		return errors.New("--wide cannot be used with --columns")
	}
	if opts.json && columnsChanged {
		return errors.New("--columns cannot be used with --json")
	}
	if opts.json && opts.wide {
		return errors.New("--wide cannot be used with --json")
	}
	if columnsChanged {
		_, err := parseListColumnExpression(listModeForOptions(opts), opts.columns)
		return err
	}
	return nil
}

func listModeForOptions(opts *listOptions) listMode {
	if opts.remote {
		return listModeRemote
	}
	return listModeLocal
}

func resolveListColumns(
	mode listMode, expression string, expressionSet, wide bool,
	rows []listTableRow,
) ([]listColumn, error) {
	if wide {
		return validListColumns(mode), nil
	}
	defaults := defaultListColumns(mode, rows)
	if !expressionSet {
		return defaults, nil
	}

	tokens, err := parseListColumnExpression(mode, expression)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, errors.New("--columns requires at least one column")
	}
	if tokens[0][0] != '+' && tokens[0][0] != '-' {
		selected := make([]listColumn, 0, len(tokens))
		seen := make(map[string]bool, len(tokens))
		for _, token := range tokens {
			if seen[token] {
				return nil, fmt.Errorf("column %q is specified more than once", token)
			}
			seen[token] = true
			selected = append(selected, *findListColumn(token))
		}
		return selected, nil
	}

	selected := make(map[string]bool, len(defaults))
	for _, column := range defaults {
		selected[column.name] = true
	}
	for _, token := range tokens {
		selected[token[1:]] = token[0] == '+'
	}
	columns := make([]listColumn, 0, len(selected))
	for _, column := range listColumns {
		if column.valid(mode) && selected[column.name] {
			columns = append(columns, column)
		}
	}
	return columns, nil
}

func parseListColumnExpression(mode listMode, expression string) ([]string, error) {
	if expression == "" {
		return nil, errors.New("--columns requires at least one column")
	}
	raw := strings.Split(expression, ",")
	tokens := make([]string, 0, len(raw))
	additive := false
	absolute := false
	for _, item := range raw {
		token := strings.TrimSpace(item)
		if token == "" {
			return nil, errors.New("--columns contains an empty column name")
		}
		name := token
		if token[0] == '+' || token[0] == '-' {
			additive = true
			name = token[1:]
			if name == "" {
				return nil, fmt.Errorf("--columns modifier %q has no column name", token)
			}
		} else {
			absolute = true
		}
		if additive && absolute {
			return nil, errors.New("--columns cannot mix absolute and additive syntax")
		}
		column := findListColumn(name)
		if column == nil {
			return nil, fmt.Errorf("unknown column %q; valid %s columns: %s",
				name, mode, strings.Join(validListColumnNames(mode), ", "))
		}
		if !column.valid(mode) {
			return nil, fmt.Errorf("column %q is not valid for %s list; valid %s columns: %s",
				name, mode, mode, strings.Join(validListColumnNames(mode), ", "))
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func defaultListColumns(mode listMode, rows []listTableRow) []listColumn {
	profiles := make(map[string]struct{})
	includeState := false
	includeDir := false
	for _, row := range rows {
		profiles[row.profile] = struct{}{}
		includeState = includeState || row.legacy
		includeDir = includeDir || row.url == ""
	}
	columns := make([]listColumn, 0, len(listColumns))
	for _, column := range listColumns {
		include := column.defaultFor(mode)
		switch column.name {
		case "profile":
			include = mode == listModeLocal && len(profiles) > 1
		case "state":
			include = mode == listModeLocal && includeState
		case "dir":
			include = includeDir
		}
		if include {
			columns = append(columns, column)
		}
	}
	return columns
}

func validListColumns(mode listMode) []listColumn {
	columns := make([]listColumn, 0, len(listColumns))
	for _, column := range listColumns {
		if column.valid(mode) {
			columns = append(columns, column)
		}
	}
	return columns
}

func validListColumnNames(mode listMode) []string {
	columns := validListColumns(mode)
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.name)
	}
	return names
}

func findListColumn(name string) *listColumn {
	for i := range listColumns {
		if listColumns[i].name == name {
			return &listColumns[i]
		}
	}
	return nil
}

func (c listColumn) valid(mode listMode) bool {
	if mode == listModeRemote {
		return c.remote
	}
	return c.local
}

func (c listColumn) defaultFor(mode listMode) bool {
	if mode == listModeRemote {
		return c.remoteDefault
	}
	return c.localDefault
}

func localListRows(uploads []airplan.ManifestRecord) []listTableRow {
	rows := make([]listTableRow, 0, len(uploads))
	for _, upload := range uploads {
		profile := upload.Profile
		displayProfile := profile
		if displayProfile == "" {
			displayProfile = "<root>"
		}
		state := "legacy"
		if airplan.IsSupportedMarkerVersion(upload.MarkerVersion) {
			state = "managed"
		}
		kind := upload.Kind
		if kind == "" {
			kind = "-"
		}
		rows = append(rows, listTableRow{
			profile: profile,
			legacy:  state == "legacy",
			url:     upload.URL,
			values: map[string]string{
				"date":      upload.Time.UTC().Format("2006-01-02 15:04"),
				"kind":      kind,
				"title":     listValue(upload.Title),
				"objects":   formatOptionalListInt(upload.Objects),
				"size":      formatOptionalListBytes(upload.TotalBytes),
				"slug":      listValue(upload.Slug),
				"profile":   displayProfile,
				"state":     state,
				"dir":       manifestUploadDir(upload),
				"page-size": formatListBytes(upload.Bytes),
				"format":    listValue(upload.Format),
				"repo":      listValue(upload.Repo),
				"bucket":    listValue(upload.Bucket),
				"url":       listValue(upload.URL),
			},
		})
	}
	return rows
}

func remoteListRows(uploads []airplan.RemoteUpload) []listTableRow {
	rows := make([]listTableRow, 0, len(uploads))
	for _, upload := range uploads {
		kind := string(upload.Kind)
		if upload.Conflict {
			kind = "conflict"
		}
		rows = append(rows, listTableRow{
			url: upload.URL,
			values: map[string]string{
				"date":    upload.LastModified.UTC().Format("2006-01-02 15:04"),
				"kind":    listValue(kind),
				"objects": fmt.Sprintf("%d", upload.Objects),
				"size":    formatListBytes(upload.Bytes),
				"slug":    listValue(upload.Slug),
				"dir":     listValue(upload.Dir),
				"url":     listValue(upload.URL),
			},
		})
	}
	return rows
}

func manifestUploadDir(upload airplan.ManifestRecord) string {
	key := upload.MarkerKey
	if key == "" {
		key = upload.Key
	}
	dir := path.Dir(key)
	if dir == "." || dir == "/" {
		return "-"
	}
	return listValue(path.Base(dir))
}

func listValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatOptionalListInt(value int) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func formatOptionalListBytes(value int64) string {
	if value == 0 {
		return "-"
	}
	return formatListBytes(value)
}

func printListTable(w io.Writer, rows []listTableRow, columns []listColumn) error {
	if len(rows) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	headers := make([]string, 0, len(columns))
	for _, column := range columns {
		headers = append(headers, column.header)
	}
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	values := make([]string, len(columns))
	for _, row := range rows {
		for i, column := range columns {
			values[i] = row.values[column.name]
		}
		if _, err := fmt.Fprintln(tw, strings.Join(values, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
