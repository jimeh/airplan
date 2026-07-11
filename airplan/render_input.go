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

	// NoExternalAssets disables airplan-managed view-time asset loading.
	NoExternalAssets bool

	// MermaidURL overrides the Mermaid module URL. Empty uses the default.
	MermaidURL string

	// Repository is "auto", "none", or an explicit repository URL.
	// The zero value disables repository discovery for direct callers.
	Repository string

	// WorkingDirectory is the invocation directory used by automatic
	// repository discovery. Empty uses the process working directory.
	WorkingDirectory string

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
	if ctx == nil {
		return nil, errors.New("airplan: nil context")
	}
	if opts.Template != nil && opts.TemplatePath != "" {
		return nil, errors.New(
			"airplan: render input: template and template path are " +
				"mutually exclusive",
		)
	}
	mermaidURL, err := resolveMermaidURL(opts.MermaidURL)
	if err != nil {
		return nil, err
	}
	opts.MermaidURL = mermaidURL
	if opts.Repository != "" && opts.Repository != "auto" &&
		opts.Repository != "none" {
		opts.Repository, err = NormalizeRepositoryURL(opts.Repository)
		if err != nil {
			return nil, err
		}
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
		frontMatter, frontMatterErr := parseFrontMatter(data)
		if frontMatterErr != nil {
			return nil, frontMatterErr
		}
		doc.Title = resolveMarkdownTitle(
			in.Title, frontMatter.title, frontMatter.body, in.Name, slug,
		)
		repositoryURL, repositoryErr := resolveRepository(
			ctx, opts.Repository, in.Name, opts.WorkingDirectory,
		)
		if repositoryErr != nil {
			return nil, repositoryErr
		}
		doc.sourceObjectName = slug + ".md"
		if opts.IncludeSource {
			doc.SourcePath = "./" + doc.sourceObjectName
		}
		doc.HTML, err = renderMarkdown(data, frontMatter, RenderOptions{
			Title:            doc.Title,
			Slug:             slug,
			SourceName:       sourceName,
			SourcePath:       doc.SourcePath,
			Indexable:        opts.Indexable,
			NoExternalAssets: opts.NoExternalAssets,
			MermaidURL:       opts.MermaidURL,
			RepositoryURL:    repositoryURL,
			Template:         tmpl,
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
			Title:            doc.Title,
			Slug:             slug,
			SourceName:       sourceName,
			SourcePath:       doc.SourcePath,
			Indexable:        opts.Indexable,
			NoExternalAssets: opts.NoExternalAssets,
			MermaidURL:       opts.MermaidURL,
			Lang:             in.Lang,
			Template:         tmpl,
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

func resolveMarkdownTitle(
	explicit string,
	frontMatterTitle string,
	body []byte,
	filename string,
	slug string,
) string {
	if explicit != "" {
		return explicit
	}
	if frontMatterTitle != "" {
		return frontMatterTitle
	}
	return ResolveTitle("", body, filename, slug)
}

func (d *RenderedDocument) sourceContentType() string {
	if d.Format == FormatMarkdown {
		return sourceContentType
	}
	return textContentType
}
