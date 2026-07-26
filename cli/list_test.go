package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestListTableShowsActiveUploads(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"active/plan.html",` +
			`"url":"https://plans.example.com/active/plan.html",` +
			`"bucket":"plans","profile":"work",` +
			`"title":"Active plan","bytes":18432,` +
			`"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T15:04:12Z",` +
			`"key":"deleted/plan.html",` +
			`"url":"https://plans.example.com/deleted/plan.html",` +
			`"bucket":"plans","profile":"work",` +
			`"title":"Deleted plan","bytes":42,` +
			`"marker_version":1}`,
		`{"type":"delete","time":"2026-07-09T09:12:44Z",` +
			`"key":"deleted/plan.html"}`,
		`{"type":"upload","time":"2026-07-08T16:05:13Z",` +
			`"key":"untitled/plan.html",` +
			`"url":"https://plans.example.com/untitled/plan.html",` +
			`"bucket":"plans","profile":"work","bytes":7,` +
			`"marker_version":1}`,
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	table := parseListTable(t, stdout)
	table.assertHeader(t, "DATE", "KIND", "TITLE", "SIZE", "URL")
	table.assertColumn(t, "DATE", "2026-07-08 14:03", "2026-07-08 16:05")
	table.assertColumn(t, "TITLE", "Active plan", "-")
	table.assertColumn(t, "SIZE", "18 KiB", "7 B")
	table.assertColumn(t, "URL",
		"https://plans.example.com/active/plan.html",
		"https://plans.example.com/untitled/plan.html",
	)
	for _, unwanted := range []string{
		"Deleted plan",
		"https://plans.example.com/deleted/plan.html",
	} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("stdout contains tombstoned upload %q:\n%s",
				unwanted, stdout)
		}
	}
}

func TestListShowsLegacyUploadsWithoutWarnings(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",`+
			`"key":"legacy/plan.html",`+
			`"url":"https://plans.example.com/legacy/plan.html",`+
			`"bucket":"plans","profile":"work",`+
			`"title":"Legacy plan","bytes":42}`+"\n")

	stdout, stderr, err := executeList(t)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	table := parseListTable(t, stdout)
	table.assertColumn(t, "STATE", "legacy")
	table.assertColumn(t, "TITLE", "Legacy plan")
}

// listRecord builds a one-line manifest upload record from field fragments.
func listRecord(fields ...string) string {
	return `{"type":"upload",` + strings.Join(fields, ",") + `}`
}

// writeTwoProfileManifest writes two fully populated managed records in
// different profiles, in chronological file order.
func writeTwoProfileManifest(t *testing.T, path string) {
	t.Helper()

	writeManifest(t, path, strings.Join([]string{
		listRecord(
			`"time":"2026-07-08T14:03:11Z"`,
			`"key":"`+deleteDirA+`/plan.html"`,
			`"marker_key":"`+deleteDirA+`/`+airplan.MarkerFilename+`"`,
			`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"kind":"document"`, `"slug":"plan"`, `"format":"md"`,
			`"title":"Work plan"`,
			`"repo":"https://github.com/acme/service"`,
			`"bytes":18432`, `"marker_version":3`,
		),
		listRecord(
			`"time":"2026-07-08T15:04:12Z"`,
			`"key":"`+deleteDirB+`/index.html"`,
			`"marker_key":"`+deleteDirB+`/`+
				airplan.CollectionMarkerFilename+`"`,
			`"url":"https://plans.example.com/`+deleteDirB+`/index.html"`,
			`"bucket":"plans"`, `"profile":"home"`,
			`"kind":"collection"`, `"title":"Home shots"`,
			`"bytes":9216`, `"marker_version":3`,
		),
	}, "\n")+"\n")
}

// writeAirplanBackendConfig serves body from a fake airplan backend's manifest
// endpoint and returns a config file selecting it.
func writeAirplanBackendConfig(t *testing.T, body string) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/uploads" {
				t.Errorf("path = %q, want /api/v1/uploads", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		},
	))
	t.Cleanup(server.Close)

	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(
		"backend = \"airplan\"\napi_url = %q\n"+
			"api_token = \"01234567890123456789012345678901\"\n"+
			"timeout = \"5s\"\n",
		server.URL,
	)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestListTableAutoProfileColumn(t *testing.T) {
	t.Run("fires for multiple profiles", func(t *testing.T) {
		path := setListState(t)
		writeTwoProfileManifest(t, path)

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		table.assertHeader(t, "DATE", "PROFILE", "KIND", "TITLE", "SIZE", "URL")
		table.assertColumn(t, "PROFILE", "work", "home")
	})

	t.Run("stays hidden for one profile", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path, listRecord(
			`"time":"2026-07-08T14:03:11Z"`,
			`"key":"`+deleteDirA+`/plan.html"`,
			`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"kind":"document"`, `"title":"Work plan"`,
			`"bytes":10`, `"marker_version":3`,
		)+"\n")

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		table.assertHeader(t, "DATE", "KIND", "TITLE", "SIZE", "URL")
		if strings.Contains(stdout, "work") {
			t.Fatalf("stdout names the only profile:\n%s", stdout)
		}
	})

	t.Run("shows root history as <root>", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path, strings.Join([]string{
			listRecord(
				`"time":"2026-07-08T14:03:11Z"`,
				`"key":"`+deleteDirA+`/plan.html"`,
				`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
				`"bucket":"plans"`, `"profile":"work"`,
				`"title":"Work plan"`, `"bytes":10`, `"marker_version":3`,
			),
			listRecord(
				`"time":"2026-07-08T15:03:11Z"`,
				`"key":"`+deleteDirB+`/plan.html"`,
				`"url":"https://plans.example.com/`+deleteDirB+`/plan.html"`,
				`"bucket":"plans"`, `"title":"Root plan"`,
				`"bytes":10`, `"marker_version":3`,
			),
		}, "\n")+"\n")

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		table.assertColumn(t, "PROFILE", "work", "<root>")
	})
}

func TestListTableAutoStateColumn(t *testing.T) {
	t.Run("fires for legacy history", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path, strings.Join([]string{
			listRecord(
				`"time":"2026-07-08T14:03:11Z"`,
				`"key":"`+deleteDirA+`/plan.html"`,
				`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
				`"bucket":"plans"`, `"profile":"work"`,
				`"title":"Managed plan"`, `"bytes":10`, `"marker_version":3`,
			),
			listRecord(
				`"time":"2026-07-08T15:03:11Z"`,
				`"key":"`+deleteDirB+`/plan.html"`,
				`"url":"https://plans.example.com/`+deleteDirB+`/plan.html"`,
				`"bucket":"plans"`, `"profile":"work"`,
				`"title":"Legacy plan"`, `"bytes":10`,
			),
		}, "\n")+"\n")

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		table.assertHeader(t, "DATE", "STATE", "KIND", "TITLE", "SIZE", "URL")
		table.assertColumn(t, "STATE", "managed", "legacy")
	})

	t.Run("stays hidden when every row is managed", func(t *testing.T) {
		path := setListState(t)
		writeTwoProfileManifest(t, path)

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		if table.index("STATE") >= 0 {
			t.Fatalf("header = %q, want no STATE column", table.header)
		}
	})
}

