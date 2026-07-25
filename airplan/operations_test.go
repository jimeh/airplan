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

func TestListFilterManifest(t *testing.T) {
	threshold := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	records := []ManifestRecord{
		{
			Type: "upload", Time: threshold.Add(-time.Hour),
			Key: "a/plan.html", Kind: "document", Slug: "plan",
		},
		{
			Type: "upload", Time: threshold,
			Key: "b/index.html", Kind: "collection",
		},
		{
			Type: "upload", Time: threshold.Add(time.Hour),
			Key: "c/notes.html", Kind: "document", Slug: "notes",
		},
	}
	keys := func(records []ManifestRecord) string {
		out := make([]string, 0, len(records))
		for _, rec := range records {
			out = append(out, rec.Key)
		}
		return strings.Join(out, ",")
	}

	tests := []struct {
		name   string
		filter ListFilter
		want   string
	}{
		{"empty keeps all", ListFilter{}, "a/plan.html,b/index.html,c/notes.html"},
		{
			// The threshold itself is included: time >= newer-than.
			"newer-than inclusive boundary",
			ListFilter{NewerThan: threshold},
			"b/index.html,c/notes.html",
		},
		{
			// The threshold itself is excluded: time < older-than,
			// matching the purge --older-than boundary.
			"older-than exclusive boundary",
			ListFilter{OlderThan: threshold},
			"a/plan.html",
		},
		{
			"kind document",
			ListFilter{Kind: UploadKindDocument},
			"a/plan.html,c/notes.html",
		},
		{
			"kind collection",
			ListFilter{Kind: UploadKindCollection},
			"b/index.html",
		},
		{
			// Collections are excluded even from a wildcard slug.
			"slug wildcard excludes collections",
			ListFilter{Slug: "*"},
			"a/plan.html,c/notes.html",
		},
		{"slug glob", ListFilter{Slug: "pl*"}, "a/plan.html"},
		{
			"limit keeps most recent ascending",
			ListFilter{Limit: 2},
			"b/index.html,c/notes.html",
		},
		{
			"limit larger than the set",
			ListFilter{Limit: 10},
			"a/plan.html,b/index.html,c/notes.html",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.filter.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			got := keys(tt.filter.FilterManifest(records))
			if got != tt.want {
				t.Fatalf("FilterManifest = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListFilterRemote(t *testing.T) {
	threshold := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	uploads := []RemoteUpload{
		{
			Dir: "docdir", Kind: UploadKindDocument, Slug: "plan",
			LastModified: threshold.Add(-time.Hour),
		},
		{
			Dir: "coldir", Kind: UploadKindCollection,
			LastModified: threshold,
		},
		{Dir: "confdir", Conflict: true, LastModified: threshold},
	}
	dirs := func(uploads []RemoteUpload) string {
		out := make([]string, 0, len(uploads))
		for _, upload := range uploads {
			out = append(out, upload.Dir)
		}
		return strings.Join(out, ",")
	}

	tests := []struct {
		name   string
		filter ListFilter
		want   string
	}{
		{"empty keeps all", ListFilter{}, "docdir,coldir,confdir"},
		{
			"newer-than inclusive boundary",
			ListFilter{NewerThan: threshold},
			"coldir,confdir",
		},
		{
			"older-than exclusive boundary",
			ListFilter{OlderThan: threshold},
			"docdir",
		},
		{
			// Conflicts are neither kind and match no kind filter.
			"kind document excludes conflicts",
			ListFilter{Kind: UploadKindDocument},
			"docdir",
		},
		{
			"slug wildcard selects documents only",
			ListFilter{Slug: "*"},
			"docdir",
		},
		{"limit", ListFilter{Limit: 1}, "confdir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dirs(tt.filter.FilterRemote(uploads))
			if got != tt.want {
				t.Fatalf("FilterRemote = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListFilterValidate(t *testing.T) {
	tests := []struct {
		name   string
		filter ListFilter
		want   string
	}{
		{
			"invalid kind",
			ListFilter{Kind: "bogus"},
			`invalid kind "bogus"`,
		},
		{
			"invalid slug pattern",
			ListFilter{Slug: "["},
			"invalid slug pattern",
		},
		{
			"negative limit",
			ListFilter{Limit: -1},
			"limit must not be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want %q", err, tt.want)
			}
		})
	}
}
