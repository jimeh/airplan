package airplan

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"strings"

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

//go:embed assets/collection.html.tmpl
var builtinCollectionTemplateLayout string

//go:embed assets/generated/readable/page.css
var readablePageCSS string

//go:embed assets/generated/readable/collection.css
var readableCollectionCSS string

//go:embed assets/generated/readable/theme-init.js
var readableThemeInitJS string

//go:embed assets/generated/readable/page.js
var readablePageJS string

//go:embed assets/generated/readable/mermaid.js.tmpl
var readableMermaidJS string

//go:embed assets/generated/readable/collection.js
var readableCollectionJS string

//go:embed assets/generated/minified/page.css
var minifiedPageCSS string

//go:embed assets/generated/minified/collection.css
var minifiedCollectionCSS string

//go:embed assets/generated/minified/theme-init.js
var minifiedThemeInitJS string

//go:embed assets/generated/minified/page.js
var minifiedPageJS string

//go:embed assets/generated/minified/mermaid.js.tmpl
var minifiedMermaidJS string

//go:embed assets/generated/minified/collection.js
var minifiedCollectionJS string

//go:embed assets/generated/icons.html.tmpl
var generatedIconTemplates string

//go:embed assets/theme-toggle.html.tmpl
var themeToggle string

// builtinTemplate is the exact reusable custom-template source printed
// by `airplan template`. Shared and page-specific assets are baked in;
// SyntaxCSS remains data because it is coupled to generated highlighting
// classes (SPEC.md §3).
var builtinTemplate = bakeTemplate(builtinTemplateLayout,
	templateReplacement{"<!-- airplan:icon-templates -->", generatedIconTemplates},
	templateReplacement{"/* airplan:page-css */", readablePageCSS},
	templateReplacement{"/* airplan:theme-init-js */", readableThemeInitJS},
	templateReplacement{"/* airplan:page-js */", readablePageJS},
	templateReplacement{"/* airplan:mermaid-js */", readableMermaidJS},
	templateReplacement{"<!-- airplan:theme-toggle -->", themeToggle},
)

// builtinCollectionTemplate is the exact reusable custom-template source
// printed by `airplan template collection`.
var builtinCollectionTemplate = bakeTemplate(builtinCollectionTemplateLayout,
	templateReplacement{"<!-- airplan:icon-templates -->", generatedIconTemplates},
	templateReplacement{"/* airplan:collection-css */", readableCollectionCSS},
	templateReplacement{"/* airplan:theme-init-js */", readableThemeInitJS},
	templateReplacement{"/* airplan:collection-js */", readableCollectionJS},
	templateReplacement{"<!-- airplan:theme-toggle -->", themeToggle},
)

// executableBuiltinTemplate omits source comments because html/template
// replaces literal CSS and JS comments with whitespace when parsing.
// Keeping the comments in builtinTemplate makes its dumped customization
// source useful without introducing trailing spaces in rendered pages.
var executableBuiltinTemplate = bakeTemplate(builtinTemplateLayout,
	templateReplacement{"<!-- airplan:icon-templates -->", generatedIconTemplates},
	templateReplacement{"/* airplan:page-css */", minifiedPageCSS},
	templateReplacement{"/* airplan:theme-init-js */", minifiedThemeInitJS},
	templateReplacement{"/* airplan:page-js */", minifiedPageJS},
	templateReplacement{"/* airplan:mermaid-js */", minifiedMermaidJS},
	templateReplacement{"<!-- airplan:theme-toggle -->", themeToggle},
)

var executableBuiltinCollectionTemplate = bakeTemplate(
	builtinCollectionTemplateLayout,
	templateReplacement{"<!-- airplan:icon-templates -->", generatedIconTemplates},
	templateReplacement{"/* airplan:collection-css */", minifiedCollectionCSS},
	templateReplacement{"/* airplan:theme-init-js */", minifiedThemeInitJS},
	templateReplacement{"/* airplan:collection-js */", minifiedCollectionJS},
	templateReplacement{"<!-- airplan:theme-toggle -->", themeToggle},
)

