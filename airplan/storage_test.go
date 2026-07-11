package airplan

import (
	"context"
	"errors"
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
		{
			name: "percent encodes each object key segment",
			cfg: Config{
				PublicBaseURL: "https://plans.example.com/base/",
			},
			key: "team/Jiméh plans/abc/plan #1.html",
			wantURL: "https://plans.example.com/base/team/" +
				"Jim%C3%A9h%20plans/abc/plan%20%231.html",
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
		"no-store",
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

func TestStorageGetBytesIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "1234567890")
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	body, err := st.getBytes(context.Background(), "random/.airplan.json", 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "12345" {
		t.Fatalf("body = %q, want one byte past limit", body)
	}
}

func TestStorageGetBytesNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	_, err := st.getBytes(context.Background(), "random/missing", 4)
	if !errors.Is(err, errObjectNotFound) {
		t.Fatalf("error = %v, want errObjectNotFound", err)
	}
}

func TestStorageDeleteMarkerUsesSingleObjectRequest(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			method, path = r.Method, r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	err := st.deleteMarker(context.Background(), "random/.airplan.json")
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/plans/random/.airplan.json" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestStorageDeleteKeysReturnsPerObjectError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0"?>`+
				`<DeleteResult><Error><Key>random/page.html</Key>`+
				`<Code>AccessDenied</Code><Message>denied</Message>`+
				`</Error></DeleteResult>`)
		},
	))
	t.Cleanup(server.Close)

	st := newTestStorage(t, server.URL)
	err := st.deleteKeys(context.Background(), []string{"random/page.html"})
	if err == nil || !strings.Contains(err.Error(), "random/page.html") ||
		!strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v", err)
	}
}

type capturedRequest struct {
	path   string
	header http.Header
	body   []byte
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

func TestNewClientMissingCredentials(t *testing.T) {
	// Force every link of the SDK credential chain to fail fast.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")

	cfg := &Config{
		Endpoint: "http://127.0.0.1:1",
		Bucket:   "plans",
	}
	_, err := New(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected missing-credentials error")
	}
	if !strings.Contains(err.Error(), "no usable credentials") {
		t.Errorf("error = %v", err)
	}
}