func TestListTableAutoDirColumn(t *testing.T) {
	// A locally written record always carries a URL, so the rule fires for
	// served history: an airplan backend may report a record without one.
	t.Run("fires when a served row has no URL", func(t *testing.T) {
		isolateEnv(t)
		body := `{"records":[` + strings.Join([]string{
			listRecord(
				`"time":"2026-07-08T14:03:11Z"`,
				`"key":"`+deleteDirA+`/plan.html"`,
				`"marker_key":"`+deleteDirA+`/`+airplan.MarkerFilename+`"`,
				`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
				`"bucket":"plans"`, `"kind":"document"`,
				`"title":"Linked plan"`, `"bytes":10`, `"marker_version":3`,
			),
			listRecord(
				`"time":"2026-07-08T15:03:11Z"`,
				`"key":"`+deleteDirB+`/plan.html"`,
				`"marker_key":"`+deleteDirB+`/`+airplan.MarkerFilename+`"`,
				`"bucket":"plans"`, `"kind":"document"`,
				`"title":"Unlinked plan"`, `"bytes":10`, `"marker_version":3`,
			),
		}, ",") + `],"warnings":[]}`
		config := writeAirplanBackendConfig(t, body)

		// The isolated environment supplies S3 credentials the airplan
		// backend does not use, so stderr carries their inactive-field
		// warnings.
		stdout, stderr, err := executeList(t, "--config", config)
		if err != nil {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		if strings.Contains(stderr, "skipping") {
			t.Fatalf("stderr = %q, want no manifest warnings", stderr)
		}
		table := parseListTable(t, stdout)
		table.assertHeader(t,
			"DATE", "KIND", "TITLE", "SIZE", "DIRECTORY", "URL")
		table.assertColumn(t, "DIRECTORY", deleteDirA, deleteDirB)
		table.assertColumn(t, "URL",
			"https://plans.example.com/"+deleteDirA+"/plan.html", "-")
	})

	t.Run("stays hidden when every row has a URL", func(t *testing.T) {
		path := setListState(t)
		writeTwoProfileManifest(t, path)

		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		table := parseListTable(t, stdout)
		if table.index("DIRECTORY") >= 0 {
			t.Fatalf("header = %q, want no DIRECTORY column", table.header)
		}
	})
}

func TestListTableWideShowsEveryLocalColumn(t *testing.T) {
	path := setListState(t)
	writeTwoProfileManifest(t, path)

	stdout, stderr, err := executeList(t, "--wide")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	table := parseListTable(t, stdout)
	table.assertHeader(t,
		"DATE", "PROFILE", "STATE", "KIND", "TITLE", "SIZE", "PAGE SIZE",
		"SLUG", "DIRECTORY", "FORMAT", "REPO", "BUCKET", "URL",
	)
	table.assertColumn(t, "KIND", "document", "collection")
	table.assertColumn(t, "SLUG", "plan", "-")
	table.assertColumn(t, "DIRECTORY", deleteDirA, deleteDirB)
	table.assertColumn(t, "FORMAT", "md", "-")
	table.assertColumn(t, "REPO", "https://github.com/acme/service", "-")
	table.assertColumn(t, "BUCKET", "plans", "plans")
	table.assertColumn(t, "PAGE SIZE", "18 KiB", "9 KiB")
	table.assertColumn(t, "STATE", "managed", "managed")
}

func TestListTableColumnSelection(t *testing.T) {
	path := setListState(t)
	writeTwoProfileManifest(t, path)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			"absolute set",
			[]string{"--columns", "date,title,url"},
			[]string{"DATE", "TITLE", "URL"},
		},
		{
			"absolute set uses canonical order",
			[]string{"--columns", "url,title,date"},
			[]string{"DATE", "TITLE", "URL"},
		},
		{
			"absolute set excludes auto columns",
			[]string{"--columns", "date,title"},
			[]string{"DATE", "TITLE"},
		},
		{
			"absolute set collapses duplicates",
			[]string{"--columns", "date,date,url"},
			[]string{"DATE", "URL"},
		},
		{
			"additive adjustments",
			[]string{"--columns", "+dir,-title"},
			[]string{"DATE", "PROFILE", "KIND", "SIZE", "DIRECTORY", "URL"},
		},
		{
			"additive removal of an auto column",
			[]string{"--columns", "-profile"},
			[]string{"DATE", "KIND", "TITLE", "SIZE", "URL"},
		},
		{
			"additive addition already present",
			[]string{"--columns", "+date"},
			[]string{"DATE", "PROFILE", "KIND", "TITLE", "SIZE", "URL"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeList(t, tt.args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			parseListTable(t, stdout).assertHeader(t, tt.want...)
		})
	}
}

func TestListColumnFlagErrors(t *testing.T) {
	path := setListState(t)
	writeTwoProfileManifest(t, path)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"unknown name",
			[]string{"--columns", "date,nope"},
			`unknown list column "nope"`,
		},
		{
			"unknown name lists valid columns",
			[]string{"--columns", "nope"},
			"valid columns: date, profile, state, kind, title, size, " +
				"page-size, slug, dir, format, repo, bucket, url",
		},
		{
			"remote-only name in local mode",
			[]string{"--columns", "objects"},
			`list column "objects" is only available with --remote`,
		},
		{
			"mixed absolute and additive syntax",
			[]string{"--columns", "date,+dir"},
			"--columns cannot mix an absolute column set with " +
				"+/- adjustments",
		},
		{
			"empty selection",
			[]string{"--columns", ""},
			"--columns requires at least one column name",
		},
		{
			"empty name in list",
			[]string{"--columns", "date,,url"},
			"--columns requires at least one column name",
		},
		{
			"every column removed",
			[]string{
				"--columns",
				"-date,-profile,-kind,-title,-size,-url",
			},
			"--columns left no columns to print",
		},
		{
			"columns with json",
			[]string{"--columns", "date", "--json"},
			"--columns cannot be combined with --json",
		},
		{
			"wide with json",
			[]string{"--wide", "--json"},
			"--wide cannot be combined with --json",
		},
		{
			"wide with columns",
			[]string{"--wide", "--columns", "date"},
			"--wide cannot be combined with --columns",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := executeList(t, tt.args...)
			if err == nil {
				t.Fatalf("error = nil, want %q\nstdout: %s", tt.want, stdout)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestListRemoteColumnFlagErrors(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	config := writeCLIConfig(t, fake.server.URL)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"local-only name in remote mode",
			[]string{"--columns", "title"},
			`list column "title" is not available with --remote`,
		},
		{
			"unknown name lists remote columns",
			[]string{"--columns", "nope"},
			"valid columns: date, kind, objects, size, slug, dir, url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--remote", "--config", config}, tt.args...)
			stdout, _, err := executeList(t, args...)
			if err == nil {
				t.Fatalf("error = nil, want %q\nstdout: %s", tt.want, stdout)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
			if fake.listCalls() != 0 {
				t.Fatalf("LIST calls = %d, want none before column validation",
					fake.listCalls())
			}
		})
	}
}

func TestListOrdersByRecordTime(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		listRecord(
			`"time":"2026-07-09T09:00:00Z"`,
			`"key":"`+deleteDirC+`/plan.html"`,
			`"url":"https://plans.example.com/`+deleteDirC+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"title":"Newest"`, `"bytes":10`, `"marker_version":3`,
		),
		listRecord(
			`"time":"2026-07-07T09:00:00Z"`,
			`"key":"`+deleteDirA+`/plan.html"`,
			`"url":"https://plans.example.com/`+deleteDirA+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"title":"Oldest"`, `"bytes":10`, `"marker_version":3`,
		),
		listRecord(
			`"time":"2026-07-08T09:00:00Z"`,
			`"key":"`+deleteDirB+`/plan.html"`,
			`"url":"https://plans.example.com/`+deleteDirB+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"title":"Middle"`, `"bytes":10`, `"marker_version":3`,
		),
	}, "\n")+"\n")

	t.Run("table", func(t *testing.T) {
		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		parseListTable(t, stdout).assertColumn(
			t, "TITLE", "Oldest", "Middle", "Newest",
		)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--json")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		var records []airplan.ManifestRecord
		if err := json.Unmarshal([]byte(stdout), &records); err != nil {
			t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
		}
		got := make([]string, 0, len(records))
		for _, record := range records {
			got = append(got, record.Title)
		}
		if !slices.Equal(got, []string{"Oldest", "Middle", "Newest"}) {
			t.Fatalf("titles = %q, want oldest first", got)
		}
	})

	t.Run("reverse", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--reverse")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		parseListTable(t, stdout).assertColumn(
			t, "TITLE", "Newest", "Middle", "Oldest",
		)
	})

	t.Run("reverse json", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--reverse", "--json")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v",
				stdout, stderr, err)
		}
		var records []airplan.ManifestRecord
		if err := json.Unmarshal([]byte(stdout), &records); err != nil {
			t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
		}
		if len(records) != 3 || records[0].Title != "Newest" ||
			records[2].Title != "Oldest" {
			t.Fatalf("records = %+v, want newest first", records)
		}
	})
}

