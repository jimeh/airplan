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

func TestReadManifestReturnsEmptyReadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, _, err := readManifest(path); err == nil ||
		!strings.Contains(err.Error(), "read manifest") {
		t.Fatalf("error = %v, want manifest read failure", err)
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
		Kind:          string(UploadKindDocument),
		Slug:          "plan",
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
		`"bucket":"plans","profile":"work","kind":"document","slug":"plan",` +
		`"title":"Refactor auth",` +
		`"bytes":18432,"marker_version":3}`
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

func TestReadManifestRecognizesLegacyAndSkipsUnsupportedMarkerVersion(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	data := strings.Join([]string{
		`{"type":"upload","time":"2026-07-08T13:03:11Z",` +
			`"key":"legacy.html","url":"https://plans.example.com/legacy.html",` +
			`"bucket":"plans","bytes":1}`,
		`{"type":"upload","time":"2026-07-08T13:30:11Z",` +
			`"key":"future.html","url":"https://plans.example.com/future.html",` +
			`"bucket":"plans","bytes":1,"marker_version":4}`,
		`{"type":"upload","time":"2026-07-08T14:03:11Z",` +
			`"key":"v1.html","url":"https://plans.example.com/v1.html",` +
			`"bucket":"plans","bytes":1,"marker_version":1}`,
		`{"type":"upload","time":"2026-07-08T15:03:11Z",` +
			`"key":"v2.html","url":"https://plans.example.com/v2.html",` +
			`"bucket":"plans","bytes":1,"marker_version":2}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].Key != "legacy.html" ||
		records[1].Key != "v1.html" || records[2].Key != "v2.html" ||
		len(warnings) != 1 {
		t.Fatalf("records = %+v, warnings = %v", records, warnings)
	}
	if uploads := ManifestUploads(records); len(uploads) != 3 {
		t.Fatalf("ManifestUploads = %+v, want legacy, v1, and v2", uploads)
	}
	if uploads := ActiveUploads(records); len(uploads) != 2 ||
		uploads[0].Key != "v1.html" || uploads[1].Key != "v2.html" {
		t.Fatalf("ActiveUploads = %+v, want v1 and v2", uploads)
	}
}

func TestMatchingManifestUploadsRequiresMatchingURLHost(t *testing.T) {
	dir := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	record := ManifestRecord{
		Type:          "upload",
		Key:           dir + "/plan.html",
		SourceKey:     dir + "/plan.md",
		URL:           "https://plans.example.com/base/" + dir + "/plan.html",
		MarkerVersion: MarkerVersion,
	}
	records := []ManifestRecord{record}

	for _, target := range []string{
		"https://other.example.com/base/" + dir + "/plan.html",
		"https://other.example.com/" + dir + "/.airplan.json",
		"ftp://plans.example.com/base/" + dir + "/plan.html",
		"https://plans.example.com/%zz",
		"https://plans.example.com/base/another/plan.html",
	} {
		if matches := MatchingManifestUploads(records, target); len(matches) != 0 {
			t.Fatalf("target %q matched unrelated URL: %+v", target, matches)
		}
	}
	for _, target := range []string{
		"http://plans.example.com/base/" + dir + "/plan.html?download=1#page",
		"https://plans.example.com/base/" + dir + "/.airplan.json",
		dir + "/plan.md",
	} {
		if matches := MatchingManifestUploads(records, target); len(matches) != 1 {
			t.Fatalf("target %q matches = %+v, want record", target, matches)
		}
	}
	for _, invalid := range []ManifestRecord{
		{Type: "upload", Key: dir + "/plan.html", URL: "://bad"},
		{
			Type: "upload", Key: dir + "/plan.html",
			URL: "ftp://plans.example.com/" + dir + "/plan.html",
		},
		{
			Type: "upload", Key: "invalid/plan.html",
			URL: "https://plans.example.com/invalid/plan.html",
		},
	} {
		if matches := MatchingManifestUploads(
			[]ManifestRecord{invalid},
			"https://plans.example.com/"+dir+"/plan.html",
		); len(matches) != 0 {
			t.Fatalf("invalid record matched URL: %+v", invalid)
		}
	}
}

func TestMatchingManifestUploadsMatchesFlatLegacyKeys(t *testing.T) {
	record := ManifestRecord{
		Type: "upload", Key: "legacy.html", SourceKey: "legacy.md",
		URL: "https://plans.example.com/legacy.html",
	}
	for _, target := range []string{
		"legacy.html",
		"/legacy.md",
		"https://plans.example.com/base/legacy.md",
	} {
		matches := MatchingManifestUploads([]ManifestRecord{record}, target)
		if len(matches) != 1 || matches[0].Key != record.Key {
			t.Fatalf("target %q matches = %+v, want legacy record",
				target, matches)
		}
	}
}

func TestMatchingManifestUploadsMatchesCollectionDirectChildren(t *testing.T) {
	dir := strings.Repeat("c", 26)
	record := ManifestRecord{
		Type: "upload", Kind: string(UploadKindCollection),
		Key: dir + "/index.html", MarkerKey: dir + "/" + CollectionMarkerFilename,
		URL: "https://plans.example.com/base/" + dir + "/index.html",
	}
	for _, target := range []string{
		dir + "/shot.png",
		"https://plans.example.com/base/" + dir + "/demo.webm",
	} {
		if matches := MatchingManifestUploads([]ManifestRecord{record}, target); len(matches) != 1 {
			t.Fatalf("target %q matches = %+v", target, matches)
		}
	}
	if matches := MatchingManifestUploads([]ManifestRecord{record},
		"https://other.example.com/base/"+dir+"/demo.webm"); len(matches) != 0 {
		t.Fatalf("cross-host target matched: %+v", matches)
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
			Kind:          string(UploadKindDocument),
			Slug:          "upload",
			Bytes:         1,
			MarkerVersion: 1,
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

func TestManifestUploadsChronologicalTombstonesAreReversible(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	pageKey := dir + "/plan.html"
	markerKey := dir + "/" + MarkerFilename
	base := ManifestRecord{
		Type: "upload", Time: time.Date(2026, 7, 21, 1, 0, 0, 0, time.UTC),
		Key: pageKey, MarkerKey: markerKey, URL: "https://example.com/" + pageKey,
		Bucket: "plans", Profile: "work", Format: "md", Bytes: 10,
		MarkerVersion: 1,
	}
	tombstone := ManifestRecord{
		Type: "delete", Time: base.Time.Add(time.Minute), Key: pageKey,
		MarkerKey: markerKey, Bucket: "plans", Profile: "work",
		Reason: "remote_missing",
	}
	restored := base
	restored.Time = tombstone.Time.Add(time.Minute)
	restored.MarkerVersion = MarkerVersion
	restored.Title = "restored"

	got := ManifestUploads([]ManifestRecord{base, tombstone, restored})
	if len(got) != 1 || got[0].Title != "restored" {
		t.Fatalf("active records = %+v", got)
	}
	duplicate := restored
	duplicate.Time = duplicate.Time.Add(time.Minute)
	duplicate.Title = "latest"
	got = ManifestUploads([]ManifestRecord{base, tombstone, restored, duplicate})
	if len(got) != 1 || got[0].Title != "latest" {
		t.Fatalf("deduplicated records = %+v", got)
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
