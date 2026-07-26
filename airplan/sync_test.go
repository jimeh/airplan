package airplan

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSyncManifestImportsPrunesAndRestores(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dirV1 := strings.Repeat("a", 26)
	dirV2 := strings.Repeat("b", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dirV1,
		CreatedAt: when, Format: "html", Page: "old.html",
	}, []byte("old page"))
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dirV2,
		CreatedAt: when.Add(time.Minute), Kind: UploadKindDocument,
		Slug: "new", Format: "md", Objects: []MarkerObject{
			{Name: "new.html", Role: MarkerRolePage, Bytes: 10, ContentType: pageContentType},
			{Name: "new.md", Role: MarkerRoleSource, Bytes: 8, ContentType: sourceContentType},
		}, Title: "New plan",
		Repo: "https://github.com/acme/repo",
	}, []byte("0123456789"))
	fake.addObject(dirV2+"/new.md", []byte("# source"), when)

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 2 || len(result.Tombstoned) != 0 {
		t.Fatalf("first sync = %+v", result)
	}
	if result.Added[0].MarkerVersion != 1 ||
		result.Added[0].Bytes != int64(len("old page")) ||
		result.Added[1].Repo != "https://github.com/acme/repo" ||
		result.Added[1].Profile != "work" || result.Added[1].Format != "md" {
		t.Fatalf("imported records = %+v", result.Added)
	}

	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(second.Added) != 0 || second.Unchanged != 2 {
		t.Fatalf("second sync = %+v, %v", second, err)
	}

	fake.removeMarker(dirV2 + "/" + MarkerFilename)
	pruned, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(pruned.Tombstoned) != 1 ||
		pruned.Tombstoned[0].Reason != "remote_missing" {
		t.Fatalf("pruned sync = %+v, %v", pruned, err)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(ActiveUploads(records)) != 1 {
		t.Fatalf("active after prune = %+v, %v", ActiveUploads(records), err)
	}

	fake.addMarker(t, UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dirV2,
		CreatedAt: when.Add(time.Minute), Kind: UploadKindDocument,
		Slug: "new", Format: "md", Objects: []MarkerObject{
			{Name: "new.html", Role: MarkerRolePage, Bytes: 10, ContentType: pageContentType},
			{Name: "new.md", Role: MarkerRoleSource, Bytes: 8, ContentType: sourceContentType},
		}, Title: "New plan",
		Repo: "https://github.com/acme/repo",
	})
	restored, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(restored.Added) != 1 {
		t.Fatalf("restored sync = %+v, %v", restored, err)
	}
	records, _, _ = ReadManifest(manifest)
	if len(ActiveUploads(records)) != 2 {
		t.Fatalf("active after restore = %+v", ActiveUploads(records))
	}
}

