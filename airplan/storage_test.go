package airplan

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPublicURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          Config
		key          string
		wantURL      string
		wantFallback bool
	}{
		{
			name: "public base without trailing slash",
			cfg: Config{
				PublicBaseURL: "https://plans.example.com",
				Endpoint:      "https://example.r2.cloudflarestorage.com",
				Bucket:        "ignored",
			},
			key:     "abc/page.html",
			wantURL: "https://plans.example.com/abc/page.html",
		},
		{
			name: "public base with trailing slash",
			cfg: Config{
				PublicBaseURL: "https://plans.example.com/",
				Endpoint:      "https://example.r2.cloudflarestorage.com",
				Bucket:        "ignored",
			},
			key:     "abc/page.html",
			wantURL: "https://plans.example.com/abc/page.html",
		},
		{
			name: "path style fallback",
			cfg: Config{
				Endpoint: "https://example.r2.cloudflarestorage.com/",
				Bucket:   "plans",
			},
			key: "abc/page.html",
			wantURL: "https://example.r2.cloudflarestorage.com/plans/" +
				"abc/page.html",
			wantFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotURL, gotFallback := PublicURL(&tt.cfg, tt.key)
			if gotURL != tt.wantURL {
				t.Fatalf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotFallback != tt.wantFallback {
				t.Fatalf("fallback = %v, want %v", gotFallback, tt.wantFallback)
			}
		})
	}
}

func TestStoragePutHeaders(t *testing.T) {
	var (
		mu       sync.Mutex
		captured capturedRequest
	)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)

			mu.Lock()
			captured = captureRequest(r)
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	err := st.put(context.Background(), object{
		Key:         "random/page.html",
		Body:        []byte("<!doctype html>"),
		ContentType: "text/html; charset=utf-8",
		Metadata: map[string]string{
			"title": "Launch Plan",
		},
	})
	if err != nil {
		t.Fatalf("put returned error: %v", err)
	}

	mu.Lock()
	got := captured
	mu.Unlock()

	if got.path != "/plans/random/page.html" {
		t.Fatalf("path = %q, want %q", got.path, "/plans/random/page.html")
	}
	assertHeader(t, got.header, "Content-Type", "text/html; charset=utf-8")
	assertHeader(
		t,
		got.header,
		"Cache-Control",
		"public, max-age=31536000, immutable",
	)
	assertHeader(t, got.header, "X-Amz-Meta-Title", "Launch Plan")
}

func TestStoragePutErrorMentionsKey(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusInternalServerError)
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	err := st.put(context.Background(), object{
		Key:         "random/error.html",
		Body:        []byte("broken"),
		ContentType: "text/html; charset=utf-8",
	})
	if err == nil {
		t.Fatal("put returned nil error")
	}
	if !strings.Contains(err.Error(), "random/error.html") {
		t.Fatalf("error = %q, want it to mention key", err)
	}
}

type capturedRequest struct {
	path   string
	header http.Header
}

func captureRequest(r *http.Request) capturedRequest {
	return capturedRequest{
		path:   r.URL.Path,
		header: r.Header.Clone(),
	}
}

func newTestStorage(t *testing.T, endpoint string) *storage {
	t.Helper()

	st, err := newStorage(context.Background(), &Config{
		Endpoint:        endpoint,
		Bucket:          "plans",
		AccessKeyID:     "test-access-key-id",
		SecretAccessKey: "test-secret-access-key",
	})
	if err != nil {
		t.Fatalf("newStorage returned error: %v", err)
	}
	return st
}

func assertHeader(
	t *testing.T,
	header http.Header,
	name string,
	want string,
) {
	t.Helper()

	if got := header.Get(name); got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}
