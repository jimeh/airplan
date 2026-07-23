package serverlog

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLevelAndTraceRendering(t *testing.T) {
	for _, test := range []struct {
		value string
		want  slog.Level
	}{
		{value: "", want: slog.LevelInfo},
		{value: "error", want: slog.LevelError},
		{value: "warn", want: slog.LevelWarn},
		{value: "info", want: slog.LevelInfo},
		{value: "debug", want: slog.LevelDebug},
		{value: "trace", want: LevelTrace},
	} {
		got, err := ParseLevel(test.value)
		if err != nil || got != test.want {
			t.Fatalf("ParseLevel(%q) = %v, %v, want %v",
				test.value, got, err, test.want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("ParseLevel accepted an unknown level")
	}

	var output bytes.Buffer
	logger := New(&output, LevelTrace)
	logger.Log(context.Background(), LevelTrace, "trace event")
	if !strings.Contains(output.String(), "level=TRACE") {
		t.Fatalf("trace level not rendered as TRACE: %s", output.String())
	}
}

func TestInfoFilteringAndSafeMCPLogger(t *testing.T) {
	const sentinel = "private-sdk-error-sentinel"
	var output bytes.Buffer
	logger := New(&output, slog.LevelInfo)
	logger.Debug("debug event")
	logger.Log(context.Background(), LevelTrace, "trace event")
	if output.Len() != 0 {
		t.Fatalf("info logger emitted verbose events: %s", output.String())
	}

	output.Reset()
	debugLogger := New(&output, slog.LevelDebug)
	sdkLogger := SafeMCPLogger(debugLogger)
	sdkLogger.Error("server connect failed", "error", sentinel)
	if output.Len() != 0 {
		t.Fatalf("SDK lifecycle event emitted below trace: %s", output.String())
	}

	traceLogger := New(&output, LevelTrace)
	sdkLogger = SafeMCPLogger(traceLogger)
	sdkLogger.Info("server session connected", "session_id", sentinel)
	if !strings.Contains(output.String(), "level=TRACE") ||
		!strings.Contains(output.String(), "event=session_connected") {
		t.Fatalf("known safe MCP lifecycle event missing: %s", output.String())
	}
	if strings.Contains(output.String(), sentinel) ||
		strings.Contains(output.String(), "server session connected") {
		t.Fatalf("safe MCP logger leaked SDK detail: %s", output.String())
	}
}

func TestHTTPMiddlewareLogsOnlySafePath(t *testing.T) {
	const sentinel = "private-capability-url-sentinel"
	var output bytes.Buffer
	logger := New(&output, slog.LevelDebug)
	handler := HTTPMiddleware(
		logger,
		"rest",
		func(*http.Request) string { return "unmatched" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	request := httptest.NewRequest(
		http.MethodGet, "/"+sentinel+"?secret="+sentinel, nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	logs := output.String()
	if !strings.Contains(logs, "path=unmatched") ||
		!strings.Contains(logs, "status=204") {
		t.Fatalf("request metadata missing: %s", logs)
	}
	if strings.Contains(logs, sentinel) {
		t.Fatalf("request log leaked path/query: %s", logs)
	}
	if recorder.Header().Get(RequestIDHeader) == "" {
		t.Fatal("response request ID is empty")
	}
}

func TestNestedRequestIDMiddlewareReusesOneGeneratedID(t *testing.T) {
	var outerID, innerID string
	inner := RequestIDMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		innerID = RequestID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	outer := RequestIDMiddleware(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		outerID = RequestID(r.Context())
		inner.ServeHTTP(w, r)
	}))
	recorder := httptest.NewRecorder()
	outer.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
	)
	returnedID := recorder.Header().Get(RequestIDHeader)
	if outerID == "" || innerID != outerID || returnedID != outerID {
		t.Fatalf("request IDs: outer=%q inner=%q returned=%q",
			outerID, innerID, returnedID)
	}
}

func TestHTTPLoggingIgnoresClientRequestIDAndUnknownMethod(t *testing.T) {
	const (
		requestIDSentinel = "private-token-request-id-sentinel"
		methodSentinel    = "PRIVATE_METHOD_SENTINEL"
	)
	var output bytes.Buffer
	logger := New(&output, slog.LevelDebug)
	handler := HTTPMiddleware(
		logger,
		"rest",
		func(*http.Request) string { return "unmatched" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	request := httptest.NewRequest(methodSentinel, "/ignored", nil)
	request.Header.Set(RequestIDHeader, requestIDSentinel)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	logs := output.String()
	if strings.Contains(logs, requestIDSentinel) ||
		strings.Contains(logs, methodSentinel) {
		t.Fatalf("request log leaked client-controlled metadata: %s", logs)
	}
	if !strings.Contains(logs, "method=unknown") {
		t.Fatalf("unknown method was not normalized: %s", logs)
	}
	returnedID := recorder.Header().Get(RequestIDHeader)
	if returnedID == "" || returnedID == requestIDSentinel {
		t.Fatalf("response request ID = %q", returnedID)
	}
}

func TestRESTSizeLimitReasonIsNeutral(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, slog.LevelDebug)
	handler := HTTPMiddleware(
		logger,
		"rest",
		func(*http.Request) string { return "/api/v1/uploads/documents" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}),
	)
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(
			http.MethodPost, "/api/v1/uploads/documents", nil,
		),
	)
	if !strings.Contains(output.String(), "reason=size_limit") ||
		strings.Contains(output.String(), "reason=body_limit") {
		t.Fatalf("unexpected REST 413 classification: %s", output.String())
	}
}

func TestInfoHTTPLoggingIsQuietExceptServerFailures(t *testing.T) {
	var output bytes.Buffer
	logger := New(&output, slog.LevelInfo)
	status := http.StatusNoContent
	handler := HTTPMiddleware(
		logger,
		"rest",
		func(*http.Request) string { return "/healthz" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}),
	)
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
	)
	if output.Len() != 0 {
		t.Fatalf("successful info request emitted logs: %s", output.String())
	}

	status = http.StatusInternalServerError
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
	)
	if !strings.Contains(output.String(), "level=ERROR") ||
		!strings.Contains(output.String(), "msg=\"request failed\"") {
		t.Fatalf("server failure was not logged: %s", output.String())
	}
}
