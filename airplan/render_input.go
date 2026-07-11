package airplan

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"path/filepath"
	"unicode/utf8"
)

// RenderInputOptions controls the local input-to-HTML pipeline.
type RenderInputOptions struct {
	// Indexable omits the robots noindex directive when true.
	Indexable bool

	// IncludeSource gives rendered pages the relative source path they
	// will use after upload. Local previews leave this false.
	IncludeSource bool

	// Template is an already-parsed custom page template. It is mutually
	// exclusive with TemplatePath.
	Template *template.Template

	// TemplatePath loads a custom page template from disk.
	TemplatePath string
}

// RenderedDocument is the result of detecting and rendering one input
// without performing storage or manifest operations.
type RenderedDocument struct {
	// HTML is the complete rendered page.
	HTML []byte

	// Format is the detected or explicitly selected input format.
	Format Format

	// Title is the resolved page title.
	Title string

	// Slug is the resolved page slug.
	Slug string

	// SourcePath is the page-relative source path, or "" when source is
	// not included.
	SourcePath string

	// Warnings contains non-fatal rendering outcomes.
	Warnings []string

	source           []byte
	sourceObjectName string
}

// RenderInput reads, detects, and renders one input entirely locally. It
// never validates connection settings, accesses storage, or writes the
// upload manifest (SPEC.md §6).
func RenderInput(
	ctx context.Context,
	in Input,
	opts RenderInputOptions,
) (*RenderedDocument, error) {
	if opts.Template != nil && opts.TemplatePath != "" {
		return nil, errors.New(
			"airplan: render input: template and template path are " +
				"mutually exclusive",
		)
	}

	tmpl := opts.Template
	var tmplErr error
	if tmpl == nil && opts.TemplatePath != "" {
		tmpl, tmplErr = LoadTemplate(opts.TemplatePath)
	}
	return renderInput(ctx, in, opts, tmpl, tmplErr)
}

func renderInput(
	ctx context.Context,
	in Input,
	opts RenderInputOptions,
	tmpl *template.Template,
	tmplErr error,
) (*RenderedDocument, error) {
	if in.Reader == nil {
		return nil, errors.New("airplan: input reader is nil")
	}

	limit := in.MaxSize
	switch {
	case limit == 0:
		limit = DefaultMaxInputSize
	case limit < 0:
		limit = 0
	}
	data, err := readInput(ctx, in.Reader, limit)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, ErrEmptyInput
	}
	if !utf8.Valid(data) {
		return nil, ErrInvalidUTF8
	}
	if IsBinary(data) {
		return nil, ErrBinaryInput
	}

	var format Format
	if in.Format != "" {
		format, err = ParseFormat(in.Format)
		if err != nil {
			return nil, err
		}
	} else {
		format = DetectFormat(in.Name, data)
	}

	slug := in.Slug
	if slug == "" {
		slug = filenameStem(in.Name)
	}
	slug = SanitizeSlug(slug)

	doc := &RenderedDocument{
		Format: format,
		Slug:   slug,
		source: data,
	}
	sourceName := ""
	if in.Name != "" {
		sourceName = filepath.Base(in.Name)
	}

	if tmplErr != nil && format != FormatHTML {
		return nil, tmplErr
	}

	switch format {
	case FormatMarkdown:
		doc.Title = ResolveTitle(in.Title, data, in.Name, slug)
		doc.sourceObjectName = slug + ".md"
		if opts.IncludeSource {
			doc.SourcePath = "./" + doc.sourceObjectName
		}
		doc.HTML, err = RenderMarkdown(data, RenderOptions{
			Title:      doc.Title,
			Slug:       slug,
			SourceName: sourceName,
			SourcePath: doc.SourcePath,
			Indexable:  opts.Indexable,
			Template:   tmpl,
		})

	case FormatText:
		doc.Title = in.Title
		if doc.Title == "" {
			doc.Title = sourceName
		}
		if doc.Title == "" {
			doc.Title = slug
		}
		doc.sourceObjectName = slug + "." + sourceExt(in.Name)
		if opts.IncludeSource {
			doc.SourcePath = "./" + doc.sourceObjectName
		}
		doc.HTML, err = RenderText(data, in.Name, RenderOptions{
			Title:      doc.Title,
			Slug:       slug,
			SourceName: sourceName,
			SourcePath: doc.SourcePath,
			Indexable:  opts.Indexable,
			Lang:       in.Lang,
			Template:   tmpl,
		})

	case FormatHTML:
		doc.Title = ResolveTitle(in.Title, nil, in.Name, slug)
		if tmpl != nil || tmplErr != nil {
			warning := "custom template ignored for HTML input — " +
				"HTML input is used as-is"
			if tmplErr != nil {
				warning += " (note: the template also failed to load: " +
					tmplErr.Error() + ")"
			}
			doc.Warnings = append(doc.Warnings, warning)
		}

		doc.HTML = data
		if !opts.Indexable {
			var outcome NoindexResult
			doc.HTML, outcome = InjectNoindex(data)
			if outcome == NoindexNoHead {
				doc.Warnings = append(doc.Warnings,
					"no <head> tag found; HTML left unmodified, "+
						"without a noindex meta tag")
			}
		}

	default:
		return nil, fmt.Errorf("airplan: unsupported format %v", format)
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (d *RenderedDocument) sourceContentType() string {
	if d.Format == FormatMarkdown {
		return sourceContentType
	}
	return textContentType
}
