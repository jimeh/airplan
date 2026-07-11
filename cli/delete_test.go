package cli

import (
	"bytes"
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

const (
	deleteDirA = "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	deleteDirB = "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	deleteDirC = "cccccccccccccccccccccccccc"
)

func TestDeleteCommand(t *testing.T) {
	isolateEnv(t)
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {
			deleteDirA + "/plan.html",
			deleteDirA + "/plan.md",
		},
	}, nil)

	stdout, stderr, err := executeCommand(t, "unused", "",
		"delete",
		"--config", writeCLIConfig(t, fake.server.URL),
		"https://plans.example.com/"+deleteDirA+"/plan.html",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "deleted 3 objects") ||
		!strings.Contains(stderr, "key "+deleteDirA+"/plan.html") {
		t.Fatalf("stderr = %q, want delete summary", stderr)
	}
	if fake.deleteCalls() != 1 {
		t.Fatalf("delete calls = %d, want 1", fake.deleteCalls())
	}
	if fake.markerDeleteCalls() != 1 {
		t.Fatalf("marker delete calls = %d, want 1", fake.markerDeleteCalls())
	}

	manifest := filepath.Join(stateHome, "airplan", "manifest.jsonl")
	records, warnings, err := airplan.ReadManifest(manifest)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("ReadManifest: %v %v", err, warnings)
	}
	if len(records) != 1 || records[0].Type != "delete" ||
		records[0].Key != deleteDirA+"/plan.html" {
		t.Fatalf("tombstone = %+v", records)
	}
}

type fakeDeleteS3 struct {
	server        *httptest.Server
	mu            sync.Mutex
	keys          map[string][]string
	fail          map[string]bool
	posts         int
	markerDeletes int
}

func newFakeDeleteS3(
	t *testing.T,
	keys map[string][]string,
	fail map[string]bool,
) *fakeDeleteS3 {
	t.Helper()

	fake := &fakeDeleteS3{
		keys: keys,
		fail: fail,
	}
	fake.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			switch r.Method {
			case "GET":
				if r.URL.Query().Get("list-type") == "2" {
					fake.handleList(w, r)
				} else {
					fake.handleMarker(w, r)
				}
			case "POST":
				fake.handleDelete(w, string(body))
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

func (f *fakeDeleteS3) handleList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")

	f.mu.Lock()
	keys := append([]string(nil), f.keys[prefix]...)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<ListBucketResult><IsTruncated>false</IsTruncated>`)
	if len(keys) > 0 {
		fmt.Fprintf(w, "<Contents><Key>%s%s</Key><Size>100</Size>",
			prefix, airplan.MarkerFilename)
		fmt.Fprint(w, "<LastModified>2026-07-01T00:00:00Z</LastModified>")
		fmt.Fprintln(w, "</Contents>")
	}
	for _, key := range keys {
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>10</Size>", key)
		fmt.Fprint(w, "<LastModified>2026-07-01T00:00:00Z</LastModified>")
		fmt.Fprintln(w, "</Contents>")
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}

func (f *fakeDeleteS3) handleMarker(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/plans/")
	dir := strings.TrimSuffix(key, "/"+airplan.MarkerFilename)
	prefix := dir + "/"

	f.mu.Lock()
	keys := append([]string(nil), f.keys[prefix]...)
	f.mu.Unlock()
	page := ""
	for _, candidate := range keys {
		if strings.HasSuffix(candidate, ".html") {
			page = strings.TrimPrefix(candidate, prefix)
			break
		}
	}
	if page == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: dir, CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Format: "html", Page: page,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (f *fakeDeleteS3) handleDelete(w http.ResponseWriter, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for dir := range f.fail {
		if strings.Contains(body, dir) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	f.posts++
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0"?><DeleteResult></DeleteResult>`)
}

func (f *fakeDeleteS3) deleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}

func (f *fakeDeleteS3) markerDeleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.markerDeletes
}

func writeCLIConfig(t *testing.T, endpoint string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	data := fmt.Sprintf(`
endpoint = %q
bucket = "plans"
public_base_url = "https://plans.example.com"
timeout = "0"
`, endpoint)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func executeCommand(
	t *testing.T,
	stdin string,
	extraEnvProfile string,
	args ...string,
) (string, string, error) {
	t.Helper()
	if extraEnvProfile != "" {
		t.Setenv("AIRPLAN_PROFILE", extraEnvProfile)
	}

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
