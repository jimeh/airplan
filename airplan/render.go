package airplan

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

//go:embed assets/page.html.tmpl
var builtinTemplateLayout string

//go:embed assets/page.css
var pageCSS string

//go:embed assets/page.js
var pageJS string

// builtinTemplate is the exact reusable custom-template source printed
// by `airplan template`. Page-specific CSS and JavaScript are baked in;
// SyntaxCSS remains data because it is coupled to generated highlighting
// classes (SPEC.md §3).
var builtinTemplate = bakeBuiltinTemplate(pageCSS, pageJS)

// executableBuiltinTemplate omits source comments because html/template
// replaces literal CSS and JS comments with whitespace when parsing.
// Keeping the comments in builtinTemplate makes its dumped customization
// source useful without introducing trailing spaces in rendered pages.
var executableBuiltinTemplate = bakeBuiltinTemplate(
	templateAsset(pageCSS),
	templateAsset(pageJS),
)

func bakeBuiltinTemplate(css, js string) string {
	return strings.NewReplacer(
		"/* airplan:page-css */", css,
		"/* airplan:page-js */", js,
	).Replace(builtinTemplateLayout)
}

// templateAsset removes source-only full-line comments before CSS and
// JavaScript are parsed as literal html/template content. html/template
// replaces those comments with spaces, which would otherwise introduce
// trailing whitespace into generated pages.
func templateAsset(source string) string {
	lines := strings.Split(source, "\n")
	out := make([]string, 0, len(lines))
	inBlockComment := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inBlockComment {
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// Chroma styles used for syntax highlighting in light and dark mode.
const (
	syntaxStyleLight = "github"
	syntaxStyleDark  = "github-dark"
)

var pageTmpl = template.Must(
	template.New("page").Parse(executableBuiltinTemplate),
)

// RenderOptions controls markdown-to-HTML page rendering (SPEC.md §3).
type RenderOptions struct {
	// Title is the resolved page title (see ResolveTitle).
	Title string

	// Slug is the resolved slug, exposed to the page template.
	Slug string

	// SourceName is the original source basename, or "" for stdin.
	SourceName string

	// SourcePath is the relative path to the sibling uploaded .md
	// object (e.g. "./plan.md"). "" means the source was not uploaded
	// and the download link is omitted.
	SourcePath string

	// Indexable omits the robots noindex meta tag when true.
	Indexable bool

	// NoExternalAssets disables airplan-managed view-time asset loading.
	NoExternalAssets bool

	// MermaidURL overrides the Mermaid module URL. Empty uses the default.
	MermaidURL string

	// Lang overrides the highlight language for text input
	// (SPEC.md §3). "" derives it from the filename.
	Lang string

	// Template is a custom page template (SPEC.md §3), executed
	// against TemplateData. nil uses the built-in page. A custom
	// template takes full responsibility for the page: styles,
	// noindex meta, and any interactivity.
	Template *template.Template
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
			alertExtension{},
			mermaidExtension{},
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
			// Markdown and raw HTML inputs share the same trust
			// boundary: preserve the author's HTML and URL destinations.
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

// RenderMarkdown renders markdown source to an HTML page with embedded CSS,
// dark/light-aware syntax highlighting, and conditional Mermaid support
// (SPEC.md §3).
func RenderMarkdown(src []byte, opts RenderOptions) ([]byte, error) {
	md := newMarkdown()
	doc := md.Parser().Parse(text.NewReader(src))
	headings := extractHeadings(doc, src)

	var body bytes.Buffer
	if err := md.Renderer().Render(&body, src, doc); err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}

	// The raw source is embedded highlighted so the rendered/source
	// toggle and "copy markdown" work entirely offline (SPEC.md §3).
	sourceHTML, language, err := highlightSource(src, "", "markdown")
	if err != nil {
		return nil, err
	}

	return renderPage(TemplateData{
		RenderedHTML:          template.HTML(body.String()),
		SourceText:            string(src),
		HighlightedSourceHTML: sourceHTML,
		Headings:              headings,
		TOC:                   tocHeadings(headings),
		Format:                FormatMarkdown.String(),
		Language:              language,
		HasMermaid:            hasMermaid(doc, src),
	}, opts)
}

// RenderText renders plain-text source as a standalone page whose
// body is one syntax-highlighted code block headed by the original
// filename (SPEC.md §3). The highlight language comes from the source
// filename; stdin and unrecognized names fall back to unhighlighted
// plain text and no filename header.
func RenderText(src []byte, name string, opts RenderOptions) ([]byte, error) {
	body, language, err := highlightSource(src, name, opts.Lang)
	if err != nil {
		return nil, err
	}

	fileName := ""
	if name != "" {
		fileName = filepath.Base(name)
	}
	return renderPage(TemplateData{
		RenderedHTML:          body,
		SourceText:            string(src),
		HighlightedSourceHTML: body,
		Format:                FormatText.String(),
		Language:              language,
		SourceName:            fileName,
	}, opts)
}

// renderPage supplies the shared fields before executing either a custom or
// the built-in standalone page template.
func renderPage(data TemplateData, opts RenderOptions) ([]byte, error) {
	mermaidURL, err := resolveMermaidURL(opts.MermaidURL)
	if err != nil {
		return nil, err
	}
	opts.MermaidURL = mermaidURL
	syntax, err := syntaxCSS()
	if err != nil {
		return nil, err
	}

	data.Title = opts.Title
	data.SourcePath = opts.SourcePath
	data.Slug = opts.Slug
	data.Indexable = opts.Indexable
	data.NoExternalAssets = opts.NoExternalAssets
	data.MermaidURL = opts.MermaidURL
	if data.SourceName == "" {
		data.SourceName = opts.SourceName
	}
	data.SyntaxCSS = template.CSS(syntax)

	var out bytes.Buffer
	tmpl := pageTmpl
	label := "page"
	if opts.Template != nil {
		tmpl = opts.Template
		label = "custom template"
	}
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("execute %s: %w", label, err)
	}
	return out.Bytes(), nil
}

// highlightSource renders source bytes as one chroma-highlighted,
// class-based code block. The lexer comes from lang when given, else
// the filename; unrecognized values fall back to plain text
// (SPEC.md §3).
func highlightSource(
	src []byte,
	name string,
	lang string,
) (template.HTML, string, error) {
	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	} else {
		lexer = lexers.Match(filepath.Base(name))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	it, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		return "", "", fmt.Errorf("tokenise source: %w", err)
	}

	var buf bytes.Buffer
	f := chromahtml.New(chromahtml.WithClasses(true))
	err = f.Format(&buf, styles.Get(syntaxStyleLight), it)
	if err != nil {
		return "", "", fmt.Errorf("highlight source: %w", err)
	}
	return template.HTML(buf.String()), lexerLanguage(lexer), nil
}

