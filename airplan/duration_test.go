package airplan

import (
	"strings"
	"testing"
	"time"
)

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

func TestParseTimeFilter(t *testing.T) {
	loc := time.FixedZone("test", -4*60*60)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Time
	}{
		{"7d", now.Add(-7 * 24 * time.Hour)},
		{"2w", now.Add(-14 * 24 * time.Hour)},
		{"36h", now.Add(-36 * time.Hour)},
		{"1h30m", now.Add(-90 * time.Minute)},
		{"2026", time.Date(2026, 1, 1, 0, 0, 0, 0, loc)},
		{"2026-07", time.Date(2026, 7, 1, 0, 0, 0, 0, loc)},
		{"2026-07-01", time.Date(2026, 7, 1, 0, 0, 0, 0, loc)},
		{"2026/07/01", time.Date(2026, 7, 1, 0, 0, 0, 0, loc)},
		{"2026-07-01 09:30", time.Date(2026, 7, 1, 9, 30, 0, 0, loc)},
		{
			"2026-07-01T09:30:00",
			time.Date(2026, 7, 1, 9, 30, 0, 0, loc),
		},
		{
			"2026-07-01T09:30:00Z",
			time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC),
		},
		{
			// An explicit RFC 3339 offset is honored exactly, not
			// reinterpreted in the local zone.
			"2026-07-01T09:30:00+02:00",
			time.Date(2026, 7, 1, 9, 30, 0, 0,
				time.FixedZone("", 2*60*60)),
		},
		{
			"2026-07-01T09:30:00.125+23:59",
			time.Date(2026, 7, 1, 9, 30, 0, 125_000_000,
				time.FixedZone("", 23*60*60+59*60)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseTimeFilterIn(tt.in, now, loc)
			if err != nil {
				t.Fatalf("parseTimeFilterIn(%q) error = %v", tt.in, err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("parseTimeFilterIn(%q) = %s, want %s",
					tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTimeFilterUsesLocalZone(t *testing.T) {
	now := time.Now()
	got, err := ParseTimeFilter("2026-07-01", now)
	if err != nil {
		t.Fatalf("ParseTimeFilter error = %v", err)
	}
	want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ParseTimeFilter = %s, want %s", got, want)
	}
}

func TestParseTimeFilterAcrossDSTMidnight(t *testing.T) {
	// Chile's spring-forward transition happens at local midnight, so
	// the bare date's nominal midnight does not exist. The parsed
	// value must agree with time.Date's normalization for the same
	// nominal local time.
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Skipf("time zone database unavailable: %v", err)
	}
	now := time.Now()
	got, err := parseTimeFilterIn("2026-09-06", now, loc)
	if err != nil {
		t.Fatalf("parseTimeFilterIn error = %v", err)
	}
	want := time.Date(2026, 9, 6, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("parseTimeFilterIn = %s, want %s", got, want)
	}
}

func TestParseTimeFilterErrors(t *testing.T) {
	now := time.Now()
	tests := []struct {
		in   string
		want string
	}{
		{"", "use a duration like 30d or a date like 2026-07-01"},
		{"xyz", "use a duration like 30d or a date like 2026-07-01"},
		{"30", "use a duration like 30d or a date like 2026-07-01"},
		{"-1d", "use a duration like 30d or a date like 2026-07-01"},
		{"1mo", "use a duration like 30d or a date like 2026-07-01"},
		{"1y", "use a duration like 30d or a date like 2026-07-01"},
		{"yesterday", "use a duration like 30d or a date like 2026-07-01"},
		{"2026-13-45", "expected forms like 2026-07-01"},
		{"2026-07-01X", "expected forms like 2026-07-01"},
		{"2026-07-01T09:30", "expected forms like 2026-07-01"},
		{"2026-07-01 09:30:00", "expected forms like 2026-07-01"},
		{"2026-07-01T09:30:00+24:00", "expected forms like 2026-07-01"},
		{"2026-07-01T09:30:00+00:60", "expected forms like 2026-07-01"},
		{"2026-07-01T09:30:00,5Z", "expected forms like 2026-07-01"},
		// Slash dates without a leading four-digit year are refused,
		// never guessed: day-first and month-first are both plausible.
		{"03/04/2026", "ambiguous day/month order; use YYYY-MM-DD"},
		{"3/4/26", "ambiguous day/month order; use YYYY-MM-DD"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseTimeFilter(tt.in, now)
			if err == nil {
				t.Fatalf("ParseTimeFilter(%q) error = nil", tt.in)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseTimeFilter(%q) error = %q, want %q",
					tt.in, err, tt.want)
			}
		})
	}
}
