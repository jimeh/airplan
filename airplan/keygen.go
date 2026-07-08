package airplan

import "errors"

// RandomDirName returns the random directory portion of an object key:
// 16 bytes from a cryptographically secure random source, encoded as
// lowercase base32 (RFC 4648 alphabet, no padding) — 26 characters,
// 128 bits of entropy (SPEC.md §8). Never a seeded PRNG.
func RandomDirName() (string, error) {
	return "", errors.New("airplan: RandomDirName not implemented")
}

// SanitizeSlug sanitizes the human-readable filename portion of the
// key (SPEC.md §8): lowercased, any run of characters outside
// [a-z0-9-] becomes a single "-", leading/trailing "-" trimmed, max 64
// characters. If sanitization leaves an empty string, it returns
// "plan".
func SanitizeSlug(s string) string {
	return ""
}

// BuildKey joins an optional key prefix, the random directory, and a
// filename into an object key: [<prefix>/]<dir>/<filename>
// (SPEC.md §8).
func BuildKey(prefix, dir, filename string) string {
	return ""
}
