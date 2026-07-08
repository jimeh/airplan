// Package airplan is the core library behind the airplan CLI: config
// loading, input detection, markdown rendering, key generation, and
// uploads to S3-compatible storage. Behavior is specified by SPEC.md;
// anything the CLI can do, a Go consumer can do by calling this
// package directly.
package airplan

import (
	"context"
	"errors"
	"io"
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
	return nil, errors.New("airplan: New not implemented")
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
	return nil, errors.New("airplan: Upload not implemented")
}
