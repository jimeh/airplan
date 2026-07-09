package airplan

import (
	"fmt"
	"net/url"
	"strings"
)

// isRandomDir reports whether a path segment looks like an airplan
// random directory: 26 characters of lowercase RFC 4648 base32
// (SPEC.md §8).
func isRandomDir(seg string) bool {
	if len(seg) != 26 {
		return false
	}
	for _, r := range seg {
		if (r < 'a' || r > 'z') && (r < '2' || r > '7') {
			return false
		}
	}
	return true
}

// KeyFromURLOrKey resolves a delete/purge target (SPEC.md §9): a full
// URL (public_base_url-based or path-style), a bare object key, or
// the random directory itself. cfg may be nil for bare keys.
func KeyFromURLOrKey(cfg *Config, s string) (string, error) {
	key := s
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("airplan: parse url %q: %w", s, err)
		}
		key = strings.TrimPrefix(u.Path, "/")
		// Path-style URLs (<endpoint>/<bucket>/<key>) carry the bucket
		// as the first segment.
		if cfg != nil && cfg.Bucket != "" {
			key = strings.TrimPrefix(key, cfg.Bucket+"/")
		}
	}

	key = strings.Trim(key, "/")
	if _, err := uploadDirPrefix(key); err != nil {
		return "", err
	}
	return key, nil
}

// uploadDirPrefix returns an upload's directory prefix
// ("[key_prefix/]<random>/") for one of its object keys — the unit of
// deletion (SPEC.md §9). Object filenames always contain a dot
// (SPEC.md §3, §8), so they can never be mistaken for the extensionless
// 26-char base32 directory segment.
func uploadDirPrefix(key string) (string, error) {
	segs := strings.Split(key, "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if isRandomDir(segs[i]) {
			return strings.Join(segs[:i+1], "/") + "/", nil
		}
	}
	return "", fmt.Errorf(
		"airplan: %q does not look like an airplan key "+
			"(no 26-char base32 directory segment)", key)
}
