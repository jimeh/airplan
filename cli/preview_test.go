package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestPreviewRendersCollectionWithCustomTemplate(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "shot one.svg")
	second := filepath.Join(dir, "demo.webm")
	tmpl := filepath.Join(dir, "collection.tmpl")
	if err := os.WriteFile(first, []byte("<svg></svg>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpl, []byte(`{{.Title}}|{{range .Files}}{{.Name}}={{.Path}};{{end}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{
		"preview", "--files", "--repo", "none",
		"--title", "Evidence", "--collection-template", tmpl, first, second,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "Evidence|shot one.svg=./shot%20one.svg;demo.webm=./demo.webm;" {
		t.Fatalf("preview = %q", stdout.String())
	}
}

func TestDocumentPreviewRejectsCollectionOnlyFlags(t *testing.T) {
	for _, flag := range []string{
		"--collection-template=collection.tmpl",
		"--max-total-size=1MiB",
	} {
		t.Run(flag, func(t *testing.T) {
			isolateEnv(t)
			cmd := newRootCmd()
			cmd.SetIn(strings.NewReader("# Plan\n"))
			cmd.SetArgs([]string{"preview", flag, "-"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(
				err.Error(), "only valid for collection previews",
			) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCollectionPreviewHonorsCanceledContext(t *testing.T) {
	isolateEnv(t)
	input := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(input, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"preview", "--files", "--repo", "none", input})
	err := cmd.Execute()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestCollectionPreviewReportsConfigWarnings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission warning does not apply on Windows")
	}
	isolateEnv(t)
	dir := t.TempDir()
	input := filepath.Join(dir, "shot.png")
	config := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(input, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(
		"access_key_id = \"id\"\nsecret_access_key = \"secret\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCmd()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"preview", "--files", "--repo", "none", "--config", config, input,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(),
		"contains credentials and is group- or world-readable") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPreviewExternalAssetFlagExplicitFalseOverridesEnvironment(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_NO_EXTERNAL_ASSETS", "true")

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("```mermaid\ngraph TD\n  A --> B\n```\n"))
	cmd.SetArgs([]string{
		"preview", "--no-external-assets=false", "-",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "cdn.jsdelivr.net/npm/mermaid@") {
		t.Fatal("explicit false did not re-enable Mermaid module loading")
	}
}

func TestPreviewEmptyMermaidURLFlagResetsEnvironment(t *testing.T) {
	isolateEnv(t)
	customURL := "https://assets.example.test/custom-mermaid.mjs"
	t.Setenv("AIRPLAN_MERMAID_URL", customURL)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("```mermaid\ngraph TD\n  A --> B\n```\n"))
	cmd.SetArgs([]string{"preview", "--mermaid-url", "", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	page := stdout.String()
	if strings.Contains(page, customURL) {
		t.Fatal("empty Mermaid URL flag retained inherited custom URL")
	}
	if !strings.Contains(page, "cdn.jsdelivr.net/npm/mermaid@") {
		t.Fatal("empty Mermaid URL flag did not restore built-in default")
	}
}

func TestPreviewRepositoryFlagLinksReferences(t *testing.T) {
	isolateEnv(t)

	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("See #42.\n"))
	cmd.SetArgs([]string{
		"preview", "--repo", "git@github.example:acme/project.git", "-",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(),
		`href="https://github.example/acme/project/issues/42"`) {
		t.Fatalf("repository reference was not linked: %s", stdout.String())
	}

	cmd = newRootCmd()
	stdout.Reset()
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("See #42.\n"))
	cmd.SetArgs([]string{"preview", "--repo", "none", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "/issues/42") {
		t.Fatal("--repo none linked a repository reference")
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
	temps, err := filepath.Glob(filepath.Join(dir, ".notes.html.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary preview files remain: %v", temps)
	}
}

func TestWritePreviewAtomicLeavesDestinationOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "existing")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(destination, "keep")
	if err := os.WriteFile(sentinel, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(destination, []byte("new"), 0o644); err == nil {
		t.Fatal("writeFileAtomic error = nil, want rename failure")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "old" {
		t.Fatalf("sentinel = %q, error = %v", got, err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".existing.tmp-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary files = %v, error = %v", temps, err)
	}
}

func TestPreviewAppliesConfiguredTimeout(t *testing.T) {
	isolateEnv(t)
	t.Setenv("AIRPLAN_TIMEOUT", "20ms")

	reader := &blockingPreviewReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(reader.release)
	cmd := newRootCmd()
	cmd.SetIn(reader)
	cmd.SetArgs([]string{"preview", "-"})

	started := time.Now()
	err := cmd.Execute()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("preview timeout took %s", time.Since(started))
	}
	select {
	case <-reader.started:
	default:
		t.Fatal("preview did not start reading input")
	}
}

type blockingPreviewReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingPreviewReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
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

func TestPreviewRefusesInputAliases(t *testing.T) {
	for _, tt := range []struct {
		name string
		link func(string, string) error
	}{
		{"hardlink", os.Link},
		{"symlink", os.Symlink},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			dir := t.TempDir()
			input := filepath.Join(dir, "plan.md")
			alias := filepath.Join(dir, "alias.md")
			if err := os.WriteFile(input, []byte("# Keep me\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := tt.link(input, alias); err != nil {
				t.Skipf("create %s: %v", tt.name, err)
			}

			cmd := newRootCmd()
			cmd.SetArgs([]string{"preview", "--output", alias, input})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "must not overwrite") {
				t.Fatalf("error = %v, want overwrite refusal", err)
			}
		})
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
	cmd.SetIn(strings.NewReader(
		"<!doctype html><!-- <meta name=robots> -->" +
			"<html><head></head></html>",
	))
	cmd.SetArgs([]string{"preview", "--format", "html", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(),
		`<head><meta name="robots" content="noindex, nofollow">`) {
		t.Fatalf("preview did not inject noindex: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "<!-- <meta name=robots> -->") {
		t.Fatalf("preview changed the comment: %s", stdout.String())
	}
}