func TestSyncManifestRequiresConfirmedAbsenceAndHonorsDryRun(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("c", 26)
	marker := UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}
	fake.addUpload(t, marker, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	if _, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	fake.hideMarker(dir + "/" + MarkerFilename)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true, DryRun: true})
	if err != nil || len(result.Tombstoned) != 0 || result.Retained != 1 {
		t.Fatalf("dry run = %+v, %v", result, err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(before) {
		t.Fatalf("manifest changed during dry run: %v", err)
	}
}

func TestSyncManifestClassifiesInvalidAndIncomplete(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	invalidDir := strings.Repeat("d", 26)
	incompleteDir := strings.Repeat("e", 26)
	fake.addObject(invalidDir+"/"+MarkerFilename, []byte(`{"schema":`), when)
	fake.addMarker(t, UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: incompleteDir,
		CreatedAt: when, Kind: UploadKindDocument, Slug: "plan", Format: "html",
		Objects: []MarkerObject{{
			Name: "plan.html", Role: MarkerRolePage,
			Bytes: 99, ContentType: pageContentType,
		}},
	})
	fake.addObject(incompleteDir+"/plan.html", []byte("short"), when)
	client := newSyncClient(t, fake.server.URL,
		filepath.Join(t.TempDir(), "manifest.jsonl"))
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || result.Invalid != 1 || result.Incomplete != 1 ||
		len(result.Added) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestSyncManifestDoesNotDuplicateManifestWarnings(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("w", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.WriteFile(manifest, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := newSyncClient(t, fake.server.URL, manifest).SyncManifest(
		context.Background(), SyncManifestOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one manifest warning", result.Warnings)
	}
}

func TestSyncManifestConcurrentRunsDoNotDuplicateImports(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("z", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	start := make(chan struct{})
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := client.SyncManifest(context.Background(),
				SyncManifestOptions{Prune: true})
			errors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 1 ||
		len(ActiveUploads(records)) != 1 {
		t.Fatalf("records = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
}

func TestCommitSyncManifestDoesNotTombstoneConcurrentRestoration(t *testing.T) {
	dir := strings.Repeat("r", 26)
	markerKey := dir + "/" + MarkerFilename
	pageKey := dir + "/plan.html"
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	initial := ManifestRecord{
		Type: "upload", Time: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		Key: pageKey, MarkerKey: markerKey,
		URL:    "https://plans.example.com/" + pageKey,
		Bucket: "plans", Profile: "work", Format: "html", Bytes: 4,
		MarkerVersion: MarkerVersion,
	}
	if err := appendManifestRecord(context.Background(), manifest, initial); err != nil {
		t.Fatal(err)
	}
	initialRecords, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	restored := initial
	restored.Time = initial.Time.Add(time.Minute)
	restored.Title = "concurrently restored"
	if err := appendManifestRecord(context.Background(), manifest, restored); err != nil {
		t.Fatal(err)
	}
	result := &SyncManifestResult{Tombstoned: []ManifestRecord{{
		Type: "delete", Time: restored.Time.Add(time.Minute), Key: pageKey,
		MarkerKey: markerKey, Bucket: "plans", Profile: "work",
		Reason: "remote_missing",
	}}}
	client := &Client{cfg: &Config{Bucket: "plans", Profile: "work"}}
	if err := client.commitSyncManifest(context.Background(), manifest,
		len(initialRecords), result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tombstoned) != 0 {
		t.Fatalf("stale tombstones appended = %+v", result.Tombstoned)
	}
	records, warnings, err := ReadManifest(manifest)
	active := ActiveUploads(records)
	if err != nil || len(warnings) != 0 || len(records) != 2 ||
		len(active) != 1 || active[0].Title != restored.Title {
		t.Fatalf("records = %+v, active = %+v, warnings = %v, error = %v",
			records, active, warnings, err)
	}
}

func TestSyncManifestConcurrencyLimit(t *testing.T) {
	for _, test := range []struct {
		name        string
		concurrency int
		want        int
	}{
		{name: "serial", concurrency: 1, want: 1},
		{name: "default", concurrency: 0, want: DefaultRemoteConcurrency},
		{name: "larger override", concurrency: 12, want: 12},
	} {
		t.Run(test.name, func(t *testing.T) {
			when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
			fake := newSyncStorage(t)
			fake.delay = 10 * time.Millisecond
			for index := range 20 {
				dir := strings.Repeat(string(rune('f'+index)), 26)
				page := fmt.Sprintf("plan-%d.html", index)
				fake.addUpload(t, UploadMarker{
					Schema: MarkerSchema, Version: 1, Directory: dir,
					CreatedAt: when, Format: "html", Page: page,
				}, []byte("page"))
			}
			client := newSyncClient(t, fake.server.URL,
				filepath.Join(t.TempDir(), "manifest.jsonl"))
			if _, err := client.SyncManifest(context.Background(),
				SyncManifestOptions{Concurrency: test.concurrency}); err != nil {
				t.Fatal(err)
			}
			if fake.maxInFlight != test.want {
				t.Fatalf("max in flight = %d, want %d",
					fake.maxInFlight, test.want)
			}
		})
	}
}

func declaredDocumentMarker(dir string, when time.Time) UploadMarker {
	return UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: when, Kind: UploadKindDocument, Slug: "new",
		Format: "md", Objects: []MarkerObject{
			{
				Name: "new.html", Role: MarkerRolePage, Bytes: 10,
				ContentType: pageContentType,
			},
			{
				Name: "new.md", Role: MarkerRoleSource, Bytes: 8,
				ContentType: sourceContentType,
			},
		}, Title: "New plan",
	}
}

func addDeclaredDocumentUpload(
	t *testing.T, fake *syncStorage, dir string, when time.Time,
) UploadMarker {
	t.Helper()
	marker := declaredDocumentMarker(dir, when)
	fake.addUpload(t, marker, []byte("0123456789"))
	fake.addObject(dir+"/new.md", []byte("12345678"), when)
	return marker
}

func declaredMarkerTotals(t *testing.T, marker UploadMarker) (int, int64) {
	t.Helper()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	total := int64(len(body))
	for _, object := range marker.Objects {
		total += object.Bytes
	}
	return len(marker.Objects) + 1, total
}

func TestSyncImportRecordsDeclaredTotals(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	docDir := strings.Repeat("d", 26)
	docMarker := addDeclaredDocumentUpload(t, fake, docDir, when)
	colDir := strings.Repeat("e", 26)
	colMarker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: colDir,
		CreatedAt: when.Add(time.Minute), Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 4,
				ContentType: pageContentType,
			},
			{
				Name: "shot.png", Role: MarkerRoleFile, Bytes: 6,
				ContentType: "image/png",
			},
		}, Title: "Shots",
	}
	fake.addUpload(t, colMarker, []byte("page"))
	fake.addObject(colDir+"/shot.png", []byte("123456"), when)

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(result.Added) != 2 {
		t.Fatalf("sync = %+v, %v", result, err)
	}

	wantDocObjects, wantDocTotal := declaredMarkerTotals(t, docMarker)
	wantColObjects, wantColTotal := declaredMarkerTotals(t, colMarker)
	doc, col := result.Added[0], result.Added[1]
	if doc.Kind != string(UploadKindDocument) {
		doc, col = col, doc
	}
	if doc.Objects != wantDocObjects || doc.TotalBytes != wantDocTotal {
		t.Fatalf("document totals = (%d, %d), want (%d, %d)",
			doc.Objects, doc.TotalBytes, wantDocObjects, wantDocTotal)
	}
	if col.Objects != wantColObjects || col.TotalBytes != wantColTotal {
		t.Fatalf("collection totals = (%d, %d), want (%d, %d)",
			col.Objects, col.TotalBytes, wantColObjects, wantColTotal)
	}
}

func backfillManifestLine(dir string, when time.Time) string {
	return `{"type":"upload","time":"` + when.UTC().Format(time.RFC3339) +
		`","key":"` + dir + `/new.html",` +
		`"source_key":"` + dir + `/new.md",` +
		`"marker_key":"` + dir + `/.airplan.json",` +
		`"url":"https://plans.example.com/` + dir + `/new.html",` +
		`"bucket":"plans","profile":"work","format":"md",` +
		`"kind":"document","title":"New plan","bytes":10,` +
		`"marker_version":3}` + "\n"
}

func TestSyncBackfillsDeclaredTotals(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("f", 26)
	marker := addDeclaredDocumentUpload(t, fake, dir, when)

	// The active local record predates objects/total_bytes.
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.WriteFile(manifest,
		[]byte(backfillManifestLine(dir, when)), 0o600); err != nil {
		t.Fatal(err)
	}
	client := newSyncClient(t, fake.server.URL, manifest)

	dryRun, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true, DryRun: true})
	if err != nil || len(dryRun.Enriched) != 1 || len(dryRun.Added) != 0 {
		t.Fatalf("dry run = %+v, %v", dryRun, err)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 {
		t.Fatalf("dry run wrote manifest: %+v, %v", records, err)
	}

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Enriched) != 1 ||
		result.Unchanged != 1 {
		t.Fatalf("sync = %+v", result)
	}
	wantObjects, wantTotal := declaredMarkerTotals(t, marker)
	enriched := result.Enriched[0]
	if enriched.Objects != wantObjects || enriched.TotalBytes != wantTotal {
		t.Fatalf("enriched totals = (%d, %d), want (%d, %d)",
			enriched.Objects, enriched.TotalBytes, wantObjects, wantTotal)
	}
	if !enriched.Time.Equal(when) || enriched.Title != "New plan" ||
		enriched.MarkerKey != dir+"/.airplan.json" {
		t.Fatalf("enriched record = %+v, want original identity", enriched)
	}

	// Backfill values match a fresh import of the same content.
	freshManifest := filepath.Join(t.TempDir(), "fresh.jsonl")
	fresh := newSyncClient(t, fake.server.URL, freshManifest)
	imported, err := fresh.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(imported.Added) != 1 {
		t.Fatalf("fresh import = %+v, %v", imported, err)
	}
	if imported.Added[0].Objects != enriched.Objects ||
		imported.Added[0].TotalBytes != enriched.TotalBytes {
		t.Fatalf("import totals = (%d, %d), enriched = (%d, %d)",
			imported.Added[0].Objects, imported.Added[0].TotalBytes,
			enriched.Objects, enriched.TotalBytes)
	}

	// Reduction collapses the enriched append onto the original record.
	records, _, err = ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	uploads := ManifestUploads(records)
	if len(records) != 2 || len(uploads) != 1 ||
		uploads[0].Objects != wantObjects {
		t.Fatalf("records = %+v, uploads = %+v", records, uploads)
	}

	// Idempotence: a second sync appends nothing.
	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(second.Enriched) != 0 || len(second.Added) != 0 ||
		second.Unchanged != 1 {
		t.Fatalf("second sync = %+v, %v", second, err)
	}
	after, _, err := ReadManifest(manifest)
	if err != nil || len(after) != 2 {
		t.Fatalf("second sync appended records: %+v, %v", after, err)
	}
}

