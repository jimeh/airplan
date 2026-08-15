package airplan

import (
	"strings"
	"testing"
	"time"
)

func filterTestTime(hour int) time.Time {
	return time.Date(2026, 7, 8, hour, 0, 0, 0, time.UTC)
}

// filterTestBound returns a settable boundary for a filter under test.
func filterTestBound(hour int) *time.Time {
	when := filterTestTime(hour)
	return &when
}

// filterTestRecords returns three managed document uploads an hour apart, in
// listing order.
func filterTestRecords() []ManifestRecord {
	return []ManifestRecord{
		manifestUploadRecord(filterTestTime(12), "work", "plans", "", "a"),
		manifestUploadRecord(filterTestTime(13), "work", "plans", "", "b"),
		manifestUploadRecord(filterTestTime(14), "work", "plans", "", "c"),
	}
}

func recordTitles(records []ManifestRecord) []string {
	titles := make([]string, 0, len(records))
	for _, record := range records {
		titles = append(titles, record.Title)
	}
	return titles
}

func assertTitles(t *testing.T, got []string, want ...string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("titles = %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("titles = %q, want %q", got, want)
		}
	}
}

// TestListFilterTimeBoundaries pins the inclusive/exclusive boundary pair:
// NewerThan keeps a record recorded exactly at the threshold, OlderThan does
// not, matching purge's existing CreatedBefore comparison.
func TestListFilterTimeBoundaries(t *testing.T) {
	records := filterTestRecords()
	threshold := filterTestBound(13)

	tests := []struct {
		name   string
		filter ListFilter
		want   []string
	}{
		{"no bounds", ListFilter{}, []string{"a", "b", "c"}},
		{
			"newer than keeps the threshold",
			ListFilter{NewerThan: threshold},
			[]string{"b", "c"},
		},
		{
			"older than excludes the threshold",
			ListFilter{OlderThan: threshold},
			[]string{"a"},
		},
		{
			"both bounds select one record",
			ListFilter{NewerThan: threshold, OlderThan: filterTestBound(14)},
			[]string{"b"},
		},
		{
			"empty window selects nothing",
			ListFilter{NewerThan: filterTestBound(14), OlderThan: filterTestBound(13)},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.FilterManifestRecords(records)
			assertTitles(t, recordTitles(got), tt.want...)
		})
	}
}

// TestListFilterZeroTimeIsABound covers year one, which is a legal boundary
// value: whether a bound applies must come from whether it was set, never from
// the time it happens to hold.
func TestListFilterZeroTimeIsABound(t *testing.T) {
	records := filterTestRecords()
	zero := time.Time{}
	if !zero.IsZero() {
		t.Fatal("fixture is not a zero time")
	}

	older := ListFilter{OlderThan: &zero}.FilterManifestRecords(records)
	if len(older) != 0 {
		t.Fatalf("older than year one = %q, want none", recordTitles(older))
	}
	newer := ListFilter{NewerThan: &zero}.FilterManifestRecords(records)
	assertTitles(t, recordTitles(newer), "a", "b", "c")

	uploads := []RemoteUpload{{
		Dir: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: UploadKindDocument,
		LastModified: filterTestTime(12),
	}}
	if got := (ListFilter{OlderThan: &zero}).FilterRemoteUploads(
		uploads,
	); len(got) != 0 {
		t.Fatalf("remote older than year one = %+v, want none", got)
	}
}

func TestListFilterKind(t *testing.T) {
	document := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "", "a")
	collection := manifestUploadRecord(
		filterTestTime(13), "work", "plans", "", "b")
	collection.Kind = string(UploadKindCollection)
	collection.Slug = ""
	legacy := manifestUploadRecord(
		filterTestTime(14), "work", "plans", "", "c")
	legacy.Kind = ""
	legacy.MarkerVersion = 0
	records := []ManifestRecord{document, collection, legacy}

	// A managed record that declares no kind renders as "-" (SPEC.md §9), so
	// it must not answer to either kind filter either.
	managed := manifestUploadRecord(
		filterTestTime(15), "work", "plans", "", "d")
	managed.Kind = ""
	records = append(records, managed)

	tests := []struct {
		name string
		kind UploadKind
		want []string
	}{
		{"unset keeps every kind", "", []string{"a", "b", "c", "d"}},
		// SPEC.md §9: readers infer document for valid older records that
		// omit kind, so legacy history stays selectable.
		{
			"document includes legacy history", UploadKindDocument,
			[]string{"a", "c"},
		},
		{"collection", UploadKindCollection, []string{"b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListFilter{Kind: tt.kind}.FilterManifestRecords(records)
			assertTitles(t, recordTitles(got), tt.want...)
		})
	}
}

