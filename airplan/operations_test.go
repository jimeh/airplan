package airplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListManifestHistoryOrdersByTimeThenMarkerKey(t *testing.T) {
	dirA := strings.Repeat("a", 26)
	dirB := strings.Repeat("b", 26)
	dirC := strings.Repeat("c", 26)
	record := func(dir, timestamp string) string {
		return `{"type":"upload","time":"` + timestamp + `",` +
			`"key":"` + dir + `/plan.html",` +
			`"marker_key":"` + dir + `/.airplan.json",` +
			`"url":"https://plans.example.com/` + dir + `/plan.html",` +
			`"bucket":"plans","kind":"document","bytes":10,` +
			`"marker_version":3}`
	}
	// Sync-style file order: the newest record first, then older
	// imports appended in marker-key order, plus a marker-key tie at
	// the oldest time appended in reverse key order.
	manifest := strings.Join([]string{
		record(dirC, "2026-07-08T16:00:00Z"),
		record(dirB, "2026-07-08T14:00:00Z"),
		record(dirA, "2026-07-08T14:00:00Z"),
	}, "\n") + "\n"

	path := filepath.Join(t.TempDir(), "manifest.jsonl")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	listed, err := ListManifestHistory(path, nil)
	if err != nil {
		t.Fatalf("ListManifestHistory: %v", err)
	}
	if len(listed.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", listed.Warnings)
	}
	got := make([]string, 0, len(listed.Records))
	for _, rec := range listed.Records {
		got = append(got, rec.MarkerKey)
	}
	want := []string{
		dirA + "/.airplan.json",
		dirB + "/.airplan.json",
		dirC + "/.airplan.json",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
	first := listed.Records[0].Time
	if !first.Equal(time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)) {
		t.Fatalf("first record time = %v", first)
	}
}
