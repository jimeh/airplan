// Package airplan is the core library behind the airplan CLI: config
// loading, input detection, markdown rendering, key generation, and
// uploads to S3-compatible storage. Behavior is specified by SPEC.md;
// anything the CLI can do, a Go consumer can do by calling this
// package directly.
package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
)

// pageContentType is the Content-Type of every uploaded page object
// (SPEC.md §5).
const pageContentType = "text/html; charset=utf-8"

// sourceContentType is the Content-Type of the uploaded markdown
// source object (SPEC.md §5).
const sourceContentType = "text/markdown; charset=utf-8"

// Client uploads plan documents per the pipeline in SPEC.md §1:
// detect format → render (markdown) or noindex-splice (HTML) →
// generate key → upload page (+ markdown source) → assemble URL.
type Client struct {
	cfg *Config
	st  *storage
}

// New validates cfg and returns a ready Client.
func New(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("airplan: nil config")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	st, err := newStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Client{cfg: cfg, st: st}, nil
}

// Input describes one document to upload.
type Input struct {
	// Reader supplies the document bytes.
	Reader io.Reader

	// Name is the source filename ("" for stdin). Used for format
	// detection and the slug/title fallback chains.
	Name string

	// Format overrides detection: "md" or "html". "" means auto
	// (SPEC.md §2).
	Format string

	// Slug overrides the filename portion of the URL (SPEC.md §8).
	Slug string

	// Title overrides the page title (SPEC.md §3).
	Title string
}

// Result describes a completed upload. Bytes and ContentType describe
// the uploaded page object — the one URL points at — not the markdown
// source (SPEC.md §6).
type Result struct {
	URL         string
	Key         string
	SourceURL   string // "" for HTML input or under no-source
	SourceKey   string // "" likewise
	Bucket      string
	Bytes       int64
	ContentType string
	Title       string

	// Warnings collects non-fatal issues (e.g. HTML input with no
	// <head> tag, public URL assembled without public_base_url) for
	// the caller to print to stderr.
	Warnings []string
}

// Upload runs the full pipeline for one document and returns the
// public URL(s). It never returns a URL that was not successfully
// uploaded (SPEC.md §1). For markdown input the original source
// uploads first; failure of either upload fails the whole call
// (SPEC.md §5).
func (c *Client) Upload(ctx context.Context, in Input) (*Result, error) {
	if in.Reader == nil {
		return nil, errors.New("airplan: input reader is nil")
	}
	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return nil, fmt.Errorf("airplan: read input: %w", err)
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

	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate key: %w", err)
	}

	res := &Result{Bucket: c.cfg.Bucket}

	var page []byte
	var title string
	switch format {
	case FormatMarkdown:
		title = ResolveTitle(in.Title, data, in.Name, slug)

		sourcePath := ""
		if !c.cfg.NoSource {
			sourcePath = "./" + slug + ".md"
		}
		page, err = RenderMarkdown(data, RenderOptions{
			Title:      title,
			Slug:       slug,
			SourcePath: sourcePath,
			Indexable:  c.cfg.Indexable,
		})
		if err != nil {
			return nil, err
		}

		// The source uploads first: an orphaned source object is
		// harmless, an orphaned page URL is a broken download link
		// (SPEC.md §5).
		if !c.cfg.NoSource {
			sourceKey := BuildKey(c.cfg.KeyPrefix, dir, slug+".md")
			err = c.st.put(ctx, object{
				Key:         sourceKey,
				Body:        data,
				ContentType: sourceContentType,
				Metadata:    titleMetadata(title),
			})
			if err != nil {
				return nil, err
			}
			res.SourceKey = sourceKey
		}

	case FormatHTML:
		// HTML has no markdown source to inspect; the title chain is
		// explicit title → filename → slug (SPEC.md §4: the document
		// itself is never parsed).
		title = ResolveTitle(in.Title, nil, in.Name, slug)

		page = data
		if !c.cfg.Indexable {
			out, outcome := InjectNoindex(data)
			page = out
			if outcome == NoindexNoHead {
				res.Warnings = append(res.Warnings,
					"no <head> tag found; uploaded HTML unmodified, "+
						"without a noindex meta tag")
			}
		}

	default:
		return nil, fmt.Errorf("airplan: unsupported format %v", format)
	}

	pageKey := BuildKey(c.cfg.KeyPrefix, dir, slug+".html")
	err = c.st.put(ctx, object{
		Key:         pageKey,
		Body:        page,
		ContentType: pageContentType,
		Metadata:    titleMetadata(title),
	})
	if err != nil {
		return nil, err
	}

	res.Key = pageKey
	res.Bytes = int64(len(page))
	res.ContentType = pageContentType
	res.Title = title

	url, fallback := PublicURL(c.cfg, pageKey)
	if fallback {
		res.Warnings = append(res.Warnings,
			"public_base_url is not set; assembled the URL from the "+
				"endpoint and bucket — it may not be publicly reachable")
	}
	res.URL = url

	if res.SourceKey != "" {
		res.SourceURL, _ = PublicURL(c.cfg, res.SourceKey)
	}

	return res, nil
}

// titleMetadata builds the x-amz-meta-title metadata map (SPEC.md §5),
// or nil when the title is empty. HTTP header values must be ASCII and
// S3 implementations vary in how they treat anything else, so
// non-ASCII titles are RFC 2047 Q-encoded (mime.WordDecoder reverses
// it when reading the metadata back).
func titleMetadata(title string) map[string]string {
	if title == "" {
		return nil
	}
	return map[string]string{
		"title": mime.QEncoding.Encode("utf-8", title),
	}
}

// filenameStem returns the base name without its extension, or "".
func filenameStem(name string) string {
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