func lexerLanguage(lexer chroma.Lexer) string {
	config := lexer.Config()
	if len(config.Aliases) > 0 {
		return config.Aliases[0]
	}
	return strings.ToLower(config.Name)
}

// matchesLexerFilename reports whether the highlighter recognizes a
// bare filename (Makefile, Dockerfile, …) — used by format detection
// for extensionless names (SPEC.md §2).
func matchesLexerFilename(name string) bool {
	return lexers.Match(name) != nil
}

// extractHeadings returns every markdown heading and marks a leading H1
// as the document title. Blank lines are absent from the AST; invisible
// HTML comments are ignored when deciding whether the H1 leads the
// visible document (SPEC.md §3).
func extractHeadings(doc ast.Node, src []byte) []Heading {
	var title ast.Node
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if isHTMLComment(n, src) {
			continue
		}
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			title = n
		}
		break
	}

	var headings []Heading
	_ = ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		id, _ := h.AttributeString("id")
		headings = append(headings, Heading{
			Level:   h.Level,
			ID:      attributeString(id),
			Text:    strings.TrimSpace(nodeText(h, src)),
			IsTitle: n == title,
		})
		return ast.WalkSkipChildren, nil
	})
	return headings
}

func isHTMLComment(n ast.Node, src []byte) bool {
	block, ok := n.(*ast.HTMLBlock)
	if !ok {
		return false
	}
	var content bytes.Buffer
	content.Write(block.Lines().Value(src))
	if block.HasClosure() {
		content.Write(block.ClosureLine.Value(src))
	}
	text := bytes.TrimSpace(content.Bytes())
	return bytes.HasPrefix(text, []byte("<!--")) &&
		bytes.HasSuffix(text, []byte("-->"))
}

func attributeString(value any) string {
	switch value := value.(type) {
	case []byte:
		return string(value)
	case string:
		return value
	default:
		return ""
	}
}

func tocHeadings(headings []Heading) []Heading {
	toc := make([]Heading, 0, len(headings))
	for _, heading := range headings {
		if heading.IsTitle || heading.Level > 3 {
			continue
		}
		toc = append(toc, heading)
	}
	if len(toc) < 2 {
		return nil
	}
	return toc
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