func TestSyncBackfillLeavesNonCompleteMarkersUntouched(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	t.Run("incomplete", func(t *testing.T) {
		fake := newSyncStorage(t)
		dir := strings.Repeat("g", 26)
		marker := declaredDocumentMarker(dir, when)
		// Stored page is shorter than the declared page bytes.
		fake.addMarker(t, marker)
		fake.addObject(dir+"/new.html", []byte("short"), when)
		fake.addObject(dir+"/new.md", []byte("12345678"), when)

		manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
		if err := os.WriteFile(manifest,
			[]byte(backfillManifestLine(dir, when)), 0o600); err != nil {
			t.Fatal(err)
		}
		client := newSyncClient(t, fake.server.URL, manifest)
		result, err := client.SyncManifest(context.Background(),
			SyncManifestOptions{Prune: true})
		if err != nil || len(result.Enriched) != 0 {
			t.Fatalf("sync = %+v, %v", result, err)
		}
		records, _, err := ReadManifest(manifest)
		if err != nil || len(records) != 1 || records[0].Objects != 0 {
			t.Fatalf("records = %+v, %v", records, err)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		fake := newSyncStorage(t)
		dir := strings.Repeat("h", 26)
		fake.addObject(dir+"/"+MarkerFilename, []byte("{not json"), when)
		fake.addObject(dir+"/new.html", []byte("0123456789"), when)

		manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
		if err := os.WriteFile(manifest,
			[]byte(backfillManifestLine(dir, when)), 0o600); err != nil {
			t.Fatal(err)
		}
		client := newSyncClient(t, fake.server.URL, manifest)
		result, err := client.SyncManifest(context.Background(),
			SyncManifestOptions{Prune: true})
		if err != nil || len(result.Enriched) != 0 {
			t.Fatalf("sync = %+v, %v", result, err)
		}
		records, _, err := ReadManifest(manifest)
		if err != nil || len(records) != 1 || records[0].Objects != 0 {
			t.Fatalf("records = %+v, %v", records, err)
		}
	})
}

// TestSyncClassifiesDualMarkerConflictAsInvalid covers the conflict
// short-circuit in inspectListedUploadBody. A directory carrying both
// marker names grants no authority, so sync must classify it invalid
// without fetching either marker, and import nothing.
func TestSyncClassifiesDualMarkerConflictAsInvalid(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("q", 26)
	fake.addObject(dir+"/"+MarkerFilename, []byte("{}"), when)
	fake.addObject(dir+"/"+CollectionMarkerFilename, []byte("{}"), when)
	fake.addObject(dir+"/index.html", []byte("0123456789"), when)

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Invalid != 1 || len(result.Added) != 0 {
		t.Fatalf("result = %+v, want one invalid and no imports", result)
	}
	fake.mu.Lock()
	fetched := fake.gets[dir+"/"+MarkerFilename] +
		fake.gets[dir+"/"+CollectionMarkerFilename]
	fake.mu.Unlock()
	if fetched != 0 {
		t.Fatalf("marker fetches = %d; a conflict is decided from the "+
			"listing alone", fetched)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 0 {
		t.Fatalf("records = %+v, %v, want nothing imported", records, err)
	}
}

// TestSyncFailureForContextClassifiesJobs pins the wire-visible
// operation each job kind reports when the run is cancelled before it is
// scheduled. Enrich jobs carry a local record like prune jobs do, so
// without the explicit enrich check they would wrongly claim
// "confirm_absence" — asserting an absence the sync never probed.
func TestSyncFailureForContextClassifiesJobs(t *testing.T) {
	record := ManifestRecord{Key: "dir/plan.html"}
	upload := RemoteUpload{MarkerKey: "dir/" + MarkerFilename}
	for name, tc := range map[string]struct {
		job  syncJob
		want string
	}{
		"import": {syncJob{markerKey: "k", upload: &upload}, "fetch"},
		"prune":  {syncJob{markerKey: "k", local: &record}, "confirm_absence"},
		"enrich": {
			syncJob{
				markerKey: "k", upload: &upload, local: &record, enrich: true,
			},
			"fetch",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := syncFailureForContext(tc.job, context.Canceled)
			if got.Operation != tc.want {
				t.Fatalf("operation = %q, want %q", got.Operation, tc.want)
			}
			if got.MarkerKey != "k" || got.Error != context.Canceled.Error() {
				t.Fatalf("failure = %+v", got)
			}
		})
	}
}

// TestSyncBackfillSkipsVersionsThatCannotDeclareInventory pins that
// backfill eligibility is decided from local history (SPEC.md §9). A
// marker version that can never declare every counted object's size
// must cost no remote request at all, otherwise every sync would
// re-fetch the same markers forever and discard the result.
func TestSyncBackfillSkipsVersionsThatCannotDeclareInventory(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	// line builds an active record predating objects/total_bytes at the
	// given marker version, with or without a recorded source.
	line := func(dir string, version int, source bool) string {
		sourceKey := ""
		if source {
			sourceKey = `"source_key":"` + dir + `/new.md",`
		}
		return `{"type":"upload","time":"` + when.UTC().Format(time.RFC3339) +
			`","key":"` + dir + `/new.html",` + sourceKey +
			`"marker_key":"` + dir + `/.airplan.json",` +
			`"url":"https://plans.example.com/` + dir + `/new.html",` +
			`"bucket":"plans","profile":"work","format":"md",` +
			`"kind":"document","title":"New plan","bytes":10,` +
			`"marker_version":` + strconv.Itoa(version) + `}` + "\n"
	}

	for name, tc := range map[string]struct {
		version   int
		source    bool
		wantFetch bool
	}{
		"v1 declares no page bytes":      {1, false, false},
		"v1 with source":                 {1, true, false},
		"v2 with source lacks src bytes": {2, true, false},
		"v2 without source qualifies":    {2, false, true},
		"v3 always qualifies":            {MarkerVersion, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			fake := newSyncStorage(t)
			dir := strings.Repeat("k", 26)
			marker := declaredDocumentMarker(dir, when)
			fake.addUpload(t, marker, []byte("0123456789"))
			fake.addObject(dir+"/new.md", []byte("12345678"), when)

			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			if err := os.WriteFile(manifest,
				[]byte(line(dir, tc.version, tc.source)), 0o600); err != nil {
				t.Fatal(err)
			}
			client := newSyncClient(t, fake.server.URL, manifest)
			if _, err := client.SyncManifest(context.Background(),
				SyncManifestOptions{Prune: true}); err != nil {
				t.Fatal(err)
			}

			fake.mu.Lock()
			fetches := fake.gets[dir+"/"+MarkerFilename]
			fake.mu.Unlock()
			if tc.wantFetch && fetches == 0 {
				t.Fatal("eligible record must fetch its marker")
			}
			if !tc.wantFetch && fetches != 0 {
				t.Fatalf("marker fetched %d time(s); an ineligible version "+
					"must cost no remote request", fetches)
			}
		})
	}
}

