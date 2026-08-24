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
	"reflect"
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
		{
			name: "nested escaped object",
			page: "https://demo.example/team%20plans/" +
				"abcdefghijklmnopqrstuvwxyz/index.html",
			file: "docs/design notes.html",
			want: "https://demo.example/team%20plans/" +
				"abcdefghijklmnopqrstuvwxyz/docs/design%20notes.html",
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

func TestDemoObjectURLRejectsUnsafeObjectName(t *testing.T) {
	_, err := demoObjectURL(
		"https://demo.example/abcdefghijklmnopqrstuvwxyz/index.html",
		"index.html", "../outside.txt",
	)
	if err == nil || !strings.Contains(err.Error(), "invalid object name") {
		t.Fatalf("error = %v, want invalid object name", err)
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
		if got, want := len(content.objects), len(demo.objects); got != want {
			t.Fatalf("%s objects = %d, want %d", demo.id, got, want)
		}
		if demo.reference != "airplan-demo-document-bundle" {
			continue
		}
		rendered := renderBundleDemoObjects(t, demo)
		configuredNames := make([]string, len(content.objects))
		renderedNames := make([]string, len(rendered))
		for index := range content.objects {
			configuredNames[index] = content.objects[index].name
		}
		for index := range rendered {
			renderedNames[index] = rendered[index].name
		}
		if !reflect.DeepEqual(configuredNames, renderedNames) {
			t.Fatalf(
				"%s configured objects = %v, rendered objects = %v",
				demo.id, configuredNames, renderedNames,
			)
		}
		for index := range content.objects {
			if !bytes.Equal(content.objects[index].body, rendered[index].body) {
				t.Fatalf(
					"%s object %q fixture bytes differ from renderer output",
					demo.id, content.objects[index].name,
				)
			}
		}
	}
}

func renderBundleDemoObjects(t *testing.T, d demo) []demoObject {
	t.Helper()
	plan, err := airplan.PlanLocalPaths(airplan.LocalPathPlanOptions{
		Paths: d.inputPaths,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != airplan.UploadKindDocument || len(plan.PagePaths) == 0 ||
		len(plan.AssetPaths) == 0 {
		t.Fatalf("%s input plan is not a document bundle: %+v", d.id, plan)
	}
	root := filepath.Dir(plan.Entrypoint)
	read := func(filename string) ([]byte, string) {
		t.Helper()
		body, readErr := os.ReadFile(filename)
		if readErr != nil {
			t.Fatal(readErr)
		}
		logical, relErr := filepath.Rel(root, filename)
		if relErr != nil {
			t.Fatal(relErr)
		}
		return body, filepath.ToSlash(logical)
	}
	entryBody, entryPath := read(plan.Entrypoint)
	input := airplan.DocumentInput{
		Entry: airplan.PageInput{
			Reader: bytes.NewReader(entryBody), Path: entryPath,
		},
		Title: demoArgValue(d, "--title"),
		Slug:  demoArgValue(d, "--slug"),
	}
	input.Entry.Title = input.Title
	for _, filename := range plan.PagePaths {
		body, logical := read(filename)
		input.Pages = append(input.Pages, airplan.PageInput{
			Reader: bytes.NewReader(body), Path: logical,
		})
	}
	assetBodies := make(map[string][]byte, len(plan.AssetPaths))
	for _, filename := range plan.AssetPaths {
		body, logical := read(filename)
		assetBodies[logical] = body
		input.Assets = append(input.Assets, airplan.AssetInput{
			Reader: bytes.NewReader(body), Path: logical, Size: int64(len(body)),
		})
	}
	bundle, err := airplan.RenderDocument(
		context.Background(), input,
		airplan.DocumentRenderOptions{RenderInputOptions: airplan.RenderInputOptions{
			IncludeSource: true,
			Repository:    demoArgValue(d, "--repo"),
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	objects := make([]demoObject, 0, len(bundle.Pages)*2+len(bundle.Assets))
	for _, page := range bundle.Pages {
		objects = append(objects, demoObject{name: page.PagePath, body: page.HTML})
		if page.SourcePath != "" {
			objects = append(objects, demoObject{
				name: page.SourcePath, body: page.Source,
			})
		}
	}
	for _, asset := range bundle.Assets {
		objects = append(objects, demoObject{
			name: asset.Path, body: assetBodies[asset.Path],
		})
	}
	return objects
}

func demoArgValue(d demo, name string) string {
	for index := 0; index+1 < len(d.args); index++ {
		if d.args[index] == name {
			return d.args[index+1]
		}
	}
	return ""
}

func TestValidatePublishedResultRejectsMissingSource(t *testing.T) {
	fixture := newDemoFixture(t, "page", "source")
	pageURL := "https://demo.example/upload/example.html"
	err := validatePublishedResult(publishedResult{
		URL: pageURL,
		Pages: []airplan.PageResult{{
			Path: "example.md", URL: pageURL,
		}},
	}, fixture.entry)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want missing object inventory", err)
	}
}

func TestValidatePublishedResultAcceptsSupportedUploadShapes(t *testing.T) {
	t.Run("document", func(t *testing.T) {
		fixture := newDemoFixture(t, "page", "source")
		pageURL := "https://demo.example/upload/example.html"
		err := validatePublishedResult(publishedResult{
			URL: pageURL,
			Pages: []airplan.PageResult{{
				Path: "example.md", URL: pageURL,
				SourceURL: "https://demo.example/upload/example.md",
			}},
		}, fixture.entry)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("collection", func(t *testing.T) {
		dir := t.TempDir()
		page := writeFixtureFile(t, dir, "index.html", "overview")
		image := writeFixtureFile(t, dir, "shot.svg", "image")
		d := demo{
			id: "collection", pageName: "index.html",
			objects: []expectedDemoObject{
				{name: "index.html", path: page},
				{name: "shot.svg", path: image},
			},
		}
		pageURL := "https://demo.example/upload/index.html"
		err := validatePublishedResult(publishedResult{
			URL: pageURL,
			Files: []airplan.FileResult{{
				Name: "shot.svg", URL: "https://demo.example/upload/shot.svg",
			}},
		}, d)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("document bundle", func(t *testing.T) {
		fixture := newBundleDemoFixture(t)
		pageURL := "https://demo.example/upload/implementation-plan.html"
		result := publishedBundleResult(t, fixture.entry, pageURL)
		if err := validatePublishedResult(result, fixture.entry); err != nil {
			t.Fatal(err)
		}
	})
}

func TestValidatePublishedResultRejectsInvalidInventories(t *testing.T) {
	fixture := newBundleDemoFixture(t)
	pageURL := "https://demo.example/upload/implementation-plan.html"

	tests := []struct {
		name   string
		mutate func(*publishedResult)
		want   string
	}{
		{
			name: "missing nested page",
			mutate: func(result *publishedResult) {
				result.Pages = append(result.Pages[:1], result.Pages[2:]...)
			},
			want: "missing object",
		},
		{
			name: "unexpected object",
			mutate: func(result *publishedResult) {
				result.Assets[0].URL = "https://demo.example/upload/images/other.svg"
			},
			want: "unexpected object URL",
		},
		{
			name: "duplicate object",
			mutate: func(result *publishedResult) {
				result.Assets = append(result.Assets, result.Assets[0])
			},
			want: "duplicate object URL",
		},
		{
			name: "cross directory",
			mutate: func(result *publishedResult) {
				result.Assets[0].URL = "https://demo.example/other/request-flow.svg"
			},
			want: "outside the upload directory",
		},
		{
			name: "malformed URL",
			mutate: func(result *publishedResult) {
				result.Assets[0].URL = "https://demo.example/%zz"
			},
			want: "malformed published object URL",
		},
		{
			name: "mixed shapes",
			mutate: func(result *publishedResult) {
				result.Files = []airplan.FileResult{{URL: result.Assets[0].URL}}
			},
			want: "mixes collection and document objects",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := publishedBundleResult(t, fixture.entry, pageURL)
			test.mutate(&result)
			err := validatePublishedResult(result, fixture.entry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadDemoContentRejectsInvalidObjectInventory(t *testing.T) {
	file := writeFixtureFile(t, t.TempDir(), "body", "body")
	for _, test := range []struct {
		name    string
		objects []expectedDemoObject
		want    string
	}{
		{
			name: "duplicate",
			objects: []expectedDemoObject{
				{name: "index.html", path: file},
				{name: "index.html", path: file},
			},
			want: "duplicate object name",
		},
		{
			name: "escaping",
			objects: []expectedDemoObject{
				{name: "index.html", path: file},
				{name: "../outside.txt", path: file},
			},
			want: "invalid object name",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadDemoContent(demo{
				id: "invalid", pageName: "index.html", objects: test.objects,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
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

func TestUpdateReadmeKeepsFreshCompleteBundle(t *testing.T) {
	fixture := newBundleDemoFixture(t)
	current := "https://demo.example/current/implementation-plan.html"
	fixture.writeReadme(t, current)
	fetcher := freshDemoFetcher(t, fixture.entry, current)
	err := fixture.update(t, fetcher, "", false, publishFunc(
		func(context.Context, demo) (string, error) {
			t.Fatal("fresh bundle was uploaded again")
			return "", nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wantURL(t, current)
}

func TestUpdateReadmeUploadsWhenBundleObjectIsStale(t *testing.T) {
	for _, objectName := range []string{
		"docs/design.html", "docs/design.md", "images/request-flow.svg",
	} {
		t.Run(objectName, func(t *testing.T) {
			fixture := newBundleDemoFixture(t)
			current := "https://demo.example/current/implementation-plan.html"
			fixture.writeReadme(t, current)
			fetcher := freshDemoFetcher(t, fixture.entry, current)
			staleURL, err := demoObjectURL(
				current, fixture.entry.pageName, objectName,
			)
			if err != nil {
				t.Fatal(err)
			}
			fetcher[staleURL] = []byte("stale")
			want := "https://demo.example/refreshed/implementation-plan.html"
			published := 0
			err = fixture.update(t, fetcher, "", false, publishFunc(
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
		})
	}
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
			pageName:   "index.html",
			objects: []expectedDemoObject{
				{name: "index.html", path: goldenPath},
				{name: "shot.svg", path: imagePath},
				{name: "notes.txt", path: notesPath},
			},
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

func TestUpdateReadmeReusesExistingCandidateWhenItLacksNewReference(
	t *testing.T,
) {
	existing := newDemoFixture(t, "existing page", "existing source")
	newDemo := newDemoFixture(t, "new page", "new source")
	newDemo.entry.id = "new demo"
	newDemo.entry.reference = "demo-new"

	currentExisting := "https://demo.example/current-existing/example.html"
	currentNew := "https://demo.example/current-new/example.html"
	candidateExisting := "https://demo.example/candidate-existing/example.html"
	freshNew := "https://demo.example/fresh-new/example.html"
	readme := fmt.Sprintf(
		"[Existing][%s]\n[New][%s]\n\n[%s]: %s\n[%s]: %s\n",
		existing.entry.reference, newDemo.entry.reference,
		existing.entry.reference, currentExisting,
		newDemo.entry.reference, currentNew,
	)
	readmePath := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(readmePath, []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(t.TempDir(), "candidate.md")
	writeDemoReadme(
		t, candidatePath, existing.entry.reference, candidateExisting,
	)
	fetcher := freshDemoFetcher(t, existing.entry, candidateExisting)
	published := 0
	err := updateReadme(
		context.Background(), fetcher,
		publishFunc(func(_ context.Context, got demo) (string, error) {
			published++
			if got.reference != newDemo.entry.reference {
				t.Fatalf("published %q, want new demo", got.reference)
			}
			return freshNew, nil
		}),
		[]demo{existing.entry, newDemo.entry}, readmePath, candidatePath, false,
		io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatalf("published %d times, want 1", published)
	}
	updated, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	urls, err := demoURLs(updated, []demo{existing.entry, newDemo.entry})
	if err != nil {
		t.Fatal(err)
	}
	if urls[existing.entry.reference] != candidateExisting ||
		urls[newDemo.entry.reference] != freshNew {
		t.Fatalf("updated URLs = %v", urls)
	}
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
			inputPaths: []string{sourcePath},
			pageName:   "example.html",
			objects: []expectedDemoObject{
				{name: "example.html", path: goldenPath},
				{name: "example.md", path: sourcePath},
			},
		},
	}
}

func newBundleDemoFixture(t *testing.T) demoFixture {
	t.Helper()
	dir := t.TempDir()
	objects := []struct {
		name string
		body string
	}{
		{name: "implementation-plan.html", body: "entry page"},
		{name: "implementation-plan.md", body: "entry source"},
		{name: "docs/design.html", body: "design page"},
		{name: "docs/design.md", body: "design source"},
		{name: "examples/server.go.html", body: "server page"},
		{name: "examples/server.go", body: "server source"},
		{name: "images/request-flow.svg", body: "flow asset"},
	}
	d := demo{
		id: "bundle", reference: "demo-bundle",
		inputPaths: []string{
			filepath.Join(dir, "implementation-plan.md"),
			filepath.Join(dir, "docs", "design.md"),
			filepath.Join(dir, "examples", "server.go"),
			filepath.Join(dir, "images", "request-flow.svg"),
		},
		pageName: "implementation-plan.html",
	}
	for _, object := range objects {
		objectPath := writeFixtureFile(t, dir, object.name, object.body)
		d.objects = append(d.objects, expectedDemoObject{
			name: object.name, path: objectPath,
		})
	}
	return demoFixture{
		dir: dir, readmePath: filepath.Join(dir, "README.md"), entry: d,
	}
}

func freshDemoFetcher(t *testing.T, d demo, pageURL string) stubFetcher {
	t.Helper()
	content, err := loadDemoContent(d)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := make(stubFetcher, len(content.objects))
	for _, object := range content.objects {
		objectURL, err := demoObjectURL(pageURL, d.pageName, object.name)
		if err != nil {
			t.Fatal(err)
		}
		fetcher[objectURL] = object.body
	}
	return fetcher
}

func publishedBundleResult(
	t *testing.T, d demo, pageURL string,
) publishedResult {
	t.Helper()
	objectURL := func(name string) string {
		t.Helper()
		value, err := demoObjectURL(pageURL, d.pageName, name)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	return publishedResult{
		URL: pageURL,
		Pages: []airplan.PageResult{
			{
				Path: "implementation-plan.md", URL: pageURL,
				SourceURL: objectURL("implementation-plan.md"),
			},
			{
				Path: "docs/design.md", URL: objectURL("docs/design.html"),
				SourceURL: objectURL("docs/design.md"),
			},
			{
				Path:      "examples/server.go",
				URL:       objectURL("examples/server.go.html"),
				SourceURL: objectURL("examples/server.go"),
			},
		},
		Assets: []airplan.AssetResult{{
			Path: "images/request-flow.svg",
			URL:  objectURL("images/request-flow.svg"),
		}},
	}
}

func writeFixtureFile(t *testing.T, root, name, body string) string {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return target
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
