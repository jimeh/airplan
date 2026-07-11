package airplan

import (
	"context"
	"errors"
	"html/template"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestRenderMarkdownDefinitionListsAndGFMExtendedAutolinks(t *testing.T) {
	source := []byte(strings.Join([]string{
		"Term", ": Definition", "", "Visit https://example.com/a_(b).",
		"Email person@example.com.", "",
	}, "\n"))
	out := render(t, source, RenderOptions{Title: "Extensions"})
	rendered := renderedSection(t, out)
	for _, fragment := range []string{
		"<dl>", "<dt>Term</dt>", "<dd>Definition</dd>",
		`href="https://example.com/a_(b)"`,
		`href="mailto:person@example.com"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Errorf("rendered output missing %q: %s", fragment, rendered)
		}
	}
}

func TestRenderInputFrontMatter(t *testing.T) {
	tests := []struct {
		name   string
		source string
		title  string
		format string
	}{
		{"yaml", "---\ntitle: Front title\ntags: [one]\n---\n# Body title\n", "Front title", "yaml"},
		{"toml", "+++\ntitle = \"TOML title\"\n+++\n# Body title\n", "TOML title", "toml"},
		{"bom", "\ufeff---\ntitle: BOM title\n---\nBody\n", "BOM title", "yaml"},
		{"non-string title", "---\ntitle: 42\n---\n# Body title\n", "Body title", "yaml"},
		{"nested mapping", "---\nmeta:\n  owner: team\n  priority: high\n---\n# Body title\n", "Body title", "yaml"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpl := templateForTest(t,
				`{{.Title}}|{{.FrontMatterFormat}}|{{.FrontMatterTitle}}|`+
					`{{.FrontMatterText}}|{{.RenderedHTML}}`)
			doc, err := RenderInput(context.Background(), Input{
				Reader: strings.NewReader(test.source), Name: "fallback.md",
			}, RenderInputOptions{Template: tmpl})
			if err != nil {
				t.Fatal(err)
			}
			output := string(doc.HTML)
			if doc.Title != test.title ||
				!strings.Contains(output, "|"+test.format+"|") {
				t.Fatalf("title = %q, output = %q", doc.Title, output)
			}
			if strings.Contains(output, "<hr") ||
				strings.Contains(output, "title: Front title</p>") {
				t.Fatalf("frontmatter leaked into body: %s", output)
			}
		})
	}

	t.Run("explicit title wins", func(t *testing.T) {
		doc, err := RenderInput(context.Background(), Input{
			Reader: strings.NewReader("---\ntitle: Front\n---\n# Heading\n"),
			Title:  "Explicit",
		}, RenderInputOptions{})
		if err != nil || doc.Title != "Explicit" {
			t.Fatalf("document = %#v, error = %v", doc, err)
		}
	})

	for name, source := range map[string]string{
		"unclosed YAML":             "---\ntitle: nope\n",
		"invalid YAML":              "---\nkey: [\n---\nbody\n",
		"non-map YAML":              "---\n- item\n---\nbody\n",
		"unclosed TOML":             "+++\ntitle = \"nope\"\n",
		"invalid TOML":              "+++\ntitle = [\n+++\nbody\n",
		"opener-only YAML":          "---",
		"opener-only TOML":          "+++",
		"duplicate YAML key":        "---\ntitle: First\ntitle: Second\n---\nbody\n",
		"nested duplicate YAML key": "---\nmeta:\n  owner: one\n  owner: two\n---\nbody\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := RenderInput(context.Background(), Input{
				Reader: strings.NewReader(source), Format: "md",
			}, RenderInputOptions{})
			if err == nil || !strings.Contains(err.Error(), "frontmatter") {
				t.Fatalf("error = %v", err)
			}
		})
	}

	t.Run("leading whitespace is not frontmatter", func(t *testing.T) {
		doc, err := RenderInput(context.Background(), Input{
			Reader: strings.NewReader("\n---\ntitle: Body text\n---\n"),
			Name:   "fallback.md",
		}, RenderInputOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Title != "fallback" ||
			strings.Contains(string(doc.HTML), `class="frontmatter"`) {
			t.Fatalf("leading block treated as frontmatter: %#v", doc)
		}
	})

	t.Run("CRLF block is exact", func(t *testing.T) {
		source := []byte("---\r\ntitle: CRLF\r\n---\r\n# Body\r\n")
		frontMatter, err := parseFrontMatter(source)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(frontMatter.text),
			"---\r\ntitle: CRLF\r\n---\r\n"; got != want {
			t.Fatalf("frontmatter = %q, want %q", got, want)
		}
		if got := string(frontMatter.body); got != "# Body\r\n" {
			t.Fatalf("body = %q", got)
		}
	})

	t.Run("forced non-Markdown is uninterpreted", func(t *testing.T) {
		source := "---\ntitle: Metadata\n---\nbody\n"
		textDoc, err := RenderInput(context.Background(), Input{
			Reader: strings.NewReader(source), Format: "txt", Name: "notes.txt",
		}, RenderInputOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if textDoc.Title != "notes.txt" ||
			!strings.Contains(string(textDoc.HTML), "title") {
			t.Fatalf("text document interpreted frontmatter: %#v", textDoc)
		}
		htmlDoc, err := RenderInput(context.Background(), Input{
			Reader: strings.NewReader(source), Format: "html",
		}, RenderInputOptions{Indexable: true})
		if err != nil || string(htmlDoc.HTML) != source {
			t.Fatalf("HTML = %q, error = %v", htmlDoc.HTML, err)
		}
	})
}

func TestRenderMarkdownFrontMatterDetailsAndSource(t *testing.T) {
	source := []byte("---\ntitle: Plan\n---\n# Heading\n")
	out := render(t, source, RenderOptions{Title: "Plan"})
	if !strings.Contains(out, `<details class="frontmatter">`) ||
		!strings.Contains(out, `<summary>Frontmatter <span>yaml</span>`) {
		t.Fatalf("frontmatter details missing: %s", out)
	}
	if strings.Contains(renderedSection(t, out), "<hr") {
		t.Fatal("frontmatter delimiters rendered as horizontal rules")
	}
	if !strings.Contains(out, "title") || !strings.Contains(out, "Plan") {
		t.Fatal("original source/frontmatter missing")
	}
}

func TestRenderMarkdownRepositoryReferences(t *testing.T) {
	sha := strings.Repeat("a", 40)
	source := []byte(strings.Join([]string{
		"See #12, owner/other#34, and " + sha + ".",
		"Next #13  ", "new line",
		"", "> [!NOTE]", "> Review #14.",
		"", "Do not link docs/acme/repo#15; do link (acme/repo#16).",
		"", "`#99` [#98](https://example.com) https://example.com/#97",
		"", "Escaped \\#93.",
		"", "```", "#96", "```", "", "<span>#95</span>", "",
		"```mermaid", "graph TD", "  A[#94] --> B", "```", "",
	}, "\n"))
	out := render(t, source, RenderOptions{
		Title: "Links", RepositoryURL: "https://github.example/acme/current",
	})
	rendered := renderedSection(t, out)
	for _, destination := range []string{
		`href="https://github.example/acme/current/issues/12"`,
		`href="https://github.example/owner/other/issues/34"`,
		`href="https://github.example/acme/current/commit/` + sha + `"`,
		`href="https://github.example/acme/current/issues/13"`,
		`href="https://github.example/acme/current/issues/14"`,
		`href="https://github.example/acme/repo/issues/16"`,
	} {
		if !strings.Contains(rendered, destination) {
			t.Errorf("missing repository link %q: %s", destination, rendered)
		}
	}
	if !strings.Contains(rendered, "#13</a><br>") {
		t.Errorf("repository transformation lost a hard line break: %s", rendered)
	}
	for _, protected := range []string{
		"#99", "#98", "#96", "#95", "#94", "#93", "#15",
	} {
		if strings.Contains(rendered, "/issues/"+protected[1:]+`"`) {
			t.Errorf("protected reference %s was linked: %s", protected, rendered)
		}
	}
}

