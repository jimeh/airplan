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
	"strconv"
	"strings"
)

// pageContentType is the Content-Type of every uploaded page object
// (SPEC.md §5).
const pageContentType = "text/html; charset=utf-8"

// sourceContentType is the Content-Type of the uploaded markdown
// source object (SPEC.md §5).
const sourceContentType = "text/markdown; charset=utf-8"

// textContentType is the Content-Type of text input's uploaded
// original file (SPEC.md §3, §5).
const textContentType = "text/plain; charset=utf-8"

// DefaultMaxInputSize is the default input size limit (SPEC.md §2).
// Documents are loaded whole into memory for rendering or splicing;
// anything past this default is invariably the wrong file.
// Input.MaxSize overrides it.
const DefaultMaxInputSize = 10 << 20 // 10 MiB

// ErrInputTooLarge is returned by Upload when the input exceeds the
// effective size limit. No more than the limit is buffered before the
// overflow is detected. The returned error wraps this sentinel and
// names the actual limit.
var ErrInputTooLarge = errors.New(
	"airplan: input exceeds the maximum input size",
)

// ErrBinaryInput is returned by Upload when the input contains a NUL
// byte within its first 8 KiB (SPEC.md §2). airplan uploads UTF-8
// text documents; there is no bypass.
var ErrBinaryInput = errors.New(
	"airplan: input looks like binary data; " +
		"only UTF-8 text documents are supported",
)

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
	if err := st.checkCredentials(ctx); err != nil {
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

	// MaxSize is the input size limit in bytes (SPEC.md §2):
	// 0 means DefaultMaxInputSize, negative means no limit.
	MaxSize int64
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
	limit := in.MaxSize
	switch {
	case limit == 0:
		limit = DefaultMaxInputSize
	case limit < 0:
		limit = 0 // readInput treats <= 0 as unlimited
	}
	data, err := readInput(in.Reader, limit)
	if err != nil {
		return nil, err
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

	case FormatText:
		// The document is never interpreted, so the title chain is
		// explicit title → filename → slug (SPEC.md §3).
		title = ResolveTitle(in.Title, nil, in.Name, slug)

		sourceName := slug + "." + sourceExt(in.Name)
		sourcePath := ""
		if !c.cfg.NoSource {
			sourcePath = "./" + sourceName
		}
		page, err = RenderText(data, in.Name, RenderOptions{
			Title:      title,
			Slug:       slug,
			SourcePath: sourcePath,
			Indexable:  c.cfg.Indexable,
		})
		if err != nil {
			return nil, err
		}

		if !c.cfg.NoSource {
			sourceKey := BuildKey(c.cfg.KeyPrefix, dir, sourceName)
			err = c.st.put(ctx, object{
				Key:         sourceKey,
				Body:        data,
				ContentType: textContentType,
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

// readInput reads r fully, enforcing limit (in bytes; <= 0 means
// unlimited). It buffers at most one byte past the limit before
// returning ErrInputTooLarge (SPEC.md §2).
func readInput(r io.Reader, limit int64) ([]byte, error) {
	if limit > 0 {
		r = io.LimitReader(r, limit+1)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("airplan: read input: %w", err)
	}
	if limit > 0 && int64(len(data)) > limit {
		return nil, fmt.Errorf("%w of %s", ErrInputTooLarge,
			formatSize(limit))
	}
	return data, nil
}

// ParseSize parses a human-friendly byte size (SPEC.md §2): a plain
// integer byte count, or an integer with a k/m/g suffix meaning
// binary multiples (KiB/MiB/GiB). An optional trailing "b" or "ib" is
// accepted and case is ignored: "10MB", "512k", "1gib", "1048576".
func ParseSize(s string) (int64, error) {
	orig := s
	s = strings.ToLower(strings.TrimSpace(s))

	var mult int64 = 1
	s = strings.TrimSuffix(s, "ib")
	s = strings.TrimSuffix(s, "b")
	switch {
	case strings.HasSuffix(s, "k"):
		mult = 1 << 10
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "m"):
		mult = 1 << 20
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "g"):
		mult = 1 << 30
		s = s[:len(s)-1]
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf(
			"airplan: invalid size %q (examples: 10MB, 512k, 1048576)",
			orig,
		)
	}
	if n > (1<<62)/mult {
		return 0, fmt.Errorf("airplan: size %q is out of range", orig)
	}
	return n * mult, nil
}

// formatSize renders a byte count human-readably: exact binary
// multiples as KiB/MiB/GiB, anything else as a plain byte count.
func formatSize(n int64) string {
	for _, u := range []struct {
		div  int64
		name string
	}{
		{1 << 30, "GiB"},
		{1 << 20, "MiB"},
		{1 << 10, "KiB"},
	} {
		if n >= u.div && n%u.div == 0 {
			return fmt.Sprintf("%d %s", n/u.div, u.name)
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

// filenameStem returns the base name without its extension, or "".
func filenameStem(name string) string {
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// sourceExt returns the sibling source object's extension for text
// input (SPEC.md §3): the source filename's extension sanitized to
// [a-z0-9], or "txt" when absent (stdin) or when it would collide
// with the page object.
func sourceExt(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))

	var b strings.Builder
	for _, r := range ext {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	ext = b.String()

	if ext == "" || ext == "html" || ext == "htm" {
		return "txt"
	}
	return ext
}
