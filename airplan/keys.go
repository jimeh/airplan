package airplan

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var errInvalidTarget = errors.New("airplan: invalid upload target")

type invalidTargetError struct {
	err error
}

func (e *invalidTargetError) Error() string {
	return e.err.Error()
}

func (e *invalidTargetError) Unwrap() error {
	return errInvalidTarget
}

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

// KeyFromURLOrKey resolves a remote management target (SPEC.md §9): a full
// URL (public_base_url-based or path-style), a bare object key, or the random
// directory itself. When a key_prefix is configured, the target must fall
// under it. cfg may be nil for bare keys.
func KeyFromURLOrKey(cfg *Config, s string) (string, error) {
	key, err := keyFromURLOrKey(cfg, s)
	if err != nil {
		return "", &invalidTargetError{err: err}
	}
	return key, nil
}

func keyFromURLOrKey(cfg *Config, s string) (string, error) {
	key := s
	if strings.Contains(s, "://") {
		// Parsing up front means query strings and fragments — think
		// URLs copy-pasted with ?utm_source=... — never leak into the
		// resolved key.
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("airplan: parse url %q: %w", s, err)
		}
		if !strings.EqualFold(u.Scheme, "http") &&
			!strings.EqualFold(u.Scheme, "https") {
			return "", fmt.Errorf(
				"airplan: URL %q must use http or https", s)
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
			key, err = stripURLBucket(key, cfg.Bucket, s)
			if err != nil {
				return "", err
			}
		case cfg != nil && cfg.Endpoint == "" &&
			cfg.PublicBaseURL == "" && cfg.Bucket != "":
			// Keep accepting path-style URLs when only the bucket is
			// available to the library caller.
			key, err = stripURLBucket(key, cfg.Bucket, s)
			if err != nil {
				return "", err
			}
		case cfg != nil &&
			(cfg.Endpoint != "" || cfg.PublicBaseURL != ""):
			return "", fmt.Errorf(
				"airplan: URL %q does not match the configured endpoint "+
					"or public_base_url", s)
		}
	}

	return validateTarget(cfg, key)
}

func stripURLBucket(path, bucket, rawURL string) (string, error) {
	segment, rest, found := strings.Cut(path, "/")
	if !found || segment != bucket || rest == "" {
		return "", fmt.Errorf(
			"airplan: URL %q must contain configured bucket %q as its exact path segment",
			rawURL, bucket,
		)
	}
	return rest, nil
}

// baseURLMatches reports whether u is served from the configured
// public_base_url (same host, path under the base's path). HTTP and
// HTTPS are equivalent here because parsing a management target does not
// fetch the supplied URL.
func baseURLMatches(base string, u *url.URL) bool {
	if base == "" {
		return false
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" ||
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
	keyPrefix := ""
	if cfg != nil {
		keyPrefix = cfg.KeyPrefix
	}
	if _, err := uploadDirPrefixForKeyPrefix(key, keyPrefix); err != nil {
		return "", err
	}
	return key, nil
}

// KeyMatchesPrefix reports whether key's random directory is immediately
// beneath keyPrefix. It distinguishes root uploads from uploads under another
// configured prefix (SPEC.md §9).
func KeyMatchesPrefix(key, keyPrefix string) bool {
	key = strings.Trim(key, "/")
	prefix := strings.Trim(keyPrefix, "/")
	rest := key
	if prefix != "" {
		prefixWithSlash := prefix + "/"
		if !strings.HasPrefix(key, prefixWithSlash) {
			return false
		}
		rest = strings.TrimPrefix(key, prefixWithSlash)
	}
	segment, _, _ := strings.Cut(rest, "/")
	return isRandomDir(segment)
}

// uploadDirPrefixForKeyPrefix returns the upload directory when the configured
// key prefix is known. The random directory is always the first segment below
// keyPrefix, so a collection member whose basename also looks random remains
// unambiguous.
func uploadDirPrefixForKeyPrefix(key, keyPrefix string) (string, error) {
	key = strings.Trim(key, "/")
	prefix := strings.Trim(keyPrefix, "/")
	rest := key
	if prefix != "" {
		prefixWithSlash := prefix + "/"
		if !strings.HasPrefix(key, prefixWithSlash) {
			return "", fmt.Errorf(
				"airplan: %q is outside the configured key_prefix %q — "+
					"refusing to touch it", key, keyPrefix)
		}
		rest = strings.TrimPrefix(key, prefixWithSlash)
	}
	segment, _, _ := strings.Cut(rest, "/")
	if !isRandomDir(segment) {
		if prefix == "" {
			return uploadDirPrefix(key)
		}
		return "", fmt.Errorf(
			"airplan: %q does not look like an airplan key "+
				"(no 26-char base32 directory segment)", key)
	}
	if prefix == "" {
		return segment + "/", nil
	}
	return prefix + "/" + segment + "/", nil
}

// uploadDirPrefix returns an upload's directory prefix
// ("[key_prefix/]<random>/") for one of its object keys — the unit of
// deletion (SPEC.md §9). Call uploadDirPrefixForKeyPrefix when resolving a
// user target because collection member names may themselves look random.
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
