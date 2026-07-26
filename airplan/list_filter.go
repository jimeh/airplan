package airplan

import (
	"errors"
	"fmt"
	"path"
	"time"
)

// ListFilter selects listed uploads by time, kind, slug, and count
// (SPEC.md §9). It is selection, not presentation: the same filter applies to
// manifest history and to a remote bucket listing, and to table and JSON
// output alike. The zero value selects everything.
type ListFilter struct {
	// NewerThan keeps uploads recorded at or after this time when non-zero.
	NewerThan time.Time

	// OlderThan keeps uploads recorded strictly before this time when
	// non-zero, matching the purge age boundary.
	OlderThan time.Time

	// Kind keeps only this upload kind when non-empty. Manifest records that
	// omit kind count as documents, mirroring the reader inference for legacy
	// history; a remote dual-marker conflict declares no kind and matches
	// neither value.
	Kind UploadKind

	// Slug is a glob matched against document slugs. Collections never match,
	// and neither does an upload with no known slug.
	Slug string

	// Limit, when non-nil, keeps that many most recent uploads and drops the
	// rest; zero keeps none. Callers pass uploads in listing order (ascending
	// by time), which every listing path produces, and the kept uploads stay
	// in that order.
	Limit *int
}

// Validate reports an unusable filter before any listing work happens.
func (f ListFilter) Validate() error {
	switch f.Kind {
	case "", UploadKindDocument, UploadKindCollection:
	default:
		return fmt.Errorf(
			"airplan: invalid kind %q (want document or collection)", f.Kind,
		)
	}
	if f.Slug != "" {
		if _, err := path.Match(f.Slug, ""); err != nil {
			return fmt.Errorf("airplan: invalid slug pattern: %w", err)
		}
	}
	if f.Limit != nil && *f.Limit < 0 {
		return errors.New("airplan: limit must not be negative")
	}
	return nil
}

// FilterManifestRecords returns the records the filter selects, preserving
// their input order.
func (f ListFilter) FilterManifestRecords(
	records []ManifestRecord,
) []ManifestRecord {
	kept := make([]ManifestRecord, 0, len(records))
	for _, record := range records {
		if f.matchesRecord(record) {
			kept = append(kept, record)
		}
	}
	return applyListLimit(kept, f.Limit)
}

// FilterRemoteUploads returns the remote uploads the filter selects,
// preserving their input order.
func (f ListFilter) FilterRemoteUploads(
	uploads []RemoteUpload,
) []RemoteUpload {
	kept := make([]RemoteUpload, 0, len(uploads))
	for _, upload := range uploads {
		if f.matchesUpload(upload) {
			kept = append(kept, upload)
		}
	}
	return applyListLimit(kept, f.Limit)
}

func (f ListFilter) matchesRecord(record ManifestRecord) bool {
	if !f.matchesTime(record.Time) {
		return false
	}
	kind := UploadKindDocument
	if record.Kind == string(UploadKindCollection) {
		kind = UploadKindCollection
	}
	if f.Kind != "" && kind != f.Kind {
		return false
	}
	if f.Slug == "" {
		return true
	}
	// ManifestRecordSlug is authoritative about which records have a slug at
	// all: collections have none, so they never match a pattern.
	return matchesSlugPattern(f.Slug, ManifestRecordSlug(record))
}

func (f ListFilter) matchesUpload(upload RemoteUpload) bool {
	if !f.matchesTime(upload.LastModified) {
		return false
	}
	if f.Kind != "" && upload.Kind != f.Kind {
		return false
	}
	if f.Slug == "" {
		return true
	}
	if upload.Kind != UploadKindDocument {
		return false
	}
	return matchesSlugPattern(f.Slug, upload.Slug)
}

// matchesTime keeps NewerThan inclusive and OlderThan exclusive, so the two
// bounds partition a timeline without overlap or gap.
func (f ListFilter) matchesTime(when time.Time) bool {
	if !f.NewerThan.IsZero() && when.Before(f.NewerThan) {
		return false
	}
	if !f.OlderThan.IsZero() && !when.Before(f.OlderThan) {
		return false
	}
	return true
}

func matchesSlugPattern(pattern, slug string) bool {
	if slug == "" {
		return false
	}
	matched, err := path.Match(pattern, slug)
	return err == nil && matched
}

// applyListLimit keeps the trailing limit entries, which are the most recent
// ones because listings are ordered ascending by time (SPEC.md §9).
func applyListLimit[T any](items []T, limit *int) []T {
	if limit == nil {
		return items
	}
	if *limit <= 0 {
		return items[:0]
	}
	if len(items) <= *limit {
		return items
	}
	return items[len(items)-*limit:]
}
