package airplan

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseAge parses --older-than durations for cleanup commands
// (SPEC.md §9). It accepts Go duration strings plus d (24h) and w
// (7d) units, composed as adjacent number+unit pairs.
func ParseAge(s string) (time.Duration, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, fmt.Errorf("airplan: invalid age %q", s)
	}
	if strings.HasPrefix(raw, "-") {
		return 0, fmt.Errorf("airplan: invalid age %q (must not be negative)", s)
	}

	if d, err := time.ParseDuration(raw); err == nil {
		if d < 0 {
			return 0, fmt.Errorf(
				"airplan: invalid age %q (must not be negative)", s)
		}
		return d, nil
	}

	var total time.Duration
	for i := 0; i < len(raw); {
		start := i
		hasDigit := false
		for i < len(raw) && ((raw[i] >= '0' && raw[i] <= '9') ||
			raw[i] == '.') {
			if raw[i] >= '0' && raw[i] <= '9' {
				hasDigit = true
			}
			i++
		}
		if !hasDigit {
			return 0, fmt.Errorf("airplan: invalid age %q", s)
		}
		num := raw[start:i]

		unitStart := i
		for i < len(raw) && (raw[i] < '0' || raw[i] > '9') &&
			raw[i] != '.' {
			i++
		}
		if unitStart == i {
			return 0, fmt.Errorf(
				"airplan: invalid age %q (bare numbers are ambiguous)", s)
		}
		unit := raw[unitStart:i]

		var d time.Duration
		switch unit {
		case "d", "w":
			n, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, fmt.Errorf("airplan: invalid age %q", s)
			}
			if n < 0 {
				return 0, fmt.Errorf(
					"airplan: invalid age %q (must not be negative)", s)
			}
			hours := 24.0
			if unit == "w" {
				hours *= 7
			}
			d = time.Duration(n * hours * float64(time.Hour))
		default:
			var err error
			d, err = time.ParseDuration(num + unit)
			if err != nil {
				return 0, fmt.Errorf("airplan: invalid age %q", s)
			}
		}
		if d < 0 || total+d < total {
			return 0, fmt.Errorf(
				"airplan: invalid age %q (must not be negative)", s)
		}
		total += d
	}

	return total, nil
}

// timeFilterLayouts are the accepted absolute time-filter forms
// (SPEC.md §9), tried in order. Unspecified fields default to zero, so
// partial dates resolve to the first instant of their period.
var timeFilterLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02",
	"2006-01",
	"2006",
}

// ParseTimeFilter parses a list or purge time boundary (SPEC.md §9).
// It accepts exactly two forms: a ParseAge duration (7d, 2w, 36h,
// 1h30m) subtracted from now, or an absolute timestamp. Input with a
// leading four-digit year followed by "-" or "/" (or a bare YYYY) is
// absolute; bare dates resolve to local midnight, while an RFC 3339
// value with an explicit offset is honored exactly.
func ParseTimeFilter(s string, now time.Time) (time.Time, error) {
	return parseTimeFilterIn(s, now, time.Local)
}

func parseTimeFilterIn(
	s string, now time.Time, loc *time.Location,
) (time.Time, error) {
	raw := strings.TrimSpace(s)
	if isAbsoluteTimeFilter(raw) {
		for _, layout := range timeFilterLayouts {
			if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (expected forms like 2026-07-01 "+
				"or 2026-07-01T09:30:00Z)", s)
	}
	if strings.ContainsRune(raw, '/') {
		// Day-first and month-first orders are both plausible, and this
		// parser also feeds destructive purge boundaries, so slash dates
		// without a leading four-digit year are refused, not guessed.
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (ambiguous day/month order; "+
				"use YYYY-MM-DD)", s)
	}
	d, err := ParseAge(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (use a duration like 30d or a "+
				"date like 2026-07-01)", s)
	}
	return now.Add(-d), nil
}

// isAbsoluteTimeFilter reports whether the input selects the absolute
// timestamp form: a leading four-digit year that stands alone or is
// followed by "-" or "/". Everything else is treated as a duration.
func isAbsoluteTimeFilter(raw string) bool {
	if len(raw) < 4 {
		return false
	}
	for _, r := range raw[:4] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(raw) == 4 || raw[4] == '-' || raw[4] == '/'
}