// TestSyncBackfillFetchFailureReportsFetchOperation pins the wire-visible
// SyncFailure.Operation for a backfill job. syncFailureForContext
// distinguishes enrich jobs from prune jobs even though both carry a
// local record, so an enrich failure must report "fetch" and never
// "confirm_absence" — the marker was listed, so nothing about its
// absence was ever established.
func TestSyncBackfillFetchFailureReportsFetchOperation(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("j", 26)
	marker := declaredDocumentMarker(dir, when)
	fake.addUpload(t, marker, []byte("0123456789"))
	fake.addObject(dir+"/new.md", []byte("12345678"), when)
	// Listed, so it pairs with the active record and becomes an enrich
	// job, but its marker GET fails rather than 404s.
	fake.failGet[dir+"/"+MarkerFilename] = true

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.WriteFile(manifest,
		[]byte(backfillManifestLine(dir, when)), 0o600); err != nil {
		t.Fatal(err)
	}
	client := newSyncClient(t, fake.server.URL, manifest)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err == nil {
		t.Fatal("sync must report the fetch failure")
	}
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %+v, want exactly one", result.Failures)
	}
	if got := result.Failures[0].Operation; got != "fetch" {
		t.Fatalf("operation = %q, want \"fetch\"", got)
	}
	if len(result.Enriched) != 0 || len(result.Tombstoned) != 0 {
		t.Fatalf("result = %+v, want no manifest changes", result)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 || records[0].Objects != 0 {
		t.Fatalf("records = %+v, %v; record must stay untouched",
			records, err)
	}
}

