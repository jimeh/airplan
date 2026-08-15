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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
		Schema: MarkerSchema, Version: 3, Directory: dirV2,
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
		Schema: MarkerSchema, Version: 3, Directory: dirV2,
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
		Schema: MarkerSchema, Version: 3, Directory: incompleteDir,
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

func TestSyncManifestReconcilesProtectionBothWays(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("p", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	sentinel, err := encodeProtectionSentinel(when, "keep")
	if err != nil {
		t.Fatal(err)
	}
	fake.addObject(dir+"/"+ProtectedFilename, sentinel, when)

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)

	// Import: a protected remote directory yields upload + protect records.
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 1 || len(result.Protection) != 1 ||
		result.Protection[0].Type != "protect" ||
		result.Protection[0].ProtectReason != "keep" ||
		!result.Protection[0].Time.Equal(when) {
		t.Fatalf("import result = %+v", result)
	}
	active := mustActiveUploads(t, manifest)
	if len(active) != 1 || !active[0].Protected ||
		active[0].ProtectReason != "keep" {
		t.Fatalf("active = %+v", active)
	}

	// Converged: no further protection records.
	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(second.Protection) != 0 || second.Unchanged != 1 {
		t.Fatalf("second sync = %+v, %v", second, err)
	}

	// Remote unprotected while locally protected appends unprotect.
	fake.removeMarker(dir + "/" + ProtectedFilename)
	third, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(third.Protection) != 1 ||
		third.Protection[0].Type != "unprotect" {
		t.Fatalf("third sync = %+v, %v", third, err)
	}
	active = mustActiveUploads(t, manifest)
	if len(active) != 1 || active[0].Protected {
		t.Fatalf("active = %+v", active)
	}

	// Remote protected while locally unprotected appends protect. The
	// already-active path is presence-based, so no reason is recorded.
	fake.addObject(dir+"/"+ProtectedFilename, sentinel, when)
	fourth, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(fourth.Protection) != 1 ||
		fourth.Protection[0].Type != "protect" ||
		fourth.Protection[0].ProtectReason != "" {
		t.Fatalf("fourth sync = %+v, %v", fourth, err)
	}
	active = mustActiveUploads(t, manifest)
	if len(active) != 1 || !active[0].Protected {
		t.Fatalf("active = %+v", active)
	}
}

// TestSyncManifestReconcilesCollectionProtection covers the collection path of
// protection reconciliation. Sync-generated protection records carry the
// collection marker key but no explicit kind, so this asserts the reduced
// record round trips without warnings — mustActiveUploads fails on any — and
// that its identity resolves to the collection marker rather than the document
// one (SPEC.md §9).
func TestSyncManifestReconcilesCollectionProtection(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("c", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: when, Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 8,
				ContentType: pageContentType,
			},
			{
				Name: "shot.png", Role: MarkerRoleFile, Bytes: 3,
				ContentType: "image/png",
			},
		},
	}, []byte("overview"))
	fake.addObject(dir+"/shot.png", []byte("png"), when)
	sentinel, err := encodeProtectionSentinel(when, "keep")
	if err != nil {
		t.Fatal(err)
	}
	fake.addObject(dir+"/"+ProtectedFilename, sentinel, when)

	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)

	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 1 || len(result.Protection) != 1 ||
		result.Protection[0].Type != "protect" {
		t.Fatalf("import result = %+v", result)
	}
	markerKey := dir + "/" + CollectionMarkerFilename
	if result.Protection[0].MarkerKey != markerKey {
		t.Fatalf("protection marker key = %q, want %q",
			result.Protection[0].MarkerKey, markerKey)
	}
	active := mustActiveUploads(t, manifest)
	if len(active) != 1 || !active[0].Protected ||
		active[0].Kind != string(UploadKindCollection) {
		t.Fatalf("active = %+v", active)
	}

	// The unprotect direction must round trip cleanly for collections too.
	fake.removeMarker(dir + "/" + ProtectedFilename)
	second, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{Prune: true})
	if err != nil || len(second.Protection) != 1 ||
		second.Protection[0].Type != "unprotect" ||
		second.Protection[0].MarkerKey != markerKey {
		t.Fatalf("second sync = %+v, %v", second, err)
	}
	active = mustActiveUploads(t, manifest)
	if len(active) != 1 || active[0].Protected {
		t.Fatalf("active = %+v", active)
	}
}

