package airplan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultManifestPathHonorsXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	got, err := DefaultManifestPath()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(stateHome, "airplan", "manifest.jsonl")
	if got != want {
		t.Errorf("DefaultManifestPath() = %q, want %q", got, want)
	}
}

func TestReadManifestMissingFile(t *testing.T) {
	got, warnings, err := readManifest(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("records = %#v, want nil", got)
	}
	if warnings != nil {
		t.Fatalf("warnings = %#v, want nil", warnings)
	}
}

func TestAppendManifestRecordCreatesManifestAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "airplan", "manifest.jsonl")
	firstTime := time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC)
	secondTime := firstTime.Add(time.Hour)
	first := ManifestRecord{
		Type:      "upload",
		Time:      firstTime,
		Key:       "vq3n/plan.html",
		SourceKey: "vq3n/plan.md",
		URL:       "https://plans.example.com/vq3n/plan.html",
		Bucket:    "plans",
		Profile:   "work",
		Title:     "Refactor auth",
		Bytes:     18432,
	}
	second := ManifestRecord{
		Type: "delete",
		Time: secondTime,
		Key:  "vq3n/plan.html",
	}

	if err := appendManifestRecord(path, first); err != nil {
		t.Fatal(err)
	}
	if err := appendManifestRecord(path, second); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		assertFileMode(t, filepath.Dir(path), 0o700)
		assertFileMode(t, path, 0o600)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2: %q", len(lines), data)
	}

	wantLine := `{"type":"upload","time":"2026-07-08T14:03:11Z",` +
		`"key":"vq3n/plan.html","source_key":"vq3n/plan.md",` +
		`"url":"https://plans.example.com/vq3n/plan.html",` +
		`"bucket":"plans","profile":"work","title":"Refactor auth",` +
		`"bytes":18432}`
	if lines[0] != wantLine {
		t.Fatalf("first line = %s, want %s", lines[0], wantLine)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	wantRecords := []ManifestRecord{first, second}
	if diff := compareManifestRecords(records, wantRecords); diff != "" {
		t.Fatal(diff)
	}
}

func TestAppendManifestRecordConcurrentWritesStayIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	const count = 64

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := ManifestRecord{
				Type: "upload",
				Time: time.Date(2026, 7, 8, 14, 3, i%60, 0,
					time.UTC),
				Key: fmt.Sprintf("key-%02d/plan.html", i),
			}
			errs <- appendManifestRecord(path, rec)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != count {
		t.Fatalf("line count = %d, want %d", len(lines), count)
	}
	seen := make(map[string]bool, count)
	for i, line := range lines {
		var rec ManifestRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d did not unmarshal: %v: %q", i+1, err, line)
		}
		seen[rec.Key] = true
	}
	for i := range count {
		key := fmt.Sprintf("key-%02d/plan.html", i)
		if !seen[key] {
			t.Fatalf("missing key %q", key)
		}
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(records) != count {
		t.Fatalf("records = %d, want %d", len(records), count)
	}
}

func TestReadManifestSkipsMalformedAndUnknownType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	data := strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"upload.html","extra":"ignored"}`,
		`not json`,
		`{"type":"future","time":"2026-07-08T14:03:12Z",` +
			`"key":"future.html"}`,
		`{"type":"delete","time":"2026-07-08T14:03:13Z",` +
			`"key":"delete.html","future":true}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning", warnings)
	}
	if warnings[0] != "skipping malformed manifest line 2" {
		t.Fatalf("warning = %q", warnings[0])
	}
	want := []ManifestRecord{
		{
			Type: "upload",
			Time: time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC),
			Key:  "upload.html",
		},
		{
			Type: "delete",
			Time: time.Date(2026, 7, 8, 14, 3, 13, 0, time.UTC),
			Key:  "delete.html",
		},
	}
	if diff := compareManifestRecords(records, want); diff != "" {
		t.Fatal(diff)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func compareManifestRecords(got, want []ManifestRecord) string {
	if len(got) != len(want) {
		return fmt.Sprintf("record count = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Sprintf("record %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	return ""
}
