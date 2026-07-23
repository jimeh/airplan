package httpapi

import (
	"context"
	"net/http"

	"github.com/jimeh/airplan/internal/serverlog"
)

const requestIDHeader = serverlog.RequestIDHeader

// RequestID returns the constrained request identifier from ctx.
func RequestID(ctx context.Context) string {
	return serverlog.RequestID(ctx)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return serverlog.RequestIDMiddleware(next)
}
