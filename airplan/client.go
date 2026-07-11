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
	"html/template"
	"io"
	"math"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// markerContentType is the Content-Type of ownership markers (SPEC.md §5).
const markerContentType = "application/json"

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

// ErrInvalidUTF8 is returned by Upload or RenderInput when the input
// is not valid UTF-8 (SPEC.md §2). There is no bypass.
var ErrInvalidUTF8 = errors.New(
	"airplan: input is not valid UTF-8; " +
		"only UTF-8 text documents are supported",
)

// ErrEmptyInput is returned by Upload or RenderInput for a zero-byte
// document. Whitespace-only input remains valid (SPEC.md §2).
var ErrEmptyInput = errors.New("airplan: input is empty")

// ErrUninitializedClient is returned when a Client method is called on a nil
// or zero-value Client. Construct clients with New before use.
var ErrUninitializedClient = errors.New(
	"airplan: client is not initialized; construct it with airplan.New",
)

// Client uploads plan documents per the pipeline in SPEC.md §1:
// detect format → render (markdown) or noindex-splice (HTML) →
// generate key → upload marker → upload source (if any) → upload page →
// assemble URL.
type Client struct {
	cfg      *Config
	st       *storage
	template *template.Template

	// templateErr is a deferred custom-template load failure: fatal
	// for markdown/text uploads, a warning for HTML input.
	templateErr error
}

// New validates cfg and returns a ready Client.
func New(ctx context.Context, cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("airplan: nil config")
	}
	if ctx == nil {
		return nil, errors.New("airplan: nil context")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// The template is loaded eagerly but its failure is deferred to
	// Upload: templates don't apply to HTML input (SPEC.md §3), so a
	// broken template path must not block an HTML upload — while
	// markdown/text uploads still fail before anything is uploaded.
	var tmpl *template.Template
	var tmplErr error
	if cfg.Template != "" {
		tmpl, tmplErr = LoadTemplate(cfg.Template)
	}

	st, err := newStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := st.checkCredentials(ctx); err != nil {
		return nil, err
	}

	return &Client{
		cfg:         cfg,
		st:          st,
		template:    tmpl,
		templateErr: tmplErr,
	}, nil
}

// Input describes one document to upload.
type Input struct {
	// Reader supplies the document bytes. Context cancellation stops waiting
	// for a blocked Reader, but cannot interrupt an arbitrary Reader itself;
	// callers retaining a long-lived reader must unblock or close it.
	Reader io.Reader

	// Name is the source filename ("" for stdin). Used for format
	// detection and the slug/title fallback chains.
	Name string

	// Format overrides detection: "md", "html", or "txt". "" means auto
	// (SPEC.md §2).
	Format string

	// Slug overrides the filename portion of the URL (SPEC.md §8).
	Slug string

	// Title overrides the page title (SPEC.md §3).
	Title string

	// MaxSize is the input size limit in bytes (SPEC.md §2):
	// 0 means DefaultMaxInputSize, negative means no limit.
	MaxSize int64

	// Lang overrides the highlight language for text input
	// (SPEC.md §3). "" derives it from the filename.
	Lang string
}

// Result describes a completed upload. Bytes and ContentType describe
// the uploaded page object — the one URL points at — not the markdown
// source (SPEC.md §6).
type Result struct {
	URL           string
	Key           string
	SourceURL     string // "" for HTML input or under no-source
	SourceKey     string // "" likewise
	Bucket        string
	Bytes         int64
	ContentType   string
	Title         string
	CreatedAt     time.Time
	MarkerVersion int

	// Warnings collects non-fatal issues (e.g. HTML input with no
	// <head> tag, public URL assembled without public_base_url) for
	// the caller to print to stderr.
	Warnings []string
}

