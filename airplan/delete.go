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
	Keys []string `json:"keys"`

	// PageKey is the marker-declared page key and manifest tombstone key.
	PageKey   string     `json:"page_key"`
	MarkerKey string     `json:"marker_key"`
	Kind      UploadKind `json:"kind"`

	// Warnings collects non-fatal manifest outcomes.
	Warnings []string `json:"warnings,omitempty"`
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
	if c.remote != nil {
		return c.remote.DeleteUpload(ctx, urlOrKey)
	}
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
		if errors.Is(err, errOwnershipMarkerMissing) {
			return c.reconcileMissingMarker(ctx, dirPrefix, key)
		}
		return nil, err
	}
	marker, err := DecodeUploadMarkerForName(resolved.Body, dir, resolved.Basename)
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
	for _, object := range objects {
		if object.Key != resolved.Key &&
			(object.Key == dirPrefix+MarkerFilename ||
				object.Key == dirPrefix+CollectionMarkerFilename) {
			return nil, markerInvalid(MarkerErrorConflictingMarkers,
				errors.New("conflicting ownership markers"))
		}
	}
	payloadKeys := make([]string, 0, len(objects))
	for _, object := range objects {
		if object.Key != resolved.Key {
			payloadKeys = append(payloadKeys, object.Key)
		}
	}
	if err := c.st.deleteKeys(ctx, payloadKeys); err != nil {
		return nil, err
	}
	if err := c.st.deleteMarker(ctx, resolved.Key); err != nil {
		return nil, err
	}

	res := &DeleteResult{
		Keys:      append(payloadKeys, resolved.Key),
		PageKey:   dirPrefix + marker.Page,
		MarkerKey: resolved.Key,
		Kind:      marker.Kind,
	}
	c.recordDelete(ctx, res)
	return res, nil
}

func validateDeleteTarget(
	key, dirPrefix string, marker *UploadMarker,
) error {
	dirKey := strings.TrimSuffix(dirPrefix, "/")
	markerName, _ := MarkerFilenameForKind(marker.Kind)
	allowed := key == dirKey || key == dirPrefix+markerName ||
		key == dirPrefix+marker.Page
	if marker.Source != "" {
		allowed = allowed || key == dirPrefix+marker.Source
	}
	for _, object := range marker.Objects {
		allowed = allowed || key == dirPrefix+object.Name
	}
	if !allowed {
		return invalidTargetf(
			"airplan: delete target %q is not the directory, marker, or declared payload",
			key,
		)
	}
	return nil
}

func (c *Client) reconcileMissingMarker(
	ctx context.Context, dirPrefix, target string,
) (*DeleteResult, error) {
	record, err := c.ensureGoneRecord(dirPrefix, target)
	if err != nil {
		return nil, fmt.Errorf(
			"airplan: ownership marker is missing; cannot delete or reconcile %q: %w",
			target, err,
		)
	}
	res := &DeleteResult{
		PageKey: record.Key, MarkerKey: manifestMarkerKey(record),
		Kind: UploadKind(record.Kind),
	}
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

	markerKey := res.MarkerKey
	if markerKey == "" {
		markerKey = markerKeyForPage(res.PageKey)
	}
	rec := ManifestRecord{
		Type:      "delete",
		Time:      time.Now().UTC().Truncate(time.Second),
		Key:       res.PageKey,
		MarkerKey: markerKey,
		Bucket:    c.cfg.Bucket,
		Profile:   c.cfg.Profile,
		Reason:    "deleted",
		Kind:      string(res.Kind),
	}
	if err := appendManifestRecord(ctx, path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"tombstone not recorded: "+err.Error())
	}
}

func markerKeyForPage(pageKey string) string {
	dirPrefix, err := uploadDirPrefix(pageKey)
	if err != nil {
		return ""
	}
	return dirPrefix + MarkerFilename
}

// ensureGoneRecord returns the exact active local upload matching dirPrefix.
// The record must be complete, current, and belong to the active connection;
// local history never grants authority to mutate remote objects.
func (c *Client) ensureGoneRecord(
	dirPrefix, target string,
) (ManifestRecord, error) {
	if c.cfg.DisableManifest {
		return ManifestRecord{}, errors.New("local manifest is disabled")
	}
	records, warnings, err := ReadManifest(c.cfg.ManifestPath)
	if err != nil {
		return ManifestRecord{}, fmt.Errorf("read local manifest: %w", err)
	}
	for _, record := range ActiveUploads(records) {
		recordDir, err := uploadDirPrefix(record.Key)
		if err != nil || recordDir != dirPrefix {
			continue
		}
		if record.Profile != c.cfg.Profile {
			return ManifestRecord{}, &ManifestProfileMismatchError{
				Recorded: record.Profile,
				Active:   c.cfg.Profile,
			}
		}
		if record.Bucket != c.cfg.Bucket {
			return ManifestRecord{}, fmt.Errorf(
				"manifest record belongs to bucket %q; active connection uses bucket %q",
				record.Bucket, c.cfg.Bucket,
			)
		}
		dirKey := strings.TrimSuffix(dirPrefix, "/")
		allowed := target == dirKey || target == manifestMarkerKey(record) ||
			target == record.Key || target == record.SourceKey
		if record.Kind == string(UploadKindCollection) {
			rel := strings.TrimPrefix(target, dirPrefix)
			allowed = allowed || rel != target && rel != "" &&
				!strings.Contains(rel, "/")
		}
		if !allowed {
			return ManifestRecord{}, fmt.Errorf(
				"target %q is not the directory, marker, or recorded payload",
				target,
			)
		}
		return record, nil
	}
	if len(warnings) > 0 {
		return ManifestRecord{}, fmt.Errorf("local manifest is incomplete: %s", warnings[0])
	}
	return ManifestRecord{}, errors.New("no matching active marker-versioned manifest record")
}

func profileLabel(profile string) string {
	if profile == "" {
		return "<root>"
	}
	return profile
}
