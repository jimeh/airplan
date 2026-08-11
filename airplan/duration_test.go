package airplan

import (
	"strings"
	"testing"
	"time"

	// Embedded zone data keeps the local-midnight and DST cases below working
	// on hosts without a system tzdata database, including Windows CI.
	_ "time/tzdata"
)

// testZone loads a fixed named zone for local-midnight assertions.
func testZone(t *testing.T) *time.Location {
	t.Helper()

	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation: %v", err)
	}
	return zone
}

func TestParseTimeFilterDurations(t *testing.T) {
	zone := testZone(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, zone)

	tests := []struct {
		in   string
		want time.Time
	}{
		{"7d", now.Add(-7 * 24 * time.Hour)},
		{"2w", now.Add(-14 * 24 * time.Hour)},
		{"36h", now.Add(-36 * time.Hour)},
		{"1h30m", now.Add(-90 * time.Minute)},
		{"90m", now.Add(-90 * time.Minute)},
		{"1w2d", now.Add(-9 * 24 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTimeFilter(tt.in, now)
			if err != nil {
				t.Fatalf("ParseTimeFilter(%q) error = %v", tt.in, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseTimeFilter(%q) = %s, want %s",
					tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTimeFilterAbsolute(t *testing.T) {
	zone := testZone(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, zone)

	tests := []struct {
		in   string
		want time.Time
	}{
		{"2026", time.Date(2026, 1, 1, 0, 0, 0, 0, zone)},
		{"2026-07", time.Date(2026, 7, 1, 0, 0, 0, 0, zone)},
		{"2026-07-01", time.Date(2026, 7, 1, 0, 0, 0, 0, zone)},
		{"2026/07/01", time.Date(2026, 7, 1, 0, 0, 0, 0, zone)},
		{"2026-07-01 09:30", time.Date(2026, 7, 1, 9, 30, 0, 0, zone)},
		{"2026-07-01T09:30", time.Date(2026, 7, 1, 9, 30, 0, 0, zone)},
		{"2026-07-01T09:30:00", time.Date(2026, 7, 1, 9, 30, 0, 0, zone)},
		{
			"2026-07-01T09:30:00Z",
			time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC),
		},
		{
			"2026-07-01T09:30:00.125Z",
			time.Date(2026, 7, 1, 9, 30, 0, 125_000_000, time.UTC),
		},
		{
			"2026-07-01T09:30:00+02:00",
			time.Date(2026, 7, 1, 7, 30, 0, 0, time.UTC),
		},
		{
			"2026-07-01T09:30:00.125+23:59",
			time.Date(2026, 6, 30, 9, 31, 0, 125_000_000, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTimeFilter(tt.in, now)
			if err != nil {
				t.Fatalf("ParseTimeFilter(%q) error = %v", tt.in, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseTimeFilter(%q) = %s, want %s",
					tt.in, got.UTC(), tt.want.UTC())
			}
		})
	}
}

// TestParseTimeFilterLocalMidnightCrossesDST pins bare dates to local midnight
// in the caller's zone: the two midnights either side of a DST start are 23
// hours apart, not 24, and both resolve to the correct UTC instant.
func TestParseTimeFilterLocalMidnightCrossesDST(t *testing.T) {
	zone := testZone(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, zone)

	before, err := ParseTimeFilter("2026-03-08", now)
	if err != nil {
		t.Fatal(err)
	}
	after, err := ParseTimeFilter("2026-03-09", now)
	if err != nil {
		t.Fatal(err)
	}
	wantBefore := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)
	wantAfter := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	if !before.Equal(wantBefore) {
		t.Errorf("2026-03-08 = %s, want %s", before.UTC(), wantBefore)
	}
	if !after.Equal(wantAfter) {
		t.Errorf("2026-03-09 = %s, want %s", after.UTC(), wantAfter)
	}
	if got := after.Sub(before); got != 23*time.Hour {
		t.Errorf("gap = %s, want 23h across the DST start", got)
	}
}

func TestParseTimeFilterErrors(t *testing.T) {
	zone := testZone(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, zone)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// A destructive purge must never guess between D/M/Y and M/D/Y.
			"ambiguous slash date",
			"03/04/2026",
			`airplan: invalid time "03/04/2026" ` +
				`(ambiguous date; write the year first, as 2026-03-04)`,
		},
		{
			"two-digit year slash date",
			"26/03/04",
			`airplan: invalid time "26/03/04" ` +
				`(ambiguous date; write the year first, as 2026-03-04)`,
		},
		{
			"empty",
			"",
			`airplan: invalid time "" (want a duration like 30d ` +
				`or a date like 2026-07-01)`,
		},
		{
			"words",
			"yesterday",
			`airplan: invalid time "yesterday" (want a duration like 30d ` +
				`or a date like 2026-07-01)`,
		},
		{"bare number", "30", "bare numbers are ambiguous"},
		{"negative duration", "-7d", "must not be negative"},
		{"impossible month", "2026-13-01", "invalid time"},
		{"seconds without minutes", "2026-07-01T09", "invalid time"},
		{"trailing text", "2026-07-01 09:30 sharp", "invalid time"},
		{"comma RFC 3339 fraction", "2026-07-01T09:30:00,5Z", "invalid time"},
		{"offset hour 24", "2026-07-01T09:30:00+24:00", "invalid time"},
		{"offset minute 60", "2026-07-01T09:30:00+00:60", "invalid time"},
		{"local dot fraction", "2026-07-01T09:30:00.5", "invalid time"},
		{"local comma fraction", "2026-07-01T09:30:00,5", "invalid time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeFilter(tt.in, now)
			if err == nil {
				t.Fatalf("ParseTimeFilter(%q) = %s, want an error", tt.in, got)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseTimeFilter(%q) error = %q, want it to contain %q",
					tt.in, err, tt.want)
			}
		})
	}
}

func TestParseAge(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"36h", 36 * time.Hour},
		{"1w2d", 9 * 24 * time.Hour},
		{"1.5d", 36 * time.Hour},
		{".5w", 84 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseAge(tt.in)
			if err != nil {
				t.Fatalf("ParseAge(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseAge(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseAgeErrors(t *testing.T) {
	for _, in := range []string{"", "30", "-1d", "xyz"} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseAge(in); err == nil {
				t.Fatalf("ParseAge(%q) error = nil, want error", in)
			}
		})
	}
}
