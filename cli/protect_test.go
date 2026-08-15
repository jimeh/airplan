package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestProtectAndUnprotectCommands(t *testing.T) {
	isolateEnv(t)
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	fake := newFakeProtectS3(t, deleteDirA, "plan.html")

	stdout, stderr, err := executeCommand(t, "", "",
		"protect", "--reason", "README demo link",
		"--config", writeCLIConfig(t, fake.server.URL),
		"https://plans.example.com/"+deleteDirA+"/plan.html",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr,
		"protected upload (key "+deleteDirA+"/plan.html)") {
		t.Fatalf("stderr = %q, want protect summary", stderr)
	}
	sentinelKey := deleteDirA + "/" + airplan.ProtectedFilename
	if body, ok := fake.object(sentinelKey); !ok ||
		!strings.Contains(string(body), "README demo link") {
		t.Fatalf("sentinel = %q, %v", body, ok)
	}

	stdout, stderr, err = executeCommand(t, "", "",
		"unprotect", "--config", writeCLIConfig(t, fake.server.URL),
		deleteDirA,
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr,
		"unprotected upload (key "+deleteDirA+"/plan.html)") {
		t.Fatalf("stderr = %q, want unprotect summary", stderr)
	}
	if _, ok := fake.object(sentinelKey); ok {
		t.Fatal("sentinel still present after unprotect")
	}

	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 || len(records) != 2 ||
		records[0].Type != "protect" || records[1].Type != "unprotect" ||
		records[0].ProtectReason != "README demo link" {
		t.Fatalf("manifest = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
}

func TestProtectRejectsOversizedReason(t *testing.T) {
	isolateEnv(t)
	fake := newFakeProtectS3(t, deleteDirA, "plan.html")

	_, _, err := executeCommand(t, "", "",
		"protect", "--reason", strings.Repeat("x", 257),
		"--config", writeCLIConfig(t, fake.server.URL),
		deleteDirA,
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds 256 characters") {
		t.Fatalf("error = %v, want reason bound", err)
	}
	if calls := fake.putCalls(); calls != 0 {
		t.Fatalf("put calls = %d, want 0", calls)
	}
}

func TestDeleteRefusesProtectedWithoutForce(t *testing.T) {
	isolateEnv(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {
			deleteDirA + "/plan.html",
			deleteDirA + "/" + airplan.ProtectedFilename,
		},
	}, nil)

	_, _, err := executeCommand(t, "", "",
		"delete", "--config", writeCLIConfig(t, fake.server.URL),
		deleteDirA+"/plan.html",
	)
	if err == nil || !strings.Contains(err.Error(), "upload is protected") ||
		!strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v, want protected refusal with --force hint", err)
	}
	if fake.deleteCalls() != 0 || fake.markerDeleteCalls() != 0 {
		t.Fatalf("deletes = %d/%d, want none",
			fake.deleteCalls(), fake.markerDeleteCalls())
	}

	stdout, stderr, err := executeCommand(t, "", "",
		"delete", "--force", "--config", writeCLIConfig(t, fake.server.URL),
		deleteDirA+"/plan.html",
	)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "deleted 3 objects") {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
	// The sentinel and the marker each use a dedicated DELETE request.
	if fake.deleteCalls() != 1 || fake.markerDeleteCalls() != 2 {
		t.Fatalf("deletes = %d/%d, want 1 payload batch and 2 deletes",
			fake.deleteCalls(), fake.markerDeleteCalls())
	}
}

