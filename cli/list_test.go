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
			`"title":"Active plan","bytes":18432,` +
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
			`"bucket":"plans","bytes":7,"marker_version":1}`,
	}, "\n")+"\n")

	stdout, stderr, err := executeList(t)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, want := range []string{
		"DATE", "PROFILE", "STATE", "TITLE", "SIZE", "URL",
		"work", "managed",
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
	for _, want := range []string{"work", "legacy", "Legacy plan"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
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
			"DATE", "OBJECTS", "SIZE", "SLUG", "DIRECTORY", "URL",
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

func TestListLocalRejectsConfig(t *testing.T) {
	setListState(t)
	_, _, err := executeList(t, "--config", "config.toml")
	if err == nil || err.Error() != "--config requires --remote" {
		t.Fatalf("error = %v", err)
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
