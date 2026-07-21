package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
)

type publishFunc func(context.Context, demo) (string, error)

func (f publishFunc) Publish(ctx context.Context, d demo) (string, error) {
	return f(ctx, d)
}

type fetchKey struct {
	url    string
	source bool
}

type stubFetcher map[fetchKey][]byte

func (f stubFetcher) Fetch(
	_ context.Context, pageURL string, source bool,
) ([]byte, error) {
	body, ok := f[fetchKey{url: pageURL, source: source}]
	if !ok {
		return nil, fmt.Errorf("no stored object for %s (source=%t)",
			pageURL, source)
	}
	return append([]byte(nil), body...), nil
}

func TestUploadDirectoryURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "root prefix",
			in:   "https://demo.example/abcdefghijklmnopqrstuvwxyz/example.html",
			want: "https://demo.example/abcdefghijklmnopqrstuvwxyz",
		},
		{
			name: "escaped prefix",
			in: "https://demo.example/team%20plans/" +
				"abcdefghijklmnopqrstuvwxyz/example.html",
			want: "https://demo.example/team%20plans/abcdefghijklmnopqrstuvwxyz",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := uploadDirectoryURL(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("uploadDirectoryURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAirplanFetcherUsesRealGetUploadSelection(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	marker, err := airplan.EncodeUploadMarker(airplan.UploadMarker{
		Schema: airplan.MarkerSchema, Version: 1,
		Directory: dir,
		CreatedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
		Format:    "md", Page: "example.html", Source: "example.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string][]byte{
		dir + "/" + airplan.MarkerFilename: marker,
		dir + "/example.html":              []byte("page bytes\x00"),
		dir + "/example.md":                []byte("source bytes\n"),
	}
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			key := strings.TrimPrefix(r.URL.Path, "/plans/")
			body, ok := objects[key]
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w,
					`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
				return
			}
			_, _ = w.Write(body)
		},
	))
	t.Cleanup(server.Close)

	ctx := context.Background()
	client, err := airplan.New(ctx, &airplan.Config{
		Endpoint: server.URL, Bucket: "plans",
		AccessKeyID: "test", SecretAccessKey: "test",
		PublicBaseURL: "https://demo.example", DisableManifest: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	fetch := airplanFetcher{client: client}
	pageURL := "https://demo.example/" + dir + "/example.html"

	page, err := fetch.Fetch(ctx, pageURL, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(page, objects[dir+"/example.html"]) {
		t.Fatalf("page = %q", page)
	}
	source, err := fetch.Fetch(ctx, pageURL, true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, objects[dir+"/example.md"]) {
		t.Fatalf("source = %q", source)
	}
}

func TestUpdateReadmeKeepsFreshCurrentDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		{url: current}:               []byte("page"),
		{url: current, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	published := false
	err := fixture.update(t, fetcher, "", false, publishFunc(
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
	fixture.wantURL(t, current)
}

func TestUpdateReadmePrefersFreshCurrentDemoOverFreshCandidate(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	candidate := "https://demo.example/candidate/example.html"
	fetcher := stubFetcher{
		{url: current}:                 []byte("page"),
		{url: current, source: true}:   []byte("source"),
		{url: candidate}:               []byte("page"),
		{url: candidate, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	writeDemoReadme(t, candidatePath, fixture.entry.reference, candidate)
	err := fixture.update(t, fetcher, candidatePath, false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh demo was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, current)
}

func TestUpdateReadmeReusesFreshCandidate(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	candidate := "https://demo.example/candidate/example.html"
	fetcher := stubFetcher{
		{url: current}:                 []byte("old page"),
		{url: current, source: true}:   []byte("source"),
		{url: candidate}:               []byte("page"),
		{url: candidate, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	writeDemoReadme(t, candidatePath, fixture.entry.reference, candidate)
	err := fixture.update(t, fetcher, candidatePath, false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh candidate was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, candidate)
}

func TestUpdateReadmeUploadsStaleDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		{url: current}:               []byte("old page"),
		{url: current, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	want := "https://demo.example/new/example.html"
	published := 0
	err := fixture.update(t, fetcher, "", false, publishFunc(
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
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		{url: current}:               []byte("page"),
		{url: current, source: true}: []byte("old source"),
	}
	fixture.writeReadme(t, current)
	want := "https://demo.example/source-refresh/example.html"
	published := 0
	err := fixture.update(t, fetcher, "", false, publishFunc(
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
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		{url: current}:               []byte("page"),
		{url: current, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	if err := os.WriteFile(candidatePath, []byte("Older candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := fixture.update(t, fetcher, candidatePath, false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh current demo was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, current)
}

func TestUpdateReadmeForceUploadsWithFreshCandidate(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	candidate := "https://demo.example/candidate/example.html"
	fetcher := stubFetcher{
		{url: current}:                 []byte("page"),
		{url: current, source: true}:   []byte("source"),
		{url: candidate}:               []byte("page"),
		{url: candidate, source: true}: []byte("source"),
	}
	fixture.writeReadme(t, current)
	candidatePath := filepath.Join(fixture.dir, "candidate.md")
	writeDemoReadme(t, candidatePath, fixture.entry.reference, candidate)
	want := "https://demo.example/forced/example.html"
	err := fixture.update(t, fetcher, candidatePath, true, publishFunc(
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
	fetch fetcher,
	candidatePath string,
	force bool,
	publisher publisher,
) error {
	t.Helper()
	return updateReadme(
		context.Background(), fetch, publisher,
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
