package airplan

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
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
