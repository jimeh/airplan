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
