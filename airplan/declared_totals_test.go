package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// expectedDeclaredTotals recomputes an upload's declared totals straight from
// the marker body the fixture stored, independently of the writer under test.
func expectedDeclaredTotals(
	t *testing.T, fake *syncStorage, markerKey string,
) (int, int64) {
	t.Helper()

	fake.mu.Lock()
	body := append([]byte(nil), fake.objects[markerKey]...)
	fake.mu.Unlock()
	if len(body) == 0 {
		t.Fatalf("marker %q is missing from storage", markerKey)
	}
	var marker struct {
		Objects []struct {
			Bytes int64 `json:"bytes"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(body, &marker); err != nil {
		t.Fatalf("json.Unmarshal marker: %v", err)
	}
	total := int64(len(body))
	for _, object := range marker.Objects {
		total += object.Bytes
	}
	return len(marker.Objects) + 1, total
}

func manifestRecordsByMarker(
	t *testing.T, path string,
) map[string]ManifestRecord {
	t.Helper()

	records, warnings, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %q, want none", warnings)
	}
	byMarker := make(map[string]ManifestRecord, len(records))
	for _, record := range ManifestUploads(records) {
		byMarker[manifestMarkerKey(record)] = record
	}
	return byMarker
}

// uploadFixtures writes one document with a source, one HTML document without
// one, and one collection, returning their marker keys by name.
func uploadFixtures(
	t *testing.T, client *Client,
) map[string]string {
	t.Helper()

	ctx := context.Background()
	document, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("# Plan\n\nbody text\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.Upload(ctx, Input{
		Reader: strings.NewReader("<html><head></head><body>hi</body></html>"),
		Name:   "page.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	image := []byte("not really a png")
	collection, err := client.UploadFiles(ctx, FilesInput{
		Files: []FileInput{
			{
				Name: "shot.png", Reader: bytes.NewReader(image),
				Size: int64(len(image)),
			},
			{Name: "notes.txt", Reader: strings.NewReader("notes"), Size: 5},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"document":   document.MarkerKey,
		"page":       page.MarkerKey,
		"collection": collection.MarkerKey,
	}
}

// addStrayObject drops an undeclared sibling into an upload's directory. The
// marker does not list it, so it must not change declared totals — this is
// what separates declared counting from observing the directory.
func addStrayObject(fake *syncStorage, markerKey string) {
	dirPrefix := strings.TrimSuffix(markerKey, path.Base(markerKey))
	fake.addObject(dirPrefix+"stray.bin", bytes.Repeat([]byte("x"), 4096),
		filterTestTime(12))
}

func TestUploadRecordsDeclaredTotals(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)

	markers := uploadFixtures(t, client)
	records := manifestRecordsByMarker(t, manifest)
	if len(records) != 3 {
		t.Fatalf("records = %+v, want 3 uploads", records)
	}
	for name, markerKey := range markers {
		t.Run(name, func(t *testing.T) {
			record, ok := records[markerKey]
			if !ok {
				t.Fatalf("no record for marker %q", markerKey)
			}
			wantObjects, wantBytes := expectedDeclaredTotals(
				t, fake, markerKey,
			)
			if record.Objects != wantObjects ||
				record.TotalBytes != wantBytes {
				t.Fatalf("objects = %d, total_bytes = %d; want %d and %d",
					record.Objects, record.TotalBytes, wantObjects, wantBytes)
			}
			// The marker plus its declared objects, never the page alone.
			if record.TotalBytes <= record.Bytes {
				t.Fatalf("total_bytes = %d, want more than page bytes %d",
					record.TotalBytes, record.Bytes)
			}
		})
	}
	// A document with a source declares marker, page, and source; the HTML
	// page declares marker and page; the collection declares marker, index,
	// and two members.
	if got := records[markers["document"]].Objects; got != 3 {
		t.Errorf("document objects = %d, want 3", got)
	}
	if got := records[markers["page"]].Objects; got != 2 {
		t.Errorf("page objects = %d, want 2", got)
	}
	if got := records[markers["collection"]].Objects; got != 4 {
		t.Errorf("collection objects = %d, want 4", got)
	}
}

func TestMarkerDeclaredTotalsVersionEligibility(t *testing.T) {
	tests := []struct {
		name        string
		marker      UploadMarker
		wantOK      bool
		wantObjects int
		wantBytes   int64
	}{
		{
			name:   "v1",
			marker: UploadMarker{Version: 1, Page: "page.html"},
		},
		{
			name:   "v2 with source",
			marker: UploadMarker{Version: 2, Page: "page.html", PageBytes: 20, Source: "page.md"},
		},
		{
			name:   "v2 without source",
			marker: UploadMarker{Version: 2, Page: "page.html", PageBytes: 20},
			wantOK: true, wantObjects: 2, wantBytes: 120,
		},
		{
			name: "v3",
			marker: UploadMarker{Version: MarkerVersion, Objects: []MarkerObject{
				{Name: "page.html", Role: MarkerRolePage, Bytes: 20},
			}},
			wantOK: true, wantObjects: 2, wantBytes: 120,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objects, total, ok := MarkerDeclaredTotals(test.marker, 100)
			if ok != test.wantOK || objects != test.wantObjects || total != test.wantBytes {
				t.Fatalf("MarkerDeclaredTotals = %d/%d/%t, want %d/%d/%t",
					objects, total, ok, test.wantObjects, test.wantBytes, test.wantOK)
			}
		})
	}
}

func TestManifestRecordNeedsTotalsVersionEligibility(t *testing.T) {
	tests := []struct {
		name   string
		record ManifestRecord
		want   bool
	}{
		{"v1", ManifestRecord{MarkerVersion: 1}, false},
		{"v2 with source", ManifestRecord{MarkerVersion: 2, SourceKey: "page.md"}, false},
		{"v2 without source", ManifestRecord{MarkerVersion: 2}, true},
		{"v3", ManifestRecord{MarkerVersion: MarkerVersion}, true},
		{"already complete", ManifestRecord{MarkerVersion: MarkerVersion, Objects: 2, TotalBytes: 120}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := manifestRecordNeedsTotals(test.record); got != test.want {
				t.Fatalf("manifestRecordNeedsTotals = %t, want %t", got, test.want)
			}
		})
	}
}

// TestSyncImportMatchesUploadDeclaredTotals is the load-bearing check for
// "declared, not observed": the same upload must report the same totals
// whether it was recorded locally or imported from its marker by sync.
func TestSyncImportMatchesUploadDeclaredTotals(t *testing.T) {
	fake := newSyncStorage(t)
	uploaded := filepath.Join(t.TempDir(), "uploaded.jsonl")
	client := newSyncClient(t, fake.server.URL, uploaded)
	markers := uploadFixtures(t, client)

	for _, markerKey := range markers {
		addStrayObject(fake, markerKey)
	}
	imported := filepath.Join(t.TempDir(), "imported.jsonl")
	syncClient := newSyncClient(t, fake.server.URL, imported)
	result, err := syncClient.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 3 || len(result.Enriched) != 0 {
		t.Fatalf("sync = %+v", result)
	}

	uploadedRecords := manifestRecordsByMarker(t, uploaded)
	importedRecords := manifestRecordsByMarker(t, imported)
	for name, markerKey := range markers {
		want := uploadedRecords[markerKey]
		got := importedRecords[markerKey]
		if got.Objects != want.Objects || got.TotalBytes != want.TotalBytes {
			t.Errorf(
				"%s imported objects = %d, total_bytes = %d; "+
					"want %d and %d from upload",
				name, got.Objects, got.TotalBytes,
				want.Objects, want.TotalBytes,
			)
		}
		if got.Objects == 0 {
			t.Errorf("%s imported without declared totals", name)
		}
	}
}

// stripDeclaredTotals rewrites a manifest as pre-0.30 history: same records,
// neither total.
func stripDeclaredTotals(t *testing.T, path string) {
	t.Helper()

	records, _, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := range records {
		records[index].Objects = 0
		records[index].TotalBytes = 0
	}
	if err := rewriteManifest(t, path, records); err != nil {
		t.Fatal(err)
	}
}

func manifestLineCount(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Split(strings.TrimRight(string(data), "\n"), "\n"))
}

// TestSyncBackfillRestoresDeclaredTotals covers the third sync job kind: local
// history that predates declared totals converges to exactly the values a
// fresh upload of the same content records, in one pass, and stays there.
func TestSyncBackfillRestoresDeclaredTotals(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	markers := uploadFixtures(t, client)
	want := manifestRecordsByMarker(t, manifest)
	stripDeclaredTotals(t, manifest)
	for _, markerKey := range markers {
		addStrayObject(fake, markerKey)
	}
	strippedLines := manifestLineCount(t, manifest)

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Tombstoned) != 0 {
		t.Fatalf("backfill added or tombstoned records: %+v", result)
	}
	if len(result.Enriched) != 3 {
		t.Fatalf("enriched = %+v, want 3", result.Enriched)
	}
	// A record selected for enrichment is reported by its outcome, so it is
	// not also counted as unchanged.
	if result.Unchanged != 0 || result.Deferred != 0 {
		t.Fatalf("unchanged = %d, deferred = %d; want 0 and 0",
			result.Unchanged, result.Deferred)
	}
	if got := manifestLineCount(t, manifest); got != strippedLines+3 {
		t.Fatalf("manifest lines = %d, want %d", got, strippedLines+3)
	}

	got := manifestRecordsByMarker(t, manifest)
	for name, markerKey := range markers {
		if got[markerKey].Objects != want[markerKey].Objects ||
			got[markerKey].TotalBytes != want[markerKey].TotalBytes {
			t.Errorf("%s backfilled to %d/%d, want %d/%d", name,
				got[markerKey].Objects, got[markerKey].TotalBytes,
				want[markerKey].Objects, want[markerKey].TotalBytes)
		}
		// Enrichment restates an existing upload: identity and time survive.
		if !got[markerKey].Time.Equal(want[markerKey].Time) ||
			got[markerKey].Key != want[markerKey].Key ||
			got[markerKey].Title != want[markerKey].Title {
			t.Errorf("%s identity changed: %+v want %+v",
				name, got[markerKey], want[markerKey])
		}
	}

	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Enriched) != 0 || len(second.Added) != 0 {
		t.Fatalf("second sync = %+v, want no appends", second)
	}
	if second.Unchanged != 3 {
		t.Fatalf("second sync unchanged = %d, want 3", second.Unchanged)
	}
	if got := manifestLineCount(t, manifest); got != strippedLines+3 {
		t.Fatalf("second sync wrote records: %d lines", got)
	}
}

func TestSyncBackfillDryRunWritesNothing(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	uploadFixtures(t, client)
	stripDeclaredTotals(t, manifest)
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 3 {
		t.Fatalf("dry run planned %d enrichments, want 3",
			len(result.Enriched))
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("dry run wrote the manifest:\n%s", after)
	}
}

// TestSyncBackfillLeavesUnusableMarkersAlone covers the guard against
// guessing: an incomplete or invalid marker leaves both fields absent and the
// record untouched.
func TestSyncBackfillLeavesUnusableMarkersAlone(t *testing.T) {
	tests := []struct {
		name   string
		damage func(t *testing.T, fake *syncStorage, markerKey string)
	}{
		{
			name: "incomplete",
			damage: func(t *testing.T, fake *syncStorage, markerKey string) {
				t.Helper()
				dirPrefix := strings.TrimSuffix(markerKey, MarkerFilename)
				fake.mu.Lock()
				defer fake.mu.Unlock()
				for key := range fake.objects {
					if strings.HasPrefix(key, dirPrefix) && key != markerKey {
						fake.objects[key] = []byte("truncated")
					}
				}
			},
		},
		{
			name: "invalid",
			damage: func(t *testing.T, fake *syncStorage, markerKey string) {
				t.Helper()
				fake.addObject(markerKey, []byte("{not json"),
					filterTestTime(12))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newSyncStorage(t)
			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			client := newSyncClient(t, fake.server.URL, manifest)
			res, err := client.Upload(context.Background(), Input{
				Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
			})
			if err != nil {
				t.Fatal(err)
			}
			stripDeclaredTotals(t, manifest)
			before, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			tt.damage(t, fake, res.MarkerKey)

			result, err := client.SyncManifest(context.Background(),
				SyncManifestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Enriched) != 0 {
				t.Fatalf("enriched = %+v, want none", result.Enriched)
			}
			// Deferred rather than silent: the marker is re-inspected on
			// every run, so an operator has to be able to see why.
			if result.Deferred != 1 || len(result.Failures) != 0 {
				t.Fatalf("deferred = %d, failures = %+v; want 1 and none",
					result.Deferred, result.Failures)
			}
			after, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("manifest changed:\n%s", after)
			}
			records := manifestRecordsByMarker(t, manifest)
			if records[res.MarkerKey].Objects != 0 ||
				records[res.MarkerKey].TotalBytes != 0 {
				t.Fatalf("record gained totals: %+v",
					records[res.MarkerKey])
			}
		})
	}
}

// TestSyncBackfillFailureDoesNotFailTheRun covers the operational contract for
// enrichment: it is metadata completion for uploads already in history, so a
// storage failure defers it with a warning instead of failing sync. Otherwise
// one unreadable marker would fail a cron sync forever, since the record keeps
// qualifying on every run.
func TestSyncBackfillFailureDoesNotFailTheRun(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	stripDeclaredTotals(t, manifest)
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	fake.failGet(res.MarkerKey)

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatalf("sync failed on an enrichment error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("failures = %+v, want none", result.Failures)
	}
	if result.Deferred != 1 {
		t.Fatalf("deferred = %d, want 1", result.Deferred)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enriched = %+v, want none", result.Enriched)
	}
	warned := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, res.MarkerKey) &&
			strings.Contains(warning, "declared totals") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings = %q, want the deferred marker named",
			result.Warnings)
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("manifest changed:\n%s", after)
	}
}

// TestSyncBackfillSkipsVersionOneHistory keeps sync from re-fetching v1
// markers, which cannot declare the page size needed for exact totals.
func TestSyncBackfillSkipsVersionOneHistory(t *testing.T) {
	fake := newSyncStorage(t)
	dir := strings.Repeat("d", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: filterTestTime(12), Format: "html", Page: "old.html",
	}, []byte("old page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)

	first, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Added) != 1 || first.Added[0].Objects != 0 ||
		first.Added[0].TotalBytes != 0 {
		t.Fatalf("import = %+v, want no declared totals", first.Added)
	}
	lines := manifestLineCount(t, manifest)

	markerKey := dir + "/" + MarkerFilename
	fetches := fake.getCount(markerKey)

	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Enriched) != 0 || second.Unchanged != 1 {
		t.Fatalf("second sync = %+v, want no enrichment", second)
	}
	if got := manifestLineCount(t, manifest); got != lines {
		t.Fatalf("manifest lines = %d, want %d", got, lines)
	}
	// The record's own marker_version rules enrichment out, so the second run
	// spends no request discovering that again.
	if got := fake.getCount(markerKey); got != fetches {
		t.Fatalf("marker fetches = %d, want %d; version-one history refetched",
			got, fetches)
	}
}

func TestSyncBackfillVersionTwoEligibility(t *testing.T) {
	when := filterTestTime(12)
	for _, test := range []struct {
		name       string
		source     string
		wantEnrich bool
	}{
		{"without source", "", true},
		{"with source", "page.md", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newSyncStorage(t)
			dir := strings.Repeat("v", 26)
			format := "html"
			if test.source != "" {
				format = "md"
			}
			marker := UploadMarker{
				Schema: MarkerSchema, Version: 2, Directory: dir,
				CreatedAt: when, Format: format, Page: "page.html",
				PageBytes: 9, Source: test.source,
			}
			fake.addUpload(t, marker, []byte("123456789"))
			if test.source != "" {
				fake.addObject(dir+"/"+test.source, []byte("source"), when)
			}
			markerKey := dir + "/" + MarkerFilename
			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			sourceKey := ""
			if test.source != "" {
				sourceKey = dir + "/" + test.source
			}
			record := ManifestRecord{
				Type: "upload", Time: when, Key: dir + "/page.html",
				MarkerKey: markerKey, SourceKey: sourceKey,
				URL:    "https://plans.example.com/" + dir + "/page.html",
				Bucket: "plans", Profile: "work", Format: format,
				Kind: string(UploadKindDocument), Slug: "page", Bytes: 9,
				MarkerVersion: 2,
			}
			if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
				t.Fatal(err)
			}
			client := newSyncClient(t, fake.server.URL, manifest)
			before := fake.getCount(markerKey)
			result, err := client.SyncManifest(context.Background(), SyncManifestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if test.wantEnrich {
				body, err := EncodeUploadMarker(marker)
				if err != nil {
					t.Fatal(err)
				}
				wantTotal := int64(len(body)) + marker.PageBytes
				if len(result.Enriched) != 1 || result.Enriched[0].Objects != 2 ||
					result.Enriched[0].TotalBytes != wantTotal {
					t.Fatalf("result = %+v, want v2 enrichment", result)
				}
				if fake.getCount(markerKey) <= before {
					t.Fatal("eligible v2 marker was not fetched")
				}
				afterFirst := fake.getCount(markerKey)
				second, err := client.SyncManifest(context.Background(), SyncManifestOptions{})
				if err != nil || len(second.Enriched) != 0 {
					t.Fatalf("second sync = %+v, %v; want convergence", second, err)
				}
				if fake.getCount(markerKey) != afterFirst {
					t.Fatalf("converged marker fetched %d extra times",
						fake.getCount(markerKey)-afterFirst)
				}
				return
			}
			if len(result.Enriched) != 0 || fake.getCount(markerKey) != before {
				t.Fatalf("result = %+v, gets = %d, want ineligible v2 skipped",
					result, fake.getCount(markerKey)-before)
			}
		})
	}
}

// TestSyncBackfillNeverResurrectsTombstonedIdentity drives the commit path
// directly with a tombstone appended after planning, the window the guard
// exists for.
func TestSyncBackfillNeverResurrectsTombstonedIdentity(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	stripDeclaredTotals(t, manifest)
	records, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planned := records[0]
	planned.Objects = 3
	planned.TotalBytes = 999
	initialLen := len(records)

	// Concurrent deletion lands between planning and the commit.
	if err := appendManifestRecord(context.Background(), manifest,
		ManifestRecord{
			Type: "delete", Time: records[0].Time.Add(time.Minute),
			Key: res.Key, MarkerKey: res.MarkerKey, Bucket: "plans",
			Profile: "work", Reason: "deleted",
		}); err != nil {
		t.Fatal(err)
	}
	lines := manifestLineCount(t, manifest)

	result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
	if err := client.commitSyncManifest(
		context.Background(), manifest, initialLen, result,
	); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enriched = %+v, want none after a concurrent delete",
			result.Enriched)
	}
	if got := manifestLineCount(t, manifest); got != lines {
		t.Fatalf("manifest lines = %d, want %d", got, lines)
	}
	current, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if got := ActiveUploads(current); len(got) != 0 {
		t.Fatalf("active uploads = %+v, want the upload to stay deleted", got)
	}
}

// TestSyncBackfillMergesIntoTheCurrentRecord covers a concurrent update: a
// newer record for the same identity was appended while the marker was being
// inspected, so enrichment must add its two fields to that record rather than
// re-appending the snapshot it planned from, which would revert the update.
func TestSyncBackfillMergesIntoTheCurrentRecord(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	if _, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	}); err != nil {
		t.Fatal(err)
	}
	stripDeclaredTotals(t, manifest)
	records, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planned := records[0]
	planned.Objects = 3
	planned.TotalBytes = 4096
	initialLen := len(records)

	// A concurrent writer re-titles the same upload while the marker is being
	// inspected.
	updated := records[0]
	updated.Title = "Retitled plan"
	if err := appendManifestRecord(
		context.Background(), manifest, updated,
	); err != nil {
		t.Fatal(err)
	}

	result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
	if err := client.commitSyncManifest(
		context.Background(), manifest, initialLen, result,
	); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 1 {
		t.Fatalf("enriched = %+v, want one record", result.Enriched)
	}
	got := manifestRecordsByMarker(t, manifest)[planned.MarkerKey]
	if got.Title != "Retitled plan" {
		t.Fatalf("title = %q, want the concurrent update preserved", got.Title)
	}
	if got.Objects != 3 || got.TotalBytes != 4096 {
		t.Fatalf("totals = %d/%d, want 3/4096", got.Objects, got.TotalBytes)
	}
}

func TestSyncBackfillRejectsStaleInspectionIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*ManifestRecord)
	}{
		{"changed time", func(record *ManifestRecord) {
			record.Time = record.Time.Add(time.Minute)
		}},
		{"changed key", func(record *ManifestRecord) {
			record.Key = strings.TrimSuffix(record.Key, ".html") + "-new.html"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newSyncStorage(t)
			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			client := newSyncClient(t, fake.server.URL, manifest)
			if _, err := client.Upload(context.Background(), Input{
				Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
			}); err != nil {
				t.Fatal(err)
			}
			stripDeclaredTotals(t, manifest)
			records, _, err := ReadManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			planned := records[0]
			planned.Objects, planned.TotalBytes = 3, 4096
			updated := records[0]
			test.mutate(&updated)
			if err := appendManifestRecord(context.Background(), manifest, updated); err != nil {
				t.Fatal(err)
			}
			lines := manifestLineCount(t, manifest)
			result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
			if err := client.commitSyncManifest(context.Background(), manifest, len(records), result); err != nil {
				t.Fatal(err)
			}
			if len(result.Enriched) != 0 || manifestLineCount(t, manifest) != lines {
				t.Fatalf("result = %+v, want stale enrichment skipped", result)
			}
		})
	}
}

// TestSyncBackfillSkipsUnknownIdentity covers the other half of the commit
// guard: a planned record whose identity is not in the reread snapshot at all,
// as opposed to one that was tombstoned.
func TestSyncBackfillSkipsUnknownIdentity(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	if _, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	}); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	lines := manifestLineCount(t, manifest)

	stranger := records[0]
	stranger.Key = strings.Repeat("z", 26) + "/plan.html"
	stranger.MarkerKey = strings.Repeat("z", 26) + "/" + MarkerFilename
	stranger.Objects = 3
	stranger.TotalBytes = 4096

	result := &SyncManifestResult{Enriched: []ManifestRecord{stranger}}
	if err := client.commitSyncManifest(
		context.Background(), manifest, len(records), result,
	); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enriched = %+v, want none", result.Enriched)
	}
	if got := manifestLineCount(t, manifest); got != lines {
		t.Fatalf("manifest lines = %d, want %d", got, lines)
	}
}

// TestSyncBackfillSkipsOutOfScopeRecords keeps enrichment inside the resolved
// profile and bucket: a record naming another one is not this service's to
// complete, even though its marker key is present in the same listing.
func TestSyncBackfillSkipsOutOfScopeRecords(t *testing.T) {
	fake := newSyncStorage(t)
	seed := filepath.Join(t.TempDir(), "seed.jsonl")
	client := newSyncClient(t, fake.server.URL, seed)
	if _, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	}); err != nil {
		t.Fatal(err)
	}
	stripDeclaredTotals(t, seed)
	seeded, _, err := ReadManifest(seed)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		mutate func(rec *ManifestRecord)
	}{
		{
			name:   "other profile",
			mutate: func(rec *ManifestRecord) { rec.Profile = "personal" },
		},
		{
			name:   "other bucket",
			mutate: func(rec *ManifestRecord) { rec.Bucket = "archive" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := seeded[0]
			test.mutate(&record)
			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			if err := rewriteManifest(
				t, manifest, []ManifestRecord{record},
			); err != nil {
				t.Fatal(err)
			}
			scoped := newSyncClient(t, fake.server.URL, manifest)

			result, err := scoped.SyncManifest(context.Background(),
				SyncManifestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Enriched) != 0 || result.Deferred != 0 {
				t.Fatalf("result = %+v, want the record left alone", result)
			}
			current, _, err := ReadManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			for _, got := range current {
				if got.Profile == record.Profile &&
					got.Bucket == record.Bucket && got.Objects != 0 {
					t.Fatalf("out-of-scope record gained totals: %+v", got)
				}
			}
		})
	}
}

// TestScopedActiveUploadsRespectsKeyPrefix pins the third scope dimension at
// the gate every sync job kind shares.
func TestScopedActiveUploadsRespectsKeyPrefix(t *testing.T) {
	inside := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "team/current", "a")
	outside := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "team/old", "b")

	active := scopedActiveUploads(
		[]ManifestRecord{inside, outside},
		&Config{Profile: "work", Bucket: "plans", KeyPrefix: "team/current"},
	)
	if len(active) != 1 {
		t.Fatalf("active = %+v, want only the in-prefix record", active)
	}
	if _, ok := active[inside.MarkerKey]; !ok {
		t.Fatalf("active = %+v, want %q", active, inside.MarkerKey)
	}
}

// TestSyncBackfillDefersMarkerWithoutDeclaredSizes covers a record recorded as
// v3 whose remote marker declares no sizes: nothing is guessed, and the run
// reports why rather than silently doing nothing forever.
func TestSyncBackfillDefersMarkerWithoutDeclaredSizes(t *testing.T) {
	fake := newSyncStorage(t)
	dir := strings.Repeat("e", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: filterTestTime(12), Format: "html", Page: "old.html",
	}, []byte("old page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)

	// Local history claims a v3 upload while storage holds a v1 marker, so
	// the record qualifies for enrichment but the marker cannot supply it.
	markerKey := dir + "/" + MarkerFilename
	if err := appendManifestRecord(context.Background(), manifest,
		ManifestRecord{
			Type: "upload", Time: filterTestTime(12),
			Key: dir + "/old.html", MarkerKey: markerKey,
			URL:     "https://plans.example.com/" + dir + "/old.html",
			Bucket:  "plans",
			Profile: "work", Kind: string(UploadKindDocument), Slug: "old",
			Bytes: 8, MarkerVersion: MarkerVersion,
		}); err != nil {
		t.Fatal(err)
	}
	lines := manifestLineCount(t, manifest)

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 || result.Deferred != 1 {
		t.Fatalf("result = %+v, want one deferred record", result)
	}
	if got := manifestLineCount(t, manifest); got != lines {
		t.Fatalf("manifest lines = %d, want %d", got, lines)
	}
	warned := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "declares no object sizes") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings = %q, want the reason reported", result.Warnings)
	}
}

// TestSyncBackfillSkipsAlreadyEnrichedIdentity covers the second commit guard:
// a concurrent run that enriched the same identity first leaves nothing to do.
func TestSyncBackfillSkipsAlreadyEnrichedIdentity(t *testing.T) {
	fake := newSyncStorage(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	if _, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n"), Name: "plan.md",
	}); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	planned := records[0]
	planned.Objects = 99
	planned.TotalBytes = 999
	lines := manifestLineCount(t, manifest)

	result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
	if err := client.commitSyncManifest(
		context.Background(), manifest, len(records), result,
	); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enriched = %+v, want none", result.Enriched)
	}
	if got := manifestLineCount(t, manifest); got != lines {
		t.Fatalf("manifest lines = %d, want %d", got, lines)
	}
}

// TestManifestPartialDeclaredTotalsAreInvalid covers a half-written pair from
// another implementation: backfill only completes records missing both fields,
// so a record carrying one of them would render inconsistently forever. It is
// skipped with a warning like any other invalid line.
func TestManifestPartialDeclaredTotalsAreInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	complete := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "", "a")
	complete.Objects = 3
	complete.TotalBytes = 4096
	partial := manifestUploadRecord(
		filterTestTime(13), "work", "plans", "", "b")
	partial.Objects = 3
	if err := rewriteManifest(
		t, path, []ManifestRecord{complete, partial},
	); err != nil {
		t.Fatal(err)
	}

	records, warnings, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Title != "a" {
		t.Fatalf("records = %+v, want only the complete pair", records)
	}
	if len(warnings) != 1 ||
		!strings.Contains(warnings[0], "must be set together") {
		t.Fatalf("warnings = %q, want one explaining the pair", warnings)
	}
}

func TestManifestExplicitZeroDeclaredTotalsAreInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	dir := strings.Repeat("z", 26)
	line := fmt.Sprintf(
		`{"type":"upload","time":"2026-07-08T12:00:00Z",`+
			`"key":%q,"marker_key":%q,"url":%q,"bucket":"plans",`+
			`"kind":"document","slug":"plan","bytes":10,`+
			`"objects":0,"total_bytes":0,"marker_version":3}`+"\n",
		dir+"/plan.html", dir+"/"+MarkerFilename,
		"https://plans.example.com/"+dir+"/plan.html",
	)
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	records, warnings, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 || len(warnings) != 1 ||
		!strings.Contains(warnings[0], "must be positive") {
		t.Fatalf("records = %+v, warnings = %q; want explicit-zero refusal",
			records, warnings)
	}
}

// TestManifestRecordDeclaredTotalsJSON pins the wire names and omission rules
// for the two fields, which SPEC §9 calls a cross-implementation contract.
func TestManifestRecordDeclaredTotalsJSON(t *testing.T) {
	record := ManifestRecord{
		Type: "upload", Time: filterTestTime(12),
		Key:       "aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",
		MarkerKey: "aaaaaaaaaaaaaaaaaaaaaaaaaa/" + MarkerFilename,
		URL:       "https://plans.example.com/aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",
		Bucket:    "plans", Profile: "work", Kind: "document", Slug: "plan",
		Format: "md", Title: "Plan", Bytes: 18432, Objects: 3,
		TotalBytes: 19000, MarkerVersion: MarkerVersion,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"upload","time":"2026-07-08T12:00:00Z",` +
		`"key":"aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",` +
		`"marker_key":"aaaaaaaaaaaaaaaaaaaaaaaaaa/.airplan.json",` +
		`"url":"https://plans.example.com/aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",` +
		`"bucket":"plans","profile":"work","format":"md","kind":"document",` +
		`"slug":"plan","title":"Plan","bytes":18432,"objects":3,` +
		`"total_bytes":19000,"marker_version":3}`
	if string(encoded) != want {
		t.Fatalf("encoded =\n%s\nwant\n%s", encoded, want)
	}

	legacy := record
	legacy.Objects = 0
	legacy.TotalBytes = 0
	encoded, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "objects") ||
		strings.Contains(string(encoded), "total_bytes") {
		t.Fatalf("absent totals were encoded: %s", encoded)
	}
}
