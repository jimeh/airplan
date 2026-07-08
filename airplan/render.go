package airplan

import "errors"

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

// RenderMarkdown renders markdown source to a fully standalone HTML
// page: embedded CSS, no external assets, dark/light aware, syntax
// highlighted code blocks (SPEC.md §3).
func RenderMarkdown(src []byte, opts RenderOptions) ([]byte, error) {
	return nil, errors.New("airplan: RenderMarkdown not implemented")
}

// ExtractTitle returns the text of the first level-1 heading in the
// markdown source, or "" if there is none.
func ExtractTitle(src []byte) string {
	return ""
}

// ResolveTitle applies the title fallback chain of SPEC.md §3:
// explicit --title, else first <h1>, else source filename, else the
// resolved slug.
func ResolveTitle(explicit string, src []byte, filename, slug string) string {
	return ""
}
