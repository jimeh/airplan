package airplan

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"sync"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
)

//go:embed assets/page.html.tmpl
var builtinTemplate string

//go:embed assets/page.css
var pageCSS string

// Chroma styles used for syntax highlighting in light and dark mode.
const (
	syntaxStyleLight = "github"
	syntaxStyleDark  = "github-dark"
)

var pageTmpl = template.Must(
	template.New("page").Parse(builtinTemplate),
)

// RenderOptions controls markdown-to-HTML page rendering (SPEC.md §3).
type RenderOptions struct {
	// Title is the resolved page title (see ResolveTitle).
	Title string

	// Slug is the resolved slug, exposed to the page template.
	Slug string

	// SourcePath is the relative path to the sibling uploaded .md
	// object (e.g. "./plan.md"). "" means the source was not uploaded
	// and the download link is omitted.
	SourcePath string

	// Indexable omits the robots noindex meta tag when true.
	Indexable bool
}

// pageData feeds the built-in page template. The exported field names
// Title, Body, SourcePath, and Slug match the custom-template data
// contract of SPEC.md §3.
type pageData struct {
	Title      string
	Body       template.HTML
	SourcePath string
	Slug       string
	Noindex    bool
	CSS        template.CSS
	SyntaxCSS  template.CSS
}

// newMarkdown builds the goldmark instance implementing the dialect of
// SPEC.md §3: CommonMark + GFM (tables, strikethrough, task lists,
// autolinks) + footnotes + heading anchors, with class-based chroma
// highlighting so CSS can switch palettes on prefers-color-scheme.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(true),
				),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			// Plan documents are the author's own content; raw HTML
			// passes through like on GitHub READMEs rendered locally.
			html.WithUnsafe(),
		),
	)
}

// syntaxCSS renders chroma's class-based stylesheets for the light and
// dark palettes, each scoped to its own prefers-color-scheme media
// query so highlighting follows the page theme (SPEC.md §3). Both
// palettes must be fully scoped: the styles define different token
// class sets, so an unscoped light palette would leak dark-on-dark
// colors into dark mode for classes the dark style doesn't override.
var syntaxCSS = sync.OnceValues(func() (string, error) {
	f := chromahtml.New(chromahtml.WithClasses(true))

	var b strings.Builder
	b.WriteString("@media (prefers-color-scheme: light) {\n")
	err := f.WriteCSS(&b, styles.Get(syntaxStyleLight))
	if err != nil {
		return "", fmt.Errorf("render light syntax css: %w", err)
	}
	b.WriteString("}\n")

	b.WriteString("@media (prefers-color-scheme: dark) {\n")
	err = f.WriteCSS(&b, styles.Get(syntaxStyleDark))
	if err != nil {
		return "", fmt.Errorf("render dark syntax css: %w", err)
	}
	b.WriteString("}\n")

	return b.String(), nil
})

// RenderMarkdown renders markdown source to a fully standalone HTML
// page: embedded CSS, no external assets, dark/light aware, syntax
// highlighted code blocks (SPEC.md §3).
func RenderMarkdown(src []byte, opts RenderOptions) ([]byte, error) {
	var body bytes.Buffer
	if err := newMarkdown().Convert(src, &body); err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}

	syntax, err := syntaxCSS()
	if err != nil {
		return nil, err
	}

	data := pageData{
		Title:      opts.Title,
		Body:       template.HTML(body.String()),
		SourcePath: opts.SourcePath,
		Slug:       opts.Slug,
		Noindex:    !opts.Indexable,
		CSS:        template.CSS(pageCSS),
		SyntaxCSS:  template.CSS(syntax),
	}

	var out bytes.Buffer
	if err := pageTmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("execute page template: %w", err)
	}
	return out.Bytes(), nil
}

// ExtractTitle returns the text of the first level-1 heading in the
// markdown source, or "" if there is none.
func ExtractTitle(src []byte) string {
	doc := newMarkdown().Parser().Parse(text.NewReader(src))

	var title string
	_ = ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level != 1 {
			return ast.WalkContinue, nil
		}
		title = nodeText(h, src)
		return ast.WalkStop, nil
	})

	return strings.TrimSpace(title)
}

// nodeText collects the plain text content beneath a node.
func nodeText(node ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(node, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		if t, ok := n.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// ResolveTitle applies the title fallback chain of SPEC.md §3:
// explicit --title, else first <h1>, else source filename, else the
// resolved slug.
func ResolveTitle(explicit string, src []byte, filename, slug string) string {
	if explicit != "" {
		return explicit
	}
	if t := ExtractTitle(src); t != "" {
		return t
	}
	if filename != "" {
		base := filepath.Base(filename)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if base != "" && base != "." {
			return base
		}
	}
	return slug
}
