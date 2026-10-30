package httpapi

import "context"

// MaxMetadataBytes is the protocol limit for one multipart metadata part. It
// equals the 4 MiB ownership-marker ceiling because upload metadata declares
// the same inventory as the marker (SPEC.md §11).
const MaxMetadataBytes = int64(4 << 20)

// MultipartPartOverheadBytes bounds the framing of one multipart file part:
// its boundary line, Content-Disposition with an escaped filename, and
// Content-Type header.
const MultipartPartOverheadBytes = int64(8 << 10)

// baseBodyOverheadBytes covers the metadata part and request framing that do
// not scale with the number of file parts.
const baseBodyOverheadBytes = int64(16 << 20)

// RequestBodyOverhead returns the request-body allowance beyond declared
// payload bytes for an upload with at most maxItems file parts.
func RequestBodyOverhead(maxItems int) int64 {
	return baseBodyOverheadBytes + int64(maxItems)*MultipartPartOverheadBytes
}

type generatedPageLimitContextKey struct{}

type itemLimitContextKey struct{}

func withGeneratedPageLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, generatedPageLimitContextKey{}, limit)
}

// GeneratedPageLimit returns the server's effective generated-HTML ceiling
// for the current document operation. It is zero outside the HTTP server.
func GeneratedPageLimit(ctx context.Context) int64 {
	limit, _ := ctx.Value(generatedPageLimitContextKey{}).(int64)
	return limit
}

func withItemLimit(ctx context.Context, limit int) context.Context {
	return context.WithValue(ctx, itemLimitContextKey{}, limit)
}

// ItemLimit returns the server's file-count limit for the current document or
// collection operation. It is zero outside the HTTP server.
func ItemLimit(ctx context.Context) int {
	limit, _ := ctx.Value(itemLimitContextKey{}).(int)
	return limit
}
