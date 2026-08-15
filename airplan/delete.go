package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var standaloneDeleteReservationBody = []byte(
	`{"schema":"airplan-standalone-delete-reservation","version":1}`,
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

type missingMarkerRecordError struct {
	message string
}

func (e *missingMarkerRecordError) Error() string {
	return e.message
}

func (e *missingMarkerRecordError) Unwrap() error {
	return errOwnershipMarkerMissing
}

// DeleteOptions controls targeted upload deletion (SPEC.md §9).
type DeleteOptions struct {
	// Force deletes the upload even when it is purge-protected.
	Force bool
}

// DeleteUpload validates the upload's ownership marker, removes every
// non-marker object, removes the marker separately, and appends a manifest
// tombstone (SPEC.md §9). It refuses purge-protected uploads; use
// DeleteUploadWithOptions to force deletion.
func (c *Client) DeleteUpload(
	ctx context.Context, urlOrKey string,
) (*DeleteResult, error) {
	return c.DeleteUploadWithOptions(ctx, urlOrKey, DeleteOptions{})
}

// DeleteUploadWithOptions is DeleteUpload with explicit options. Without
// Force, a purge-protected upload fails with *UploadProtectedError. With
// Force, deletion removes payloads first, then the protection sentinel, and
// the ownership marker last, so protection persists until every payload is
// gone (SPEC.md §9).
func (c *Client) DeleteUploadWithOptions(
	ctx context.Context, urlOrKey string, opts DeleteOptions,
) (*DeleteResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.DeleteUpload(ctx, urlOrKey, opts)
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
	if err := validateManagedTarget("delete", key, dirPrefix, marker); err != nil {
		return nil, err
	}

	objects, err := c.st.listKeys(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	sentinelKey := dirPrefix + ProtectedFilename
	protected := false
	for _, object := range objects {
		if object.Key == sentinelKey {
			protected = true
		}
		if object.Key != resolved.Key &&
			(object.Key == dirPrefix+MarkerFilename ||
				object.Key == dirPrefix+CollectionMarkerFilename) {
			return nil, markerInvalid(MarkerErrorConflictingMarkers,
				errors.New("conflicting ownership markers"))
		}
	}
	// This listing-based guard is the delete-time enforcement layer: it
	// catches protection set on another machine and not yet synced locally
	// (SPEC.md §9). A listing failure already failed the delete above, so
	// undeterminable protection can never fall through to deletion.
	if protected && !opts.Force {
		reason := ""
		if body, err := c.st.getBytes(ctx, sentinelKey, MaxMarkerSize); err == nil {
			_, reason = decodeProtectionSentinel(body)
		}
		return nil, &UploadProtectedError{Target: urlOrKey, Reason: reason}
	}
	standaloneReserved := false
	if marker.Kind == UploadKindDocument && marker.Format == "md" &&
		marker.Source != "" && marker.Revision == nil {
		if err := c.reserveStandaloneDelete(ctx, dirPrefix); err != nil {
			return nil, err
		}
		standaloneReserved = true
	}
	var survivingVersions *VersionsMetadata
	interruptedLinkedDelete := false
	unannouncedCandidate := false
	if marker.Revision != nil {
		survivingVersions, err = c.tombstoneLinkedRevision(ctx, dirPrefix, marker)
		if err != nil {
			declaredPayloadPresent := false
			for _, observed := range objects {
				for _, declared := range marker.Objects {
					if observed.Key == dirPrefix+declared.Name {
						declaredPayloadPresent = true
						break
					}
				}
			}
			switch {
			case errors.Is(err, errObjectNotFound) && !declaredPayloadPresent:
				// Tombstones precede payload deletion. No local metadata and no
				// declared payload is the recoverable marker-last interruption.
				interruptedLinkedDelete = true
			case errors.Is(err, errObjectNotFound):
				unannouncedCandidate, err = c.revisionCandidateIsUnannounced(
					ctx, dirPrefix, marker,
				)
				if err != nil {
					return nil, err
				}
				if unannouncedCandidate {
					survivingVersions = nil
					// The candidate marker was deliberately created first, but its
					// predecessor never announced it. It is a managed rollback orphan.
				} else {
					return nil, errObjectNotFound
				}
			default:
				return nil, err
			}
		}
	}
	payloadKeys := make([]string, 0, len(objects))
	reservationKey := dirPrefix + VersionsFilename
	for _, object := range objects {
		if object.Key != resolved.Key && object.Key != sentinelKey &&
			(!standaloneReserved || object.Key != reservationKey) {
			payloadKeys = append(payloadKeys, object.Key)
		}
	}
	if err := c.st.deleteKeys(ctx, payloadKeys); err != nil {
		return nil, err
	}
	deletedKeys := payloadKeys
	// The protection sentinel outlives every payload. If marker deletion later
	// fails, the marker remains protected; a standalone delete reservation is a
	// separate durable tombstone and is never removed (SPEC.md §9).
	if protected {
		if err := c.st.deleteObject(ctx, sentinelKey); err != nil {
			return nil, err
		}
		deletedKeys = append(deletedKeys, sentinelKey)
	}
	if err := c.st.deleteMarker(ctx, resolved.Key); err != nil {
		return nil, err
	}

	res := &DeleteResult{
		Keys:      append(deletedKeys, resolved.Key),
		PageKey:   dirPrefix + marker.Page,
		MarkerKey: resolved.Key,
		Kind:      marker.Kind,
	}
	if interruptedLinkedDelete {
		res.Warnings = append(res.Warnings,
			"completed an interrupted linked-revision deletion")
	}
	if unannouncedCandidate {
		res.Warnings = append(res.Warnings,
			"removed an unannounced revision candidate left by rollback")
	}
	c.recordDelete(ctx, res)
	if survivingVersions != nil {
		c.recordSurvivingRevisionLinks(ctx, res, *survivingVersions)
	}
	return res, nil
}

func (c *Client) revisionCandidateIsUnannounced(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (bool, error) {
	if marker == nil || marker.Revision == nil || marker.Revision.Number <= 1 ||
		marker.Revision.PreviousURL == "" {
		return false, nil
	}
	previousKey, err := KeyFromURLOrKey(c.cfg, marker.Revision.PreviousURL)
	if err != nil {
		return false, err
	}
	previousPrefix, err := uploadDirPrefixForKeyPrefix(previousKey, c.cfg.KeyPrefix)
	if err != nil {
		return false, err
	}
	reservation, reservationErr := c.st.getBytes(
		ctx, previousPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if marker.Revision.Number == 2 && reservationErr == nil &&
		bytes.Equal(reservation, standaloneDeleteReservationBody) {
		return true, nil
	}
	if reservationErr != nil && !errors.Is(reservationErr, errObjectNotFound) {
		return false, fmt.Errorf("airplan: inspect revision candidate predecessor reservation: %w",
			reservationErr)
	}
	previous, err := c.loadRevisionDocument(ctx, marker.Revision.PreviousURL)
	if err != nil {
		return false, fmt.Errorf("airplan: inspect revision candidate predecessor: %w", err)
	}
	if previous.versions == nil {
		return marker.Revision.Number == 2 && previous.marker.Revision == nil, nil
	}
	if previous.versions.ChainID != marker.Revision.ChainID ||
		previous.versions.CurrentRevision != marker.Revision.Number-1 {
		return false, nil
	}
	pageURL, _, err := PublicURL(c.cfg, targetDirPrefix+marker.Page)
	if err != nil {
		return false, err
	}
	for _, entry := range previous.versions.Revisions {
		if !entry.Deleted && entry.URL == pageURL {
			return false, nil
		}
	}
	return true, nil
}

func (c *Client) reserveStandaloneDelete(
	ctx context.Context, dirPrefix string,
) error {
	key := dirPrefix + VersionsFilename
	err := c.st.putIfAbsent(ctx, object{
		Key: key, Body: standaloneDeleteReservationBody,
		ContentType: markerContentType,
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrConflict) {
		return fmt.Errorf("airplan: reserve standalone deletion: %w", err)
	}
	existing, readErr := c.st.getBytes(ctx, key, MaxVersionsMetadataSize)
	if readErr == nil && bytes.Equal(existing, standaloneDeleteReservationBody) {
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("airplan: inspect standalone deletion reservation: %w", readErr)
	}
	return fmt.Errorf("airplan: standalone document changed while deletion was starting: %w",
		ErrConflict)
}

func (c *Client) tombstoneLinkedRevision(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (*VersionsMetadata, error) {
	targetMetadataKey := targetDirPrefix + VersionsFilename
	body, targetETag, _, err := c.st.getBytesWithETag(ctx,
		targetMetadataKey, MaxVersionsMetadataSize)
	if err != nil {
		return nil, fmt.Errorf("airplan: read revision metadata before delete: %w", err)
	}
	metadata, err := DecodeVersionsMetadata(body, c.cfg,
		targetDirPrefix+marker.Page)
	targetReserved := false
	if err != nil {
		metadata, err = decodeVersionsMetadata(body, c.cfg, "", true)
		if err != nil {
			return nil, err
		}
		targetReserved = true
	}
	if metadata.ChainID != marker.Revision.ChainID ||
		metadata.CurrentRevision != marker.Revision.Number {
		return nil, errors.New("airplan: marker and revision metadata conflict")
	}

	serializationKey := targetMetadataKey
	serializationETag := targetETag
	serializationBody := body
	seen := map[string]bool{targetMetadataKey: true}
	for !targetReserved && metadata.CurrentRevision != metadata.LatestRevision {
		latestEntry := liveVersionsRevision(metadata, metadata.LatestRevision)
		if latestEntry == nil {
			return nil, errors.New("airplan: revision metadata has no live latest entry")
		}
		latestDoc, loadErr := c.loadRevisionDocument(ctx, latestEntry.URL)
		if loadErr != nil {
			return nil, fmt.Errorf("airplan: resolve latest revision before delete: %w", loadErr)
		}
		serializationKey = latestDoc.dirPrefix + VersionsFilename
		if seen[serializationKey] {
			return nil, errors.New("airplan: revision metadata latest link forms a cycle")
		}
		seen[serializationKey] = true
		if latestDoc.versions == nil ||
			latestDoc.versions.ChainID != marker.Revision.ChainID {
			return nil, errors.New("airplan: latest revision metadata conflicts with delete target")
		}
		metadata = latestDoc.versions
		serializationETag = latestDoc.versionsETag
		serializationBody = latestDoc.versionsBody
	}

	deletedAt := time.Now().UTC().Truncate(time.Second)
	live := 0
	latest := 0
	foundTarget := false
	alreadyTombstoned := false
	for index := range metadata.Revisions {
		revision := &metadata.Revisions[index]
		if revision.Number == marker.Revision.Number {
			foundTarget = true
			alreadyTombstoned = revision.Deleted
			if targetReserved {
				if !revision.Deleted {
					return nil, errors.New("airplan: invalid linked-delete reservation")
				}
			} else if !revision.Deleted {
				revision.URL = ""
				revision.DiffURL = ""
				revision.CreatedAt = time.Time{}
				revision.Deleted = true
				revision.DeletedAt = deletedAt
			}
			continue
		}
		if !revision.Deleted {
			live++
			latest = revision.Number
		}
	}
	if !foundTarget {
		return nil, errors.New("airplan: revision metadata does not contain the delete target")
	}
	if live == 0 {
		return nil, errors.New("airplan: deleting the final live revision is not supported")
	}
	metadata.LatestRevision = latest
	bodies, err := c.encodeMemberMetadata(*metadata)
	if err != nil {
		return nil, err
	}
	if !alreadyTombstoned {
		committedBody := bodies[metadata.CurrentRevision]
		if metadata.CurrentRevision == marker.Revision.Number {
			committedBody, err = encodeVersionsMetadata(*metadata, c.cfg, "", true)
			if err != nil {
				return nil, err
			}
		}
		if !bytes.Equal(serializationBody, committedBody) {
			// Append and delete contend on the current latest member's metadata.
			// Deleting latest publishes a short-lived invalid-current reservation;
			// deleting history publishes its valid tombstone body there.
			if putErr := c.st.putConditional(ctx, object{
				Key: serializationKey, Body: committedBody,
				ContentType: markerContentType,
			}, serializationETag); putErr != nil {
				return nil, fmt.Errorf("airplan: reserve linked revision deletion: %w", putErr)
			}
		}
	}
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		key, err := KeyFromURLOrKey(c.cfg, revision.URL)
		if err != nil {
			return nil, err
		}
		dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
		if err != nil {
			return nil, err
		}
		metadataKey := dirPrefix + VersionsFilename
		current, etag, _, err := c.st.getBytesWithETag(ctx, metadataKey,
			MaxVersionsMetadataSize)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(current, bodies[revision.Number]) {
			continue
		}
		observed, decodeErr := DecodeVersionsMetadata(current, c.cfg, key)
		if decodeErr != nil || !versionsMetadataMonotonicallyPrecedes(
			observed, metadata,
		) {
			return nil, errors.New("airplan: revision chain changed during tombstone propagation")
		}
		if err := c.st.putConditional(ctx, object{
			Key:  metadataKey,
			Body: bodies[revision.Number], ContentType: markerContentType,
		}, etag); err != nil {
			return nil, fmt.Errorf("airplan: revision tombstone propagation failed: %w", err)
		}
	}
	return metadata, nil
}

func (c *Client) recordSurvivingRevisionLinks(
	ctx context.Context, result *DeleteResult, metadata VersionsMetadata,
) {
	if c.cfg.DisableManifest {
		return
	}
	manifestPath := c.cfg.ManifestPath
	if manifestPath == "" {
		var err error
		manifestPath, err = DefaultManifestPath()
		if err != nil {
			return
		}
	}
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		doc, err := c.loadRevisionDocument(ctx, revision.URL)
		if err != nil {
			result.Warnings = append(result.Warnings, "manifest chain links not fully recorded")
			return
		}
		projection := c.updateResultFromDocument(doc).Result
		projection.RevisionChainID = metadata.ChainID
		projection.Revision = revision.Number
		projection.LatestRevision = metadata.LatestRevision
		declared := markerDeclaredTotals(*doc.marker, doc.markerBody)
		rec := completeManifestProjection(c.cfg, "link",
			time.Now().UTC().Truncate(time.Second), &projection, declared,
			doc.marker.Producer.Version, doc.marker.Render.Generation)
		if err := appendManifestRecord(ctx, manifestPath, rec); err != nil {
			result.Warnings = append(result.Warnings, "manifest link not recorded: "+err.Error())
			return
		}
	}
}

// validateManagedTarget checks a destructive or protective operation's target
// against the validated marker: the directory, the marker itself, or any
// declared payload object. op names the operation in the error text.
func validateManagedTarget(
	op, key, dirPrefix string, marker *UploadMarker,
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
			"airplan: %s target %q is not the directory, marker, or declared payload",
			op, key,
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
			return ManifestRecord{}, invalidTargetf(
				"target %q is not the directory, marker, or recorded payload",
				target,
			)
		}
		return record, nil
	}
	if len(warnings) > 0 {
		return ManifestRecord{}, fmt.Errorf("local manifest is incomplete: %s", warnings[0])
	}
	return ManifestRecord{}, &missingMarkerRecordError{
		message: "no matching active marker-versioned manifest record",
	}
}

func profileLabel(profile string) string {
	if profile == "" {
		return "<root>"
	}
	return profile
}