// writeFilterManifest writes three managed uploads — two documents around a
// collection — days apart in chronological file order.
func writeFilterManifest(t *testing.T, path string) {
	t.Helper()

	writeManifest(t, path, strings.Join([]string{
		listRecord(
			`"time":"2026-07-01T12:00:00Z"`,
			`"key":"`+deleteDirA+`/plan-alpha.html"`,
			`"marker_key":"`+deleteDirA+`/`+airplan.MarkerFilename+`"`,
			`"url":"https://plans.example.com/`+deleteDirA+`/plan-alpha.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"kind":"document"`, `"slug":"plan-alpha"`, `"format":"md"`,
			`"title":"Alpha"`, `"bytes":10`, `"marker_version":3`,
		),
		listRecord(
			`"time":"2026-07-05T12:00:00Z"`,
			`"key":"`+deleteDirB+`/index.html"`,
			`"marker_key":"`+deleteDirB+`/`+
				airplan.CollectionMarkerFilename+`"`,
			`"url":"https://plans.example.com/`+deleteDirB+`/index.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"kind":"collection"`, `"title":"Beta"`,
			`"bytes":10`, `"marker_version":3`,
		),
		listRecord(
			`"time":"2026-07-09T12:00:00Z"`,
			`"key":"`+deleteDirC+`/plan-gamma.html"`,
			`"marker_key":"`+deleteDirC+`/`+airplan.MarkerFilename+`"`,
			`"url":"https://plans.example.com/`+deleteDirC+`/plan-gamma.html"`,
			`"bucket":"plans"`, `"profile":"work"`,
			`"kind":"document"`, `"slug":"plan-gamma"`, `"format":"md"`,
			`"title":"Gamma"`, `"bytes":10`, `"marker_version":3`,
		),
	}, "\n")+"\n")
}

// listJSONTitles decodes local list --json output into record titles.
func listJSONTitles(t *testing.T, stdout string) []string {
	t.Helper()

	var records []airplan.ManifestRecord
	if err := json.Unmarshal([]byte(stdout), &records); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
	}
	titles := make([]string, 0, len(records))
	for _, record := range records {
		titles = append(titles, record.Title)
	}
	return titles
}

