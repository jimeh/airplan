package airplan

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// timeFilterLayouts are the absolute forms ParseTimeFilter accepts, in the
// order it tries them (SPEC.md §9). Forms without an offset resolve in the
// caller's location; RFC 3339 carries its own.
var timeFilterLayouts = []string{
	"2006",
	"2006-01",
	"2006-01-02",
	"2006/01/02",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02T15:04:05",
}

// ParseTimeFilter parses a listing or cleanup time boundary (SPEC.md §9). It
// accepts exactly two forms: an age, as ParseAge accepts, resolved relative to
// now; or an absolute date. A bare date means midnight in now's location, and a
// date-time without an offset is read in that location too, matching how docker
// and journalctl read them. A form carrying an offset is honored exactly as
// written. The manifest keeps storing UTC; only the input is local.
//
// Disambiguation is positional and total: four leading digits, alone or
// followed by "-" or "/", begin an absolute date, and everything else is an
// age. A slash date that does not lead with a four-digit year is refused
// rather than guessed, because this boundary also selects purge deletions.
func ParseTimeFilter(s string, now time.Time) (time.Time, error) {
	raw := strings.TrimSpace(s)
	if IsAbsoluteTimeFilter(raw) {
		if hasStrictRFC3339Syntax(raw) {
			when, err := time.Parse(time.RFC3339, raw)
			if err == nil {
				return when, nil
			}
		}
		// time.Parse accepts fractional seconds even when the layout does
		// not declare them. Offset-less forms deliberately support only the
		// exact local layouts above, while zoned fractions must pass the
		// strict RFC 3339 syntax check.
		if strings.ContainsAny(raw, ".,") {
			return invalidTimeFilter(s)
		}
		for _, layout := range timeFilterLayouts {
			when, err := time.ParseInLocation(layout, raw, now.Location())
			if err == nil {
				return when, nil
			}
		}
		return invalidTimeFilter(s)
	}
	if strings.Contains(raw, "/") {
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (ambiguous date; write the year "+
				"first, as 2026-03-04)", s)
	}
	if strings.HasPrefix(raw, "-") {
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (must not be negative)", s)
	}
	age, err := ParseAge(raw)
	if err != nil {
		if isBareNumber(raw) {
			return time.Time{}, fmt.Errorf(
				"airplan: invalid time %q (bare numbers are ambiguous; add "+
					"a unit like 30d or write a date like 2026-07-01)", s)
		}
		return time.Time{}, fmt.Errorf(
			"airplan: invalid time %q (want a duration like 30d or a date "+
				"like 2026-07-01)", s)
	}
	return now.Add(-age), nil
}

func invalidTimeFilter(s string) (time.Time, error) {
	return time.Time{}, fmt.Errorf(
		"airplan: invalid time %q (want a date like 2026-07-01, "+
			"2026-07-01 09:30, or 2026-07-01T09:30:00Z)", s)
}

// hasStrictRFC3339Syntax rejects extensions accepted by time.Parse but not
// RFC 3339, including comma fractions and out-of-range numeric offsets.
// Calendar and clock ranges remain time.Parse's responsibility.
func hasStrictRFC3339Syntax(raw string) bool {
	const dateTime = "2006-01-02T15:04:05"
	if len(raw) < len(dateTime)+1 || raw[10] != 'T' {
		return false
	}
	zoneStart := len(dateTime)
	if raw[zoneStart] == '.' {
		zoneStart++
		fractionStart := zoneStart
		for zoneStart < len(raw) && raw[zoneStart] >= '0' && raw[zoneStart] <= '9' {
			zoneStart++
		}
		if zoneStart == fractionStart {
			return false
		}
	}
	if zoneStart == len(raw)-1 && raw[zoneStart] == 'Z' {
		return true
	}
	if len(raw)-zoneStart != len("+00:00") ||
		(raw[zoneStart] != '+' && raw[zoneStart] != '-') ||
		raw[zoneStart+3] != ':' {
		return false
	}
	for _, index := range []int{
		zoneStart + 1, zoneStart + 2, zoneStart + 4, zoneStart + 5,
	} {
		if raw[index] < '0' || raw[index] > '9' {
			return false
		}
	}
	hour := int(raw[zoneStart+1]-'0')*10 + int(raw[zoneStart+2]-'0')
	minute := int(raw[zoneStart+4]-'0')*10 + int(raw[zoneStart+5]-'0')
	return hour <= 23 && minute <= 59
}

// IsAbsoluteTimeFilter reports whether ParseTimeFilter reads s as an absolute
// date rather than an age: it opens with a four-digit year that is either the
// whole value or followed by a date separator (SPEC.md §9). Callers that treat
// the two forms differently, such as purge refusing a degenerate age, use this
// rather than inspecting the parsed result.
func IsAbsoluteTimeFilter(s string) bool {
	return isAbsoluteTimeFilter(strings.TrimSpace(s))
}

func isAbsoluteTimeFilter(s string) bool {
	if len(s) < 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) == 4 || s[4] == '-' || s[4] == '/'
}

func isBareNumber(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if (s[i] < '0' || s[i] > '9') && s[i] != '.' {
			return false
		}
	}
	return true
}

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
