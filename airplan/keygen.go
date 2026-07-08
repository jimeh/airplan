package airplan

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// RandomDirName returns the random directory portion of an object key:
// 16 bytes from a cryptographically secure random source, encoded as
// lowercase base32 (RFC 4648 alphabet, no padding) — 26 characters,
// 128 bits of entropy (SPEC.md §8). Never a seeded PRNG.
func RandomDirName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	name := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString(b[:])

	return strings.ToLower(name), nil
}

// SanitizeSlug sanitizes the human-readable filename portion of the
// key (SPEC.md §8): lowercased, any run of characters outside
// [a-z0-9-] becomes a single "-", leading/trailing "-" trimmed, max 64
// characters. If sanitization leaves an empty string, it returns
// "plan".
func SanitizeSlug(s string) string {
	var b strings.Builder
	lastHyphen := false

	for _, r := range strings.ToLower(s) {
		if isSlugChar(r) {
			if r == '-' {
				if lastHyphen {
					continue
				}
				lastHyphen = true
			} else {
				lastHyphen = false
			}
			b.WriteRune(r)
			continue
		}

		if !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if len(slug) > 64 {
		slug = strings.TrimRight(slug[:64], "-")
	}
	if slug == "" {
		return "plan"
	}

	return slug
}

// BuildKey joins an optional key prefix, the random directory, and a
// filename into an object key: [<prefix>/]<dir>/<filename>
// (SPEC.md §8).
func BuildKey(prefix, dir, filename string) string {
	parts := make([]string, 0, 3)
	if prefix = strings.Trim(prefix, "/"); prefix != "" {
		parts = append(parts, prefix)
	}
	if dir != "" {
		parts = append(parts, dir)
	}
	if filename != "" {
		parts = append(parts, filename)
	}

	return strings.Join(parts, "/")
}

func isSlugChar(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
}