// TestSyncDeclaredTotalsRequireFullyDeclaredMarkers pins the contract
// that objects/total_bytes are marker-declared, never storage-observed:
// markers that do not declare every counted object's size (v1 pages,
// v1/v2 sources) leave both fields absent on import and backfill.
func TestSyncDeclaredTotalsRequireFullyDeclaredMarkers(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		marker      UploadMarker
		source      []byte
		page        []byte
		wantCounts  bool
		wantObjects int
	}{
		{
			name: "v1 leaves counts absent",
			marker: UploadMarker{
				Schema: MarkerSchema, Version: 1, Directory: "",
				CreatedAt: when, Format: "html", Page: "new.html",
			},
			page: []byte("page"),
		},
		{
			name: "v2 with source leaves counts absent",
			marker: UploadMarker{
				Schema: MarkerSchema, Version: 2, Directory: "",
				CreatedAt: when, Format: "md", Page: "new.html",
				PageBytes: 10, Source: "new.md",
			},
			page:   []byte("0123456789"),
			source: []byte("# source"),
		},
		{
			name: "v2 without source records counts",
			marker: UploadMarker{
				Schema: MarkerSchema, Version: 2, Directory: "",
				CreatedAt: when, Format: "html", Page: "new.html",
				PageBytes: 9,
			},
			page:        []byte("123456789"),
			wantCounts:  true,
			wantObjects: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newSyncStorage(t)
			dir := strings.Repeat("j", 26)
			tt.marker.Directory = dir
			fake.addUpload(t, tt.marker, tt.page)
			if tt.source != nil {
				fake.addObject(dir+"/"+tt.marker.Source, tt.source, when)
			}

			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			client := newSyncClient(t, fake.server.URL, manifest)
			result, err := client.SyncManifest(context.Background(),
				SyncManifestOptions{Prune: true})
			if err != nil || len(result.Added) != 1 {
				t.Fatalf("sync = %+v, %v", result, err)
			}
			imported := result.Added[0]
			if !tt.wantCounts {
				if imported.Objects != 0 || imported.TotalBytes != 0 {
					t.Fatalf("imported counts = (%d, %d), want absent",
						imported.Objects, imported.TotalBytes)
				}
			} else {
				body, err := EncodeUploadMarker(tt.marker)
				if err != nil {
					t.Fatal(err)
				}
				wantTotal := int64(len(body)) + tt.marker.PageBytes
				if imported.Objects != tt.wantObjects ||
					imported.TotalBytes != wantTotal {
					t.Fatalf("imported counts = (%d, %d), want (%d, %d)",
						imported.Objects, imported.TotalBytes,
						tt.wantObjects, wantTotal)
				}
			}

			// Backfill obeys the same rule: an active record missing
			// both counts stays untouched under an underdeclared
			// marker instead of gaining observed sizes.
			backfillManifest := filepath.Join(t.TempDir(), "backfill.jsonl")
			if err := os.WriteFile(backfillManifest,
				[]byte(backfillManifestLine(dir, when)),
				0o600); err != nil {
				t.Fatal(err)
			}
			backfillClient := newSyncClient(
				t, fake.server.URL, backfillManifest,
			)
			backfilled, err := backfillClient.SyncManifest(
				context.Background(), SyncManifestOptions{Prune: true},
			)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantCounts {
				if len(backfilled.Enriched) != 1 {
					t.Fatalf("backfill = %+v, want one enrichment",
						backfilled)
				}
				return
			}
			if len(backfilled.Enriched) != 0 {
				t.Fatalf("backfill = %+v, want none", backfilled)
			}
			records, _, err := ReadManifest(backfillManifest)
			if err != nil || len(records) != 1 || records[0].Objects != 0 {
				t.Fatalf("records = %+v, %v", records, err)
			}
		})
	}
}

