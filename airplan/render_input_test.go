package airplan

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderInputRejectsInvalidUTF8(t *testing.T) {
	inputs := map[string][]byte{
		"invalid byte":         {'a', 'b', 0xff, 'c'},
		"truncated sequence":   {'a', 0xe2, 0x82},
		"overlong encoding":    {0xc0, 0xaf},
		"surrogate code point": {0xed, 0xa0, 0x80},
	}
	for name, invalid := range inputs {
		for _, format := range []string{"", "md", "txt", "html"} {
			t.Run(name+"/format="+format, func(t *testing.T) {
				_, err := RenderInput(context.Background(), Input{
					Reader: bytes.NewReader(invalid), Format: format,
				}, RenderInputOptions{})
				if !errors.Is(err, ErrInvalidUTF8) {
					t.Fatalf("err = %v, want ErrInvalidUTF8", err)
				}
			})
		}
	}
}

func TestRenderInputAcceptsValidNonASCII(t *testing.T) {
	_, err := RenderInput(context.Background(), Input{
		Reader: strings.NewReader("# Héllo 👋\n"), Format: "md",
	}, RenderInputOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRenderInputExplicitHTMLPreservesExecutableContent(t *testing.T) {
	src := `<!doctype html><script>window.intentional = true</script>`
	doc, err := RenderInput(context.Background(), Input{
		Reader: strings.NewReader(src), Format: "html",
	}, RenderInputOptions{Indexable: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(doc.HTML) != src {
		t.Fatalf("explicit HTML changed: %q", doc.HTML)
	}
}

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
