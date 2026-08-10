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

func TestParseTimeFilterAcceptedForms(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 12, 30, 0, 0, location)
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{"duration days", "30d", now.Add(-30 * 24 * time.Hour)},
		{"duration composed", "2h30m", now.Add(-2*time.Hour - 30*time.Minute)},
		{"bare year", "2006", time.Date(2006, 1, 1, 0, 0, 0, 0, location)},
		{"year month", "2006-07", time.Date(2006, 7, 1, 0, 0, 0, 0, location)},
		{"dash date", "2006-07-08", time.Date(2006, 7, 8, 0, 0, 0, 0, location)},
		{"slash date", "2006/07/08", time.Date(2006, 7, 8, 0, 0, 0, 0, location)},
		{"local minute", "2006-07-08 14:03", time.Date(2006, 7, 8, 14, 3, 0, 0, location)},
		{"local seconds", "2006-07-08T14:03:11", time.Date(2006, 7, 8, 14, 3, 11, 0, location)},
		{"RFC3339 offset", "2006-07-08T14:03:11-04:00", time.Date(2006, 7, 8, 18, 3, 11, 0, time.UTC)},
		{"RFC3339 Z", "2006-07-08T14:03:11Z", time.Date(2006, 7, 8, 14, 3, 11, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, parseErr := ParseTimeFilter(tt.in, now)
			if parseErr != nil {
				t.Fatalf("ParseTimeFilter(%q) error = %v", tt.in, parseErr)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ParseTimeFilter(%q) = %s, want %s", tt.in, got, tt.want)
			}
			if !strings.Contains(tt.name, "RFC3339") &&
				!strings.HasPrefix(tt.name, "duration") && got.Location() != location {
				t.Fatalf("ParseTimeFilter(%q) location = %v, want %v",
					tt.in, got.Location(), location)
			}
			if tt.name == "RFC3339 offset" {
				_, offset := got.Zone()
				if offset != -4*60*60 {
					t.Fatalf("ParseTimeFilter(%q) offset = %d", tt.in, offset)
				}
			}
		})
	}
}

func TestParseTimeFilterBareDatesUseLocalDSTRules(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, location)
	for _, tt := range []struct {
		in         string
		wantOffset int
	}{
		{"2026-03-07", -5 * 60 * 60},
		{"2026-03-09", -4 * 60 * 60},
	} {
		got, parseErr := ParseTimeFilter(tt.in, now)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		_, offset := got.Zone()
		if offset != tt.wantOffset || got.Hour() != 0 || got.Location() != location {
			t.Fatalf("ParseTimeFilter(%q) = %s (offset %d)", tt.in, got, offset)
		}
	}
}

func TestParseTimeFilterErrors(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{
		"", "30", "-1d", "tomorrow", "2026-13", "2026-02-30",
		"2026-01-02T14:03", "2026-01-02 14:03:05",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseTimeFilter(in, now)
			if err == nil || !strings.HasPrefix(err.Error(),
				"airplan: invalid time filter") {
				t.Fatalf("ParseTimeFilter(%q) error = %v", in, err)
			}
		})
	}
}

func TestParseTimeFilterSlashDateErrorUsesISOOrder(t *testing.T) {
	_, err := ParseTimeFilter("03/04/2026", time.Now())
	if err == nil || !strings.Contains(err.Error(), "YYYY/MM/DD") ||
		!strings.Contains(err.Error(), "ISO") {
		t.Fatalf("error = %v", err)
	}
}
