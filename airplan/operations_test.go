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
