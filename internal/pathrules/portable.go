package pathrules

import "strings"

// PortableSegment reports whether a logical-path segment is portable across
// Airplan's supported filesystems.
func PortableSegment(segment string) bool {
	if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return false
	}
	base := segment
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4",
		"COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3",
		"LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	default:
		return true
	}
}
