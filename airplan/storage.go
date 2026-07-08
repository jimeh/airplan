package airplan

import (
	"context"
	"errors"
)

// storage wraps the S3-compatible client used for uploads (SPEC.md §5).
// It stays unexported: consumers upload through Client.
type storage struct{}

// newStorage builds the S3 client from cfg: custom endpoint support,
// path-style addressing when a custom endpoint is set (R2, MinIO),
// region defaulting to "auto", and request checksum calculation set to
// when-required for R2 compatibility. Credentials fall back per
// SPEC.md §7: explicit config values, else the standard AWS chain.
func newStorage(ctx context.Context, cfg *Config) (*storage, error) {
	return nil, errors.New("airplan: newStorage not implemented")
}

// object is a single object to upload.
type object struct {
	Key         string
	Body        []byte
	ContentType string
	// Metadata becomes x-amz-meta-* headers (e.g. "title" for
	// list --remote titles, SPEC.md §5).
	Metadata map[string]string
}

// put uploads one object with
// Cache-Control: public, max-age=31536000, immutable (SPEC.md §5).
func (s *storage) put(ctx context.Context, obj object) error {
	return errors.New("airplan: storage.put not implemented")
}

// PublicURL assembles the public URL for an object key:
// <public_base_url>/<key> when public_base_url is set, else path-style
// <endpoint>/<bucket>/<key> with fallback=true so the caller can warn
// that the URL may not be publicly reachable (SPEC.md §7, §8).
func PublicURL(cfg *Config, key string) (url string, fallback bool) {
	return "", false
}