func TestSyncManifestProtectionDryRunWritesNothing(t *testing.T) {
	when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	fake := newSyncStorage(t)
	dir := strings.Repeat("q", 26)
	fake.addUpload(t, UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: when, Format: "html", Page: "plan.html",
	}, []byte("page"))
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := newSyncClient(t, fake.server.URL, manifest)
	if _, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	fake.addObject(dir+"/"+ProtectedFilename, nil, when)
	result, err := client.SyncManifest(context.Background(),
		SyncManifestOptions{DryRun: true})
	if err != nil || len(result.Protection) != 1 ||
		result.Protection[0].Type != "protect" {
		t.Fatalf("dry run = %+v, %v", result, err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(before) {
		t.Fatalf("manifest changed during dry run: %v", err)
	}
}

func TestCommitSyncManifestSkipsConcurrentlyReconciledProtection(t *testing.T) {
	for _, tt := range []struct {
		name string
		// planned is the stale protection record built before a concurrent
		// protect record landed in the manifest.
		planned string
	}{
		// Same direction: the concurrent record makes the plan redundant.
		{name: "duplicate protect", planned: "protect"},
		// Opposite direction: the plan came from a snapshot taken before
		// the concurrent protect, so its disagreement is stale and must
		// not overwrite the fresher local record.
		{name: "stale unprotect", planned: "unprotect"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := strings.Repeat("s", 26)
			markerKey := dir + "/" + MarkerFilename
			pageKey := dir + "/plan.html"
			manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
			when := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
			upload := ManifestRecord{
				Type: "upload", Time: when, Key: pageKey, MarkerKey: markerKey,
				URL:    "https://plans.example.com/" + pageKey,
				Bucket: "plans", Profile: "work", Format: "html", Bytes: 4,
				MarkerVersion: MarkerVersion,
			}
			if err := appendManifestRecord(
				context.Background(), manifest, upload,
			); err != nil {
				t.Fatal(err)
			}
			initialRecords, _, err := ReadManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}

			// A concurrent protect lands between planning and commit.
			if err := appendManifestRecord(context.Background(), manifest,
				ManifestRecord{
					Type: "protect", Time: when.Add(time.Minute), Key: pageKey,
					MarkerKey: markerKey, Bucket: "plans", Profile: "work",
				}); err != nil {
				t.Fatal(err)
			}
			result := &SyncManifestResult{Protection: []ManifestRecord{{
				Type: tt.planned, Time: when.Add(2 * time.Minute),
				Key: pageKey, MarkerKey: markerKey,
				Bucket: "plans", Profile: "work",
			}}}
			client := &Client{cfg: &Config{Bucket: "plans", Profile: "work"}}
			if err := client.commitSyncManifest(context.Background(), manifest,
				len(initialRecords), result); err != nil {
				t.Fatal(err)
			}
			if len(result.Protection) != 0 {
				t.Fatalf("stale protection appended = %+v", result.Protection)
			}
			records, _, err := ReadManifest(manifest)
			if err != nil || len(records) != 2 {
				t.Fatalf("records = %+v, error = %v", records, err)
			}
			active := ActiveUploads(records)
			if len(active) != 1 || !active[0].Protected {
				t.Fatalf("active = %+v, want protected upload", active)
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
	gets        map[string]int
	failGets    map[string]bool
}

// failGet makes fetches of key fail with a server error, so tests can cover a
// transient storage failure on one object.
func (f *syncStorage) failGet(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGets == nil {
		f.failGets = make(map[string]bool)
	}
	f.failGets[key] = true
}

// getCount reports how many times a key's body has been fetched, so tests can
// assert that sync does not re-fetch markers it cannot learn anything from.
func (f *syncStorage) getCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[key]
}

func newSyncStorage(t *testing.T) *syncStorage {
	t.Helper()
	fake := &syncStorage{
		objects: make(map[string][]byte), modified: make(map[string]time.Time),
		hidden: make(map[string]bool), gets: make(map[string]int),
		failGets: make(map[string]bool),
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
	if marker.Version >= 3 {
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
	if marker.Version >= 3 {
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
	// PUT support lets one fixture hold a real upload and then be reconciled
	// by sync, so declared totals can be compared across both writers.
	if r.Method == http.MethodPut {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.addObject(strings.TrimPrefix(r.URL.Path, "/plans/"), body,
			time.Now().UTC().Truncate(time.Second))
		w.WriteHeader(http.StatusOK)
		return
	}
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
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	body, ok := f.objects[key]
	failed := f.failGets[key]
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	f.mu.Lock()
	f.inFlight--
	f.mu.Unlock()
	if failed {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w,
			`<Error><Code>InternalError</Code><Message>boom</Message></Error>`)
		return
	}
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
	if err := client.ensureStorage(context.Background()); err != nil {
		t.Fatal(err)
	}
	options := client.st.client.Options()
	options.Retryer = aws.NopRetryer{}
	client.st.client = s3.New(options)
	return client
}
