// Package serverlog provides the deliberately narrow logging surface used by
// the self-hosted HTTP server.
package serverlog

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// LevelTrace is the most verbose server log level.
const LevelTrace = slog.LevelDebug - 4

const RequestIDHeader = "X-Request-Id"

type requestIDContextKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// ParseLevel parses one documented server log level.
func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "error":
		return slog.LevelError, nil
	case "warn":
		return slog.LevelWarn, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "trace":
		return LevelTrace, nil
	default:
		return 0, fmt.Errorf(
			"airplan: invalid server log level %q "+
				"(want error, warn, info, debug, or trace)",
			value,
		)
	}
}

// New constructs the text logger used by airplan serve.
func New(writer io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key != slog.LevelKey {
				return attr
			}
			logLevel, ok := attr.Value.Any().(slog.Level)
			if ok && logLevel == LevelTrace {
				return slog.String(slog.LevelKey, "TRACE")
			}
			return attr
		},
	}))
}

// SafeMCPLogger adapts SDK logs to the server policy. SDK messages and
// attributes may contain protocol data, so only fixed event categories pass.
func SafeMCPLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return nil
	}
	return slog.New(&safeMCPHandler{base: logger.Handler()})
}

type safeMCPHandler struct {
	base slog.Handler
}

func (h *safeMCPHandler) Enabled(ctx context.Context, _ slog.Level) bool {
	return h.base.Enabled(ctx, LevelTrace)
}

func (h *safeMCPHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(
		record.Time, LevelTrace, "mcp sdk event", record.PC,
	)
	safe.AddAttrs(
		slog.String("event", safeMCPEvent(record.Message)),
		slog.String("request_id", RequestID(ctx)),
	)
	return h.base.Handle(ctx, safe)
}

func (h *safeMCPHandler) WithAttrs([]slog.Attr) slog.Handler {
	return &safeMCPHandler{base: h.base}
}

func (h *safeMCPHandler) WithGroup(string) slog.Handler {
	return &safeMCPHandler{base: h.base}
}

func safeMCPEvent(message string) string {
	switch message {
	case "server run start":
		return "run_started"
	case "server connecting":
		return "connecting"
	case "server session connected":
		return "session_connected"
	case "session initialized":
		return "session_initialized"
	case "server session disconnected":
		return "session_disconnected"
	case "server session ended":
		return "session_ended"
	case "initialized before initialize":
		return "initialization_order_invalid"
	case "duplicate initialized notification":
		return "initialization_duplicate"
	default:
		return "protocol_event"
	}
}

// RequestID returns the constrained request identifier from ctx.
func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

// RequestIDMiddleware ensures each request and response has a safe identifier.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := RequestID(r.Context())
		if !validRequestID.MatchString(requestID) {
			requestID = newRequestID()
		}
		w.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPMiddleware logs safe request lifecycle metadata.
func HTTPMiddleware(
	logger *slog.Logger,
	transport string,
	path func(*http.Request) string,
	next http.Handler,
) http.Handler {
	if logger == nil {
		return RequestIDMiddleware(next)
	}
	return RequestIDMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		requestPath := path(r)
		requestID := RequestID(r.Context())
		logger.Log(r.Context(), LevelTrace, "request started",
			"transport", transport,
			"method", safeHTTPMethod(r.Method),
			"path", requestPath,
			"request_id", requestID,
		)
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		duration := time.Since(started)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		if status == http.StatusRequestEntityTooLarge && transport != "mcp" {
			logger.DebugContext(r.Context(), "request rejected",
				"transport", transport,
				"reason", "size_limit",
				"request_id", requestID,
			)
		}
		if status >= http.StatusInternalServerError {
			logger.ErrorContext(r.Context(), "request failed",
				"transport", transport,
				"method", safeHTTPMethod(r.Method),
				"path", requestPath,
				"status", status,
				"request_id", requestID,
			)
		}
		logger.DebugContext(r.Context(), "request completed",
			"transport", transport,
			"method", safeHTTPMethod(r.Method),
			"path", requestPath,
			"status", status,
			"duration", duration,
			"request_id", requestID,
		)
	}))
}

func safeHTTPMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete,
		http.MethodOptions:
		return method
	default:
		return "unknown"
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseRecorder) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *responseRecorder) Hijack() (
	net.Conn, *bufio.ReadWriter, error,
) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return "request-id-unavailable"
}