type templateReplacement struct {
	marker, source string
}

func bakeTemplate(layout string, replacements ...templateReplacement) string {
	pairs := make([]string, 0, len(replacements)*2)
	for _, replacement := range replacements {
		pairs = append(pairs, replacement.marker, replacement.source)
	}
	return strings.NewReplacer(pairs...).Replace(layout)
}

// Source HTML uses Chroma classes; per-theme CSS supplies their colors.
const syntaxStyleLight = "github"

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

	// RepositoryURL is the resolved canonical repository context used to
	// link repository references in Markdown. Empty disables the feature.
	RepositoryURL string

	// Lang overrides the highlight language for text input
	// (SPEC.md §3). "" derives it from the filename.
	Lang string

	// Template is a custom page template (SPEC.md §3), executed
	// against TemplateData. nil uses the built-in page. A custom
	// template takes full responsibility for the page: styles,
	// noindex meta, and any interactivity.
	Template *template.Template

	// Themes is the resolved page-local catalog. nil uses the built-in defaults.
	Themes *ThemeBundle

	Revision         int
	RevisionCount    int
	PreviousRevision int
	VersionsPath     string
	DiffPath         string
	DiffText         string
	RevisionChainID  string
	PageChanged      bool
	PageDiffText     string
	CompleteDiffText string
	HasCompleteDiff  bool
	AllChangesPath   string
	structuredDiff   bool

	Pages               []DocumentTemplatePage
	CurrentPage         DocumentTemplatePage
	Entrypoint          string
	Assets              []DocumentTemplateAsset
	ManagedPagePaths    map[string]string
	CurrentLogicalPath  string
	CurrentRenderedPath string
}

// newMarkdown builds the goldmark instance implementing the dialect of
// SPEC.md §3: CommonMark + GFM (tables, strikethrough, task lists,
// autolinks) + footnotes + heading anchors, with class-based chroma
// highlighting so CSS can switch palettes on prefers-color-scheme.
func newMarkdown() goldmark.Markdown {
	return newMarkdownWithRepository("", nil)
}