func TestPurgeSkipsProtectedUploads(t *testing.T) {
	isolateEnv(t)
	now := time.Now().UTC()
	alpha := uploadRecord(deleteDirA, "alpha", "", now.Add(-time.Hour))
	beta := uploadRecord(deleteDirB, "beta", "", now.Add(-time.Hour))
	protect := airplan.ManifestRecord{
		Type: "protect", Time: now.Truncate(time.Second),
		Key:       alpha.Key,
		MarkerKey: deleteDirA + "/" + airplan.MarkerFilename,
		Bucket:    "plans", ProtectReason: "keep",
	}
	writeDefaultManifest(t, []airplan.ManifestRecord{alpha, beta, protect})
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirB + "/": {deleteDirB + "/beta.html"},
	}, nil)

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--yes", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr,
		"airplan: note: skipping protected upload "+alpha.URL) {
		t.Fatalf("stderr = %q, want protected skip note", stderr)
	}
	if !strings.Contains(stderr, "purged 1 uploads (1 protected, 0 failed)") {
		t.Fatalf("stderr = %q, want purge summary", stderr)
	}
	if fake.deleteCalls() != 1 {
		t.Fatalf("delete calls = %d, want 1", fake.deleteCalls())
	}
	records, _, err := airplan.ReadManifest("")
	if err != nil {
		t.Fatal(err)
	}
	active := airplan.ActiveUploads(records)
	if len(active) != 1 || active[0].Key != alpha.Key || !active[0].Protected {
		t.Fatalf("active = %+v, want protected alpha only", active)
	}
}

func TestPurgeDryRunExcludesProtectedCandidates(t *testing.T) {
	isolateEnv(t)
	now := time.Now().UTC()
	alpha := uploadRecord(deleteDirA, "alpha", "", now.Add(-time.Hour))
	beta := uploadRecord(deleteDirB, "beta", "", now.Add(-time.Hour))
	protect := airplan.ManifestRecord{
		Type: "protect", Time: now.Truncate(time.Second),
		Key:       alpha.Key,
		MarkerKey: deleteDirA + "/" + airplan.MarkerFilename,
		Bucket:    "plans",
	}
	writeDefaultManifest(t, []airplan.ManifestRecord{alpha, beta, protect})

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--dry-run")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "skipping protected upload "+alpha.URL) ||
		!strings.Contains(stderr, "beta.html") {
		t.Fatalf("stderr = %q, want skip note and beta candidate", stderr)
	}
	// The protected upload never appears in the candidate list.
	if strings.Contains(strings.ReplaceAll(stderr,
		"skipping protected upload "+alpha.URL, ""), "alpha.html") {
		t.Fatalf("stderr = %q, protected upload listed as candidate", stderr)
	}
}

func TestPurgeReportsDeleteTimeProtectedSkip(t *testing.T) {
	isolateEnv(t)
	now := time.Now().UTC()
	alpha := uploadRecord(deleteDirA, "alpha", "", now.Add(-time.Hour))
	writeDefaultManifest(t, []airplan.ManifestRecord{alpha})
	// The sentinel exists remotely but local history does not know yet.
	fake := newFakeDeleteS3(t, map[string][]string{
		deleteDirA + "/": {
			deleteDirA + "/alpha.html",
			deleteDirA + "/" + airplan.ProtectedFilename,
		},
	}, nil)

	stdout, stderr, err := executeCommand(t, "", "", "purge", "--all",
		"--yes", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatalf("protected skip must not fail purge: %v\nstderr:\n%s",
			err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "skipping protected upload "+alpha.URL) ||
		!strings.Contains(stderr, "purged 0 uploads (1 protected, 0 failed)") {
		t.Fatalf("stderr = %q, want skip note and summary", stderr)
	}
	if fake.deleteCalls() != 0 || fake.markerDeleteCalls() != 0 {
		t.Fatalf("deletes = %d/%d, want none",
			fake.deleteCalls(), fake.markerDeleteCalls())
	}
}

func TestListSurfacesProtection(t *testing.T) {
	isolateEnv(t)
	now := time.Now().UTC()
	alpha := uploadRecord(deleteDirA, "alpha", "", now.Add(-time.Hour))
	protect := airplan.ManifestRecord{
		Type: "protect", Time: now.Truncate(time.Second),
		Key:       alpha.Key,
		MarkerKey: deleteDirA + "/" + airplan.MarkerFilename,
		Bucket:    "plans", ProtectReason: "keep",
	}
	writeDefaultManifest(t, []airplan.ManifestRecord{alpha, protect})

	stdout, stderr, err := executeCommand(t, "", "", "list", "--json")
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr:\n%s", err, stderr)
	}
	var records []airplan.ManifestRecord
	if err := json.Unmarshal([]byte(stdout), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Protected ||
		records[0].ProtectReason != "keep" ||
		records[0].ProtectedAt.IsZero() {
		t.Fatalf("records = %+v, want protected projection", records)
	}

	// The human table carries the same reduced state as the JSON projection.
	stdout, _, err = executeCommand(t, "", "", "list")
	if err != nil {
		t.Fatal(err)
	}
	parseListTable(t, stdout).assertColumn(t, "PROTECTED", "yes")
}

func TestShowReportsProtection(t *testing.T) {
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	for _, jsonOutput := range []bool{false, true} {
		name := "human"
		if jsonOutput {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			isolateEnv(t)
			fake := newFakeProtectS3(t, deleteDirA, "plan.html")
			fake.addObject(
				deleteDirA+"/"+airplan.ProtectedFilename,
				[]byte(`{"schema":"airplan-protection","version":1,`+
					`"created_at":"2026-07-11T10:00:00Z",`+
					`"reason":"README demo link"}`),
				createdAt,
			)
			args := []string{
				"show", "--config", writeCLIConfig(t, fake.server.URL),
				deleteDirA,
			}
			if jsonOutput {
				args = append(args, "--json")
			}
			stdout, stderr, err := executeCommand(t, "", "", args...)
			if err != nil {
				t.Fatalf("Execute: %v\nstderr:\n%s", err, stderr)
			}
			if !jsonOutput {
				if !strings.Contains(stdout,
					"PROTECTED") || !strings.Contains(stdout,
					"yes (reason: README demo link)") {
					t.Fatalf("stdout = %q, want protection row", stdout)
				}
				return
			}
			var got struct {
				Protected     bool       `json:"protected"`
				ProtectedAt   *time.Time `json:"protected_at"`
				ProtectReason string     `json:"protect_reason"`
			}
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			if !got.Protected || got.ProtectedAt == nil ||
				got.ProtectReason != "README demo link" {
				t.Fatalf("show JSON = %+v", got)
			}
		})
	}
}

