package airplan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DeleteResult describes a completed delete (SPEC.md §9).
type DeleteResult struct {
	// Keys are the object keys removed, in operation order. The marker is last.
	Keys []string

	// PageKey is the marker-declared page key and manifest tombstone key.
	PageKey string

	// Warnings collects non-fatal manifest outcomes.
	Warnings []string
}

// ManifestProfileMismatchError reports that local history associates a delete
// target with a different profile than the active connection.
type ManifestProfileMismatchError struct {
	Recorded string
	Active   string
}

func (e *ManifestProfileMismatchError) Error() string {
	return fmt.Sprintf(
		"manifest record belongs to profile %q; active connection uses profile %q",
		profileLabel(e.Recorded), profileLabel(e.Active),
	)
}

// DeleteUpload validates the upload's ownership marker, removes every
// non-marker object, removes the marker separately, and appends a manifest
// tombstone (SPEC.md §9).
func (c *Client) DeleteUpload(
	ctx context.Context, urlOrKey string,
) (*DeleteResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	key, err := KeyFromURLOrKey(c.cfg, urlOrKey)
	if err != nil {
		return nil, err
	}
	dirPrefix, err := uploadDirPrefix(key)
	if err != nil {
		return nil, err
	}
	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]
	markerKey := dirPrefix + MarkerFilename

	markerBody, err := c.st.getBytes(ctx, markerKey, MaxMarkerSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return c.reconcileMissingMarker(ctx, dirPrefix, key)
		}
		return nil, err
	}
	marker, err := DecodeUploadMarker(markerBody, dir)
	if err != nil {
		return nil, err
	}
	if err := validateDeleteTarget(key, dirPrefix, marker); err != nil {
		return nil, err
	}

	objects, err := c.st.listKeys(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	payloadKeys := make([]string, 0, len(objects))
	for _, object := range objects {
		if object.Key != markerKey {
			payloadKeys = append(payloadKeys, object.Key)
		}
	}
	if err := c.st.deleteKeys(ctx, payloadKeys); err != nil {
		return nil, err
	}
	if err := c.st.deleteMarker(ctx, markerKey); err != nil {
		return nil, err
	}

	res := &DeleteResult{
		Keys:    append(payloadKeys, markerKey),
		PageKey: dirPrefix + marker.Page,
	}
	c.recordDelete(ctx, res)
	return res, nil
}

func validateDeleteTarget(
	key, dirPrefix string, marker *UploadMarker,
) error {
	dirKey := strings.TrimSuffix(dirPrefix, "/")
	allowed := key == dirKey || key == dirPrefix+MarkerFilename ||
		key == dirPrefix+marker.Page
	if marker.Source != "" {
		allowed = allowed || key == dirPrefix+marker.Source
	}
	if !allowed {
		return fmt.Errorf(
			"airplan: delete target %q is not the directory, marker, or declared payload",
			key,
		)
	}
	return nil
}

func (c *Client) reconcileMissingMarker(
	ctx context.Context, dirPrefix, target string,
) (*DeleteResult, error) {
	pageKey, err := c.ensureGonePageKey(dirPrefix, target)
	if err != nil {
		return nil, fmt.Errorf(
			"airplan: ownership marker is missing; cannot delete or reconcile %q: %w",
			target, err,
		)
	}
	res := &DeleteResult{PageKey: pageKey}
	res.Warnings = append(res.Warnings, fmt.Sprintf(
		"ownership marker is already absent under %q; recording the completed deletion",
		dirPrefix,
	))
	c.recordDelete(ctx, res)
	return res, nil
}

// recordDelete appends a delete tombstone, best-effort: marker deletion has
// already completed, so a manifest failure degrades to a warning and a retry
// can use the narrow reconciliation path.
func (c *Client) recordDelete(ctx context.Context, res *DeleteResult) {
	if c.cfg.DisableManifest {
		return
	}

	path := c.cfg.ManifestPath
	if path == "" {
		var err error
		path, err = DefaultManifestPath()
		if err != nil {
			res.Warnings = append(res.Warnings,
				"tombstone not recorded: "+err.Error())
			return
		}
	}

	rec := ManifestRecord{
		Type: "delete",
		Time: time.Now().UTC().Truncate(time.Second),
		Key:  res.PageKey,
	}
	if err := appendManifestRecord(ctx, path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"tombstone not recorded: "+err.Error())
	}
}

// ensureGonePageKey returns the exact active local upload matching dirPrefix.
// The record must be complete, current, and belong to the active connection;
// local history never grants authority to mutate remote objects.
func (c *Client) ensureGonePageKey(dirPrefix, target string) (string, error) {
	if c.cfg.DisableManifest {
		return "", errors.New("local manifest is disabled")
	}
	records, warnings, err := ReadManifest(c.cfg.ManifestPath)
	if err != nil {
		return "", fmt.Errorf("read local manifest: %w", err)
	}
	for _, record := range ActiveUploads(records) {
		if record.MarkerVersion != MarkerVersion {
			continue
		}
		recordDir, err := uploadDirPrefix(record.Key)
		if err != nil || recordDir != dirPrefix {
			continue
		}
		if record.Profile != c.cfg.Profile {
			return "", &ManifestProfileMismatchError{
				Recorded: record.Profile,
				Active:   c.cfg.Profile,
			}
		}
		if record.Bucket != c.cfg.Bucket {
			return "", fmt.Errorf(
				"manifest record belongs to bucket %q; active connection uses bucket %q",
				record.Bucket, c.cfg.Bucket,
			)
		}
		dirKey := strings.TrimSuffix(dirPrefix, "/")
		if target != dirKey && target != dirPrefix+MarkerFilename &&
			target != record.Key && target != record.SourceKey {
			return "", fmt.Errorf(
				"target %q is not the directory, marker, or recorded payload",
				target,
			)
		}
		return record.Key, nil
	}
	if len(warnings) > 0 {
		return "", fmt.Errorf("local manifest is incomplete: %s", warnings[0])
	}
	return "", errors.New("no matching active marker-versioned manifest record")
}

func profileLabel(profile string) string {
	if profile == "" {
		return "<root>"
	}
	return profile
}
