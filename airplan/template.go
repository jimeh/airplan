package airplan

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// Heading describes one markdown heading exposed to page templates.
type Heading struct {
	// Level is the markdown heading level, from 1 through 6.
	Level int

	// ID is the rendered heading anchor.
	ID string

	// Text is the heading's plain-text content.
	Text string

	// IsTitle reports whether this is a leading H1 used as the document
	// title and omitted from the built-in table of contents.
	IsTitle bool
}

// TemplateData is the data contract custom page templates code
// against (SPEC.md §3). The field set is stable API: it may grow,
// but existing fields keep their names and meaning.
type TemplateData struct {
	// Title is the resolved page title.
	Title string

	// RenderedHTML is the rendered markdown body, or text input's
	// highlighted code block.
	RenderedHTML template.HTML

	// SourceText is the original, unmodified markdown or text source.
	SourceText string

	// HighlightedSourceHTML is the syntax-highlighted original source.
	HighlightedSourceHTML template.HTML

	// SyntaxCSS styles the classes used by RenderedHTML and
	// HighlightedSourceHTML.
	SyntaxCSS template.CSS

	// Headings contains every markdown heading in document order. It is
	// empty for text input.
	Headings []Heading

	// TOC contains the H1-H3 headings used by the built-in table of
	// contents, excluding a leading title H1. It is empty when fewer than
	// two entries remain and for text input.
	TOC []Heading

	// Format is "md" or "txt".
	Format string

	// Language is the resolved source-highlighting language.
	Language string

	// SourceName is the original source basename, or "" for stdin.
	SourceName string

	// Indexable reports whether the page should allow indexing.
	Indexable bool

	// SourcePath is the relative path to the uploaded source object;
	// empty when the source wasn't uploaded (no-source). Templates
	// must handle both cases.
	SourcePath string

	// Slug is the resolved slug.
	Slug string

	// HasMermaid reports whether rendered markdown contains Mermaid diagrams.
	HasMermaid bool

	// NoExternalAssets disables airplan-managed view-time asset loading.
	NoExternalAssets bool

	// MermaidURL is the configured Mermaid ECMAScript module URL.
	MermaidURL string
}

// LoadTemplate parses a custom page template from path. The template
// takes full responsibility for the page: styles, noindex meta, and
// any interactivity (SPEC.md §3). It executes against TemplateData.
func LoadTemplate(path string) (*template.Template, error) {
	resolved := path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf(
				"airplan: resolve template path %q: %w", path, err,
			)
		}
		resolved = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}

	src, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("airplan: read template %q: %w", path, err)
	}
	tmpl, err := template.New(filepath.Base(path)).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("airplan: parse template %q: %w", path, err)
	}
	return tmpl, nil
}

// BuiltinTemplate returns the built-in page template source, for
// `airplan template` to print as a customization starting point
// (SPEC.md §6).
func BuiltinTemplate() string {
	return builtinTemplate
}
