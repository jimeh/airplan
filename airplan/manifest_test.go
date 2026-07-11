package airplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
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
		Type:          "upload",
		Time:          firstTime,
		Key:           "vq3n/plan.html",
		SourceKey:     "vq3n/plan.md",
		URL:           "https://plans.example.com/vq3n/plan.html",
		Bucket:        "plans",
		Profile:       "work",
		Title:         "Refactor auth",
		Bytes:         18432,
		MarkerVersion: MarkerVersion,
	}
	second := ManifestRecord{
		Type: "delete",
		Time: secondTime,
		Key:  "vq3n/plan.html",
	}

	if err := appendManifestRecord(context.Background(), path, first); err != nil {
		t.Fatal(err)
	}
	if err := appendManifestRecord(context.Background(), path, second); err != nil {
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
		`"bytes":18432,"marker_version":1}`
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
				Key:           fmt.Sprintf("key-%02d/plan.html", i),
				URL:           fmt.Sprintf("https://plans.example.com/key-%02d/plan.html", i),
				Bucket:        "plans",
				Bytes:         1,
				MarkerVersion: MarkerVersion,
			}
			errs <- appendManifestRecord(context.Background(), path, rec)
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

func TestAppendManifestRecordLockWaitHonorsContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := appendManifestRecord(ctx, path, ManifestRecord{
		Type: "upload", Key: "key/plan.html", MarkerVersion: MarkerVersion,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("lock wait took %s", elapsed)
	}
}

func TestReadManifestSkipsUnsupportedMarkerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	data := strings.Join([]string{
		`{"type":"upload","key":"missing.html"}`,
		`{"type":"upload","key":"future.html","marker_version":2}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"current.html","url":"https://plans.example.com/current.html",` +
			`"bucket":"plans","bytes":1,"marker_version":1}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Key != "current.html" ||
		len(warnings) != 2 {
		t.Fatalf("records = %+v, warnings = %v", records, warnings)
	}
}

func TestReadManifestSkipsMalformedAndUnknownType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	data := strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"upload.html","url":"https://plans.example.com/upload.html",` +
			`"bucket":"plans","bytes":1,"marker_version":1,"extra":"ignored"}`,
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
			Type:          "upload",
			Time:          time.Date(2026, 7, 8, 14, 3, 11, 0, time.UTC),
			Key:           "upload.html",
			URL:           "https://plans.example.com/upload.html",
			Bucket:        "plans",
			Bytes:         1,
			MarkerVersion: MarkerVersion,
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

func TestReadManifestSkipsOversizedLineAndContinues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	oversized := strings.Repeat("x", 10*1024*1024+1)
	data := `{"type":"upload","time":"2026-07-08T14:03:11Z",` +
		`"key":"before.html","url":"https://plans.example.com/before.html",` +
		`"bucket":"plans","bytes":1,"marker_version":1}` + "\n" +
		oversized + "\n" +
		`{"type":"delete","time":"2026-07-08T14:03:12Z",` +
		`"key":"after.html"}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Key != "before.html" ||
		records[1].Key != "after.html" {
		t.Fatalf("records after oversized line = %+v", records)
	}
	if len(warnings) != 1 || warnings[0] !=
		"skipping oversized manifest line 2" {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestReadManifestSkipsInvalidRecordsAndBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	data := strings.Join([]string{
		"",
		`{"type":"upload","marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"url":"https://plans.example.com/no-key.html",` +
			`"bucket":"plans","bytes":1,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"no-url.html","bucket":"plans","bytes":1,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"no-bucket.html","url":"https://plans.example.com/no-bucket.html",` +
			`"bytes":1,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"no-bytes.html","url":"https://plans.example.com/no-bytes.html",` +
			`"bucket":"plans","marker_version":1}`,
		`{"type":"delete","time":"2026-07-08T15:03:11+01:00",` +
			`"key":"offset.html"}`,
		`{"type":"delete","time":"2026-07-08T14:03:11Z"}`,
		"   ",
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"valid.html","url":"https://plans.example.com/valid.html",` +
			`"bucket":"plans","bytes":1,"marker_version":1}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Key != "valid.html" {
		t.Fatalf("records = %+v", records)
	}
	if len(warnings) != 7 {
		t.Fatalf("warnings = %v, want seven invalid-record warnings", warnings)
	}
	for _, warning := range warnings {
		if !strings.Contains(warning, "skipping invalid manifest line") {
			t.Fatalf("warning = %q", warning)
		}
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
