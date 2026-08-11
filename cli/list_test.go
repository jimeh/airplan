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
			`"title":"Active plan","bytes":18432,"objects":2,"total_bytes":18432,` +
			`"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T15:04:12Z",` +
			`"key":"deleted/plan.html",` +
			`"url":"https://plans.example.com/deleted/plan.html",` +
			`"bucket":"plans","title":"Deleted plan","bytes":42,` +
			`"marker_version":1}`,
		`{"type":"delete","time":"2026-07-09T09:12:44Z",` +
			`"key":"deleted/plan.html"}`,
		`{"type":"upload","time":"2026-07-08T16:05:13Z",` +
			`"key":"untitled/plan.html",` +
			`"url":"https://plans.example.com/untitled/plan.html",` +
			`"bucket":"plans","bytes":7,"objects":2,"total_bytes":7,"marker_version":1}`,
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, want := range []string{
		"DATE", "KIND", "PROFILE", "TITLE", "OBJECTS", "SIZE", "URL",
		"work", "document",
		"2026-07-08 14:03", "Active plan", "18 KiB",
		"https://plans.example.com/active/plan.html",
		"2026-07-08 16:05", "7 B",
		"https://plans.example.com/untitled/plan.html",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
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

func TestListTableDatesUseLocalTimeZone(t *testing.T) {
	previousLocation := time.Local
	time.Local = time.FixedZone("UTC-04", -4*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	when := time.Date(2026, 7, 8, 2, 30, 0, 0, time.UTC)
	localRows := localListRows([]airplan.ManifestRecord{{Time: when}})
	remoteRows := remoteListRows([]airplan.RemoteUpload{{LastModified: when}})
	for mode, got := range map[string]string{
		"local":  localRows[0].values["date"],
		"remote": remoteRows[0].values["date"],
	} {
		if want := "2026-07-07 22:30"; got != want {
			t.Errorf("%s DATE = %q, want %q", mode, got, want)
		}
	}
}

func TestListTableUsesDeclaredUploadTotals(t *testing.T) {
	isolateEnv(t)
	path := setListState(t)
	writeManifest(t, path,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",`+
			`"key":"aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",`+
			`"marker_key":"aaaaaaaaaaaaaaaaaaaaaaaaaa/.airplan.json",`+
			`"url":"https://plans.example.com/aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",`+
			`"bucket":"plans","title":"Active plan","bytes":3,`+
			`"objects":2,"total_bytes":2048,"marker_version":3}`+"\n")

	stdout, stderr, err := executeList(t, "--columns", "objects,size")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 || strings.Join(strings.Fields(lines[0]), " ") != "OBJECTS SIZE" ||
		strings.Join(strings.Fields(lines[1]), " ") != "2 2 KiB" {
		t.Fatalf("stdout = %q", stdout)
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
	for _, want := range []string{"STATE", "legacy", "Legacy plan"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestListLegacyTotalsAreAbsentButPageSizeRemains(t *testing.T) {
	isolateEnv(t)
	path := setListState(t)
	writeManifest(t, path,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",`+
			`"key":"legacy/plan.html",`+
			`"url":"https://plans.example.com/legacy/plan.html",`+
			`"bucket":"plans","bytes":42,"marker_version":1}`+"\n")
	stdout, stderr, err := executeList(t, "--columns", "objects,size,page-size")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 || strings.Fields(lines[0])[0] != "OBJECTS" ||
		strings.Join(strings.Fields(lines[1]), " ") != "- - 42 B" {
		t.Fatalf("stdout = %q", stdout)
	}
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

func TestListTableColumnSelection(t *testing.T) {
	isolateEnv(t)
	path := setListState(t)
	writeManifest(t, path, strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"abcdefghijklmnopqrstuvwxyz/plan.html",` +
			`"marker_key":"abcdefghijklmnopqrstuvwxyz/.airplan.json",` +
			`"url":"https://plans.example.com/abcdefghijklmnopqrstuvwxyz/plan.html",` +
			`"bucket":"plans","profile":"work","title":"Active plan",` +
			`"format":"md","kind":"document","slug":"plan",` +
			`"repo":"https://github.com/example/airplan",` +
			`"bytes":18432,"marker_version":3}`,
	}, "\n")+"\n")

	t.Run("local default", func(t *testing.T) {
		stdout, stderr, err := executeList(t)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		assertListHeaders(t, stdout, "DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "URL")
	})

	t.Run("local wide", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--wide")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		assertListHeaders(t, stdout,
			"DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "SLUG", "PROFILE", "STATE",
			"DIR", "PAGE-SIZE", "FORMAT", "REPO", "BUCKET", "URL")
	})

	t.Run("absolute", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--columns", "date,title,url")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		assertListHeaders(t, stdout, "DATE", "TITLE", "URL")
	})

	t.Run("additive", func(t *testing.T) {
		stdout, stderr, err := executeList(t, "--columns", "+dir,-title")
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		assertListHeaders(t, stdout, "DATE", "KIND", "OBJECTS", "SIZE", "DIR", "URL")
	})
}

func TestListTableAutomaticColumns(t *testing.T) {
	isolateEnv(t)
	t.Run("profile fires only for multiple result profiles", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path, strings.Join([]string{
			`{"type":"upload","time":"2026-07-08T14:03:11Z","key":"one.html",` +
				`"url":"https://example/one.html","bucket":"plans","profile":"work",` +
				`"title":"One","bytes":10,"marker_version":1}`,
			`{"type":"upload","time":"2026-07-08T14:04:11Z","key":"two.html",` +
				`"url":"https://example/two.html","bucket":"plans",` +
				`"title":"Two","bytes":11,"marker_version":1}`,
		}, "\n")+"\n")

		stdout, _, err := executeList(t)
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout,
			"DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "PROFILE", "URL")

		stdout, _, err = executeList(t, "--profile", "work")
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout, "DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "URL")
	})

	t.Run("state fires only for legacy results", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path,
			`{"type":"upload","time":"2026-07-08T14:03:11Z","key":"managed.html",`+
				`"url":"https://example/managed.html","bucket":"plans",`+
				`"title":"Managed","bytes":10,"marker_version":1}`+"\n")

		stdout, _, err := executeList(t)
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout, "DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "URL")

		writeManifest(t, path,
			`{"type":"upload","time":"2026-07-08T14:03:11Z","key":"legacy.html",`+
				`"url":"https://example/legacy.html","bucket":"plans",`+
				`"title":"Legacy","bytes":10}`+"\n")
		stdout, _, err = executeList(t)
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout,
			"DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "STATE", "URL")
	})

	t.Run("dir fires only for a row without URL", func(t *testing.T) {
		when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
		withURL := []remoteFakeObject{
			{key: deleteDirA + "/" + airplan.MarkerFilename, size: 10, lastModified: when},
			{key: deleteDirA + "/plan.html", size: 20, lastModified: when},
		}
		isolateEnv(t)
		fake := newFakeRemoteS3(t, withURL, nil, nil)
		stdout, _, err := executeList(t,
			"--remote", "--config", writeCLIConfig(t, fake.server.URL))
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout,
			"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL")

		withoutURL := []remoteFakeObject{
			{key: deleteDirB + "/" + airplan.MarkerFilename, size: 10, lastModified: when},
			{key: deleteDirB + "/plan.html", size: 20, lastModified: when},
			{key: deleteDirB + "/other.html", size: 30, lastModified: when},
		}
		fake = newFakeRemoteS3(t, withoutURL, nil, nil)
		stdout, _, err = executeList(t,
			"--remote", "--config", writeCLIConfig(t, fake.server.URL))
		if err != nil {
			t.Fatal(err)
		}
		assertListHeaders(t, stdout,
			"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "DIR", "URL")
	})
}

func TestListRemoteTableColumnSelection(t *testing.T) {
	isolateEnv(t)
	when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
	objects := []remoteFakeObject{
		{key: deleteDirA + "/" + airplan.MarkerFilename, size: 10, lastModified: when},
		{key: deleteDirA + "/plan.html", size: 20, lastModified: when},
	}
	fake := newFakeRemoteS3(t, objects, nil, nil)
	config := writeCLIConfig(t, fake.server.URL)

	tests := []struct {
		name    string
		args    []string
		headers []string
	}{
		{"default", nil, []string{"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL"}},
		{"wide", []string{"--wide"}, []string{"DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "DIR", "URL"}},
		{"absolute", []string{"--columns", "date,dir,url"}, []string{"DATE", "DIR", "URL"}},
		{"additive", []string{"--columns", "+dir,-slug"}, []string{"DATE", "KIND", "OBJECTS", "SIZE", "DIR", "URL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--remote", "--config", config}, tt.args...)
			stdout, stderr, err := executeList(t, args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
			}
			assertListHeaders(t, stdout, tt.headers...)
		})
	}
}

func TestListReversePresentationOrder(t *testing.T) {
	t.Run("local table and JSON", func(t *testing.T) {
		path := setListState(t)
		writeManifest(t, path, strings.Join([]string{
			`{"type":"upload","time":"2026-07-08T14:03:11Z","key":"old.html",` +
				`"url":"https://example/old.html","bucket":"plans",` +
				`"title":"Old","bytes":10,"marker_version":1}`,
			`{"type":"upload","time":"2026-07-08T15:03:11Z","key":"new.html",` +
				`"url":"https://example/new.html","bucket":"plans",` +
				`"title":"New","bytes":10,"marker_version":1}`,
		}, "\n")+"\n")

		for _, args := range [][]string{{"--reverse"}, {"--reverse", "--json"}} {
			stdout, stderr, err := executeList(t, args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
			}
			if strings.Index(stdout, "New") > strings.Index(stdout, "Old") {
				t.Fatalf("stdout is not newest first:\n%s", stdout)
			}
		}
	})

	t.Run("remote", func(t *testing.T) {
		isolateEnv(t)
		when := time.Date(2026, 7, 8, 14, 3, 0, 0, time.UTC)
		objects := []remoteFakeObject{
			{key: deleteDirA + "/" + airplan.MarkerFilename, size: 10, lastModified: when},
			{key: deleteDirA + "/old.html", size: 20, lastModified: when},
			{key: deleteDirB + "/" + airplan.MarkerFilename, size: 10, lastModified: when.Add(time.Hour)},
			{key: deleteDirB + "/new.html", size: 20, lastModified: when.Add(time.Hour)},
		}
		fake := newFakeRemoteS3(t, objects, nil, nil)
		stdout, stderr, err := executeList(t, "--remote", "--reverse",
			"--config", writeCLIConfig(t, fake.server.URL))
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		if strings.Index(stdout, "new") > strings.Index(stdout, "old") {
			t.Fatalf("stdout is not newest first:\n%s", stdout)
		}
	})
}

func TestListColumnValidationErrorsKeepStdoutPure(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"unknown", []string{"--columns", "date,nope"}, []string{"unknown column \"nope\"", "valid local columns"}},
		{"mixed syntax", []string{"--columns", "date,+dir"}, []string{"cannot mix absolute and additive"}},
		{"wide conflict", []string{"--wide", "--columns", "date,url"}, []string{"--wide cannot be used with --columns"}},
		{"json columns", []string{"--json", "--columns", "date,url"}, []string{"--columns cannot be used with --json"}},
		{"json wide", []string{"--json", "--wide"}, []string{"--wide cannot be used with --json"}},
		{"columns long only", []string{"-c", "date,url"}, []string{"unknown shorthand flag: 'c'"}},
		{"remote wrong mode", []string{"--remote", "--columns", "title"}, []string{"column \"title\" is not valid for remote list", "valid remote columns"}},
		{"all profiles with profile", []string{"--all-profiles", "--profile", "work"}, []string{"--all-profiles cannot be used with --profile"}},
		{"all profiles remote", []string{"--remote", "--all-profiles"}, []string{"--all-profiles is only valid for local list"}},
		{"lowercase all profiles remains unassigned", []string{"-a"}, []string{"unknown shorthand flag: 'a'"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setListState(t)
			stdout, _, err := executeList(t, tt.args...)
			if err == nil {
				t.Fatalf("error = nil, stdout = %q", stdout)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
			}
		})
	}
}

func TestListFiltersLocalTableJSONParity(t *testing.T) {
	isolateEnv(t)
	records := listFilterManifestFixture()
	writeDefaultManifest(t, records)
	args := []string{
		"--newer-than", "2026-07-08T14:00:00Z",
		"--older-than", "2026-07-08T16:00:00Z",
		"--kind", "document", "--slug", "alpha*",
	}

	table, stderr, err := executeList(t, args...)
	if err != nil || stderr != "" {
		t.Fatalf("table = %q, stderr = %q, error = %v", table, stderr, err)
	}
	assertListHeaders(t, table, "DATE", "KIND", "TITLE", "OBJECTS", "SIZE", "URL")
	for _, want := range []string{"alpha-lower title", "alpha-middle title"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}
	for _, unwanted := range []string{
		"collection title", "alpha-upper title", "beta-newer title",
	} {
		if strings.Contains(table, unwanted) {
			t.Fatalf("table contains %q:\n%s", unwanted, table)
		}
	}

	stdout, stderr, err := executeList(t, append(args, "--json")...)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var got []airplan.ManifestRecord
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{records[0].Key, records[1].Key}
	if len(got) != len(wantKeys) {
		t.Fatalf("records = %+v, want keys %v", got, wantKeys)
	}
	for i, key := range wantKeys {
		if got[i].Key != key {
			t.Fatalf("record %d key = %q, want %q", i, got[i].Key, key)
		}
	}
}

func TestListFiltersLocalLimitAndKindBoundaries(t *testing.T) {
	isolateEnv(t)
	records := listFilterManifestFixture()
	writeDefaultManifest(t, records)

	tests := []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{
			name: "limit larger than set",
			args: []string{"--kind", "document", "--limit", "10"},
			want: []string{"alpha-lower title", "alpha-middle title", "alpha-upper title", "beta-newer title"},
		},
		{
			name: "limit keeps most recent in ascending order",
			args: []string{"--kind", "document", "--limit", "2"},
			want: []string{"alpha-upper title", "beta-newer title"},
			not:  []string{"alpha-lower title", "alpha-middle title"},
		},
		{
			name: "limit then reverse",
			args: []string{"--kind", "document", "--limit", "2", "--reverse"},
			want: []string{"beta-newer title", "alpha-upper title"},
			not:  []string{"alpha-lower title", "alpha-middle title"},
		},
		{
			name: "collection kind",
			args: []string{"--kind", "collection"},
			want: []string{"collection title"},
			not:  []string{"alpha-lower title", "alpha-middle title", "alpha-upper title", "beta-newer title"},
		},
		{
			name: "slug star remains document only",
			args: []string{"--slug", "*"},
			want: []string{"alpha-lower title", "alpha-middle title", "alpha-upper title", "beta-newer title"},
			not:  []string{"collection title"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeList(t, tt.args...)
			if err != nil || stderr != "" {
				t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
			}
			previous := -1
			for _, want := range tt.want {
				index := strings.Index(stdout, want)
				if index < 0 || index < previous {
					t.Fatalf("stdout missing or misordered %q:\n%s", want, stdout)
				}
				previous = index
			}
			for _, unwanted := range tt.not {
				if strings.Contains(stdout, unwanted) {
					t.Fatalf("stdout contains %q:\n%s", unwanted, stdout)
				}
			}
		})
	}
	stdout, stderr, err := executeList(t,
		"--kind", "document", "--limit", "2", "--reverse", "--json")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var limited []airplan.ManifestRecord
	if err := json.Unmarshal([]byte(stdout), &limited); err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Slug != "beta-newer" ||
		limited[1].Slug != "alpha-upper" {
		t.Fatalf("limited JSON = %+v", limited)
	}

	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "0", "--json"}} {
		stdout, stderr, err := executeList(t, args...)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		if want := map[bool]string{false: "", true: "[]\n"}[contains(args, "--json")]; stdout != want {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestListFiltersRemoteTableJSONParity(t *testing.T) {
	isolateEnv(t)
	objects, dirs := listFilterRemoteFixture()
	fake := newFakeRemoteS3(t, objects, nil, nil)
	config := writeCLIConfig(t, fake.server.URL)
	args := []string{
		"--remote", "--config", config,
		"--newer-than", "2026-07-08T14:00:00Z",
		"--older-than", "2026-07-08T16:00:00Z",
		"--kind", "document", "--slug", "alpha*",
	}

	table, stderr, err := executeList(t, args...)
	if err != nil || stderr != "" {
		t.Fatalf("table = %q, stderr = %q, error = %v", table, stderr, err)
	}
	assertListHeaders(t, table, "DATE", "KIND", "OBJECTS", "SIZE", "SLUG", "URL")
	for _, want := range []string{"alpha-lower", "alpha-middle"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table missing %q:\n%s", want, table)
		}
	}
	for _, unwanted := range []string{
		"collection", "alpha-upper", "beta-newer", dirs[3],
	} {
		if strings.Contains(table, unwanted) {
			t.Fatalf("table contains %q:\n%s", unwanted, table)
		}
	}

	stdout, stderr, err := executeList(t, append(args, "--json")...)
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	var got []remoteListJSONRecord
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Dir != dirs[0] || got[1].Dir != dirs[1] {
		t.Fatalf("remote records = %+v, want dirs %q, %q", got, dirs[0], dirs[1])
	}
}

func TestListFiltersRemoteLimitKindAndSlugBoundaries(t *testing.T) {
	isolateEnv(t)
	objects, dirs := listFilterRemoteFixture()
	fake := newFakeRemoteS3(t, objects, nil, nil)
	config := writeCLIConfig(t, fake.server.URL)

	stdout, stderr, err := executeList(t, "--remote", "--config", config,
		"--kind", "document", "--limit", "2", "--reverse")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	if first, second := strings.Index(stdout, "beta-newer"),
		strings.Index(stdout, "alpha-upper"); first < 0 || second < first ||
		strings.Contains(stdout, "alpha-middle") {
		t.Fatalf("remote limit/reverse output:\n%s", stdout)
	}
	jsonOut, jsonStderr, jsonErr := executeList(t, "--remote", "--config", config,
		"--kind", "document", "--limit", "2", "--reverse", "--json")
	if jsonErr != nil || jsonStderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", jsonOut, jsonStderr, jsonErr)
	}
	var limited []remoteListJSONRecord
	if err := json.Unmarshal([]byte(jsonOut), &limited); err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Dir != dirs[5] || limited[1].Dir != dirs[4] {
		t.Fatalf("limited remote JSON = %+v", limited)
	}

	stdout, stderr, err = executeList(t, "--remote", "--config", config,
		"--kind", "collection")
	if err != nil || stderr != "" || !strings.Contains(stdout, dirs[2]) ||
		strings.Contains(stdout, dirs[3]) {
		t.Fatalf("collection stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}

	stdout, stderr, err = executeList(t, "--remote", "--config", config,
		"--slug", "*")
	if err != nil || stderr != "" || strings.Contains(stdout, dirs[2]) ||
		strings.Contains(stdout, dirs[3]) || !strings.Contains(stdout, "alpha-lower") ||
		!strings.Contains(stdout, "alpha-middle") ||
		!strings.Contains(stdout, "alpha-upper") ||
		!strings.Contains(stdout, "beta-newer") {
		t.Fatalf("slug stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}

	for _, args := range [][]string{
		{"--remote", "--config", config, "--limit", "0"},
		{"--remote", "--config", config, "--limit", "0", "--json"},
	} {
		stdout, stderr, err = executeList(t, args...)
		if err != nil || stderr != "" {
			t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
		}
		if want := map[bool]string{false: "", true: "[]\n"}[contains(args, "--json")]; stdout != want {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestListFilterErrorsKeepStdoutPureAndAvoidRemoteListing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"negative limit", []string{"--limit=-1"}, "--limit must not be negative"},
		{"invalid kind", []string{"--kind", "conflict"}, "--kind must be document or collection"},
		{"invalid slug", []string{"--slug", "["}, "--slug: invalid pattern"},
		{"invalid newer", []string{"--newer-than", "tomorrow"}, "--newer-than: invalid time filter"},
		{"invalid older", []string{"--older-than", "03/04/2026"}, "YYYY/MM/DD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setListState(t)
			stdout, _, err := executeList(t, tt.args...)
			if err == nil || stdout != "" || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("stdout = %q, error = %v, want %q", stdout, err, tt.want)
			}
		})
	}

	isolateEnv(t)
	fake := newFakeRemoteS3(t, nil, nil, nil)
	stdout, _, err := executeList(t, "--remote", "--slug", "[",
		"--config", writeCLIConfig(t, fake.server.URL))
	if err == nil || stdout != "" || fake.listCalls() != 0 {
		t.Fatalf("stdout = %q, error = %v, LIST calls = %d",
			stdout, err, fake.listCalls())
	}
}

func listFilterManifestFixture() []airplan.ManifestRecord {
	lower := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	records := []airplan.ManifestRecord{
		uploadRecord(deleteDirA, "alpha-lower", "work", lower),
		uploadRecord(deleteDirB, "alpha-middle", "work", lower.Add(time.Hour)),
		uploadRecord(deleteDirC, "collection", "home", lower.Add(90*time.Minute)),
		uploadRecord(strings.Repeat("d", 26), "alpha-upper", "home", lower.Add(2*time.Hour)),
		uploadRecord(strings.Repeat("e", 26), "beta-newer", "home", lower.Add(3*time.Hour)),
	}
	collection := &records[2]
	collection.Key = deleteDirC + "/index.html"
	collection.MarkerKey = deleteDirC + "/" + airplan.CollectionMarkerFilename
	collection.URL = "https://plans.example.com/" + collection.Key
	collection.Kind = string(airplan.UploadKindCollection)
	collection.Title = "collection title"
	records[3].MarkerVersion = 0
	return records
}

func listFilterRemoteFixture() ([]remoteFakeObject, []string) {
	lower := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	dirs := []string{
		deleteDirA, deleteDirB, deleteDirC,
		strings.Repeat("d", 26), strings.Repeat("e", 26),
		strings.Repeat("f", 26),
	}
	objects := []remoteFakeObject{
		{key: dirs[0] + "/" + airplan.MarkerFilename, size: 10, lastModified: lower},
		{key: dirs[0] + "/alpha-lower.html", size: 20, lastModified: lower},
		{key: dirs[1] + "/" + airplan.MarkerFilename, size: 10, lastModified: lower.Add(time.Hour)},
		{key: dirs[1] + "/alpha-middle.html", size: 20, lastModified: lower.Add(time.Hour)},
		{key: dirs[2] + "/" + airplan.CollectionMarkerFilename, size: 10, lastModified: lower.Add(90 * time.Minute)},
		{key: dirs[2] + "/index.html", size: 20, lastModified: lower.Add(90 * time.Minute)},
		{key: dirs[3] + "/" + airplan.MarkerFilename, size: 10, lastModified: lower.Add(105 * time.Minute)},
		{key: dirs[3] + "/" + airplan.CollectionMarkerFilename, size: 10, lastModified: lower.Add(105 * time.Minute)},
		{key: dirs[3] + "/conflict.html", size: 20, lastModified: lower.Add(105 * time.Minute)},
		{key: dirs[4] + "/" + airplan.MarkerFilename, size: 10, lastModified: lower.Add(2 * time.Hour)},
		{key: dirs[4] + "/alpha-upper.html", size: 20, lastModified: lower.Add(2 * time.Hour)},
		{key: dirs[5] + "/" + airplan.MarkerFilename, size: 10, lastModified: lower.Add(3 * time.Hour)},
		{key: dirs[5] + "/beta-newer.html", size: 20, lastModified: lower.Add(3 * time.Hour)},
	}
	return objects, dirs
}

func assertListHeaders(t *testing.T, stdout string, want ...string) {
	t.Helper()
	line, _, _ := strings.Cut(stdout, "\n")
	got := strings.Fields(line)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("headers = %q, want %q\nstdout:\n%s", got, want, stdout)
	}
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
		for _, want := range []string{
			"DATE", "OBJECTS", "SIZE", "SLUG", "URL",
			"2026-07-08 14:03", "3", "18.1 KiB", "plan", deleteDirA,
			"https://plans.example.com/" + key,
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout missing %q:\n%s", want, stdout)
			}
		}
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

func TestListAllProfilesOverridesEnvironmentProfile(t *testing.T) {
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

	stdout, stderr, err := executeList(t, "-A")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	for _, want := range []string{"PROFILE", "Work", "Home"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestListAllProfilesRejectsAirplanBackendBeforeRequest(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_ACCESS_KEY_ID", "")
	t.Setenv("AIRPLAN_SECRET_ACCESS_KEY", "")
	var (
		requestsMu sync.Mutex
		requests   int
	)
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, _ *http.Request,
	) {
		requestsMu.Lock()
		requests++
		requestsMu.Unlock()
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	config := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(config, []byte(fmt.Sprintf(
		"backend = \"airplan\"\napi_url = %q\n"+
			"api_token = \"01234567890123456789012345678901\"\n",
		server.URL,
	)), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeList(
		t, "--all-profiles", "--config", config,
	)
	if err == nil || !strings.Contains(err.Error(),
		"--all-profiles cannot be used with the airplan backend") {
		t.Fatalf("error = %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, want both empty", stdout, stderr)
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if requests != 0 {
		t.Fatalf("HTTP requests = %d, want 0", requests)
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

func setListState(t *testing.T) string {
	t.Helper()

	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
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