func TestSyncCommitNeverResurrectsTombstonedIdentity(t *testing.T) {
	dir := strings.Repeat("i", 26)
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	tombstone := `{"type":"delete","time":"2026-07-22T09:00:00Z",` +
		`"key":"` + dir + `/new.html",` +
		`"marker_key":"` + dir + `/.airplan.json",` +
		`"bucket":"plans","profile":"work","reason":"deleted"}` + "\n"
	contents := backfillManifestLine(dir, when) + tombstone
	if err := os.WriteFile(manifest, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	client := newSyncClient(t, "http://127.0.0.1:0", manifest)
	enriched := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/new.html",
		MarkerKey: dir + "/.airplan.json",
		URL:       "https://plans.example.com/" + dir + "/new.html",
		Bucket:    "plans", Profile: "work", Kind: "document",
		Bytes: 10, Objects: 3, TotalBytes: 300, MarkerVersion: 3,
	}
	// Simulate an enrichment planned before a concurrent tombstone: the
	// commit path must never resurrect the now-inactive identity.
	result := &SyncManifestResult{Enriched: []ManifestRecord{enriched}}
	if err := client.commitSyncManifest(
		context.Background(), manifest, 2, result,
	); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enriched = %+v, want none", result.Enriched)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(before) {
		t.Fatalf("manifest changed: %q -> %q, %v", before, after, err)
	}
}