// Upload runs the full pipeline for one document and returns the
// public URL(s). It never returns a URL that was not successfully
// uploaded. The ownership marker uploads before the optional source
// and page; failure of any PUT fails the whole call (SPEC.md §1, §5).
func (c *Client) Upload(ctx context.Context, in Input) (*Result, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	doc, err := renderInput(ctx, in, RenderInputOptions{
		Indexable:        c.cfg.Indexable,
		IncludeSource:    !c.cfg.NoSource,
		NoExternalAssets: c.cfg.NoExternalAssets,
		MermaidURL:       c.cfg.MermaidURL,
	}, c.template, c.templateErr)
	if err != nil {
		return nil, err
	}

	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate key: %w", err)
	}

	createdAt := time.Now().UTC().Truncate(time.Second)
	pageName := doc.Slug + ".html"
	pageKey := BuildKey(c.cfg.KeyPrefix, dir, pageName)
	sourceName := ""
	if doc.sourceObjectName != "" && doc.SourcePath != "" {
		sourceName = doc.sourceObjectName
	}
	marker := UploadMarker{
		Schema:    MarkerSchema,
		Version:   MarkerVersion,
		Directory: dir,
		CreatedAt: createdAt,
		Format:    doc.Format.String(),
		Page:      pageName,
		Source:    sourceName,
		Title:     doc.Title,
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		return nil, err
	}
	markerKey := BuildKey(c.cfg.KeyPrefix, dir, MarkerFilename)
	if err := c.st.put(ctx, object{
		Key:         markerKey,
		Body:        markerBody,
		ContentType: markerContentType,
	}); err != nil {
		return nil, err
	}

	res := &Result{
		Bucket:        c.cfg.Bucket,
		Title:         doc.Title,
		CreatedAt:     createdAt,
		MarkerVersion: MarkerVersion,
		Warnings:      append([]string(nil), doc.Warnings...),
	}

	// The marker uploads first so any later failure remains visibly owned and
	// recoverable through remote management (SPEC.md §5).
	if doc.sourceObjectName != "" && doc.SourcePath != "" {
		sourceKey := BuildKey(
			c.cfg.KeyPrefix, dir, doc.sourceObjectName,
		)
		err = c.st.put(ctx, object{
			Key:         sourceKey,
			Body:        doc.source,
			ContentType: doc.sourceContentType(),
			Metadata:    titleMetadata(doc.Title),
		})
		if err != nil {
			return nil, err
		}
		res.SourceKey = sourceKey
	}

	err = c.st.put(ctx, object{
		Key:         pageKey,
		Body:        doc.HTML,
		ContentType: pageContentType,
		Metadata:    titleMetadata(doc.Title),
	})
	if err != nil {
		return nil, err
	}

	res.Key = pageKey
	res.Bytes = int64(len(doc.HTML))
	res.ContentType = pageContentType

	url, fallback, err := PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	if fallback {
		res.Warnings = append(res.Warnings, publicURLFallbackWarning)
	}
	res.URL = url

	if res.SourceKey != "" {
		res.SourceURL, _, err = PublicURL(c.cfg, res.SourceKey)
		if err != nil {
			return nil, err
		}
	}

	c.recordUpload(ctx, res)

	return res, nil
}

func (c *Client) validate(ctx context.Context) error {
	if c == nil || c.cfg == nil {
		return ErrUninitializedClient
	}
	if ctx == nil {
		return errors.New("airplan: nil context")
	}
	return nil
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
// returning ErrInputTooLarge (SPEC.md §2). Cancelling ctx aborts the
// wait — so stalled input cannot outlive its operation timeout —
// though the reading goroutine itself stays blocked until the
// underlying reader unblocks or the process exits.
func readInput(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	if limit > 0 && limit < math.MaxInt64 {
		r = io.LimitReader(r, limit+1)
	}

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		ch <- result{data, err}
	}()

	var data []byte
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("airplan: read input: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("airplan: read input: %w", res.err)
		}
		data = res.data
	}

	if limit > 0 && int64(len(data)) > limit {
		return nil, fmt.Errorf("%w of %s", ErrInputTooLarge,
			formatSize(limit))
	}
	return data, nil
}

// ParseSize parses a human-friendly byte size (SPEC.md §2): a plain
// integer byte count, or an integer with a k/m/g suffix meaning
// binary multiples (KiB/MiB/GiB). Unit suffixes may have a trailing
// "b" or "ib", and case is ignored: "10MB", "512k", "1gib",
// "1048576". A tail without k/m/g, such as "10ib", is invalid.
func ParseSize(s string) (int64, error) {
	orig := s
	s = strings.ToLower(strings.TrimSpace(s))

	var mult int64 = 1
	for _, unit := range []struct {
		suffix string
		mult   int64
	}{
		{"kib", 1 << 10},
		{"kb", 1 << 10},
		{"k", 1 << 10},
		{"mib", 1 << 20},
		{"mb", 1 << 20},
		{"m", 1 << 20},
		{"gib", 1 << 30},
		{"gb", 1 << 30},
		{"g", 1 << 30},
	} {
		if strings.HasSuffix(s, unit.suffix) {
			mult = unit.mult
			s = strings.TrimSuffix(s, unit.suffix)
			break
		}
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
