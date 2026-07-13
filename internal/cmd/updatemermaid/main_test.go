package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestUpdateMinimumAge(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		published time.Time
		wantProbe bool
	}{
		{"exactly eligible", now.Add(-72 * time.Hour), true},
		{"one nanosecond too new", now.Add(-72*time.Hour + time.Nanosecond), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withManifest(t, `{"package":"mermaid","version":"11.15.0",`+
				`"released_at":"2026-06-01T00:00:00Z"}`)
			probed := false
			client := &http.Client{Transport: roundTripFunc(
				func(req *http.Request) (*http.Response, error) {
					body := ""
					status := http.StatusOK
					if req.URL.Host == "registry.npmjs.org" {
						body = `{"dist-tags":{"latest":"11.16.0"},"time":{` +
							`"11.15.0":"2026-06-01T00:00:00Z",` +
							`"11.16.0":"` + tc.published.Format(time.RFC3339Nano) + `"}}`
					} else {
						probed = true
						status = http.StatusPartialContent
					}
					return &http.Response{
						StatusCode: status,
						Status:     http.StatusText(status),
						Body:       io.NopCloser(strings.NewReader(body)),
					}, nil
				},
			)}
			if err := update(now, client, true); err != nil {
				t.Fatal(err)
			}
			if probed != tc.wantProbe {
				t.Fatalf("CDN probe = %v, want %v", probed, tc.wantProbe)
			}
		})
	}
}

func TestUpdateFailsClosedOnMalformedTime(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	withManifest(t, `{"package":"mermaid","version":"11.15.0",`+
		`"released_at":"2026-06-01T00:00:00Z"}`)
	client := registryClient(`{"dist-tags":{"latest":"11.16.0"},` +
		`"time":{"11.16.0":"not-a-time"}}`)
	if err := update(now, client, true); err == nil {
		t.Fatal("malformed registry time was accepted")
	}
}

func TestUpdateSelectsEligibleReleaseBehindTooNewLatest(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	withManifest(t, `{"package":"mermaid","version":"11.14.0",`+
		`"released_at":"2026-06-01T00:00:00Z"}`)
	probedURL := ""
	client := &http.Client{Transport: roundTripFunc(
		func(req *http.Request) (*http.Response, error) {
			body := ""
			status := http.StatusOK
			if req.URL.Host == "registry.npmjs.org" {
				body = `{"dist-tags":{"latest":"11.16.0"},"time":{` +
					`"11.14.0":"2026-06-01T00:00:00Z",` +
					`"11.15.0":"` + now.Add(-72*time.Hour).Format(time.RFC3339Nano) + `",` +
					`"11.16.0":"` + now.Add(-24*time.Hour).Format(time.RFC3339Nano) + `"}}`
			} else {
				probedURL = req.URL.String()
				status = http.StatusPartialContent
			}
			return &http.Response{
				StatusCode: status,
				Status:     http.StatusText(status),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	)}
	if err := update(now, client, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probedURL, "mermaid@11.15.0/") {
		t.Fatalf("probed URL = %q, want eligible 11.15.0", probedURL)
	}
}

func TestUpdateSelectsEligibleReleaseWithinCurrentMajor(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	withManifest(t, `{"package":"mermaid","version":"11.14.0",`+
		`"released_at":"2026-06-01T00:00:00Z"}`)
	probedURL := ""
	client := &http.Client{Transport: roundTripFunc(
		func(req *http.Request) (*http.Response, error) {
			body := ""
			status := http.StatusOK
			if req.URL.Host == "registry.npmjs.org" {
				body = `{"dist-tags":{"latest":"12.0.0"},"time":{` +
					`"11.14.0":"2026-06-01T00:00:00Z",` +
					`"11.15.0":"` +
					now.Add(-72*time.Hour).Format(time.RFC3339Nano) + `",` +
					`"12.0.0":"` +
					now.Add(-72*time.Hour).Format(time.RFC3339Nano) + `"}}`
			} else {
				probedURL = req.URL.String()
				status = http.StatusPartialContent
			}
			return &http.Response{
				StatusCode: status,
				Status:     http.StatusText(status),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	)}
	if err := update(now, client, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probedURL, "mermaid@11.15.0/") {
		t.Fatalf("probed URL = %q, want same-major 11.15.0", probedURL)
	}
}

func TestRestoreFilesRestoresGoldenDirectory(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	originalPath := filepath.Join(
		"airplan", "testdata", "TestRenderMarkdownGolden", "original.html",
	)
	newPath := filepath.Join(
		"airplan", "testdata", "TestRenderMarkdownGolden", "new.html",
	)
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := restoreFiles(map[string][]byte{
		originalPath: []byte("original"),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("restored contents = %q, want %q", got, "original")
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("new golden still exists: %v", err)
	}
}

func TestRestoreFilesReportsFailures(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	goldenDir := filepath.Join(
		"airplan", "testdata", "TestRenderMarkdownGolden",
	)
	removePath := filepath.Join(goldenDir, "new.html")
	if err := os.MkdirAll(removePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(removePath, "child"), []byte("data"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	restorePath := filepath.Join(goldenDir, "original")
	if err := os.MkdirAll(restorePath, 0o755); err != nil {
		t.Fatal(err)
	}

	err = restoreFiles(map[string][]byte{restorePath: []byte("original")})
	if err == nil {
		t.Fatal("restoreFiles succeeded despite remove and restore failures")
	}
	for _, path := range []string{removePath, restorePath} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not mention %q", err, path)
		}
	}
}

func withManifest(t *testing.T, contents string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "deps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestPath),
		[]byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	trackedPaths := []string{
		"airplan/mermaid_generated.go",
		"airplan/testdata/TestRenderMarkdownGolden/basic.html",
	}
	for _, path := range trackedPaths {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func registryClient(body string) *http.Client {
	return &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK, Status: "OK",
				Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	)}
}