func TestListFilterProtected(t *testing.T) {
	records := filterTestRecords()
	records[1].Protected = true
	protected := true
	unprotected := false

	assertTitles(t, recordTitles(
		(ListFilter{Protected: &protected}).FilterManifestRecords(records),
	), "b")
	assertTitles(t, recordTitles(
		(ListFilter{Protected: &unprotected}).FilterManifestRecords(records),
	), "a", "c")

	uploads := []RemoteUpload{
		{Dir: "a", Protected: false},
		{Dir: "b", Protected: true},
		{Dir: "c", Protected: false},
	}
	filtered := (ListFilter{Protected: &protected}).FilterRemoteUploads(uploads)
	if len(filtered) != 1 || filtered[0].Dir != "b" {
		t.Fatalf("protected remote uploads = %+v, want b", filtered)
	}
	filtered = (ListFilter{Protected: &unprotected}).FilterRemoteUploads(uploads)
	if len(filtered) != 2 || filtered[0].Dir != "a" || filtered[1].Dir != "c" {
		t.Fatalf("unprotected remote uploads = %+v, want a and c", filtered)
	}
}

func TestManifestRecordKindInfersOnlyLegacyDocuments(t *testing.T) {
	tests := []struct {
		name   string
		record ManifestRecord
		want   UploadKind
	}{
		{"recorded document", ManifestRecord{Kind: "document", MarkerVersion: 3}, UploadKindDocument},
		{"recorded collection", ManifestRecord{Kind: "collection", MarkerVersion: 3}, UploadKindCollection},
		{"legacy omitted kind and version", ManifestRecord{}, UploadKindDocument},
		{"managed omitted kind", ManifestRecord{MarkerVersion: 3}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ManifestRecordKind(test.record); got != test.want {
				t.Fatalf("ManifestRecordKind = %q, want %q", got, test.want)
			}
		})
	}
}

func TestListFilterSlug(t *testing.T) {
	plan := manifestUploadRecord(filterTestTime(12), "work", "plans", "", "a")
	plan.Slug = "plan-alpha"
	collection := manifestUploadRecord(
		filterTestTime(13), "work", "plans", "", "b")
	collection.Kind = string(UploadKindCollection)
	collection.Slug = ""
	derived := manifestUploadRecord(
		filterTestTime(14), "work", "plans", "", "c")
	derived.Slug = ""
	derived.Key = "cccccccccccccccccccccccccc/plan-beta.html"
	derived.MarkerKey = ""
	records := []ManifestRecord{plan, collection, derived}

	tests := []struct {
		name string
		slug string
		want []string
	}{
		{"exact recorded slug", "plan-alpha", []string{"a"}},
		// Legacy records without a recorded slug derive it from the page key,
		// exactly as purge --slug does.
		{"derived slug", "plan-beta", []string{"c"}},
		{"glob", "plan-*", []string{"a", "c"}},
		{"star excludes collections", "*", []string{"a", "c"}},
		{"no match", "other", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListFilter{Slug: tt.slug}.FilterManifestRecords(records)
			assertTitles(t, recordTitles(got), tt.want...)
		})
	}
}

// TestManifestRecordSlug pins where the "collections have no slug" rule lives,
// including for a record that names one against the schema.
func TestManifestRecordSlug(t *testing.T) {
	document := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "", "a")
	document.Slug = "plan-alpha"
	if got := ManifestRecordSlug(document); got != "plan-alpha" {
		t.Errorf("recorded slug = %q, want plan-alpha", got)
	}

	legacy := document
	legacy.Slug = ""
	legacy.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaa/plan-beta.html"
	if got := ManifestRecordSlug(legacy); got != "plan-beta" {
		t.Errorf("derived slug = %q, want plan-beta", got)
	}

	collection := document
	collection.Kind = string(UploadKindCollection)
	if got := ManifestRecordSlug(collection); got != "" {
		t.Errorf("collection slug = %q, want empty", got)
	}
}

// TestPurgeSlugMatchesListSlug pins the parity SPEC §9 claims for --slug,
// including that an upload with no derivable slug matches neither.
func TestPurgeSlugMatchesListSlug(t *testing.T) {
	named := manifestUploadRecord(filterTestTime(12), "work", "plans", "", "a")
	named.Slug = "plan-alpha"
	slugless := manifestUploadRecord(
		filterTestTime(13), "work", "plans", "", "b")
	slugless.Slug = ""
	slugless.Key = "bbbbbbbbbbbbbbbbbbbbbbbbbb/plan"
	slugless.MarkerKey = ""
	collection := manifestUploadRecord(
		filterTestTime(14), "work", "plans", "", "c")
	collection.Kind = string(UploadKindCollection)
	collection.Slug = ""
	records := []ManifestRecord{named, slugless, collection}

	for _, pattern := range []string{"*", "plan-*", "plan-alpha", "other"} {
		t.Run(pattern, func(t *testing.T) {
			listed := ListFilter{Slug: pattern}.FilterManifestRecords(records)
			purged := make([]ManifestRecord, 0, len(records))
			for _, record := range records {
				if purgeRecordMatches(
					record, PurgePlanOptions{Slug: pattern},
				) {
					purged = append(purged, record)
				}
			}
			assertTitles(t, recordTitles(purged), recordTitles(listed)...)
		})
	}
}