func newMarkdownWithRepository(repository string, source []byte) goldmark.Markdown {
	extensions := []goldmark.Extender{
		extension.GFM,
		extension.DefinitionList,
		newColumnsExtension(source),
		extension.Footnote,
		alertExtension{},
		mermaidExtension{},
		highlighting.NewHighlighting(
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(true),
			),
		),
	}
	if repository != "" {
		extensions = append(extensions, newRepositoryLinkExtension(repository))
	}
	return goldmark.New(
		goldmark.WithExtensions(extensions...),
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

func compactScopedSyntaxCSS(css string, scopes ...string) (string, error) {
	var out strings.Builder
	for line := range strings.SplitSeq(css, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		brace := strings.Index(line, "{")
		if brace < 1 || !strings.HasSuffix(line, "}") {
			return "", fmt.Errorf("airplan: compact unexpected Chroma CSS rule %q", line)
		}

		selectors := strings.Split(line[:brace], ",")
		written := 0
		for _, selector := range selectors {
			selector = strings.TrimSpace(selector)
			if len(scopes) == 0 {
				if written > 0 {
					out.WriteByte(',')
				}
				out.WriteString(selector)
				written++
				continue
			}
			for _, scope := range scopes {
				if written > 0 {
					out.WriteByte(',')
				}
				out.WriteString(scope)
				out.WriteByte(' ')
				out.WriteString(selector)
				written++
			}
		}
		out.WriteByte('{')
		declarations := strings.Split(line[brace+1:len(line)-1], ";")
		written = 0
		for _, declaration := range declarations {
			declaration = strings.TrimSpace(declaration)
			if declaration == "" {
				continue
			}
			colon := strings.IndexByte(declaration, ':')
			if colon < 1 {
				return "", fmt.Errorf("airplan: compact unexpected Chroma CSS declaration %q", declaration)
			}
			if written > 0 {
				out.WriteByte(';')
			}
			out.WriteString(strings.TrimSpace(declaration[:colon]))
			out.WriteByte(':')
			out.WriteString(strings.TrimSpace(declaration[colon+1:]))
			written++
		}
		out.WriteByte('}')
	}
	return out.String(), nil
}

// RenderMarkdown renders markdown source to an HTML page with embedded CSS,
// dark/light-aware syntax highlighting, and conditional Mermaid support
// (SPEC.md §3).
func RenderMarkdown(src []byte, opts RenderOptions) ([]byte, error) {
	frontMatter, err := parseFrontMatter(src)
	if err != nil {
		return nil, err
	}
	return renderMarkdown(src, frontMatter, opts)
}

func renderMarkdown(
	src []byte, frontMatter frontMatter, opts RenderOptions,
) ([]byte, error) {
	bodySource := frontMatter.body
	md := newMarkdownWithRepository(opts.RepositoryURL, bodySource)
	doc := md.Parser().Parse(text.NewReader(bodySource))
	if err := rewriteManagedPageLinks(doc, opts); err != nil {
		return nil, err
	}
	headings := extractHeadings(doc, bodySource)

	var body bytes.Buffer
	if err := md.Renderer().Render(&body, bodySource, doc); err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}

	// The raw source is embedded highlighted so the rendered/source
	// toggle and "copy markdown" work entirely offline (SPEC.md §3).
	sourceHTML, language, err := highlightSource(src, "", "markdown")
	if err != nil {
		return nil, err
	}
	var highlightedFrontMatter template.HTML
	if len(frontMatter.text) > 0 {
		highlightedFrontMatter, _, err = highlightSource(
			frontMatter.text, "", frontMatter.format,
		)
		if err != nil {
			return nil, err
		}
	}
	pageDiffText := opts.PageDiffText
	completeDiffText := opts.CompleteDiffText
	pageChanged := opts.PageChanged
	if !opts.structuredDiff && opts.Revision > 1 && opts.DiffPath != "" {
		pageChanged = true
	}
	if pageDiffText == "" && opts.DiffText != "" {
		pageDiffText = opts.DiffText
		completeDiffText = opts.DiffText
		pageChanged = true
	}
	var highlightedPageDiff template.HTML
	if pageDiffText != "" {
		highlightedPageDiff, _, err = highlightSource([]byte(pageDiffText), DiffFilename, "diff")
		if err != nil {
			return nil, err
		}
	}
	var highlightedCompleteDiff template.HTML
	if completeDiffText != "" {
		highlightedCompleteDiff, _, err = highlightSource([]byte(completeDiffText), DiffFilename, "diff")
		if err != nil {
			return nil, err
		}
	}

	return renderPage(TemplateData{
		RenderedHTML:                template.HTML(body.String()),
		SourceText:                  string(src),
		HighlightedSourceHTML:       sourceHTML,
		Headings:                    headings,
		TOC:                         tocHeadings(headings),
		Format:                      FormatMarkdown.String(),
		Language:                    language,
		HasMermaid:                  hasMermaid(doc, bodySource),
		FrontMatterText:             string(frontMatter.text),
		FrontMatterFormat:           frontMatter.format,
		FrontMatterTitle:            frontMatter.title,
		HighlightedFrontMatterHTML:  highlightedFrontMatter,
		RepositoryURL:               opts.RepositoryURL,
		Revision:                    opts.Revision,
		RevisionCount:               opts.RevisionCount,
		PreviousRevision:            opts.PreviousRevision,
		VersionsPath:                opts.VersionsPath,
		DiffPath:                    opts.DiffPath,
		DiffText:                    opts.DiffText,
		RevisionChainID:             opts.RevisionChainID,
		PageChanged:                 pageChanged,
		PageDiffText:                pageDiffText,
		HighlightedPageDiffHTML:     highlightedPageDiff,
		CompleteDiffText:            completeDiffText,
		HasCompleteDiff:             opts.HasCompleteDiff,
		HighlightedCompleteDiffHTML: highlightedCompleteDiff,
		AllChangesPath:              opts.AllChangesPath,
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
		Revision:              opts.Revision,
		RevisionCount:         opts.RevisionCount,
		PreviousRevision:      opts.PreviousRevision,
		VersionsPath:          opts.VersionsPath,
		DiffPath:              opts.DiffPath,
		RevisionChainID:       opts.RevisionChainID,
		PageChanged:           opts.PageChanged,
		PageDiffText:          opts.PageDiffText,
		CompleteDiffText:      opts.CompleteDiffText,
		HasCompleteDiff:       opts.HasCompleteDiff,
		AllChangesPath:        opts.AllChangesPath,
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
	bundle := opts.Themes
	if bundle == nil {
		bundle = defaultThemeBundle()
	}

	data.Title = opts.Title
	data.SourcePath = opts.SourcePath
	data.Slug = opts.Slug
	data.Indexable = opts.Indexable
	data.NoExternalAssets = opts.NoExternalAssets
	data.MermaidURL = opts.MermaidURL
	data.Revision = opts.Revision
	data.RevisionCount = opts.RevisionCount
	data.PreviousRevision = opts.PreviousRevision
	data.VersionsPath = opts.VersionsPath
	data.DiffPath = opts.DiffPath
	data.DiffText = opts.DiffText
	data.RevisionChainID = opts.RevisionChainID
	data.PageChanged = opts.PageChanged || data.PageChanged
	if data.PageDiffText == "" && opts.DiffText != "" {
		data.PageDiffText = opts.DiffText
		data.CompleteDiffText = opts.DiffText
		data.PageChanged = true
	}
	if !opts.structuredDiff && opts.Revision > 1 && opts.DiffPath != "" {
		data.PageChanged = true
	}
	if data.PageDiffText == "" {
		data.PageDiffText = opts.PageDiffText
	}
	if data.CompleteDiffText == "" {
		data.CompleteDiffText = opts.CompleteDiffText
	}
	data.AllChangesPath = opts.AllChangesPath
	data.HasCompleteDiff = opts.HasCompleteDiff || data.HasCompleteDiff
	if data.DiffText != "" && data.HighlightedDiffHTML == "" {
		highlighted, _, highlightErr := highlightSource(
			[]byte(data.DiffText), DiffFilename, "diff",
		)
		if highlightErr != nil {
			return nil, highlightErr
		}
		data.HighlightedDiffHTML = highlighted
	}
	if data.PageDiffText != "" && data.HighlightedPageDiffHTML == "" {
		highlighted, _, highlightErr := highlightSource(
			[]byte(data.PageDiffText), DiffFilename, "diff",
		)
		if highlightErr != nil {
			return nil, highlightErr
		}
		data.HighlightedPageDiffHTML = highlighted
	}
	if data.CompleteDiffText != "" && data.HighlightedCompleteDiffHTML == "" {
		highlighted, _, highlightErr := highlightSource(
			[]byte(data.CompleteDiffText), DiffFilename, "diff",
		)
		if highlightErr != nil {
			return nil, highlightErr
		}
		data.HighlightedCompleteDiffHTML = highlighted
	}
	if len(opts.Pages) == 0 {
		logical := opts.CurrentLogicalPath
		if logical == "" {
			logical = opts.SourceName
		}
		if logical == "" {
			logical = "document." + data.Format
		}
		entrypoint := opts.CurrentRenderedPath
		if entrypoint == "" {
			entrypoint = opts.Slug + ".html"
		}
		current := DocumentTemplatePage{
			Path: logical, Title: opts.Title, URL: entrypoint, Current: true,
		}
		data.Pages = []DocumentTemplatePage{current}
		data.CurrentPage = current
		data.Entrypoint = entrypoint
	} else {
		data.Pages = opts.Pages
		data.CurrentPage = opts.CurrentPage
		data.Entrypoint = opts.Entrypoint
	}
	data.PageNavigation = buildDocumentPageNavigation(data.Pages)
	data.CurrentPageBreadcrumbs = strings.Split(data.CurrentPage.Path, "/")
	if data.AllChangesPath == "" && data.Revision > 1 && data.DiffPath != "" {
		data.AllChangesPath = data.Entrypoint + "#airplan-all-changes"
	}
	if !opts.structuredDiff && data.Revision > 1 && data.DiffPath != "" {
		data.HasCompleteDiff = true
	}
	data.Assets = opts.Assets
	if data.SourceName == "" {
		data.SourceName = opts.SourceName
	}
	data.SyntaxCSS = bundle.SyntaxCSS
	data.ThemeCSS = bundle.CSS
	data.ThemeCatalogJSON = bundle.CatalogJSON
	if data.HasMermaid {
		data.MermaidThemeJSON = bundle.MermaidJSON
	}
	data.DefaultLightTheme = bundle.DefaultLight
	data.DefaultDarkTheme = bundle.DefaultDark
	data.AppearanceEnabled = len(bundle.Catalog) > 1

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

func buildDocumentPageNavigation(
	pages []DocumentTemplatePage,
) []DocumentTemplateNavigationItem {
	var navigation []DocumentTemplateNavigationItem
	for _, page := range pages {
		segments := strings.Split(page.Path, "/")
		items := &navigation
		directoryPath := ""
		for _, segment := range segments[:len(segments)-1] {
			if directoryPath == "" {
				directoryPath = segment
			} else {
				directoryPath += "/" + segment
			}
			index := navigationDirectoryIndex(*items, directoryPath)
			if index < 0 {
				*items = append(*items, DocumentTemplateNavigationItem{
					Name: segment, Path: directoryPath, IsDirectory: true,
				})
				index = len(*items) - 1
			}
			if page.Current {
				(*items)[index].Current = true
			}
			items = &(*items)[index].Children
		}
		*items = append(*items, DocumentTemplateNavigationItem{
			Name: pathpkg.Base(page.Path), Path: page.Path, Title: page.Title,
			URL: page.URL, Current: page.Current,
		})
	}
	return navigation
}

func navigationDirectoryIndex(
	items []DocumentTemplateNavigationItem,
	path string,
) int {
	for index := range items {
		if items[index].IsDirectory && items[index].Path == path {
			return index
		}
	}
	return -1
}

func rewriteManagedPageLinks(doc ast.Node, opts RenderOptions) error {
	if len(opts.ManagedPagePaths) == 0 || opts.CurrentLogicalPath == "" ||
		opts.CurrentRenderedPath == "" {
		return nil
	}
	return ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := node.(*ast.Link)
		if !ok || len(link.Destination) == 0 {
			return ast.WalkContinue, nil
		}
		parsed, err := url.Parse(string(link.Destination))
		if err != nil || parsed.IsAbs() || parsed.Host != "" ||
			strings.HasPrefix(string(link.Destination), "//") ||
			strings.HasPrefix(parsed.Path, "/") || parsed.Path == "" {
			return ast.WalkContinue, nil
		}
		logical := pathpkg.Clean(pathpkg.Join(pathpkg.Dir(opts.CurrentLogicalPath), parsed.Path))
		if logical == "." || logical == ".." || strings.HasPrefix(logical, "../") ||
			!validMarkerObjectPath(logical) {
			return ast.WalkContinue, nil
		}
		target, exists := opts.ManagedPagePaths[logical]
		if !exists {
			return ast.WalkContinue, nil
		}
		relative, err := urlPathRelative(pathpkg.Dir(opts.CurrentRenderedPath), target)
		if err != nil {
			return ast.WalkStop, err
		}
		parsed.Path = relative
		parsed.RawPath = ""
		link.Destination = []byte(parsed.String())
		return ast.WalkContinue, nil
	})
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
	doc := newMarkdownWithRepository("", src).Parser().Parse(text.NewReader(src))

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
		switch value := n.(type) {
		case *ast.Text:
			b.Write(value.Segment.Value(src))
		case *ast.String:
			b.Write(value.Value)
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