func TestRepositoryLinksPreserveHeadingText(t *testing.T) {
	tmpl, err := template.New("headings").Parse(
		`{{range .TOC}}{{.Text}}|{{end}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := RenderMarkdown(
		[]byte("# Plan\n\n## Fix #123 now\n\n## Verify\n"),
		RenderOptions{
			Title: "Plan", RepositoryURL: "https://github.com/acme/project",
			Template: tmpl,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(page), "Fix #123 now|Verify|"; got != want {
		t.Fatalf("TOC heading text = %q, want %q", got, want)
	}
}

func TestRepositoryLinksMalformedURLDoesNotPanic(t *testing.T) {
	_, err := RenderMarkdown(
		[]byte("See #123.\n"),
		RenderOptions{RepositoryURL: "%"},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRenderMarkdownPandocColumns(t *testing.T) {
	source := []byte(strings.Join([]string{
		":::: {.columns}", "::: {.column width=40%}", "## Left", "", "- one",
		":::", "::: {.column width=60%}", "## Right", "", "Term", ": Meaning",
		"", "> [!NOTE]", "> Review #18.", "", "```mermaid", "graph TD",
		"  A --> B", "```", ":::",
		"::::", "",
	}, "\n"))
	out := render(t, source, RenderOptions{
		Title: "Columns", RepositoryURL: "https://github.com/acme/project",
	})
	rendered := renderedSection(t, out)
	for _, fragment := range []string{
		`<div class="columns">`,
		`<div class="column" style="--column-width: 40%">`,
		`<h2 id="left">Left</h2>`, "<li>one</li>",
		`<div class="column" style="--column-width: 60%">`,
		"<dl>", `class="markdown-alert markdown-alert-note"`,
		`href="https://github.com/acme/project/issues/18"`,
		`<pre class="mermaid">graph TD`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Errorf("columns output missing %q: %s", fragment, rendered)
		}
	}

	invalid := []byte(":::: {.columns}\n::: {.column width=0%}\nA\n:::\n" +
		"::: {.column}\nB\n:::\n::::\n")
	invalidOut := renderedSection(t, render(t, invalid, RenderOptions{Title: "Invalid"}))
	if strings.Contains(invalidOut, `class="columns"`) ||
		!strings.Contains(invalidOut, ":::: {.columns}") {
		t.Fatalf("invalid columns were transformed: %s", invalidOut)
	}
	for name, malformed := range map[string]string{
		"unknown attribute": ":::: {.columns}\n::: {.column foo=bar}\nA\n:::\n" +
			"::: {.column}\nB\n:::\n::::\n",
		"one child": ":::: {.columns}\n::: {.column}\nA\n:::\n::::\n",
		"unterminated": ":::: {.columns}\n::: {.column}\nA\n:::\n" +
			"::: {.column}\nB\n:::\n",
	} {
		t.Run(name, func(t *testing.T) {
			output := renderedSection(t, render(t, []byte(malformed),
				RenderOptions{Title: "Malformed"}))
			if strings.Contains(output, `class="columns"`) ||
				!strings.Contains(output, ":::: {.columns}") {
				t.Fatalf("malformed columns changed: %s", output)
			}
		})
	}

	nested := []byte("::::: {.columns}\n::: {.column}\n" +
		":::: {.columns}\n::: {.column}\nA\n:::\n::: {.column}\nB\n:::\n::::\n" +
		":::\n::: {.column}\nC\n:::\n:::::\n")
	nestedOut := renderedSection(t, render(t, nested,
		RenderOptions{Title: "Nested"}))
	if strings.Contains(nestedOut, `class="columns"`) {
		t.Fatalf("nested columns were transformed: %s", nestedOut)
	}

	doc := newMarkdownWithRepository("", source).Parser().Parse(
		text.NewReader(source),
	)
	columnsCount, columnCount := 0, 0
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case kindColumns:
			columnsCount++
		case kindColumn:
			columnCount++
		}
		return ast.WalkContinue, nil
	})
	if columnsCount != 1 || columnCount != 2 {
		t.Fatalf("custom nodes = %d columns, %d column children",
			columnsCount, columnCount)
	}
}

func TestNormalizeRepositoryURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://github.com/acme/airplan.git":            "https://github.com/acme/airplan",
		"ssh://git@github.example/acme/airplan.git":      "https://github.example/acme/airplan",
		"ssh://git@github.example:2222/acme/airplan.git": "https://github.example/acme/airplan",
		"git@github.com:acme/airplan.git":                "https://github.com/acme/airplan",
	} {
		got, err := NormalizeRepositoryURL(input)
		if err != nil || got != want {
			t.Errorf("NormalizeRepositoryURL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{
		"file:///tmp/repo", "git://github.com/acme/repo", "https://user@github.com/a/b",
		"https://github.com/a/b/extra", "https://github.com/a/b?x=1", "./repo",
		"https://github.com:8443/a/b",
		"ssh://git:secret@github.com/acme/repo.git",
		"github.com:acme/repo.git",
	} {
		if _, err := NormalizeRepositoryURL(input); err == nil {
			t.Errorf("NormalizeRepositoryURL(%q) succeeded", input)
		}
	}
}

func TestRepositoryDiscovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cwdRepo := gitRepo(t, "git@github.com:acme/cwd.git")
	fileRepo := gitRepo(t, "git@github.com:acme/file.git")
	temp := t.TempDir()

	tests := []struct {
		name    string
		setting string
		file    string
		cwd     string
		want    string
	}{
		{
			"file repo wins", "auto", filepath.Join(fileRepo, "plan.md"), cwdRepo,
			"https://github.com/acme/file",
		},
		{
			"outside repo falls back", "auto", filepath.Join(temp, "plan.md"), cwdRepo,
			"https://github.com/acme/cwd",
		},
		{
			"stdin uses cwd", "auto", "", cwdRepo,
			"https://github.com/acme/cwd",
		},
		{
			"missing file origin does not fall back", "auto",
			filepath.Join(gitRepo(t, ""), "plan.md"), cwdRepo, "",
		},
		{
			"unsupported file origin does not fall back", "auto",
			filepath.Join(gitRepo(t, "git@gitlab.com:acme/file.git"), "plan.md"),
			cwdRepo, "",
		},
		{
			"invalid auto origin is quiet", "auto",
			filepath.Join(gitRepo(t, "::::"), "plan.md"), cwdRepo, "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveRepository(context.Background(),
				test.setting, test.file, test.cwd)
			if err != nil || got != test.want {
				t.Fatalf("repository = %q, %v; want %q", got, err, test.want)
			}
		})
	}

	t.Run("explicit invalid is fatal", func(t *testing.T) {
		if _, err := resolveRepository(context.Background(),
			"file:///tmp/repo", "", cwdRepo); err == nil {
			t.Fatal("invalid explicit repository succeeded")
		}
	})

	t.Run("none skips discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := resolveRepository(ctx, "none", "", "/does/not/exist")
		if err != nil || got != "" {
			t.Fatalf("repository = %q, %v", got, err)
		}
	})

	t.Run("confirmed outside falls back", func(t *testing.T) {
		runner := func(
			_ context.Context, dir string, args ...string,
		) (string, error) {
			if args[0] == "rev-parse" {
				return "fatal: not a git repository", errors.New("exit 128")
			}
			if dir != "/cwd" {
				t.Fatalf("remote lookup dir = %q, want /cwd", dir)
			}
			return "git@github.com:acme/cwd.git", nil
		}
		got, err := resolveRepositoryWithGit(context.Background(),
			"auto", "/file/plan.md", "/cwd", runner)
		if err != nil || got != "https://github.com/acme/cwd" {
			t.Fatalf("repository = %q, %v", got, err)
		}
	})

	t.Run("uncertain file probe does not fall back", func(t *testing.T) {
		calls := 0
		runner := func(
			_ context.Context, _ string, _ ...string,
		) (string, error) {
			calls++
			return "fatal: detected dubious ownership", errors.New("exit 128")
		}
		got, err := resolveRepositoryWithGit(context.Background(),
			"auto", "/file/plan.md", "/cwd", runner)
		if err != nil || got != "" || calls != 1 {
			t.Fatalf("repository = %q, error = %v, calls = %d", got, err, calls)
		}
	})
}

func gitRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	if origin != "" {
		runGit(t, dir, "remote", "add", "origin", origin)
	}
	return dir
}

func templateForTest(t *testing.T, source string) *template.Template {
	t.Helper()
	tmpl, err := template.New("test").Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
