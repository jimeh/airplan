package airplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ProtectedFilename is the purge-protection sentinel basename (SPEC.md §5).
// Presence of the sentinel object at
// [key_prefix/]<dir>/.airplan-protected.json is the entire protection
// contract; the body is advisory context only.
const ProtectedFilename = ".airplan-protected.json"

// ProtectionSchema identifies airplan purge-protection sentinels (SPEC.md §5).
const ProtectionSchema = "airplan-protection"

// ProtectionVersion is the sentinel version written by airplan.
const ProtectionVersion = 1

// MaxProtectReasonRunes bounds the advisory protection reason (SPEC.md §9).
const MaxProtectReasonRunes = 256

// ProtectionResult describes a completed protect or unprotect (SPEC.md §9).
type ProtectionResult struct {
	// ID is the upload's random directory component.
	ID string `json:"id"`

	MarkerKey string `json:"marker_key"`
	// SentinelKey is the full protection sentinel object key.
	SentinelKey string `json:"sentinel_key"`
	// PageKey is the marker-declared page key and manifest record key.
	PageKey string     `json:"page_key"`
	Kind    UploadKind `json:"kind"`

	// Protected is the resulting remote protection state.
	Protected bool `json:"protected"`
	// ProtectedAt and Reason echo the written sentinel body. Both are zero
	// after unprotect.
	ProtectedAt time.Time `json:"protected_at,omitzero"`
	Reason      string    `json:"reason,omitempty"`

	// Warnings collects non-fatal manifest outcomes.
	Warnings []string `json:"warnings,omitempty"`
}

// UploadProtectedError reports that purge protection blocked deletion of an
// upload (SPEC.md §9). Purge treats it as a skip, never a failure; targeted
// delete surfaces it with the --force escape hatch.
type UploadProtectedError struct {
	// Target is the delete target as supplied by the caller.
	Target string
	// Reason is the advisory sentinel reason when its body could be read.
	Reason string
}

func (e *UploadProtectedError) Error() string {
	reason := ""
	if e.Reason != "" {
		reason = fmt.Sprintf(" (reason: %s)", e.Reason)
	}
	return fmt.Sprintf(
		"airplan: upload is protected%s; run %q or retry with --force",
		reason, "airplan unprotect "+e.Target,
	)
}

// protectionSentinel is the advisory sentinel wire body (SPEC.md §5). Every
// field is informational: empty, malformed, oversized, or wrong-content-type
// bodies all still mean protected.
type protectionSentinel struct {
	Schema    string `json:"schema"`
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
	Reason    string `json:"reason,omitempty"`
}

func encodeProtectionSentinel(
	createdAt time.Time, reason string,
) ([]byte, error) {
	body, err := json.Marshal(protectionSentinel{
		Schema:    ProtectionSchema,
		Version:   ProtectionVersion,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
		Reason:    reason,
	})
	if err != nil {
		return nil, fmt.Errorf("airplan: encode protection sentinel: %w", err)
	}
	return body, nil
}

// decodeProtectionSentinel extracts advisory metadata from a sentinel body.
// It never fails: sentinel presence, not content, grants protection, so any
// malformed body decodes to zero values. Out-of-bound reasons are dropped so
// downstream manifest records stay valid.
func decodeProtectionSentinel(data []byte) (time.Time, string) {
	var sentinel protectionSentinel
	if err := json.Unmarshal(data, &sentinel); err != nil {
		return time.Time{}, ""
	}
	createdAt, err := time.Parse(time.RFC3339, sentinel.CreatedAt)
	if err != nil {
		createdAt = time.Time{}
	}
	if validateProtectReason(sentinel.Reason) != nil {
		sentinel.Reason = ""
	}
	return createdAt.UTC(), sentinel.Reason
}

func validateProtectReason(reason string) error {
	if !utf8.ValidString(reason) {
		return errors.New("protect reason is not valid UTF-8")
	}
	if utf8.RuneCountInString(reason) > MaxProtectReasonRunes {
		return fmt.Errorf("protect reason exceeds %d characters",
			MaxProtectReasonRunes)
	}
	return nil
}

