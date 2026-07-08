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

func TestNewFailsFastOnBrokenTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.html")
	if err := os.WriteFile(path, []byte(`{{`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint: "http://127.0.0.1:1",
		Bucket:   "plans",
		Template: path,
	}
	_, err := New(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %v, want it to name %q", err, path)
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
		`<main data-slug="{{.Slug}}">{{.Body}}</main>`,
		`<aside data-source="{{.SourcePath}}">{{.SourceHTML}}</aside>`,
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
	if len(puts) != 2 {
		t.Fatalf("got %d puts, want 2 (source first, then page)", len(puts))
	}
	page := string(puts[1].body)
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
		"HTML is uploaded as-is"
	if !containsString(res.Warnings, wantWarning) {
		t.Fatalf("warnings = %v, want %q", res.Warnings, wantWarning)
	}
	if res.SourceKey != "" {
		t.Errorf("SourceKey = %q, want empty", res.SourceKey)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 1 {
		t.Fatalf("got %d puts, want 1", len(puts))
	}
	if got := string(puts[0].body); got != src {
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