func TestShowHumanReportsUnprotected(t *testing.T) {
	isolateEnv(t)
	fake := newFakeProtectS3(t, deleteDirA, "plan.html")

	stdout, stderr, err := executeCommand(t, "", "",
		"show", "--config", writeCLIConfig(t, fake.server.URL), deleteDirA)
	if err != nil {
		t.Fatalf("Execute: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "PROTECTED") ||
		!strings.Contains(stdout, "no") {
		t.Fatalf("stdout = %q, want unprotected row", stdout)
	}
}

// fakeProtectS3 is a minimal in-memory S3 fake serving a single v1 document
// upload plus arbitrary extra objects, with PUT and DELETE support.
type fakeProtectS3 struct {
	server  *httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
	when    map[string]time.Time
	puts    int
}

func newFakeProtectS3(t *testing.T, dir, page string) *fakeProtectS3 {
	t.Helper()
	fake := &fakeProtectS3{
		objects: make(map[string][]byte),
		when:    make(map[string]time.Time),
	}
	body, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: 1,
		Directory: dir,
		CreatedAt: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		Format:    "html", Page: page, Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	fake.addObject(dir+"/"+airplan.MarkerFilename, body, when)
	fake.addObject(dir+"/"+page, []byte("0123456789"), when)
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeProtectS3) addObject(key string, body []byte, when time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), body...)
	f.when[key] = when
}

func (f *fakeProtectS3) object(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[key]
	return body, ok
}

func (f *fakeProtectS3) putCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts
}

func (f *fakeProtectS3) handle(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/plans/")
	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		prefix := r.URL.Query().Get("prefix")
		f.mu.Lock()
		type listed struct {
			key  string
			size int
			when time.Time
		}
		var items []listed
		for candidate, body := range f.objects {
			if strings.HasPrefix(candidate, prefix) {
				items = append(items, listed{
					candidate, len(body), f.when[candidate],
				})
			}
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
			`<IsTruncated>false</IsTruncated>`)
		for _, item := range items {
			fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>"+
				"<LastModified>%s</LastModified></Contents>",
				item.key, item.size, item.when.Format(time.RFC3339))
		}
		fmt.Fprintln(w, `</ListBucketResult>`)
	case r.Method == http.MethodGet:
		f.mu.Lock()
		body, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		_, _ = w.Write(body)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.puts++
		f.objects[key] = body
		f.when[key] = time.Now().UTC()
		f.mu.Unlock()
	case r.Method == http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, key)
		delete(f.when, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
