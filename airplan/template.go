package airplan

import (
	"errors"
	"html/template"
)

// TemplateData is the data contract custom page templates code
// against (SPEC.md §3). The field set is stable API: it may grow,
// but existing fields keep their names and meaning.
type TemplateData struct {
	// Title is the resolved page title.
	Title string

	// Body is the rendered markdown body, or text input's highlighted
	// code block.
	Body template.HTML

	// SourceHTML is the highlighted raw source (markdown input only;
	// empty otherwise).
	SourceHTML template.HTML

	// SourcePath is the relative path to the uploaded source object;
	// empty when the source wasn't uploaded (no-source). Templates
	// must handle both cases.
	SourcePath string

	// Slug is the resolved slug.
	Slug string

	// FileName is the original source filename (text input; empty
	// otherwise).
	FileName string
}

// LoadTemplate parses a custom page template from path. The template
// takes full responsibility for the page: styles, noindex meta, and
// any interactivity (SPEC.md §3). It executes against TemplateData.
func LoadTemplate(path string) (*template.Template, error) {
	return nil, errors.New("airplan: LoadTemplate not implemented")
}

// BuiltinTemplate returns the built-in page template source, for
// `airplan template` to print as a customization starting point
// (SPEC.md §6).
func BuiltinTemplate() string {
	return builtinTemplate
}
