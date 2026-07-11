package airplan

import (
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