func TestListFiltersSelectRecords(t *testing.T) {
	path := setListState(t)
	writeFilterManifest(t, path)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"unfiltered", nil, []string{"Alpha", "Beta", "Gamma"}},
		{
			"newer-than date",
			[]string{"--newer-than", "2026-07-05"},
			[]string{"Beta", "Gamma"},
		},
		{
			// An exact timestamp avoids depending on the host time zone.
			"newer-than keeps the threshold",
			[]string{"--newer-than", "2026-07-05T12:00:00Z"},
			[]string{"Beta", "Gamma"},
		},
		{
			"older-than excludes the threshold",
			[]string{"--older-than", "2026-07-05T12:00:00Z"},
			[]string{"Alpha"},
		},
		{
			"both bounds",
			[]string{
				"--newer-than", "2026-07-02", "--older-than", "2026-07-08",
			},
			[]string{"Beta"},
		},
		{
			"kind document",
			[]string{"--kind", "document"},
			[]string{"Alpha", "Gamma"},
		},
		{
			"kind collection",
			[]string{"--kind", "collection"},
			[]string{"Beta"},
		},
		{"slug exact", []string{"--slug", "plan-alpha"}, []string{"Alpha"}},
		{
			"slug glob",
			[]string{"--slug", "plan-*"},
			[]string{"Alpha", "Gamma"},
		},
		{
			"slug star excludes collections",
			[]string{"--slug", "*"},
			[]string{"Alpha", "Gamma"},
		},
		{
			"limit keeps most recent",
			[]string{"--limit", "2"},
			[]string{"Beta", "Gamma"},
		},
		{
			"limit beyond the set",
			[]string{"--limit", "9"},
			[]string{"Alpha", "Beta", "Gamma"},
		},
		{"limit zero", []string{"--limit", "0"}, nil},
		{
			"limit with reverse",
			[]string{"--limit", "2", "--reverse"},
			[]string{"Gamma", "Beta"},
		},
		{
			"limit applies after other filters",
			[]string{"--kind", "document", "--limit", "1"},
			[]string{"Gamma"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeList(t, tt.args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			if len(tt.want) == 0 {
				if stdout != "" {
					t.Fatalf("stdout = %q, want empty", stdout)
				}
			} else {
				parseListTable(t, stdout).assertColumn(t, "TITLE", tt.want...)
			}

			// Filters are selection, not presentation: --json must select
			// exactly the same records.
			jsonArgs := append([]string{"--json"}, tt.args...)
			stdout, stderr, err = executeList(t, jsonArgs...)
			if err != nil || stderr != "" {
				t.Fatalf("json stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			got := listJSONTitles(t, stdout)
			if len(got) != len(tt.want) {
				t.Fatalf("json titles = %q, want %q", got, tt.want)
			}
			for index := range tt.want {
				if got[index] != tt.want[index] {
					t.Fatalf("json titles = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestListFilterAgeRelativeToNow(t *testing.T) {
	path := setListState(t)
	now := time.Now().UTC()
	record := func(dir, title string, age time.Duration) string {
		return listRecord(
			`"time":"`+now.Add(-age).Format(time.RFC3339)+`"`,
			`"key":"`+dir+`/plan.html"`,
			`"marker_key":"`+dir+`/`+airplan.MarkerFilename+`"`,
			`"url":"https://plans.example.com/`+dir+`/plan.html"`,
			`"bucket":"plans"`, `"profile":"work"`, `"kind":"document"`,
			`"title":"`+title+`"`, `"bytes":10`, `"marker_version":3`,
		)
	}
	writeManifest(t, path, strings.Join([]string{
		record(deleteDirA, "Ancient", 40*24*time.Hour),
		record(deleteDirB, "Older", 20*24*time.Hour),
		record(deleteDirC, "Recent", 2*24*time.Hour),
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t, "--newer-than", "7d")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	parseListTable(t, stdout).assertColumn(t, "TITLE", "Recent")

	stdout, stderr, err = executeList(t, "--older-than", "3w")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	parseListTable(t, stdout).assertColumn(t, "TITLE", "Ancient")

	// A zero age is harmless when only selecting rows to print, so the parser
	// stays permissive; refusing it is purge's invariant, not the parser's.
	stdout, stderr, err = executeList(t, "--older-than", "0s")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	parseListTable(t, stdout).assertColumn(
		t, "TITLE", "Ancient", "Older", "Recent",
	)
}

func TestListFilterFlagErrors(t *testing.T) {
	path := setListState(t)
	writeFilterManifest(t, path)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"ambiguous slash date",
			[]string{"--older-than", "03/04/2026"},
			`--older-than: invalid time "03/04/2026" (ambiguous date; ` +
				`write the year first, as 2026-03-04)`,
		},
		{
			"unparsable newer-than",
			[]string{"--newer-than", "yesterday"},
			`--newer-than: invalid time "yesterday"`,
		},
		{
			"unknown kind",
			[]string{"--kind", "page"},
			`--kind: invalid kind "page" (want document or collection)`,
		},
		{
			"malformed slug pattern",
			[]string{"--slug", "["},
			"--slug: invalid slug pattern",
		},
		{
			"negative limit",
			[]string{"--limit", "-1"},
			"--limit must not be negative",
		},
		{
			"all-profiles with profile",
			[]string{"--all-profiles", "--profile", "work"},
			"--all-profiles cannot be combined with --profile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := executeList(t, tt.args...)
			if err == nil {
				t.Fatalf("error = nil, want %q\nstdout: %s", tt.want, stdout)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
		})
	}
}

func TestListAllProfilesOverridesProfileSelection(t *testing.T) {
	path := setListState(t)
	writeTwoProfileManifest(t, path)
	t.Setenv("AIRPLAN_PROFILE", "work")
	config := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "airplan",
		"config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`
[profiles.work]
endpoint = "https://work.invalid"
bucket = "plans"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeList(t)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	parseListTable(t, stdout).assertColumn(t, "TITLE", "Work plan")

	stdout, stderr, err = executeList(t, "--all-profiles")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	table := parseListTable(t, stdout)
	table.assertColumn(t, "TITLE", "Work plan", "Home shots")
	table.assertColumn(t, "PROFILE", "work", "home")
}

func TestListJSONShowsActiveUploads(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"active/plan.html",` +
			`"source_key":"active/plan.md",` +
			`"url":"https://plans.example.com/active/plan.html",` +
			`"bucket":"plans","profile":"work",` +
			`"title":"Active plan","bytes":18432,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T15:04:12Z",` +
			`"key":"deleted/plan.html",` +
			`"url":"https://plans.example.com/deleted/plan.html",` +
			`"bucket":"plans","title":"Deleted plan","bytes":42,` +
			`"marker_version":1}`,
		`{"type":"delete","time":"2026-07-09T09:12:44Z",` +
			`"key":"deleted/plan.html"}`,
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t, "-j")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want one JSON line", stdout)
	}

	var fields []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &fields); err != nil {
		t.Fatalf("json.Unmarshal fields: %v\nstdout: %s", err, stdout)
	}
	if len(fields) != 1 {
		t.Fatalf("got %d records, want 1\nstdout: %s", len(fields), stdout)
	}
	for _, field := range []string{
		"type", "time", "key", "url", "bucket", "title", "bytes",
	} {
		if _, ok := fields[0][field]; !ok {
			t.Fatalf("field %q missing from stdout: %s", field, stdout)
		}
	}
	if _, ok := fields[0]["source_key"]; !ok {
		t.Fatalf("source_key missing from stdout: %s", stdout)
	}

	var records []airplan.ManifestRecord
	if err := json.Unmarshal([]byte(stdout), &records); err != nil {
		t.Fatalf("json.Unmarshal records: %v\nstdout: %s", err, stdout)
	}
	if got := records[0].Key; got != "active/plan.html" {
		t.Fatalf("key = %q, want active/plan.html", got)
	}
	if strings.Contains(stdout, "Deleted plan") ||
		strings.Contains(stdout, "deleted/plan.html") {
		t.Fatalf("stdout contains tombstoned upload: %s", stdout)
	}
}

func TestListWarnsForTornLine(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"active/plan.html",` +
			`"url":"https://plans.example.com/active/plan.html",` +
			`"bucket":"plans","title":"Active plan","bytes":18432,` +
			`"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T15:04:12Z",`,
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr,
		"airplan: warning: skipping malformed manifest line 2") {
		t.Fatalf("stderr = %q, want torn-line warning", stderr)
	}
	if !strings.Contains(stdout, "Active plan") ||
		!strings.Contains(stdout,
			"https://plans.example.com/active/plan.html") {
		t.Fatalf("stdout missing active upload:\n%s", stdout)
	}
	if strings.Contains(stdout, "warning") {
		t.Fatalf("stdout contains warning text:\n%s", stdout)
	}
}

func TestListEmptyManifest(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		setListState(t)

		stdout, stderr, err := executeList(t)
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("json", func(t *testing.T) {
		setListState(t)

		stdout, stderr, err := executeList(t, "--json")
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}
		if stdout != "[]\n" {
			t.Fatalf("stdout = %q, want []", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestListRemoteTableAndJSON(t *testing.T) {
	when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
	key := deleteDirA + "/plan.html"
	markerKey := deleteDirA + "/" + airplan.MarkerFilename
	objects := []remoteFakeObject{
		{key: markerKey, size: 100, lastModified: when},
		{key: key, size: 18432, lastModified: when.Add(time.Minute)},
		{key: deleteDirA + "/plan.md", size: 20, lastModified: when},
	}

	t.Run("table", func(t *testing.T) {
		isolateEnv(t)
		fake := newFakeRemoteS3(t, objects,
			map[string]string{key: "must not be fetched"}, nil)

		stdout, stderr, err := executeList(t,
			"--remote", "--config", writeCLIConfig(t, fake.server.URL))
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		table := parseListTable(t, stdout)
		table.assertHeader(t,
			"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL")
		table.assertColumn(t, "DATE", "2026-07-08 14:03")
		table.assertColumn(t, "KIND", "document")
		table.assertColumn(t, "OBJECTS", "3")
		table.assertColumn(t, "SIZE", "18.1 KiB")
		table.assertColumn(t, "SLUG", "plan")
		table.assertColumn(t, "URL", "https://plans.example.com/"+key)
		if fake.headCalls() != 0 {
			t.Fatalf("HEAD calls = %d, want none", fake.headCalls())
		}
	})

	t.Run("json", func(t *testing.T) {
		isolateEnv(t)
		fake := newFakeRemoteS3(t, objects, nil, nil)

		stdout, stderr, err := executeList(t,
			"--remote", "--json",
			"--config", writeCLIConfig(t, fake.server.URL))
		if err != nil {
			t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if strings.Count(stdout, "\n") != 1 {
			t.Fatalf("stdout = %q, want one JSON line", stdout)
		}

		var records []struct {
			Time      time.Time `json:"time"`
			Dir       string    `json:"dir"`
			MarkerKey string    `json:"marker_key"`
			Objects   int       `json:"objects"`
			Bytes     int64     `json:"bytes"`
			Slug      string    `json:"slug"`
			Key       string    `json:"key"`
			URL       string    `json:"url"`
		}
		if err := json.Unmarshal([]byte(stdout), &records); err != nil {
			t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
		}
		if len(records) != 1 {
			t.Fatalf("records = %+v, want one record", records)
		}
		rec := records[0]
		if rec.Dir != deleteDirA || rec.MarkerKey != markerKey ||
			rec.Objects != 3 || rec.Bytes != 18552 || rec.Slug != "plan" ||
			rec.Key != key || rec.URL != "https://plans.example.com/"+key ||
			!rec.Time.Equal(when) {
			t.Fatalf("record = %+v", rec)
		}
	})
}

// remoteListFixture describes two marker directories: a document upload and a
// later collection upload, both with inferable URLs.
func remoteListFixture() []remoteFakeObject {
	when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
	later := time.Date(2026, 7, 8, 16, 3, 0, 0, time.UTC)
	return []remoteFakeObject{
		{
			key:  deleteDirA + "/" + airplan.MarkerFilename,
			size: 100, lastModified: when,
		},
		{key: deleteDirA + "/plan.html", size: 18432, lastModified: when},
		{
			key:  deleteDirC + "/" + airplan.CollectionMarkerFilename,
			size: 10, lastModified: later,
		},
		{key: deleteDirC + "/index.html", size: 30, lastModified: later},
	}
}

func TestListRemoteTableColumns(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
		date []string
	}{
		{
			"default",
			nil,
			[]string{"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL"},
			[]string{"2026-07-08 14:03", "2026-07-08 16:03"},
		},
		{
			"wide",
			[]string{"--wide"},
			[]string{
				"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "DIRECTORY", "URL",
			},
			[]string{"2026-07-08 14:03", "2026-07-08 16:03"},
		},
		{
			"absolute columns",
			[]string{"--columns", "objects,date"},
			[]string{"DATE", "OBJECTS"},
			[]string{"2026-07-08 14:03", "2026-07-08 16:03"},
		},
		{
			"additive columns",
			[]string{"--columns", "+dir,-slug"},
			[]string{"DATE", "KIND", "OBJECTS", "SIZE", "DIRECTORY", "URL"},
			[]string{"2026-07-08 14:03", "2026-07-08 16:03"},
		},
		{
			"reverse",
			[]string{"--reverse"},
			[]string{"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL"},
			[]string{"2026-07-08 16:03", "2026-07-08 14:03"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			fake := newFakeRemoteS3(t, remoteListFixture(), nil, nil)
			args := append([]string{
				"--remote", "--config", writeCLIConfig(t, fake.server.URL),
			}, tt.args...)

			stdout, stderr, err := executeList(t, args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			table := parseListTable(t, stdout)
			table.assertHeader(t, tt.want...)
			table.assertColumn(t, "DATE", tt.date...)
		})
	}
}

func TestListRemoteAutoDirColumnForUninferableURL(t *testing.T) {
	isolateEnv(t)
	when := time.Date(2026, 7, 8, 15, 3, 0, 0, time.UTC)
	objects := append(remoteListFixture(),
		remoteFakeObject{
			key:  deleteDirB + "/" + airplan.MarkerFilename,
			size: 10, lastModified: when,
		},
		remoteFakeObject{
			key:  deleteDirB + "/" + airplan.CollectionMarkerFilename,
			size: 10, lastModified: when,
		},
		remoteFakeObject{
			key: deleteDirB + "/index.html", size: 20, lastModified: when,
		},
	)
	fake := newFakeRemoteS3(t, objects, nil, nil)

	stdout, stderr, err := executeList(t,
		"--remote", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	table := parseListTable(t, stdout)
	table.assertHeader(t,
		"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "DIRECTORY", "URL")
	table.assertColumn(t, "DIRECTORY", deleteDirA, deleteDirB, deleteDirC)
	table.assertColumn(t, "KIND", "document", "conflict", "collection")
	table.assertColumn(t, "SLUG", "plan", "-", "-")
	table.assertColumn(t, "URL",
		"https://plans.example.com/"+deleteDirA+"/plan.html",
		"-",
		"https://plans.example.com/"+deleteDirC+"/index.html",
	)
}

func TestListRemoteFilters(t *testing.T) {
	document := "https://plans.example.com/" + deleteDirA + "/plan.html"
	collection := "https://plans.example.com/" + deleteDirC + "/index.html"

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"unfiltered", nil, []string{document, collection}},
		{
			"newer-than keeps the threshold",
			[]string{"--newer-than", "2026-07-08T16:03:00Z"},
			[]string{collection},
		},
		{
			"older-than excludes the threshold",
			[]string{"--older-than", "2026-07-08T16:03:00Z"},
			[]string{document},
		},
		{
			"kind document",
			[]string{"--kind", "document"},
			[]string{document},
		},
		{
			"kind collection",
			[]string{"--kind", "collection"},
			[]string{collection},
		},
		{
			"slug matches documents only",
			[]string{"--slug", "plan"},
			[]string{document},
		},
		{
			"slug star excludes collections",
			[]string{"--slug", "*"},
			[]string{document},
		},
		{
			"limit keeps most recent",
			[]string{"--limit", "1"},
			[]string{collection},
		},
		{"limit zero", []string{"--limit", "0"}, nil},
		{
			"limit with reverse",
			[]string{"--limit", "2", "--reverse"},
			[]string{collection, document},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			fake := newFakeRemoteS3(t, remoteListFixture(), nil, nil)
			config := writeCLIConfig(t, fake.server.URL)
			args := append([]string{"--remote", "--config", config}, tt.args...)

			stdout, stderr, err := executeList(t, args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			if len(tt.want) == 0 {
				if stdout != "" {
					t.Fatalf("stdout = %q, want empty", stdout)
				}
			} else {
				parseListTable(t, stdout).assertColumn(t, "URL", tt.want...)
			}

			jsonArgs := append(args, "--json")
			stdout, stderr, err = executeList(t, jsonArgs...)
			if err != nil || stderr != "" {
				t.Fatalf("json stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			var records []struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal([]byte(stdout), &records); err != nil {
				t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
			}
			got := make([]string, 0, len(records))
			for _, record := range records {
				got = append(got, record.URL)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("json urls = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListRemoteRejectsAllProfiles(t *testing.T) {
	isolateEnv(t)
	fake := newFakeRemoteS3(t, remoteListFixture(), nil, nil)

	stdout, _, err := executeList(t, "--remote", "--all-profiles",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err == nil {
		t.Fatalf("error = nil, want a mode error\nstdout: %s", stdout)
	}
	if !strings.Contains(err.Error(),
		"--all-profiles cannot be combined with --remote") {
		t.Fatalf("error = %q", err)
	}
	if fake.listCalls() != 0 {
		t.Fatalf("LIST calls = %d, want none", fake.listCalls())
	}
}

func TestListRemoteFallbackURLWarnsOnce(t *testing.T) {
	isolateEnv(t)
	when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
	objects := []remoteFakeObject{
		{
			key:  deleteDirA + "/" + airplan.MarkerFilename,
			size: 10, lastModified: when,
		},
		{key: deleteDirA + "/plan.html", size: 20, lastModified: when},
		{
			key:  deleteDirB + "/" + airplan.MarkerFilename,
			size: 10, lastModified: when,
		},
		{key: deleteDirB + "/other.html", size: 30, lastModified: when},
	}
	fake := newFakeRemoteS3(t, objects, nil, nil)
	config := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(
		"endpoint = %q\nbucket = \"plans\"\ntimeout = \"0\"\n",
		fake.server.URL,
	)
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeList(t, "-r", "--config", config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout,
		fake.server.URL+"/plans/"+deleteDirA+"/plan.html") {
		t.Fatalf("stdout = %q, want fallback URL", stdout)
	}
	if strings.Count(stderr, "public_base_url is not set") != 1 {
		t.Fatalf("stderr = %q, want one fallback warning", stderr)
	}
	if fake.listCalls() != 1 || fake.headCalls() != 0 {
		t.Fatalf("LIST calls = %d, HEAD calls = %d; want 1 and 0",
			fake.listCalls(), fake.headCalls())
	}
}

func TestListLocalExplicitProfileFilter(t *testing.T) {
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"work/plan.html","url":"https://example/work",` +
			`"bucket":"plans","profile":"work","title":"Work",` +
			`"bytes":10,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:04:11Z",` +
			`"key":"root/plan.html","url":"https://example/root",` +
			`"bucket":"plans","title":"Root","bytes":11,` +
			`"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:05:11Z",` +
			`"key":"home/plan.html","url":"https://example/home",` +
			`"bucket":"plans","profile":"home","title":"Home",` +
			`"bytes":12,"marker_version":1}`,
	}, "\n")+"\n")

	tests := []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{"no filter", nil, []string{"Work", "Root", "Home"}, nil},
		{
			"named table",
			[]string{"--profile", "work"},
			[]string{"Work"},
			[]string{"Root", "Home"},
		},
		{
			"root table",
			[]string{"--profile="},
			[]string{"Root"},
			[]string{"Work", "Home"},
		},
		{
			"named JSON",
			[]string{"--profile", "home", "--json"},
			[]string{`"title":"Home"`},
			[]string{`"title":"Work"`, `"title":"Root"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeList(t, tt.args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v",
					stdout, stderr, err)
			}
			for _, want := range tt.want {
				if !strings.Contains(stdout, want) {
					t.Fatalf("stdout missing %q: %s", want, stdout)
				}
			}
			for _, unwanted := range tt.not {
				if strings.Contains(stdout, unwanted) {
					t.Fatalf("stdout contains %q: %s", unwanted, stdout)
				}
			}
		})
	}
}

func TestListLocalNamedMissingConfig(t *testing.T) {
	setListState(t)
	_, _, err := executeList(t, "--config", "config.toml")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestListLocalFallsBackWhenDefaultConfigIsAmbiguous(t *testing.T) {
	isolateEnv(t)
	manifest := filepath.Join(
		os.Getenv("XDG_STATE_HOME"), "airplan", "manifest.jsonl",
	)
	writeManifest(t, manifest, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"work/plan.html","url":"https://example/work",` +
			`"bucket":"work","profile":"work","title":"Work",` +
			`"bytes":10,"format":"md","kind":"document",` +
			`"marker_version":3}`,
		`{"type":"upload","time":"2026-07-08T14:04:11Z",` +
			`"key":"home/plan.html","url":"https://example/home",` +
			`"bucket":"home","profile":"home","title":"Home",` +
			`"bytes":10,"format":"md","kind":"document",` +
			`"marker_version":3}`,
	}, "\n")+"\n")
	config := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "airplan", "config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`
[profiles.work]
endpoint = "https://work.invalid"
bucket = "work"

[profiles.home]
endpoint = "https://home.invalid"
bucket = "home"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeList(t)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	for _, title := range []string{"Work", "Home"} {
		if !strings.Contains(stdout, title) {
			t.Fatalf("stdout missing %q: %s", title, stdout)
		}
	}
}