func TestListFilterSlugSkipsRecordsWithoutOne(t *testing.T) {
	record := manifestUploadRecord(
		filterTestTime(12), "work", "plans", "", "a")
	record.Slug = ""
	record.Key = "aaaaaaaaaaaaaaaaaaaaaaaaaa/plan"
	record.MarkerKey = ""

	got := ListFilter{Slug: "*"}.FilterManifestRecords(
		[]ManifestRecord{record},
	)
	if len(got) != 0 {
		t.Fatalf("records = %+v, want none", got)
	}
}

func TestListFilterLimitKeepsMostRecent(t *testing.T) {
	records := filterTestRecords()
	limit := func(n int) *int { return &n }

	tests := []struct {
		name  string
		limit *int
		want  []string
	}{
		{"unset", nil, []string{"a", "b", "c"}},
		{"zero selects nothing", limit(0), nil},
		{"one keeps the newest", limit(1), []string{"c"}},
		{"two keep the newest pair", limit(2), []string{"b", "c"}},
		{
			"beyond the set keeps everything", limit(9),
			[]string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListFilter{Limit: tt.limit}.FilterManifestRecords(records)
			assertTitles(t, recordTitles(got), tt.want...)
		})
	}
}

// TestListFilterLimitAppliesAfterSelection covers limit ordering: the count
// applies to what the other filters kept, not to the input.
func TestListFilterLimitAppliesAfterSelection(t *testing.T) {
	one := 1
	got := ListFilter{
		OlderThan: filterTestBound(14), Limit: &one,
	}.FilterManifestRecords(filterTestRecords())
	assertTitles(t, recordTitles(got), "b")
}

func TestListFilterValidate(t *testing.T) {
	negative := -1
	zero := 0
	tests := []struct {
		name   string
		filter ListFilter
		want   string
	}{
		{"zero value", ListFilter{}, ""},
		{"document", ListFilter{Kind: UploadKindDocument}, ""},
		{"zero limit", ListFilter{Limit: &zero}, ""},
		{
			"unknown kind",
			ListFilter{Kind: "page"},
			`airplan: invalid kind "page" (want document or collection)`,
		},
		{
			"malformed glob",
			ListFilter{Slug: "["},
			"airplan: invalid slug pattern",
		},
		{
			"negative limit",
			ListFilter{Limit: &negative},
			"airplan: limit must not be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want it to contain %q",
					err, tt.want)
			}
		})
	}
}

func TestListFilterRemoteUploads(t *testing.T) {
	document := RemoteUpload{
		Dir: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: UploadKindDocument,
		Slug: "plan-alpha", URL: "https://plans.example.com/a/plan.html",
		LastModified: filterTestTime(12),
	}
	// A remote slug is a LIST-derived hint, so the kind guard, not an absent
	// hint, has to keep collections out of --slug results.
	collection := RemoteUpload{
		Dir: "bbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: UploadKindCollection,
		Slug:         "plan-beta",
		URL:          "https://plans.example.com/b/index.html",
		LastModified: filterTestTime(13),
	}
	conflict := RemoteUpload{
		Dir: "cccccccccccccccccccccccccc", Conflict: true,
		LastModified: filterTestTime(14),
	}
	uploads := []RemoteUpload{document, collection, conflict}
	one := 1

	tests := []struct {
		name   string
		filter ListFilter
		want   []string
	}{
		{"unfiltered", ListFilter{}, []string{
			document.Dir, collection.Dir, conflict.Dir,
		}},
		{
			"newer than keeps the threshold",
			ListFilter{NewerThan: filterTestBound(13)},
			[]string{collection.Dir, conflict.Dir},
		},
		{
			"older than excludes the threshold",
			ListFilter{OlderThan: filterTestBound(13)},
			[]string{document.Dir},
		},
		// A dual-marker conflict declares no kind, so it matches neither.
		{
			"document excludes conflicts",
			ListFilter{Kind: UploadKindDocument},
			[]string{document.Dir},
		},
		{
			"collection excludes conflicts",
			ListFilter{Kind: UploadKindCollection},
			[]string{collection.Dir},
		},
		{
			"slug matches documents only",
			ListFilter{Slug: "*"},
			[]string{document.Dir},
		},
		{
			"limit keeps the newest",
			ListFilter{Limit: &one},
			[]string{conflict.Dir},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.FilterRemoteUploads(uploads)
			dirs := make([]string, 0, len(got))
			for _, upload := range got {
				dirs = append(dirs, upload.Dir)
			}
			assertTitles(t, dirs, tt.want...)
		})
	}
}
