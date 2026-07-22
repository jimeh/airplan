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

type stubFetcher map[string][]byte

func (f stubFetcher) Fetch(
	_ context.Context, objectURL string,
) ([]byte, error) {
	body, ok := f[objectURL]
	if !ok {
		return nil, fmt.Errorf("no stored object for %s", objectURL)
	}
	return append([]byte(nil), body...), nil
}

func TestDemoObjectURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		page string
		file string
		want string
	}{
		{
			name: "page unchanged",
			page: "https://demo.example/abcdefghijklmnopqrstuvwxyz/index.html",
			file: "index.html",
			want: "https://demo.example/abcdefghijklmnopqrstuvwxyz/index.html",
		},
		{
			name: "escaped prefix",
			page: "https://demo.example/team%20plans/" +
				"abcdefghijklmnopqrstuvwxyz/index.html",
			file: "shot one.svg",
			want: "https://demo.example/team%20plans/" +
				"abcdefghijklmnopqrstuvwxyz/shot%20one.svg",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := demoObjectURL(tt.page, "index.html", tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("demoObjectURL() = %q, want %q", got, tt.want)
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

	page, err := fetch.Fetch(ctx, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(page, objects[dir+"/example.html"]) {
		t.Fatalf("page = %q", page)
	}
	sourceURL := "https://demo.example/" + dir + "/example.md"
	source, err := fetch.Fetch(ctx, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, objects[dir+"/example.md"]) {
		t.Fatalf("source = %q", source)
	}
}

func TestRepositoryDemosMatchReadmeAndFixtures(t *testing.T) {
	t.Chdir(filepath.Clean(filepath.Join("..", "..", "..")))
	readme, err := os.ReadFile(defaultReadmePath)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := demoURLs(readme, repositoryDemos)
	if err != nil {
		t.Fatal(err)
	}
	for _, demo := range repositoryDemos {
		if err := validatePageURL(urls[demo.reference], demo); err != nil {
			t.Fatalf("%s URL: %v", demo.id, err)
		}
		content, err := loadDemoContent(demo)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(content.objects), len(demo.inputPaths)+1; got != want {
			t.Fatalf("%s objects = %d, want %d", demo.id, got, want)
		}
	}
}

func TestUpdateReadmeKeepsFreshCurrentDemo(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		current: []byte("page"),
		"https://demo.example/current/example.md": []byte("source"),
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
		current: []byte("page"),
		"https://demo.example/current/example.md": []byte("source"),
		candidate: []byte("page"),
		"https://demo.example/candidate/example.md": []byte("source"),
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
		current: []byte("old page"),
		"https://demo.example/current/example.md": []byte("source"),
		candidate: []byte("page"),
		"https://demo.example/candidate/example.md": []byte("source"),
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
		current: []byte("old page"),
		"https://demo.example/current/example.md": []byte("source"),
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
		current: []byte("page"),
		"https://demo.example/current/example.md": []byte("old source"),
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

func TestUpdateReadmeUploadsWhenCollectionMemberIsStale(t *testing.T) {
	dir := t.TempDir()
	goldenPath := filepath.Join(dir, "index.html")
	imagePath := filepath.Join(dir, "shot.svg")
	notesPath := filepath.Join(dir, "notes.txt")
	for path, body := range map[string]string{
		goldenPath: "overview", imagePath: "image", notesPath: "notes",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixture := demoFixture{
		dir: dir, readmePath: filepath.Join(dir, "README.md"),
		entry: demo{
			id: "collection", reference: "demo-collection",
			inputPaths: []string{imagePath, notesPath},
			goldenPath: goldenPath, pageName: "index.html",
		},
	}
	current := "https://demo.example/current/index.html"
	fixture.writeReadme(t, current)
	fetcher := stubFetcher{
		current:                                  []byte("overview"),
		"https://demo.example/current/shot.svg":  []byte("old image"),
		"https://demo.example/current/notes.txt": []byte("notes"),
	}
	want := "https://demo.example/refreshed/index.html"
	published := 0
	err := fixture.update(t, fetcher, "", false, publishFunc(
		func(_ context.Context, got demo) (string, error) {
			published++
			if len(got.inputPaths) != 2 {
				t.Fatalf("published inputs = %v", got.inputPaths)
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

func TestUpdateReadmeAllowsCandidateWithoutNewReference(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	current := "https://demo.example/current/example.html"
	fetcher := stubFetcher{
		current: []byte("page"),
		"https://demo.example/current/example.md": []byte("source"),
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
		current: []byte("page"),
		"https://demo.example/current/example.md": []byte("source"),
		candidate: []byte("page"),
		"https://demo.example/candidate/example.md": []byte("source"),
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
			inputPaths: []string{sourcePath}, goldenPath: goldenPath,
			pageName: "example.html",
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
