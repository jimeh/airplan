package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

func TestSyncCommandImportsAndKeepsStdoutClean(t *testing.T) {
	isolateEnv(t)
	when := time.Now().UTC().Truncate(time.Second)
	fake := newFakeRemoteS3(t,
		remoteUploadObjects(deleteDirA, "plan", when), nil, nil)
	config := writeCLIConfig(t, fake.server.URL)
	stdout, stderr, err := executeCommand(t, "", "", "sync",
		"--config", config, "--concurrency", "1")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || !strings.Contains(stderr,
		"synced 1 uploads, enriched 0, tombstoned 0") {
		t.Fatalf("stdout = %q, stderr = %q", stdout, stderr)
	}
	records, warnings, err := airplan.ReadManifest("")
	if err != nil || len(warnings) != 0 || len(airplan.ActiveUploads(records)) != 1 {
		t.Fatalf("records = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
}

func TestSyncCommandJSONAndConcurrencyValidation(t *testing.T) {
	isolateEnv(t)
	stdout, _, err := executeCommand(t, "", "", "sync", "--concurrency", "0")
	if err == nil || stdout != "" || !strings.Contains(err.Error(),
		"concurrency must be between 1 and 64") {
		t.Fatalf("stdout = %q, error = %v", stdout, err)
	}

	when := time.Now().UTC().Truncate(time.Second)
	fake := newFakeRemoteS3(t,
		remoteUploadObjects(deleteDirA, "plan", when), nil, nil)
	stdout, _, err = executeCommand(t, "", "", "sync", "--json",
		"--dry-run", "--config", writeCLIConfig(t, fake.server.URL))
	if err != nil {
		t.Fatal(err)
	}
	var result syncJSONResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil ||
		result.Added != 1 || len(result.AddedRecords) != 1 {
		t.Fatalf("stdout = %q, result = %+v, error = %v", stdout, result, err)
	}
	manifest := filepath.Join(t.TempDir(), "missing.jsonl")
	if records, _, err := airplan.ReadManifest(manifest); err != nil || records != nil {
		t.Fatalf("dry run manifest = %+v, %v", records, err)
	}
}

func TestSyncJSONIncludesDeferred(t *testing.T) {
	encoded, err := json.Marshal(syncJSONFromResult(
		&airplan.SyncManifestResult{Deferred: 3},
	))
	if err != nil {
		t.Fatal(err)
	}
	var result syncJSONResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.Deferred != 3 || !strings.Contains(string(encoded), `"deferred":3`) {
		t.Fatalf("JSON = %s, result = %+v; want deferred 3", encoded, result)
	}
}

// TestSyncCommandReportsEnrichedSeparately covers backfill reporting: records
// completed with declared totals are counted apart from imports, in the human
// summary and in --json (SPEC.md §9).
func TestSyncCommandReportsEnrichedSeparately(t *testing.T) {
	isolateEnv(t)
	when := time.Now().UTC().Truncate(time.Second)
	page := []byte("plan page!")
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: airplan.MarkerVersion,
		Directory: deleteDirA, CreatedAt: when,
		Kind: airplan.UploadKindDocument, Slug: "plan", Format: "html",
		Producer: airplan.Producer{Name: "airplan", Version: "test"},
		Objects: []airplan.MarkerObject{{
			Name: "plan.html", Role: airplan.MarkerRolePage,
			Bytes: int64(len(page)), ContentType: "text/html; charset=utf-8",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	markerKey := deleteDirA + "/" + airplan.MarkerFilename
	fake := newFakeRemoteS3(t, []remoteFakeObject{
		{key: markerKey, size: int64(len(marker)), lastModified: when},
		{
			key:  deleteDirA + "/plan.html",
			size: int64(len(page)), lastModified: when,
		},
	}, nil, nil)
	fake.setMarker(markerKey, marker)
	config := writeCLIConfig(t, fake.server.URL)

	_, stderr, err := executeCommand(t, "", "", "sync", "--config", config)
	if err != nil {
		t.Fatalf("import: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "synced 1 uploads, enriched 0") ||
		!strings.Contains(stderr, "0 deferred") {
		t.Fatalf("stderr = %q, want enriched and deferred counts", stderr)
	}

	// Rewrite the imported record as pre-0.30 history, then let sync complete
	// it from the same marker.
	path, err := airplan.DefaultManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	records, _, err := airplan.ReadManifest(path)
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %+v, error = %v", records, err)
	}
	records[0].Objects = 0
	records[0].TotalBytes = 0
	encoded, err := json.Marshal(records[0])
	if err != nil {
		t.Fatal(err)
	}
	writeManifest(t, path, string(encoded)+"\n")

	stdout, stderr, err := executeCommand(t, "", "", "sync",
		"--config", config, "--json")
	if err != nil {
		t.Fatalf("backfill: %v\nstderr: %s", err, stderr)
	}
	var result syncJSONResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v\nstdout: %s", err, stdout)
	}
	if result.Added != 0 || result.Enriched != 1 ||
		len(result.EnrichedRecords) != 1 {
		t.Fatalf("result = %+v, want one enriched record", result)
	}
	if result.EnrichedRecords[0].Objects == 0 ||
		result.EnrichedRecords[0].TotalBytes == 0 {
		t.Fatalf("enriched record = %+v, want declared totals",
			result.EnrichedRecords[0])
	}
}

func TestSyncCommandFatalErrorKeepsStdoutEmpty(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	t.Cleanup(server.Close)

	stdout, _, err := executeCommand(t, "", "", "sync", "--json",
		"--config", writeCLIConfig(t, server.URL))
	if err == nil || stdout != "" {
		t.Fatalf("stdout = %q, error = %v", stdout, err)
	}
}

func TestPurgeConcurrencyRequiresRemote(t *testing.T) {
	isolateEnv(t)
	_, _, err := executeCommand(t, "", "", "purge", "--all",
		"--dry-run", "--concurrency", "2")
	if err == nil || !strings.Contains(err.Error(),
		"--concurrency requires --remote") {
		t.Fatalf("error = %v", err)
	}
	_, _, err = executeCommand(t, "", "", "purge", "--remote", "--all",
		"--dry-run", "--concurrency", "65")
	if err == nil || !strings.Contains(err.Error(),
		"concurrency must be between 1 and 64") {
		t.Fatalf("out-of-range error = %v", err)
	}
}
