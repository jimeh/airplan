package airplan

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPurgeReconcilesStaleManifestWhenOwnershipMarkerIsGone(t *testing.T) {
	store := newUpgradeStore(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)
	uploaded, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Gone\n"), Name: "gone.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	for key := range store.objects {
		if strings.HasPrefix(key, uploaded.ID+"/") {
			delete(store.objects, key)
			delete(store.etags, key)
		}
	}
	store.mu.Unlock()
	reservationKey := uploaded.ID + "/" + VersionsFilename
	store.set(reservationKey, standaloneDeleteReservationBody)

	result, err := client.Purge(context.Background(), PurgeRequest{
		UploadIDs: []string{uploaded.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Deleted == nil ||
		!strings.Contains(strings.Join(result.Items[0].Deleted.Warnings, "\n"),
			"recording the completed deletion") {
		t.Fatalf("purge result = %+v", result)
	}
	listed, err := ListManifestHistory(manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("active manifest records = %+v", listed.Records)
	}
	if body, ok := store.get(reservationKey); !ok ||
		!bytes.Equal(body, standaloneDeleteReservationBody) {
		t.Fatal("missing-marker reconciliation disturbed deletion tombstone")
	}
}

func TestPurgeStandaloneGuardSurvivesInspectDeleteRace(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	reached := make(chan struct{})
	release := make(chan struct{})
	store.mu.Lock()
	store.pauseListPrefix = first.ID + "/"
	store.pauseListAttempt = 2
	store.pauseListReached = reached
	store.pauseListRelease = release
	store.mu.Unlock()

	purgeDone := make(chan struct {
		result *PurgeResult
		err    error
	}, 1)
	go func() {
		result, purgeErr := client.Purge(context.Background(), PurgeRequest{
			UploadIDs: []string{first.ID},
		})
		purgeDone <- struct {
			result *PurgeResult
			err    error
		}{result, purgeErr}
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("purge did not pause after stale standalone marker read")
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case outcome := <-purgeDone:
		if outcome.err != nil || len(outcome.result.Items) != 1 ||
			!outcome.result.Items[0].Versioned || outcome.result.Items[0].Deleted != nil {
			t.Fatalf("purge race result = %+v, %v", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("purge did not finish")
	}
	inspection, err := client.InspectUpload(context.Background(), second.URL)
	if err != nil || inspection.Revision != 2 || inspection.LatestRevision != 2 {
		t.Fatalf("surviving chain = %+v, %v", inspection, err)
	}
}

func TestPurgeStandaloneVersionGuardDoesNotReadPayloadBodies(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	assetBody := []byte("asset")
	uploaded, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Plan\n"), Path: "plan.md",
		},
		Assets: []AssetInput{{
			Reader: bytes.NewReader(assetBody), Path: "asset.bin",
			Size: int64(len(assetBody)),
		}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	payloadKeys := []string{uploaded.Key, uploaded.SourceKey, uploaded.Assets[0].Key}
	store.mu.Lock()
	before := make(map[string]int, len(payloadKeys))
	for _, key := range payloadKeys {
		before[key] = store.getKeyAttempts[key]
	}
	store.mu.Unlock()

	result, err := client.Purge(context.Background(), PurgeRequest{
		UploadIDs: []string{uploaded.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Deleted == nil {
		t.Fatalf("purge result = %+v", result)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, key := range payloadKeys {
		if got := store.getKeyAttempts[key]; got != before[key] {
			t.Errorf("payload GETs for %q = %d -> %d", key, before[key], got)
		}
	}
}

type unorderedListTransport struct {
	operationTransport
	list *ManifestList
}

func (t *unorderedListTransport) ListManifest(
	context.Context, ListManifestOptions,
) (*ManifestList, error) {
	if t.list == nil {
		return nil, errors.New("missing fixture")
	}
	return t.list, nil
}

// writeUnorderedManifest appends three managed uploads whose file order
// disagrees with their record time, as sync does when it imports remote
// uploads into existing local history.
func writeUnorderedManifest(t *testing.T) string {
	t.Helper()

	path := t.TempDir() + "/manifest.jsonl"
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(base.Add(2*time.Hour), "work", "plans", "", "c"),
		manifestUploadRecord(base, "work", "plans", "", "a"),
		manifestUploadRecord(base.Add(time.Hour), "work", "plans", "", "b"),
	} {
		if err := appendManifestRecord(
			context.Background(), path, record,
		); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func assertRecordTitles(
	t *testing.T, records []ManifestRecord, want ...string,
) {
	t.Helper()

	if len(records) != len(want) {
		t.Fatalf("records = %+v, want %d records", records, len(want))
	}
	for index, title := range want {
		if records[index].Title != title {
			got := make([]string, 0, len(records))
			for _, record := range records {
				got = append(got, record.Title)
			}
			t.Fatalf("titles = %q, want %q", got, want)
		}
	}
}

func TestListManifestHistoryOrdersByRecordTime(t *testing.T) {
	path := writeUnorderedManifest(t)

	listed, err := ListManifestHistory(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertRecordTitles(t, listed.Records, "a", "b", "c")
}

func TestListManifestSortsRemoteTransportBeforeLimit(t *testing.T) {
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	client := &Client{
		cfg: &Config{Backend: BackendAirplan},
		remote: &unorderedListTransport{list: &ManifestList{Records: []ManifestRecord{
			manifestUploadRecord(base.Add(2*time.Hour), "work", "plans", "", "c"),
			manifestUploadRecord(base, "work", "plans", "", "a"),
			manifestUploadRecord(base.Add(time.Hour), "work", "plans", "", "b"),
		}}},
	}
	listed, err := client.ListManifest(context.Background(),
		ListManifestOptions{Scope: ManifestScopeService})
	if err != nil {
		t.Fatal(err)
	}
	filtered := (ListFilter{Limit: intPointer(2)}).FilterManifestRecords(listed.Records)
	assertRecordTitles(t, filtered, "b", "c")
}

func intPointer(value int) *int { return &value }

// TestListManifestServiceScopeOrdersByRecordTime covers the scope used by the
// HTTP API and MCP servers, which reduce the manifest independently of
// ListManifestHistory.
func TestListManifestServiceScopeOrdersByRecordTime(t *testing.T) {
	path := writeUnorderedManifest(t)
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Bucket: "plans", Profile: "work",
		ManifestPath: path, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := client.ListManifest(context.Background(),
		ListManifestOptions{Scope: ManifestScopeService})
	if err != nil {
		t.Fatal(err)
	}
	assertRecordTitles(t, listed.Records, "a", "b", "c")
}

// TestListManifestOrdersEqualTimesByMarkerKey pins the tie-break so equal
// record times still produce one deterministic order in every listing path.
func TestListManifestOrdersEqualTimesByMarkerKey(t *testing.T) {
	path := t.TempDir() + "/manifest.jsonl"
	when := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(when, "work", "plans", "", "c"),
		manifestUploadRecord(when, "work", "plans", "", "a"),
		manifestUploadRecord(when, "work", "plans", "", "b"),
	} {
		if err := appendManifestRecord(
			context.Background(), path, record,
		); err != nil {
			t.Fatal(err)
		}
	}
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Bucket: "plans", Profile: "work",
		ManifestPath: path, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	history, err := ListManifestHistory(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertRecordTitles(t, history.Records, "a", "b", "c")

	scoped, err := client.ListManifest(context.Background(),
		ListManifestOptions{Scope: ManifestScopeService})
	if err != nil {
		t.Fatal(err)
	}
	assertRecordTitles(t, scoped.Records, "a", "b", "c")
}

// TestPlanPurgeOrdersCandidatesByRecordTime pins purge candidate order, which
// follows manifest listing order (SPEC.md §9).
func TestPlanPurgeOrdersCandidatesByRecordTime(t *testing.T) {
	path := writeUnorderedManifest(t)
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Bucket: "plans", Profile: "work",
		ManifestPath: path, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	records := make([]ManifestRecord, 0, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		records = append(records, candidate.Record)
	}
	assertRecordTitles(t, records, "a", "b", "c")
}