// ProtectUpload marks one marker-managed upload as purge-protected by writing
// its protection sentinel object (SPEC.md §9). It is idempotent: protecting an
// already protected upload rewrites the sentinel. The optional reason is
// bounded to MaxProtectReasonRunes.
func (c *Client) ProtectUpload(
	ctx context.Context, urlOrKey, reason string,
) (*ProtectionResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if err := validateProtectReason(reason); err != nil {
		return nil, fmt.Errorf("airplan: %s", err)
	}
	if c.remote != nil {
		return c.remote.ProtectUpload(ctx, urlOrKey, reason)
	}
	target, err := c.resolveProtectionTarget(ctx, urlOrKey, "protect")
	if err != nil {
		return nil, err
	}

	createdAt := time.Now().UTC().Truncate(time.Second)
	body, err := encodeProtectionSentinel(createdAt, reason)
	if err != nil {
		return nil, err
	}
	if err := c.st.put(ctx, object{
		Key:         target.sentinelKey,
		Body:        body,
		ContentType: markerContentType,
	}); err != nil {
		return nil, err
	}

	res := target.result()
	res.Protected = true
	res.ProtectedAt = createdAt
	res.Reason = reason
	c.recordProtection(ctx, "protect", res)
	return res, nil
}

// UnprotectUpload removes purge protection from one marker-managed upload by
// deleting its protection sentinel object (SPEC.md §9). Unprotecting an
// unprotected upload succeeds.
func (c *Client) UnprotectUpload(
	ctx context.Context, urlOrKey string,
) (*ProtectionResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.UnprotectUpload(ctx, urlOrKey)
	}
	target, err := c.resolveProtectionTarget(ctx, urlOrKey, "unprotect")
	if err != nil {
		return nil, err
	}
	if err := c.st.deleteObject(ctx, target.sentinelKey); err != nil {
		return nil, err
	}

	res := target.result()
	c.recordProtection(ctx, "unprotect", res)
	return res, nil
}

// protectionTarget is one validated marker-managed protection target.
type protectionTarget struct {
	dir         string
	dirPrefix   string
	markerKey   string
	sentinelKey string
	marker      *UploadMarker
}

func (t *protectionTarget) result() *ProtectionResult {
	return &ProtectionResult{
		ID:          t.dir,
		MarkerKey:   t.markerKey,
		SentinelKey: t.sentinelKey,
		PageKey:     t.dirPrefix + t.marker.Page,
		Kind:        t.marker.Kind,
	}
}

// resolveProtectionTarget validates the ownership marker through the existing
// fail-closed dual probe before either sentinel mutation, so protect and
// unprotect need no LIST permission and cannot touch unmanaged prefixes.
func (c *Client) resolveProtectionTarget(
	ctx context.Context, urlOrKey, op string,
) (*protectionTarget, error) {
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}
	key, err := KeyFromURLOrKey(c.cfg, urlOrKey)
	if err != nil {
		return nil, err
	}
	dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
	if err != nil {
		return nil, err
	}
	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]
	resolved, err := c.resolveMarker(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	marker, err := DecodeUploadMarkerForName(resolved.Body, dir, resolved.Basename)
	if err != nil {
		return nil, err
	}
	if err := validateManagedTarget(op, key, dirPrefix, marker); err != nil {
		return nil, err
	}
	return &protectionTarget{
		dir: dir, dirPrefix: dirPrefix, markerKey: resolved.Key,
		sentinelKey: dirPrefix + ProtectedFilename, marker: marker,
	}, nil
}

// recordProtection appends a protect or unprotect manifest record,
// best-effort: the sentinel mutation has already completed, so a manifest
// failure degrades to a warning and sync can reconcile later (SPEC.md §9).
func (c *Client) recordProtection(
	ctx context.Context, recordType string, res *ProtectionResult,
) {
	if c.cfg.DisableManifest {
		return
	}
	path := c.cfg.ManifestPath
	if path == "" {
		var err error
		path, err = DefaultManifestPath()
		if err != nil {
			res.Warnings = append(res.Warnings,
				"protection not recorded: "+err.Error())
			return
		}
	}
	when := res.ProtectedAt
	if when.IsZero() {
		when = time.Now().UTC().Truncate(time.Second)
	}
	rec := ManifestRecord{
		Type:          recordType,
		Time:          when,
		Key:           res.PageKey,
		MarkerKey:     res.MarkerKey,
		Bucket:        c.cfg.Bucket,
		Profile:       c.cfg.Profile,
		ProtectReason: res.Reason,
	}
	if err := appendManifestRecord(ctx, path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"protection not recorded: "+err.Error())
	}
}
