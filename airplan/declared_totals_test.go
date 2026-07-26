package airplan

import (
	"bytes"
	"context"
	"encoding/json"
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
	if result.Unchanged != 3 {
		t.Fatalf("unchanged = %d, want 3", result.Unchanged)
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

// TestSyncBackfillSkipsPreV3History keeps sync from re-fetching markers that
// can never supply declared totals.
func TestSyncBackfillSkipsPreV3History(t *testing.T) {
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
		t.Fatalf("marker fetches = %d, want %d; pre-v3 history refetched",
			got, fetches)
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
