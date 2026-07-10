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
// the random directory itself. When a key_prefix is configured, the
// target must fall under it — deletion never reaches outside the
// configured scope in a shared bucket. cfg may be nil for bare keys.
func KeyFromURLOrKey(cfg *Config, s string) (string, error) {
	key := s
	if strings.Contains(s, "://") {
		// Parsing up front means query strings and fragments — think
		// URLs copy-pasted with ?utm_source=... — never leak into the
		// resolved key.
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("airplan: parse url %q: %w", s, err)
		}

		key = strings.TrimPrefix(u.Path, "/")
		switch {
		case cfg != nil && baseURLMatches(cfg.PublicBaseURL, u):
			// A public_base_url may itself carry a path
			// (https://cdn.example.com/plans); strip that path from
			// the key.
			b, _ := url.Parse(cfg.PublicBaseURL)
			basePath := strings.Trim(b.Path, "/")
			if basePath != "" {
				key = strings.TrimPrefix(key, basePath+"/")
			}
		case cfg != nil && baseURLMatches(cfg.Endpoint, u):
			// Path-style URLs (<endpoint>/<bucket>/<key>) carry the
			// endpoint path and bucket before the key.
			b, _ := url.Parse(cfg.Endpoint)
			basePath := strings.Trim(b.Path, "/")
			if basePath != "" {
				key = strings.TrimPrefix(key, basePath+"/")
			}
			key = strings.TrimPrefix(key, cfg.Bucket+"/")
		case cfg != nil && cfg.Bucket != "":
			// Keep accepting path-style URLs when only the bucket is
			// available to the library caller.
			key = strings.TrimPrefix(key, cfg.Bucket+"/")
		}
	}

	return validateTarget(cfg, key)
}

// baseURLMatches reports whether u is served from the configured
// public_base_url (same host, path under the base's path).
func baseURLMatches(base string, u *url.URL) bool {
	if base == "" {
		return false
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" ||
		!strings.EqualFold(b.Scheme, u.Scheme) ||
		!strings.EqualFold(b.Host, u.Host) {
		return false
	}
	basePath := strings.Trim(b.Path, "/")
	if basePath == "" {
		return true
	}
	return strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), basePath+"/")
}

// validateTarget checks that a resolved key is a plausible airplan
// key and, when a key_prefix is configured, that it falls under it.
func validateTarget(cfg *Config, key string) (string, error) {
	key = strings.Trim(key, "/")
	if _, err := uploadDirPrefix(key); err != nil {
		return "", err
	}

	if cfg != nil && cfg.KeyPrefix != "" {
		prefix := strings.Trim(cfg.KeyPrefix, "/") + "/"
		if !strings.HasPrefix(key, prefix) {
			return "", fmt.Errorf(
				"airplan: %q is outside the configured key_prefix %q — "+
					"refusing to touch it", key, cfg.KeyPrefix)
		}
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
