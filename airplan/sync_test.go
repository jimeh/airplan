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
	conflictDir := strings.Repeat("v", 26)
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
	fake.addMarker(t, UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: conflictDir,
		CreatedAt: when, Kind: UploadKindDocument, Slug: "plan", Format: "html",
		Objects: []MarkerObject{{
			Name: "plan.html", Role: MarkerRolePage,
			Bytes: 4, ContentType: pageContentType,
		}},
	})
	fake.addMarker(t, UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: conflictDir,
		CreatedAt: when, Kind: UploadKindCollection,
		Objects: []MarkerObject{{
			Name: "index.html", Role: MarkerRolePage,
			Bytes: 4, ContentType: pageContentType,
		}, {
			Name: "file.txt", Role: MarkerRoleFile,
			Bytes: 1, ContentType: "text/plain",
		}},
	})
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	for _, record := range []ManifestRecord{
		{
			Type: "upload", Time: when, Key: invalidDir + "/plan.html",
			MarkerKey: invalidDir + "/" + MarkerFilename,
			URL:       "https://plans.example.com/" + invalidDir + "/plan.html",
			Bucket:    "plans", Profile: "work", Bytes: 1, MarkerVersion: 1,
		},
		{
			Type: "upload", Time: when, Key: conflictDir + "/plan.html",
			MarkerKey: conflictDir + "/" + MarkerFilename,
			URL:       "https://plans.example.com/" + conflictDir + "/plan.html",
			Bucket:    "plans", Profile: "work", Bytes: 4,
			MarkerVersion: MarkerVersion,
		},
		{
			Type: "upload", Time: when, Key: incompleteDir + "/plan.html",
			MarkerKey: incompleteDir + "/" + MarkerFilename,
			URL:       "https://plans.example.com/" + incompleteDir + "/plan.html",
			Bucket:    "plans", Profile: "work", Bytes: 5,
			MarkerVersion: MarkerVersion,
		},
	} {
		if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
			t.Fatal(err)
		}
	}
	client := newSyncClient(t, fake.server.URL, manifest)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || result.Invalid != 2 || result.Incomplete != 1 ||
		len(result.Added) != 0 || len(result.Enriched) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 3 {
		t.Fatalf("manifest changed: records=%+v warnings=%v error=%v",
			records, warnings, err)
	}
}

func TestSyncManifestEnrichesLegacyTotalsAndIsIdempotent(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("q", 26)
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: when, Kind: UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: 4, ContentType: pageContentType},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: 6, ContentType: sourceContentType},
		},
	}
	fake.addUpload(t, marker, []byte("page"))
	fake.addObject(dir+"/plan.md", []byte("source"), when)
	fake.addObject(dir+"/unrecognized.tmp", []byte("extra"), when)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	initial := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/plan.html",
		SourceKey: dir + "/plan.md", MarkerKey: dir + "/" + MarkerFilename,
		URL:    "https://plans.example.com/" + dir + "/plan.html",
		Bucket: "plans", Profile: "work", Format: "md", Kind: "document",
		Slug: "plan", Bytes: 4, MarkerVersion: MarkerVersion,
	}
	if err := appendManifestRecord(context.Background(), manifest, initial); err != nil {
		t.Fatal(err)
	}
	client := newSyncClient(t, fake.server.URL, manifest)
	dryRun, err := client.SyncManifest(context.Background(), SyncManifestOptions{DryRun: true})
	if err != nil || len(dryRun.Enriched) != 1 {
		t.Fatalf("dry run = %+v, %v", dryRun, err)
	}
	fake.mu.Lock()
	markerBytes := len(fake.objects[initial.MarkerKey])
	fake.mu.Unlock()
	wantTotal := int64(markerBytes + len("page") + len("source"))
	if got := dryRun.Enriched[0]; got.Time != when || got.Objects != 3 ||
		got.TotalBytes != wantTotal || got.Bytes != 4 {
		t.Fatalf("dry-run enriched = %+v, want objects=3 total_bytes=%d", got, wantTotal)
	}
	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 {
		t.Fatalf("dry run wrote manifest: records=%+v error=%v", records, err)
	}

	applied, err := client.SyncManifest(context.Background(), SyncManifestOptions{})
	if err != nil || len(applied.Enriched) != 1 || len(applied.Added) != 0 {
		t.Fatalf("applied = %+v, %v", applied, err)
	}
	records, _, err = ReadManifest(manifest)
	active := ActiveUploads(records)
	if err != nil || len(records) != 2 || len(active) != 1 ||
		active[0].Objects != 3 || active[0].TotalBytes != wantTotal ||
		active[0].Bytes != 4 || active[0].Time != when {
		t.Fatalf("records = %+v, active = %+v, error = %v", records, active, err)
	}
	second, err := client.SyncManifest(context.Background(), SyncManifestOptions{})
	if err != nil || len(second.Enriched) != 0 || second.Unchanged != 1 {
		t.Fatalf("second sync = %+v, %v", second, err)
	}
}

