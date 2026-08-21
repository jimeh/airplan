package httpapi

import "context"

type generatedPageLimitContextKey struct{}

func withGeneratedPageLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, generatedPageLimitContextKey{}, limit)
}

// GeneratedPageLimit returns the server's effective generated-HTML ceiling
// for the current document operation. It is zero outside the HTTP server.
func GeneratedPageLimit(ctx context.Context) int64 {
	limit, _ := ctx.Value(generatedPageLimitContextKey{}).(int64)
	return limit
}
