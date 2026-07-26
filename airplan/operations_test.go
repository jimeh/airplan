package airplan

import (
	"context"
	"testing"
	"time"
)

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