func TestSyncManifestDoesNotEnrichPartialTotals(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("p", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	record := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/plan.html",
		MarkerKey: dir + "/" + MarkerFilename,
		URL:       "https://plans.example.com/" + dir + "/plan.html",
		Bucket:    "plans", Profile: "work", Format: "html", Kind: "document",
		Bytes: 4, Objects: 2, MarkerVersion: 1,
	}
	if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
		t.Fatal(err)
	}
	result, err := newSyncClient(t, fake.server.URL, manifest).SyncManifest(
		context.Background(), SyncManifestOptions{},
	)
	if err != nil || len(result.Enriched) != 0 || result.Unchanged != 1 {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestSyncManifestDoesNotEnrichTotalBytesOnly(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("o", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	record := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/plan.html",
		MarkerKey: dir + "/" + MarkerFilename,
		URL:       "https://plans.example.com/" + dir + "/plan.html",
		Bucket:    "plans", Profile: "work", Format: "html", Kind: "document",
		Bytes: 4, TotalBytes: 100, MarkerVersion: 1,
	}
	if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
		t.Fatal(err)
	}
	result, err := newSyncClient(t, fake.server.URL, manifest).SyncManifest(
		context.Background(), SyncManifestOptions{},
	)
	if err != nil || len(result.Enriched) != 0 || result.Unchanged != 1 {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestCommitSyncManifestDoesNotResurrectTombstonedEnrichment(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	dir := strings.Repeat("n", 26)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	record := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/plan.html",
		MarkerKey: dir + "/" + MarkerFilename,
		URL:       "https://plans.example.com/" + dir + "/plan.html",
		Bucket:    "plans", Profile: "work", Bytes: 4, MarkerVersion: 1,
	}
	if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
		t.Fatal(err)
	}
	initial, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	tombstone := ManifestRecord{
		Type: "delete", Time: when.Add(time.Minute), Key: record.Key,
		MarkerKey: record.MarkerKey, Bucket: record.Bucket, Profile: record.Profile,
	}
	if err := appendManifestRecord(context.Background(), manifest, tombstone); err != nil {
		t.Fatal(err)
	}
	planned := record
	planned.Objects, planned.TotalBytes = 2, 100
	result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
	client := &Client{cfg: &Config{Bucket: "plans", Profile: "work"}}
	if err := client.commitSyncManifest(context.Background(), manifest, len(initial), result); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 0 {
		t.Fatalf("enrichment resurrected tombstone: %+v", result.Enriched)
	}
	records, _, _ := ReadManifest(manifest)
	if active := ActiveUploads(records); len(active) != 0 {
		t.Fatalf("active records = %+v", active)
	}
}

func TestCommitSyncManifestEnrichmentPreservesConcurrentMetadata(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	dir := strings.Repeat("m", 26)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	record := ManifestRecord{
		Type: "upload", Time: when, Key: dir + "/plan.html",
		MarkerKey: dir + "/" + MarkerFilename,
		URL:       "https://plans.example.com/" + dir + "/plan.html",
		Bucket:    "plans", Profile: "work", Title: "old", Bytes: 4,
		MarkerVersion: 1,
	}
	if err := appendManifestRecord(context.Background(), manifest, record); err != nil {
		t.Fatal(err)
	}
	initial, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	current := record
	current.Time = when.Add(time.Minute)
	current.Title = "concurrent title"
	current.Repo = "https://github.com/acme/repo"
	if err := appendManifestRecord(context.Background(), manifest, current); err != nil {
		t.Fatal(err)
	}
	planned := record
	planned.Objects, planned.TotalBytes = 2, 100
	result := &SyncManifestResult{Enriched: []ManifestRecord{planned}}
	client := &Client{cfg: &Config{Bucket: "plans", Profile: "work"}}
	if err := client.commitSyncManifest(context.Background(), manifest,
		len(initial), result); err != nil {
		t.Fatal(err)
	}
	if len(result.Enriched) != 1 || result.Enriched[0].Title != current.Title ||
		result.Enriched[0].Repo != current.Repo ||
		result.Enriched[0].Time != current.Time {
		t.Fatalf("enriched = %+v", result.Enriched)
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
		cap         int
		serial      bool
	}{
		{name: "serial", concurrency: 1, cap: 1, serial: true},
		{name: "default", concurrency: 0, cap: DefaultRemoteConcurrency},
		{name: "larger override", concurrency: 12, cap: 12},
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
			if test.serial && fake.maxInFlight != 1 {
				t.Fatalf("max in flight = %d, want exactly 1", fake.maxInFlight)
			}
			if !test.serial &&
				(fake.maxInFlight <= 1 || fake.maxInFlight > test.cap) {
				t.Fatalf("max in flight = %d, want > 1 and <= %d",
					fake.maxInFlight, test.cap)
			}
		})
	}
}

type syncStorage struct {
	server      *httptest.Server
	mu          sync.Mutex
	objects     map[string][]byte
	modified    map[string]time.Time
	hidden      map[string]bool
	delay       time.Duration
	inFlight    int
	maxInFlight int
}

func newSyncStorage(t *testing.T) *syncStorage {
	t.Helper()
	fake := &syncStorage{
		objects: make(map[string][]byte), modified: make(map[string]time.Time),
		hidden: make(map[string]bool),
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
