package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewRendersMarkdownWithoutUploadConfig(t *testing.T) {
	isolateEnv(t)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(
		"# Local plan\n\n## Context\n\nBody.\n\n## Tasks\n\n- One\n",
	))
	cmd.SetArgs([]string{"preview", "--slug", "local-plan", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview: %v\nstderr:\n%s", err, stderr.String())
	}

	got := stdout.String()
	for _, fragment := range []string{
		"<!doctype html>",
		`<nav class="toc"`,
		`href="#context"`,
		`id="source" hidden`,
	} {
		if !strings.Contains(got, fragment) {
			t.Errorf("preview missing %q", fragment)
		}
	}
	if strings.Contains(got, `class="download"`) {
		t.Error("local preview unexpectedly contains a source download link")
	}
	if strings.Contains(got, `class="raw"`) {
		t.Error("local preview unexpectedly contains a raw source link")
	}
}

func TestPreviewWritesOutputFile(t *testing.T) {
	isolateEnv(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "notes.txt")
	output := filepath.Join(dir, "notes.html")
	if err := os.WriteFile(input, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"preview", "--output", output, input})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty with --output", stdout.String())
	}

	page, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page, []byte("<title>notes.txt</title>")) ||
		!bytes.Contains(page, []byte("<code>notes.txt</code>")) {
		t.Fatalf("output file is not the rendered text page:\n%s", page)
	}
}

func TestPreviewRefusesToOverwriteInput(t *testing.T) {
	isolateEnv(t)

	input := filepath.Join(t.TempDir(), "plan.md")
	want := "# Keep me\n"
	if err := os.WriteFile(input, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"preview", "--output", input, input})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "must not overwrite") {
		t.Fatalf("error = %v, want overwrite refusal", err)
	}
	got, readErr := os.ReadFile(input)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != want {
		t.Fatalf("input was changed: %q", got)
	}
}

func TestPreviewCustomTemplateReceivesCanonicalData(t *testing.T) {
	isolateEnv(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "plan.md")
	templatePath := filepath.Join(dir, "custom.html")
	if err := os.WriteFile(input, []byte(
		"# Plan\n\n## First\n\n## Second\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	custom := strings.Join([]string{
		`<title>{{.Title}}</title>`,
		`<i>{{.Format}}|{{.Language}}|{{.SourceName}}|`,
		`{{len .Headings}}|{{len .TOC}}|{{.Indexable}}|`,
		`{{.SourcePath}}</i><pre>{{.SourceText}}</pre>`,
		`<style>{{.SyntaxCSS}}</style>`,
		`<main>{{.RenderedHTML}}</main>`,
		`<aside>{{.HighlightedSourceHTML}}</aside>`,
	}, "")
	if err := os.WriteFile(templatePath, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"preview", "--indexable", "--template", templatePath, input,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := stdout.String()
	for _, fragment := range []string{
		"<title>Plan</title>",
		"md|md|plan.md|3|2|true|",
		"# Plan",
		`<h2 id="first">First</h2>`,
		`class="chroma"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Errorf("custom preview missing %q:\n%s", fragment, got)
		}
	}
}

func TestPreviewHTMLAppliesNoindexWithoutNetwork(t *testing.T) {
	isolateEnv(t)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("<!doctype html><html><head></head></html>"))
	cmd.SetArgs([]string{"preview", "--format", "html", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(),
		`<head><meta name="robots" content="noindex, nofollow">`) {
		t.Fatalf("preview did not inject noindex: %s", stdout.String())
	}
}
