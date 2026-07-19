package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type publishFunc func(context.Context, demo) (string, error)

func (f publishFunc) Publish(ctx context.Context, d demo) (string, error) {
	return f(ctx, d)
}

func TestUpdateReadmeKeepsFreshCurrentDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html": "page",
		"/current/example.md":   "source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	published := false
	err := fixture.update(t, server.Client(), "", false, publishFunc(
		func(context.Context, demo) (string, error) {
			published = true
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if published {
		t.Fatal("fresh demo was uploaded again")
	}
	fixture.wantURL(t, server.URL+"/current/example.html")
}

func TestUpdateReadmeReusesFreshCandidate(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html":   "old page",
		"/current/example.md":     "source",
		"/candidate/example.html": "page",
		"/candidate/example.md":   "source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	writeDemoReadme(
		t, candidatePath, fixture.entry.reference,
		server.URL+"/candidate/example.html",
	)
	err := fixture.update(t, server.Client(), candidatePath, false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh candidate was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, server.URL+"/candidate/example.html")
}

func TestUpdateReadmeUploadsStaleDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html": "old page",
		"/current/example.md":   "source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	want := "https://demo.example/new/example.html"
	published := 0
	err := fixture.update(t, server.Client(), "", false, publishFunc(
		func(_ context.Context, got demo) (string, error) {
			published++
			if got.id != fixture.entry.id {
				t.Fatalf("published %q, want %q", got.id, fixture.entry.id)
			}
			return want, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatalf("published %d times, want 1", published)
	}
	fixture.wantURL(t, want)
}

func TestUpdateReadmeUploadsWhenSourceIsStale(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html": "page",
		"/current/example.md":   "old source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	want := "https://demo.example/source-refresh/example.html"
	published := 0
	err := fixture.update(t, server.Client(), "", false, publishFunc(
		func(context.Context, demo) (string, error) {
			published++
			return want, nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatalf("published %d times, want 1", published)
	}
	fixture.wantURL(t, want)
}

func TestUpdateReadmeAllowsCandidateWithoutNewReference(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html": "page",
		"/current/example.md":   "source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	if err := os.WriteFile(candidatePath, []byte("Older candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := fixture.update(t, server.Client(), candidatePath, false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh current demo was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, server.URL+"/current/example.html")
}

func TestUpdateReadmeForceUploadsFreshDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	server := demoServer(t, map[string]string{
		"/current/example.html": "page",
		"/current/example.md":   "source",
	})
	fixture.writeReadme(t, server.URL+"/current/example.html")
	want := "https://demo.example/forced/example.html"
	err := fixture.update(t, server.Client(), "", true, publishFunc(
		func(context.Context, demo) (string, error) { return want, nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, want)
}

type demoFixture struct {
	dir        string
	readmePath string
	entry      demo
}

func newDemoFixture(t *testing.T, pageBody string, sourceBody string) demoFixture {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "example.md")
	goldenPath := filepath.Join(dir, "example.html")
	if err := os.WriteFile(sourcePath, []byte(sourceBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenPath, []byte(pageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return demoFixture{
		dir:        dir,
		readmePath: filepath.Join(dir, "README.md"),
		entry: demo{
			id: "example", reference: "demo-example",
			sourcePath: sourcePath, goldenPath: goldenPath,
			slug: "example",
		},
	}
}

func (f demoFixture) writeReadme(t *testing.T, pageURL string) {
	t.Helper()
	writeDemoReadme(t, f.readmePath, f.entry.reference, pageURL)
}

func (f demoFixture) update(
	t *testing.T,
	client *http.Client,
	candidatePath string,
	force bool,
	publisher publisher,
) error {
	t.Helper()
	return updateReadme(
		context.Background(), client, publisher,
		[]demo{f.entry}, f.readmePath, candidatePath, force, io.Discard,
	)
}

func (f demoFixture) wantURL(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(f.readmePath)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := demoURLs(data, []demo{f.entry})
	if err != nil {
		t.Fatal(err)
	}
	if got := urls[f.entry.reference]; got != want {
		t.Fatalf("demo URL = %q, want %q", got, want)
	}
}

func writeDemoReadme(
	t *testing.T,
	path string,
	reference string,
	pageURL string,
) {
	t.Helper()
	data := fmt.Sprintf("[Example][%s]\n\n[%s]: %s\n",
		reference, reference, pageURL)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func demoServer(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			body, ok := responses[req.URL.Path]
			if !ok {
				http.NotFound(w, req)
				return
			}
			if _, err := io.Copy(w, strings.NewReader(body)); err != nil {
				t.Errorf("write response: %v", err)
			}
		},
	))
	t.Cleanup(server.Close)
	return server
}