type syncStorage struct {
	server      *httptest.Server
	mu          sync.Mutex
	objects     map[string][]byte
	modified    map[string]time.Time
	hidden      map[string]bool
	failGet     map[string]bool
	gets        map[string]int
	delay       time.Duration
	inFlight    int
	maxInFlight int
}

func newSyncStorage(t *testing.T) *syncStorage {
	t.Helper()
	fake := &syncStorage{
		objects: make(map[string][]byte), modified: make(map[string]time.Time),
		hidden: make(map[string]bool), failGet: make(map[string]bool),
		gets: make(map[string]int),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *syncStorage) addUpload(
	t *testing.T, marker UploadMarker, page []byte,
) {
	t.Helper()
	f.addMarker(t, marker)
	pageName := marker.Page
	if marker.Version == MarkerVersion {
		for _, object := range marker.Objects {
			if object.Role == MarkerRolePage {
				pageName = object.Name
			}
		}
	}
	f.addObject(marker.Directory+"/"+pageName, page, marker.CreatedAt)
}

func (f *syncStorage) addMarker(t *testing.T, marker UploadMarker) {
	t.Helper()
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	markerName := MarkerFilename
	if marker.Version == MarkerVersion {
		markerName, _ = MarkerFilenameForKind(marker.Kind)
	}
	f.addObject(marker.Directory+"/"+markerName, body, marker.CreatedAt)
}

func (f *syncStorage) addObject(key string, body []byte, modified time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = append([]byte(nil), body...)
	f.modified[key] = modified
	delete(f.hidden, key)
}

func (f *syncStorage) removeMarker(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	delete(f.modified, key)
}

func (f *syncStorage) hideMarker(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hidden[key] = true
}

func (f *syncStorage) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("list-type") == "2" {
		f.handleList(w, r)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/plans/")
	f.mu.Lock()
	f.gets[key]++
	serverError := f.failGet[key]
	f.mu.Unlock()
	if serverError {
		// A transport failure, distinct from the NoSuchKey below: absence
		// is confirmable, a 500 is not.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	body, ok := f.objects[key]
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
	if !ok {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w,
			`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		return
	}
	_, _ = w.Write(body)
}

func (f *syncStorage) handleList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	f.mu.Lock()
	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) && !f.hidden[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	type listed struct {
		key      string
		size     int
		modified time.Time
	}
	items := make([]listed, 0, len(keys))
	for _, key := range keys {
		items = append(items, listed{key, len(f.objects[key]), f.modified[key]})
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
		`<IsTruncated>false</IsTruncated>`)
	for _, item := range items {
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>"+
			"<LastModified>%s</LastModified></Contents>",
			item.key, item.size, item.modified.Format(time.RFC3339))
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}

func newSyncClient(t *testing.T, endpoint, manifest string) *Client {
	t.Helper()
	client, err := New(context.Background(), &Config{
		Endpoint: endpoint, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, Profile: "work", Timeout: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
