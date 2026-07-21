package cli

import (
	"encoding/json"
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
		"synced 1 uploads, tombstoned 0") {
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
