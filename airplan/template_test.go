package airplan

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(`title={{.Title}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tmpl, err := LoadTemplate(path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, TemplateData{Title: "hello"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "title=hello" {
		t.Errorf("output = %q, want %q", got, "title=hello")
	}
}

func TestLoadTemplateMissingFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.html")

	_, err := LoadTemplate(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want it to name %q", err, path)
	}
}

func TestLoadTemplateExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path := filepath.Join(home, ".config", "airplan", "page.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`slug={{.Slug}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	tmpl, err := LoadTemplate("~/.config/airplan/page.html")
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, TemplateData{Slug: "tilde"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "slug=tilde" {
		t.Errorf("output = %q, want %q", got, "slug=tilde")
	}
}

func TestBrokenTemplateFailsMarkdownUploadNotNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.html")
	if err := os.WriteFile(path, []byte(`{{`), 0o600); err != nil {
		t.Fatal(err)
	}

	// New must succeed: templates don't apply to HTML input, so the
	// load failure is deferred to Upload (where markdown/text fail
	// before anything is uploaded, and HTML proceeds with a warning).
	cfg := &Config{
		Endpoint:        "http://127.0.0.1:1",
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		Template:        path,
		DisableManifest: true,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New failed on broken template: %v", err)
	}

	_, err = client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# hi\n"),
		Name:   "plan.md",
	})
	if err == nil {
		t.Fatal("expected markdown upload to fail")
	}
	if !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Errorf("error = %v, want it to name %q", err, filepath.Base(path))
	}
	if !strings.Contains(err.Error(), "parse template") {
		t.Errorf("error = %v, want parse template failure", err)
	}
}

func TestUploadMarkdownUsesCustomTemplate(t *testing.T) {
	var (
		mu   sync.Mutex
		puts []capturedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			cr := captureRequest(r)
			cr.body = body
			puts = append(puts, cr)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	templatePath := filepath.Join(t.TempDir(), "page.html")
	templateBody := strings.Join([]string{
		`<title>{{.Title}}</title>`,
		`<main data-slug="{{.Slug}}">{{.RenderedHTML}}</main>`,
		`<aside data-source="{{.SourcePath}}">` +
			`{{.HighlightedSourceHTML}}</aside>`,
	}, "\n")
	if err := os.WriteFile(
		templatePath, []byte(templateBody), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
		Template:        templatePath,
		DisableManifest: true,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# From Body\n\nparagraph\n"),
		Name:   "notes/original.md",
		Slug:   "custom-slug",
		Title:  "Custom Title",
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Title != "Custom Title" {
		t.Errorf("Title = %q, want %q", res.Title, "Custom Title")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 3 {
		t.Fatalf("got %d puts, want 3 (marker, source, page)", len(puts))
	}
	page := string(puts[2].body)
	for _, want := range []string{
		`<title>Custom Title</title>`,
		`data-slug="custom-slug"`,
		`>From Body</h1>`,
		`<p>paragraph</p>`,
		`data-source="./custom-slug.md"`,
		`From Body`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "copy-source") {
		t.Error("page contains built-in template markup")
	}
}

func TestUploadHTMLWarnsCustomTemplateIgnored(t *testing.T) {
	var (
		mu   sync.Mutex
		puts []capturedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			cr := captureRequest(r)
			cr.body = body
			puts = append(puts, cr)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	templatePath := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(
		templatePath, []byte(`template {{.Title}}`), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
		Template:        templatePath,
		Indexable:       true,
		DisableManifest: true,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	src := "<!doctype html><title>HTML</title><p>as-is</p>"
	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader(src),
		Name:   "page.html",
		Format: "html",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantWarning := "custom template ignored for HTML input — " +
		"HTML input is used as-is"
	if !containsString(res.Warnings, wantWarning) {
		t.Fatalf("warnings = %v, want %q", res.Warnings, wantWarning)
	}
	if res.SourceKey != "" {
		t.Errorf("SourceKey = %q, want empty", res.SourceKey)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 2 {
		t.Fatalf("got %d puts, want 2 (marker, page)", len(puts))
	}
	if got := string(puts[1].body); got != src {
		t.Errorf("uploaded HTML = %q, want %q", got, src)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestUploadHTMLProceedsDespiteBrokenTemplate(t *testing.T) {
	var (
		mu   sync.Mutex
		puts []capturedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			puts = append(puts, captureRequest(r))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
		Template:        "/nonexistent/template.html",
		DisableManifest: true,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New must not fail on a broken template: %v", err)
	}

	doc := "<!doctype html><html><head></head><body>x</body></html>"
	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader(doc),
		Name:   "report.html",
	})
	if err != nil {
		t.Fatalf("HTML upload blocked by template error: %v", err)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "custom template ignored") &&
			strings.Contains(w, "failed to load") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want ignored+load-failure note",
			res.Warnings)
	}

	// Markdown through the same client must fail with the load error.
	_, err = client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# hi\n"),
		Name:   "plan.md",
	})
	if err == nil || !strings.Contains(err.Error(), "template") {
		t.Errorf("markdown upload error = %v, want template error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 2 {
		t.Errorf("got %d puts, want 2 (HTML marker and page)", len(puts))
	}
}
