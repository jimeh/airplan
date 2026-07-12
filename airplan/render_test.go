package airplan

import (
	"flag"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestRenderMarkdownGolden(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "basic.md"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := RenderMarkdown(src, RenderOptions{
		Title:      "Refactor auth",
		Slug:       "refactor-auth",
		SourcePath: "./refactor-auth.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "basic.html")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("rendered output differs from %s (run with -update "+
			"to refresh)", golden)
	}
}

func TestRenderMarkdownPageFeatures(t *testing.T) {
	src := []byte("# Hi\n\nsome *text*\n")

	t.Run("noindex default", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if !strings.Contains(out,
			`<meta name="robots" content="noindex, nofollow">`) {
			t.Error("missing noindex meta tag")
		}
	})

	t.Run("color scheme advertised", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if !strings.Contains(out,
			`<meta name="color-scheme" content="light dark">`) {
			t.Error("missing color-scheme meta tag")
		}
	})

	t.Run("indexable omits robots meta", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi", Indexable: true})
		if strings.Contains(out, `name="robots"`) {
			t.Error("robots meta present despite Indexable")
		}
	})

	t.Run("download link from SourcePath", func(t *testing.T) {
		out := render(t, src, RenderOptions{
			Title: "Hi", SourcePath: "./plan.md",
		})
		if !strings.Contains(out, `href="./plan.md" download`) {
			t.Error("missing download anchor")
		}
		if !strings.Contains(out, `class="raw" href="./plan.md"`) {
			t.Error("missing raw source anchor")
		}
	})

	t.Run("no download link without SourcePath", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if strings.Contains(out, "download") {
			t.Error("unexpected download anchor")
		}
	})

	t.Run("title is escaped", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "a <b> & c"})
		if !strings.Contains(out, "<title>a &lt;b&gt; &amp; c</title>") {
			t.Error("title not escaped")
		}
	})

	t.Run("document without Mermaid has no external refs", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		for _, frag := range []string{"<link", "<script src", "@import"} {
			if strings.Contains(out, frag) {
				t.Errorf("page references external asset: %s", frag)
			}
		}
	})

	t.Run("dark palette present", func(t *testing.T) {
		out := render(t, src, RenderOptions{Title: "Hi"})
		if strings.Count(out, "prefers-color-scheme: dark") < 2 {
			t.Error("expected dark palettes for page and syntax CSS")
		}
	})
}

func TestRenderMarkdownMermaid(t *testing.T) {
	src := []byte("```mermaid\ngraph TD\n  A[<unsafe>] --> B\n```\n")
	out := render(t, src, RenderOptions{Title: "Diagram"})
	if !strings.Contains(out,
		`<pre class="mermaid">graph TD`) {
		t.Fatal("Mermaid fence was not rendered as a diagram container")
	}
	if strings.Contains(out, `<pre class="mermaid"><code>`) {
		t.Fatal("Mermaid source must not be nested in a code element")
	}
	if !strings.Contains(out, `A[&lt;unsafe&gt;]`) {
		t.Fatal("Mermaid source was not HTML escaped")
	}
	if !strings.Contains(out, "await import(\""+DefaultMermaidURL+"\")") {
		t.Fatal("pinned Mermaid module import missing")
	}
	if !strings.Contains(out, "mermaid.run({nodes: [diagram]})") ||
		strings.Contains(out, "mermaid.run({nodes: diagrams})") {
		t.Fatal("Mermaid diagrams are not rendered independently")
	}
	if !strings.Contains(out,
		"diagram.classList.add('mermaid-rendered')") {
		t.Fatal("successful Mermaid diagrams are not marked as rendered")
	}
	if !strings.Contains(out, "pre.mermaid.mermaid-rendered {") {
		t.Fatal("rendered Mermaid layout is not state-scoped")
	}
	if strings.Contains(out, `class="codewrap"><pre class="mermaid"`) {
		t.Fatal("Mermaid block received code-copy wrapper")
	}

	disabled := render(t, src, RenderOptions{
		Title: "Diagram", NoExternalAssets: true,
	})
	if strings.Contains(disabled, DefaultMermaidURL) {
		t.Fatal("Mermaid module loaded with external assets disabled")
	}
	if !strings.Contains(disabled, `<pre class="mermaid">`) {
		t.Fatal("readable Mermaid source fallback missing")
	}
}

