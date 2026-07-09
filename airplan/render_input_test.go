package airplan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderInputUsesUploadSourcePathOnlyWhenRequested(t *testing.T) {
	input := func() Input {
		return Input{
			Reader: strings.NewReader("# Plan\n\nBody.\n"),
			Name:   "docs/plan.md",
		}
	}

	preview, err := RenderInput(context.Background(), input(),
		RenderInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourcePath != "" {
		t.Fatalf("preview SourcePath = %q, want empty", preview.SourcePath)
	}
	if preview.Format != FormatMarkdown || preview.Title != "Plan" ||
		preview.Slug != "plan" {
		t.Fatalf("preview resolution = %#v", preview)
	}

	upload, err := RenderInput(context.Background(), input(),
		RenderInputOptions{IncludeSource: true})
	if err != nil {
		t.Fatal(err)
	}
	if upload.SourcePath != "./plan.md" {
		t.Fatalf("upload SourcePath = %q, want ./plan.md", upload.SourcePath)
	}
}

func TestRenderInputTextExposesCanonicalSourceData(t *testing.T) {
	tmpl, err := LoadTemplate(writeTemplate(t, strings.Join([]string{
		`{{.Format}}|{{.Language}}|{{.SourceName}}|`,
		`{{.SourceText}}|{{if .HighlightedSourceHTML}}highlighted{{end}}|`,
		`{{if .SourceHTML}}legacy{{end}}`,
	}, "")))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := RenderInput(context.Background(), Input{
		Reader: strings.NewReader("package main\n"),
		Name:   "main.go",
	}, RenderInputOptions{Template: tmpl})
	if err != nil {
		t.Fatal(err)
	}
	got := string(doc.HTML)
	if !strings.Contains(got, "txt|go|main.go|package main\n|highlighted|") {
		t.Fatalf("canonical template data = %q", got)
	}
	if strings.Contains(got, "legacy") {
		t.Fatalf("legacy SourceHTML must stay empty for text: %q", got)
	}
}

func writeTemplate(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.html")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