func TestListLocalEnvironmentProfileFiltersHistory(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_PROFILE", "work")
	manifest := filepath.Join(
		os.Getenv("XDG_STATE_HOME"), "airplan", "manifest.jsonl",
	)
	writeManifest(t, manifest, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"work/plan.html","url":"https://example/work",` +
			`"bucket":"work","profile":"work","title":"Work",` +
			`"bytes":10,"format":"md","kind":"document",` +
			`"marker_version":3}`,
		`{"type":"upload","time":"2026-07-08T14:04:11Z",` +
			`"key":"home/plan.html","url":"https://example/home",` +
			`"bucket":"home","profile":"home","title":"Home",` +
			`"bytes":10,"format":"md","kind":"document",` +
			`"marker_version":3}`,
	}, "\n")+"\n")
	config := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "airplan", "config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`
[profiles.work]
endpoint = "https://work.invalid"
bucket = "work"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeList(t)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	if !strings.Contains(stdout, "Work") || strings.Contains(stdout, "Home") {
		t.Fatalf("stdout = %q, want only work history", stdout)
	}
}

func TestListLocalIgnoresMalformedInactiveS3Endpoint(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ENDPOINT", "not-a-url")
	manifest := filepath.Join(
		os.Getenv("XDG_STATE_HOME"), "airplan", "manifest.jsonl",
	)
	writeManifest(t, manifest,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",`+
			`"key":"plan.html","url":"https://example/plan",`+
			`"title":"Local","bucket":"plans","bytes":10,`+
			`"marker_version":1}`+"\n")
	stdout, stderr, err := executeList(t)
	if err != nil || stderr != "" || !strings.Contains(stdout, "Local") {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestFormatListBytes(t *testing.T) {
	tests := map[int64]string{
		0:                   "0 B",
		7:                   "7 B",
		1023:                "1023 B",
		1024:                "1 KiB",
		1536:                "1.5 KiB",
		18432:               "18 KiB",
		1048575:             "1 MiB",
		1048576:             "1 MiB",
		1073741823:          "1 GiB",
		1073741824:          "1 GiB",
		1099511627776:       "1 TiB",
		1125899906842624:    "1 PiB",
		1152921504606846976: "1 EiB",
	}

	for bytes, want := range tests {
		if got := formatListBytes(bytes); got != want {
			t.Errorf("formatListBytes(%d) = %q, want %q",
				bytes, got, want)
		}
	}
}

// listTable is a positional view of tabwriter list output. Header and row
// cells are sliced at the column offsets the header line establishes, so an
// assertion fails when a value moves to another column, when columns change
// order, or when rows change order.
type listTable struct {
	header []string
	rows   [][]string
}

// parseListTable splits list table output into header and row cells.
func parseListTable(t *testing.T, stdout string) *listTable {
	t.Helper()

	trimmed := strings.TrimRight(stdout, "\n")
	if trimmed == "" {
		t.Fatalf("stdout has no table:\n%q", stdout)
	}
	lines := strings.Split(trimmed, "\n")
	offsets := listTableOffsets([]rune(lines[0]))
	table := &listTable{header: listTableCells([]rune(lines[0]), offsets)}
	for _, line := range lines[1:] {
		table.rows = append(
			table.rows, listTableCells([]rune(line), offsets),
		)
	}
	return table
}

// listTableOffsets returns each column's start offset in a tabwriter line.
// tabwriter pads every cell but the last with at least its padding (2 spaces
// here), so content preceded by two spaces starts a new column. Header cells
// may contain a single space ("PAGE SIZE") without splitting.
func listTableOffsets(line []rune) []int {
	offsets := []int{}
	for i, r := range line {
		if r == ' ' {
			continue
		}
		if i == 0 {
			offsets = append(offsets, i)
			continue
		}
		if i >= 2 && line[i-1] == ' ' && line[i-2] == ' ' {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

func listTableCells(line []rune, offsets []int) []string {
	cells := make([]string, 0, len(offsets))
	for index, start := range offsets {
		end := len(line)
		if index+1 < len(offsets) && offsets[index+1] < end {
			end = offsets[index+1]
		}
		if start >= len(line) {
			cells = append(cells, "")
			continue
		}
		cells = append(cells, strings.TrimSpace(string(line[start:end])))
	}
	return cells
}

// assertHeader requires the exact ordered header sequence.
func (tbl *listTable) assertHeader(t *testing.T, want ...string) {
	t.Helper()

	if !slices.Equal(tbl.header, want) {
		t.Fatalf("header = %q, want %q", tbl.header, want)
	}
}

// index returns the position of a named column, or -1.
func (tbl *listTable) index(name string) int {
	return slices.Index(tbl.header, name)
}

// column returns every row's value for a named column, top to bottom.
func (tbl *listTable) column(t *testing.T, name string) []string {
	t.Helper()

	index := tbl.index(name)
	if index < 0 {
		t.Fatalf("column %q missing from header %q", name, tbl.header)
	}
	values := make([]string, 0, len(tbl.rows))
	for _, row := range tbl.rows {
		if index >= len(row) {
			t.Fatalf("row %q has no cell %d", row, index)
		}
		values = append(values, row[index])
	}
	return values
}

// assertColumn requires a named column's values in exact row order.
func (tbl *listTable) assertColumn(t *testing.T, name string, want ...string) {
	t.Helper()

	got := tbl.column(t, name)
	if !slices.Equal(got, want) {
		t.Fatalf("column %s = %q, want %q\nheader: %q",
			name, got, want, tbl.header)
	}
}

func executeList(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	cmd := newListCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// setListState points the default manifest at a temporary state directory and
// isolates the selectors local listing consults, so a developer's exported
// AIRPLAN_* variables or real config file cannot filter or redirect the
// history under test.
func setListState(t *testing.T) string {
	t.Helper()

	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, name := range []string{
		"AIRPLAN_CONFIG", "AIRPLAN_BACKEND", "AIRPLAN_API_URL",
		"AIRPLAN_API_TOKEN", "AIRPLAN_PROFILE", "AIRPLAN_MANIFEST",
	} {
		t.Setenv(name, "")
	}
	return filepath.Join(stateHome, "airplan", "manifest.jsonl")
}

func writeManifest(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
}

type remoteFakeObject struct {
	key          string
	size         int64
	lastModified time.Time
}

type fakeRemoteS3 struct {
	server        *httptest.Server
	mu            sync.Mutex
	objects       []remoteFakeObject
	titles        map[string]string
	failDelete    map[string]bool
	prefixes      []string
	posts         int
	heads         int
	markerDeletes int
	markers       map[string][]byte
	markerDelay   time.Duration
	markerActive  int
	markerMax     int
}

func newFakeRemoteS3(
	t *testing.T,
	objects []remoteFakeObject,
	titles map[string]string,
	failDelete map[string]bool,
) *fakeRemoteS3 {
	t.Helper()

	fake := &fakeRemoteS3{
		objects:    objects,
		titles:     titles,
		failDelete: failDelete,
		markers:    make(map[string][]byte),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "GET":
				if r.URL.Query().Get("list-type") == "2" {
					fake.handleList(w, r)
				} else {
					fake.handleMarker(w, r)
				}
			case "HEAD":
				fake.handleHead(w, r)
			case "POST":
				fake.handleDelete(w, r)
			case "DELETE":
				fake.mu.Lock()
				fake.markerDeletes++
				fake.mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusOK)
			}
		},
	))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeRemoteS3) handleMarker(w http.ResponseWriter, r *http.Request) {
	markerKey := strings.TrimPrefix(r.URL.Path, "/plans/")
	dirPrefix := strings.TrimSuffix(markerKey, airplan.MarkerFilename)
	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]

	f.mu.Lock()
	f.markerActive++
	if f.markerActive > f.markerMax {
		f.markerMax = f.markerActive
	}
	objects := append([]remoteFakeObject(nil), f.objects...)
	explicit := append([]byte(nil), f.markers[markerKey]...)
	delay := f.markerDelay
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.markerActive--
		f.mu.Unlock()
	}()
	if delay > 0 {
		time.Sleep(delay)
	}
	if explicit != nil {
		_, _ = w.Write(explicit)
		return
	}
	page := ""
	createdAt := time.Time{}
	for _, object := range objects {
		if object.key == markerKey {
			createdAt = object.lastModified
		}
		if strings.HasPrefix(object.key, dirPrefix) &&
			strings.HasSuffix(object.key, ".html") &&
			!strings.Contains(strings.TrimPrefix(object.key, dirPrefix), "/") {
			page = strings.TrimPrefix(object.key, dirPrefix)
		}
	}
	if page == "" || createdAt.IsZero() {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w,
			`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		return
	}
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: 1,
		Directory: dir, CreatedAt: createdAt.UTC(), Format: "html", Page: page,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (f *fakeRemoteS3) setMarkerDelay(delay time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markerDelay = delay
}

func (f *fakeRemoteS3) maxMarkerConcurrency() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.markerMax
}

func (f *fakeRemoteS3) setMarker(key string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markers[key] = append([]byte(nil), body...)
}

func (f *fakeRemoteS3) markerDeleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.markerDeletes
}

func (f *fakeRemoteS3) handleList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")

	f.mu.Lock()
	f.prefixes = append(f.prefixes, prefix)
	objects := append([]remoteFakeObject(nil), f.objects...)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<ListBucketResult><IsTruncated>false</IsTruncated>`)
	for _, obj := range objects {
		if !strings.HasPrefix(obj.key, prefix) {
			continue
		}
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>",
			obj.key, obj.size)
		fmt.Fprintf(w, "<LastModified>%s</LastModified>",
			obj.lastModified.Format(time.RFC3339))
		fmt.Fprintln(w, "</Contents>")
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}

func (f *fakeRemoteS3) handleHead(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/plans/")

	f.mu.Lock()
	f.heads++
	title, ok := f.titles[key]
	f.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("X-Amz-Meta-Title", title)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeRemoteS3) headCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.heads
}

func (f *fakeRemoteS3) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prefixes)
}

func (f *fakeRemoteS3) handleDelete(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	bodyText := string(body)

	f.mu.Lock()
	defer f.mu.Unlock()

	for dir := range f.failDelete {
		if strings.Contains(bodyText, dir) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	f.posts++
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0"?><DeleteResult></DeleteResult>`)
}

func (f *fakeRemoteS3) deleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}