func TestRenderMarkdownMermaidRequiresExactFenceLanguage(t *testing.T) {
	for _, language := range []string{"Mermaid", "mermaid-js", "mermaid extra"} {
		out := render(t, []byte("```"+language+"\ngraph TD\n```\n"),
			RenderOptions{Title: "Diagram"})
		if strings.Contains(out, `<pre class="mermaid">`) {
			t.Fatalf("language %q unexpectedly treated as Mermaid", language)
		}
		if strings.Contains(out, DefaultMermaidURL) {
			t.Fatalf("language %q unexpectedly loaded Mermaid", language)
		}
	}
}

func TestRenderMarkdownPreservesTrustedContent(t *testing.T) {
	src := []byte(strings.Join([]string{
		"# Trusted content",
		"",
		`<script>window.airplanPwned = true</script>`,
		"",
		`<img src=x onerror="window.airplanPwned = true">`,
		"",
		`inline <span onclick="window.airplanPwned = true">HTML</span>`,
		"",
		`[unsafe](javascript:alert(1))`,
		"",
		`![unsafe image](javascript:alert(2))`,
		"",
		`[safe](https://example.com/path)`,
	}, "\n"))
	out := render(t, src, RenderOptions{Title: "Trusted content"})
	rendered := renderedSection(t, out)

	for _, authored := range []string{
		"<script>window.airplanPwned", "onerror=", "onclick=",
		`href="javascript:alert(1)"`, `src="javascript:alert(2)"`,
		`href="https://example.com/path"`,
	} {
		if !strings.Contains(rendered, authored) {
			t.Errorf("rendered view omitted authored content %q: %s",
				authored, rendered)
		}
	}
}

func renderedSection(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, `id="rendered">`)
	if start < 0 {
		t.Fatal("rendered main section not found")
	}
	start = strings.LastIndex(page[:start], "<main")
	end := strings.Index(page[start:], "</main>")
	if end < 0 {
		t.Fatal("rendered main closing tag not found")
	}
	return page[start : start+end]
}

func render(t *testing.T, src []byte, opts RenderOptions) string {
	t.Helper()
	out, err := RenderMarkdown(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"simple", "# Hello World\n\nbody\n", "Hello World"},
		{"not first line", "intro\n\n# Later Title\n", "Later Title"},
		{"h2 only", "## Not a title\n", ""},
		{"empty", "", ""},
		{"inline markup", "# Fix `auth` *now*\n", "Fix auth now"},
		{"setext", "Big Title\n=========\n", "Big Title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractTitle([]byte(tt.src)); got != tt.want {
				t.Errorf("ExtractTitle(%q) = %q, want %q",
					tt.src, got, tt.want)
			}
		})
	}
}

func TestResolveTitle(t *testing.T) {
	withH1 := []byte("# From H1\n")
	noH1 := []byte("plain text\n")

	tests := []struct {
		name     string
		explicit string
		src      []byte
		filename string
		slug     string
		want     string
	}{
		{"explicit wins", "Given", withH1, "plan.md", "slug", "Given"},
		{"h1 next", "", withH1, "plan.md", "slug", "From H1"},
		{"filename next", "", noH1, "auth-plan.md", "slug", "auth-plan"},
		{"slug last", "", noH1, "", "my-slug", "my-slug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTitle(tt.explicit, tt.src, tt.filename, tt.slug)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderMarkdownInteractivity(t *testing.T) {
	src := []byte("# Hi\n\n```go\npackage main\n```\n")
	out := render(t, src, RenderOptions{Title: "Hi"})

	for name, frag := range map[string]string{
		"view toggle":           `class="viewtoggle js-only" hidden`,
		"rendered label":        `<span>Rendered</span>`,
		"source label":          `<span>Source</span>`,
		"pressed state":         `aria-pressed="true"`,
		"copy source":           `class="copy-source js-only" hidden`,
		"source heading":        `<span>Markdown source</span>`,
		"source block":          `id="source" hidden`,
		"mobile toc trigger":    `Open table of contents`,
		"native toc dialog":     `tocDialog.showModal()`,
		"coalesced toc scroll":  `requestAnimationFrame`,
		"smooth navigation":     `scroll-behavior: smooth`,
		"reduced motion scroll": `scroll-behavior: auto`,
		"embedded script":       "<script>",
		"highlighted fence":     `<span class="kn">package</span>`,
	} {
		if !strings.Contains(out, frag) {
			t.Errorf("page missing %s (%q)", name, frag)
		}
	}

	// The embedded source view must preserve the raw markdown text.
	if !strings.Contains(out, "# Hi") {
		t.Error("source view missing raw markdown")
	}
}

func TestRenderMarkdownTableOfContents(t *testing.T) {
	src := []byte(strings.Join([]string{
		"# Document title",
		"",
		"## Context",
		"",
		"### Detail",
		"",
		"# Appendix",
		"",
		"## Reference",
		"",
	}, "\n"))
	out := render(t, src, RenderOptions{Title: "Document title"})

	if strings.Contains(out, `href="#document-title"`) {
		t.Error("leading title H1 should be omitted from the ToC")
	}
	for _, fragment := range []string{
		`class="toc"`,
		`class="toc-list"`,
		`class="toc-level-2"><a href="#context">Context</a>`,
		`class="toc-level-3"><a href="#detail">Detail</a>`,
		`class="toc-level-1"><a href="#appendix">Appendix</a>`,
		`class="toc-level-2"><a href="#reference">Reference</a>`,
	} {
		if !strings.Contains(out, fragment) {
			t.Errorf("ToC missing %q", fragment)
		}
	}
}

func TestRenderMarkdownIncludesNonLeadingH1InTableOfContents(t *testing.T) {
	src := []byte("Intro first.\n\n# First section\n\n## Child\n")
	out := render(t, src, RenderOptions{Title: "First section"})
	if !strings.Contains(out, `href="#first-section">First section</a>`) {
		t.Error("non-leading first H1 should remain in the ToC")
	}
}

func TestRenderMarkdownCommentBeforeTitleDoesNotEnterTableOfContents(
	t *testing.T,
) {
	src := []byte("<!-- context -->\n\n# Title\n\n## One\n\n## Two\n")
	out := render(t, src, RenderOptions{Title: "Title"})
	if strings.Contains(out, `href="#title"`) {
		t.Error("an invisible comment should not stop a leading H1 being title")
	}
}

func TestRenderMarkdownOmitsSingleEntryTableOfContents(t *testing.T) {
	out := render(t, []byte("# Title\n\n## Only section\n"),
		RenderOptions{Title: "Title"})
	if strings.Contains(out, `class="toc"`) {
		t.Error("a single-entry ToC should be omitted")
	}
}

func TestRenderTextNoSourceToggle(t *testing.T) {
	out, err := RenderText([]byte("hello\n"), "notes.txt", RenderOptions{
		Title: "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `<div class="viewtoggle`) {
		t.Error("text pages should not render a source toggle")
	}
	if !strings.Contains(string(out), "<script>") {
		t.Error("text pages still need JS for code-block copy")
	}
}

func TestRenderCustomTemplate(t *testing.T) {
	tmpl := template.Must(template.New("t").Parse(
		"<title>{{.Title}}</title><b>{{.Slug}}</b>{{.RenderedHTML}}" +
			"{{if .HighlightedSourceHTML}}src{{end}}"))

	out, err := RenderMarkdown([]byte("# X\n"), RenderOptions{
		Title:    "Custom",
		Slug:     "x",
		Template: tmpl,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "<title>Custom</title>") ||
		!strings.Contains(got, "<b>x</b>") ||
		!strings.Contains(got, "src") {
		t.Errorf("custom template output wrong: %s", got)
	}
	if strings.Contains(got, `<div class="viewtoggle`) {
		t.Error("custom template output contains built-in markup")
	}
}
