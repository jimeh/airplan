package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

var standaloneDeleteReservationBody = []byte(
	`{"schema":"airplan-standalone-delete-reservation","version":1}`,
)

const finalRevisionDeleteReservationSchema = "airplan-final-revision-delete-reservation"

type finalRevisionDeleteReservation struct {
	Schema               string    `json:"schema"`
	Version              int       `json:"version"`
	ChainID              string    `json:"chain_id"`
	Revision             int       `json:"revision"`
	LastAssignedRevision int       `json:"last_assigned_revision"`
	DeletedAt            time.Time `json:"deleted_at"`
}

var errUploadBecameVersioned = errors.New(
	"airplan: upload became versioned while purge deletion was starting",
)

var errTargetRevisionMetadataMissing = errors.New(
	"airplan: target revision metadata is missing",
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

	revisionChainID string
	revision        int
	latestRevision  int
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

	// requireStandalone is used by purge to preserve its version-history guard
	// through the conditional operation that begins deletion.
	requireStandalone bool
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
	if opts.requireStandalone && marker.Revision != nil {
		return nil, errUploadBecameVersioned
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
			if opts.requireStandalone && errors.Is(err, ErrConflict) {
				body, readErr := c.st.getBytes(
					ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize,
				)
				if readErr == nil {
					if _, decodeErr := DecodeVersionsMetadata(
						body, c.cfg, dirPrefix+marker.Page,
					); decodeErr == nil {
						return nil, errUploadBecameVersioned
					}
				}
			}
			return nil, err
		}
		standaloneReserved = true
	}
	var survivingVersions *VersionsMetadata
	interruptedLinkedDelete := false
	unannouncedCandidate := false
	if marker.Revision != nil {
		survivingVersions, err = c.tombstoneLinkedRevision(ctx, dirPrefix, marker)
		if err != nil && errors.Is(err, errTargetRevisionMetadataMissing) {
			state, stateErr := c.classifyRevisionCandidate(ctx, dirPrefix, marker)
			if stateErr != nil {
				return nil, stateErr
			}
			switch state {
			case revisionCandidateUnannounced:
				unannouncedCandidate = true
				survivingVersions = nil
				err = nil
			case revisionCandidateAnnouncedDeleted:
				interruptedLinkedDelete = true
				survivingVersions = nil
				err = nil
			case revisionCandidateAnnouncedLive:
				survivingVersions, err = c.repairAndTombstoneAnnouncedRevision(
					ctx, dirPrefix, marker,
				)
			default:
				return nil, fmt.Errorf(
					"airplan: revision candidate announcement cannot be determined: %w",
					errObjectNotFound,
				)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	payloadKeys := make([]string, 0, len(objects))
	reservationKey := dirPrefix + VersionsFilename
	preserveDeleteReceipt := standaloneReserved || marker.Revision != nil
	for _, object := range objects {
		if object.Key != resolved.Key && object.Key != sentinelKey &&
			(!preserveDeleteReceipt || object.Key != reservationKey) {
			payloadKeys = append(payloadKeys, object.Key)
		}
	}
	if err := c.st.deleteKeys(ctx, payloadKeys); err != nil {
		return nil, err
	}
	deletedKeys := payloadKeys
	// The protection sentinel outlives every payload. If marker deletion later
	// fails, the marker remains protected. Standalone and linked delete receipts
	// are separate durable tombstones and are never removed (SPEC.md §9).
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
	if marker.Revision != nil && !unannouncedCandidate {
		res.revisionChainID = marker.Revision.ChainID
		res.revision = marker.Revision.Number
		if survivingVersions != nil {
			res.latestRevision = survivingVersions.LatestRevision
		}
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

type revisionCandidateState int

const (
	revisionCandidateUnknown revisionCandidateState = iota
	revisionCandidateUnannounced
	revisionCandidateAnnouncedLive
	revisionCandidateAnnouncedDeleted
)

func (c *Client) classifyRevisionCandidate(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (revisionCandidateState, error) {
	if marker == nil || marker.Revision == nil || marker.Revision.Number <= 1 ||
		marker.Revision.PreviousURL == "" {
		return revisionCandidateUnknown, nil
	}
	previousKey, err := KeyFromURLOrKey(c.cfg, marker.Revision.PreviousURL)
	if err != nil {
		return revisionCandidateUnknown, err
	}
	previousPrefix, err := uploadDirPrefixForKeyPrefix(previousKey, c.cfg.KeyPrefix)
	if err != nil {
		return revisionCandidateUnknown, err
	}
	pageURL, _, err := PublicURL(c.cfg, targetDirPrefix+marker.Page)
	if err != nil {
		return revisionCandidateUnknown, err
	}
	reservation, reservationErr := c.st.getBytes(
		ctx, previousPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if reservationErr == nil {
		finalReservation, ok, decodeErr := decodeFinalRevisionDeleteReservation(reservation)
		if decodeErr != nil {
			return revisionCandidateUnknown, decodeErr
		}
		if ok {
			if finalReservation.ChainID == marker.Revision.ChainID &&
				marker.Revision.Number > finalReservation.LastAssignedRevision {
				return revisionCandidateUnannounced, nil
			}
			return revisionCandidateUnknown, nil
		}
		if bytes.Equal(reservation, standaloneDeleteReservationBody) {
			if marker.Revision.Number == 2 {
				return revisionCandidateUnannounced, nil
			}
			return revisionCandidateUnknown, errors.New(
				"airplan: standalone deletion reservation cannot classify a later revision candidate",
			)
		}
		metadata, decodeErr := decodeVersionsMetadata(
			reservation, c.cfg, previousKey, true,
		)
		if decodeErr != nil {
			return revisionCandidateUnknown, decodeErr
		}
		current := &metadata.Revisions[metadata.CurrentRevision-1]
		if state, classified := revisionCandidateStateFromMetadata(
			metadata, marker, pageURL,
		); classified {
			if current.Deleted && state == revisionCandidateAnnouncedLive {
				state = c.refineRevisionCandidateFromDeleteReceipt(
					ctx, metadata, marker, pageURL, state,
				)
			}
			return state, nil
		}
		if current.Deleted {
			// A deleted predecessor's permanent receipt cannot race a new append.
			// If the complete assigned history does not contain this target, the
			// target was never announced.
			return revisionCandidateUnannounced, nil
		}
	}
	if reservationErr != nil && !errors.Is(reservationErr, errObjectNotFound) {
		return revisionCandidateUnknown, fmt.Errorf("airplan: inspect revision candidate predecessor reservation: %w",
			reservationErr)
	}
	previous, err := c.loadRevisionDocument(ctx, marker.Revision.PreviousURL)
	if err != nil {
		if errors.Is(err, errOwnershipMarkerMissing) {
			return revisionCandidateUnannounced, nil
		}
		return revisionCandidateUnknown, fmt.Errorf("airplan: inspect revision candidate predecessor: %w", err)
	}
	if previous.versions == nil {
		// A live standalone predecessor still has an updater that may win its
		// If-None-Match transition. Without a shared conditional object there is
		// no safe way for generic cleanup to distinguish that writer from an
		// abandoned revision-2 candidate, so fail closed.
		return revisionCandidateUnknown, nil
	}
	if previous.versions.ChainID != marker.Revision.ChainID {
		return revisionCandidateUnknown, nil
	}
	if state, classified := revisionCandidateStateFromMetadata(
		previous.versions, marker, pageURL,
	); classified {
		return state, nil
	}
	if marker.Revision.Number <= previous.versions.LastAssignedRevision {
		// The high-water mark already consumed this integer for another URL (or
		// tombstoned it), which is durable proof that this candidate lost.
		return revisionCandidateUnannounced, nil
	}
	if marker.Revision.Number != previous.versions.LastAssignedRevision+1 ||
		previous.versions.CurrentRevision != previous.versions.LatestRevision {
		return revisionCandidateUnknown, nil
	}
	claimBody, err := revisionCandidateCleanupClaimBody(previous.versionsBody)
	if err != nil {
		return revisionCandidateUnknown, err
	}
	// Rewriting semantically identical metadata with an alternate top-level
	// field order changes content-derived S3 ETags. It serializes cleanup against
	// the candidate writer's pending append: cleanup wins this claim or the
	// append publishes first.
	if err := c.st.putConditional(ctx, object{
		Key: previous.dirPrefix + VersionsFilename, Body: claimBody,
		ContentType: markerContentType,
	}, previous.versionsETag); err != nil {
		return revisionCandidateUnknown, fmt.Errorf("airplan: claim revision candidate cleanup: %w", err)
	}
	return revisionCandidateUnannounced, nil
}

func (c *Client) refineRevisionCandidateFromDeleteReceipt(
	ctx context.Context, receipt *VersionsMetadata, marker *UploadMarker,
	pageURL string, fallback revisionCandidateState,
) revisionCandidateState {
	// A deleted predecessor's permanent receipt may predate a later deletion.
	// Prefer any surviving member's current replica when it is available; if it
	// is unavailable, the stale live classification remains fail-closed because
	// linked deletion must still repair and verify the complete chain.
	for index := len(receipt.Revisions) - 1; index >= 0; index-- {
		entry := receipt.Revisions[index]
		if entry.Deleted || entry.Number == marker.Revision.Number {
			continue
		}
		witness, err := c.loadRevisionDocument(ctx, entry.URL)
		if err != nil || witness.versions == nil {
			continue
		}
		if state, classified := revisionCandidateStateFromMetadata(
			witness.versions, marker, pageURL,
		); classified {
			return state
		}
	}
	return fallback
}

func revisionCandidateStateFromMetadata(
	metadata *VersionsMetadata, marker *UploadMarker, pageURL string,
) (revisionCandidateState, bool) {
	if metadata == nil || marker == nil || marker.Revision == nil ||
		metadata.ChainID != marker.Revision.ChainID {
		return revisionCandidateUnknown, true
	}
	for _, entry := range metadata.Revisions {
		if entry.Number != marker.Revision.Number {
			continue
		}
		if entry.Deleted {
			return revisionCandidateAnnouncedDeleted, true
		}
		if entry.URL == pageURL {
			return revisionCandidateAnnouncedLive, true
		}
		return revisionCandidateUnannounced, true
	}
	return revisionCandidateUnknown, false
}

func (c *Client) revisionCandidateIsUnannounced(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (bool, error) {
	state, err := c.classifyRevisionCandidate(ctx, targetDirPrefix, marker)
	return state == revisionCandidateUnannounced, err
}

func (c *Client) repairAndTombstoneAnnouncedRevision(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (*VersionsMetadata, error) {
	pageURL, _, err := PublicURL(c.cfg, targetDirPrefix+marker.Page)
	if err != nil {
		return nil, err
	}
	doc, err := c.loadRevisionDocument(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("airplan: load announced revision for metadata repair: %w", err)
	}
	if doc.needsMetadata {
		doc, err = c.repairMissingRevisionMetadata(ctx, doc, nil)
		if err != nil {
			return nil, fmt.Errorf("airplan: repair announced revision metadata before delete: %w", err)
		}
	}
	if err := c.repairFirstRevisionPromotionFromMember(ctx, doc); err != nil {
		return nil, fmt.Errorf("airplan: repair first revision promotion before delete: %w", err)
	}
	return c.tombstoneLinkedRevision(ctx, targetDirPrefix, marker)
}

func revisionCandidateCleanupClaimBody(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, errors.New("airplan: revision candidate cleanup metadata is empty")
	}
	const canonicalPrefix = `{"schema":"airplan-versions","version":1,`
	const claimPrefix = `{"version":1,"schema":"airplan-versions",`
	if bytes.HasPrefix(body, []byte(canonicalPrefix)) {
		return append([]byte(claimPrefix), body[len(canonicalPrefix):]...), nil
	}
	if bytes.HasPrefix(body, []byte(claimPrefix)) {
		return append([]byte(canonicalPrefix), body[len(claimPrefix):]...), nil
	}
	claim := append([]byte(nil), body...)
	switch claim[len(claim)-1] {
	case '\n':
		claim[len(claim)-1] = ' '
	case ' ', '\t', '\r':
		claim[len(claim)-1] = '\n'
	default:
		if len(claim) >= MaxVersionsMetadataSize {
			return nil, errors.New("airplan: revision candidate cleanup metadata has no claim capacity")
		}
		claim = append(claim, '\n')
	}
	return claim, nil
}

func encodeFinalRevisionDeleteReservation(
	chainID string, revision, lastAssigned int, deletedAt time.Time,
) ([]byte, error) {
	reservation := finalRevisionDeleteReservation{
		Schema: finalRevisionDeleteReservationSchema, Version: 1,
		ChainID: chainID, Revision: revision,
		LastAssignedRevision: lastAssigned, DeletedAt: deletedAt,
	}
	if !isRandomDir(chainID) || revision <= 0 || lastAssigned < revision ||
		deletedAt.IsZero() || !isUTCSecond(deletedAt) {
		return nil, errors.New("airplan: invalid final revision delete reservation")
	}
	body, err := json.Marshal(reservation)
	if err != nil {
		return nil, fmt.Errorf("airplan: encode final revision delete reservation: %w", err)
	}
	return body, nil
}

func decodeFinalRevisionDeleteReservation(
	body []byte,
) (*finalRevisionDeleteReservation, bool, error) {
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil ||
		envelope.Schema != finalRevisionDeleteReservationSchema {
		return nil, false, nil
	}
	if err := validateJSONNames(body); err != nil {
		return nil, true, fmt.Errorf(
			"airplan: final revision delete reservation is malformed: %w", err,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var reservation finalRevisionDeleteReservation
	if err := decoder.Decode(&reservation); err != nil {
		return nil, true, fmt.Errorf(
			"airplan: final revision delete reservation is malformed: %w", err,
		)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, true, errors.New(
			"airplan: final revision delete reservation has trailing data",
		)
	}
	if reservation.Version != 1 || !isRandomDir(reservation.ChainID) ||
		reservation.Revision <= 0 ||
		reservation.LastAssignedRevision < reservation.Revision ||
		reservation.DeletedAt.IsZero() || !isUTCSecond(reservation.DeletedAt) {
		return nil, true, errors.New("airplan: invalid final revision delete reservation")
	}
	return &reservation, true, nil
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

func (c *Client) loadCanonicalAfterDeleteReservation(
	ctx context.Context, reserved *VersionsMetadata, target *RevisionDescriptor,
) (*revisionDocument, error) {
	for index := len(reserved.Revisions) - 1; index >= 0; index-- {
		revision := reserved.Revisions[index]
		if revision.Deleted {
			continue
		}
		doc, err := c.loadRevisionDocument(ctx, revision.URL)
		if errors.Is(err, errOwnershipMarkerMissing) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf(
				"airplan: resolve survivor after linked-delete reservation: %w", err,
			)
		}
		if doc.versions == nil || doc.versions.ChainID != target.ChainID {
			return nil, errors.New(
				"airplan: surviving revision metadata conflicts with delete reservation",
			)
		}
		foundTarget := false
		for _, entry := range doc.versions.Revisions {
			if entry.Number == target.Number {
				foundTarget = true
				break
			}
		}
		if !foundTarget {
			return nil, errors.New(
				"airplan: surviving revision metadata lost the delete reservation",
			)
		}
		return doc, nil
	}
	return nil, errors.New(
		"airplan: linked-delete reservation has no surviving revision metadata",
	)
}

func (c *Client) tombstoneLinkedRevision(
	ctx context.Context, targetDirPrefix string, marker *UploadMarker,
) (*VersionsMetadata, error) {
	targetMetadataKey := targetDirPrefix + VersionsFilename
	body, targetETag, _, err := c.st.getBytesWithETag(ctx,
		targetMetadataKey, MaxVersionsMetadataSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf(
				"%w: %w", errTargetRevisionMetadataMissing, err,
			)
		}
		return nil, fmt.Errorf("airplan: read revision metadata before delete: %w", err)
	}
	finalReservation, finalReserved, err := decodeFinalRevisionDeleteReservation(body)
	if err != nil {
		return nil, err
	}
	if finalReserved {
		if finalReservation.ChainID != marker.Revision.ChainID ||
			finalReservation.Revision != marker.Revision.Number {
			return nil, errors.New("airplan: marker and final revision delete reservation conflict")
		}
		return nil, nil
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
	if targetReserved {
		canonical, loadErr := c.loadCanonicalAfterDeleteReservation(
			ctx, metadata, marker.Revision,
		)
		if loadErr != nil {
			return nil, loadErr
		}
		metadata = canonical.versions
		serializationKey = canonical.dirPrefix + VersionsFilename
		serializationBody = canonical.versionsBody
		serializationETag = canonical.versionsETag
	}

	seen := map[string]bool{serializationKey: true}
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
			if !revision.Deleted {
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
		reservation, encodeErr := encodeFinalRevisionDeleteReservation(
			metadata.ChainID, marker.Revision.Number,
			metadata.LastAssignedRevision, deletedAt,
		)
		if encodeErr != nil {
			return nil, encodeErr
		}
		if putErr := c.st.putConditional(ctx, object{
			Key: targetMetadataKey, Body: reservation,
			ContentType: markerContentType,
		}, targetETag); putErr != nil {
			return nil, fmt.Errorf("airplan: reserve final revision deletion: %w", putErr)
		}
		return nil, nil
	}
	metadata.LatestRevision = latest
	bodies, err := c.encodeMemberMetadata(*metadata)
	if err != nil {
		return nil, err
	}
	expectedSerializationBody := serializationBody
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
		expectedSerializationBody = committedBody
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
			if errors.Is(err, errObjectNotFound) {
				if err := c.verifyObjectBody(
					ctx, serializationKey, expectedSerializationBody,
				); err != nil {
					return nil, err
				}
				resolved, markerErr := c.resolveMarker(ctx, dirPrefix)
				if markerErr != nil {
					return nil, errors.New("airplan: revision metadata recreation refused because the member marker is unavailable")
				}
				member, markerErr := DecodeUploadMarkerForName(resolved.Body,
					path.Base(strings.TrimSuffix(dirPrefix, "/")), resolved.Basename)
				if markerErr != nil || member.Revision == nil ||
					member.Revision.ChainID != metadata.ChainID ||
					member.Revision.Number != revision.Number {
					return nil, errors.New("airplan: revision metadata recreation refused because the member marker changed")
				}
				if putErr := c.st.putIfAbsent(ctx, object{
					Key: metadataKey, Body: bodies[revision.Number],
					ContentType: markerContentType,
				}); putErr != nil {
					return nil, fmt.Errorf("airplan: revision tombstone recreation failed: %w", putErr)
				}
				continue
			}
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
	if err := c.verifyRevisionChain(ctx, *metadata, bodies); err != nil {
		return nil, err
	}
	if !targetReserved && serializationKey != targetMetadataKey {
		receipt := *metadata
		receipt.CurrentRevision = marker.Revision.Number
		receiptBody, encodeErr := encodeVersionsMetadata(receipt, c.cfg, "", true)
		if encodeErr != nil {
			return nil, encodeErr
		}
		if putErr := c.st.putConditional(ctx, object{
			Key: targetMetadataKey, Body: receiptBody,
			ContentType: markerContentType,
		}, targetETag); putErr != nil {
			return nil, fmt.Errorf("airplan: preserve linked revision delete receipt: %w", putErr)
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
	revisionChainID, revision, err := c.missingMarkerRevisionIdentity(
		ctx, dirPrefix, record,
	)
	if err != nil {
		return nil, err
	}
	res := &DeleteResult{
		PageKey: record.Key, MarkerKey: manifestMarkerKey(record),
		Kind:            UploadKind(record.Kind),
		revisionChainID: revisionChainID,
		revision:        revision,
	}
	res.Warnings = append(res.Warnings, fmt.Sprintf(
		"ownership marker is already absent under %q; recording the completed deletion",
		dirPrefix,
	))
	c.recordDelete(ctx, res)
	return res, nil
}

// missingMarkerRevisionIdentity recovers linked-delete identity from the
// durable versions receipt when local history predates or missed the first
// link projection. A malformed or conflicting receipt fails closed rather
// than weakening the manifest tombstone that prevents stale link resurrection.
func (c *Client) missingMarkerRevisionIdentity(
	ctx context.Context, dirPrefix string, record ManifestRecord,
) (string, int, error) {
	chainID := ""
	revision := 0
	if manifestRecordHasAnnouncedRevision(record) {
		chainID = record.RevisionChainID
		revision = record.Revision
	}
	body, err := c.st.getBytes(
		ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if errors.Is(err, errObjectNotFound) {
		return chainID, revision, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf(
			"airplan: inspect missing-marker delete receipt: %w", err,
		)
	}
	if bytes.Equal(body, standaloneDeleteReservationBody) {
		if chainID != "" {
			return "", 0, errors.New(
				"airplan: local manifest and standalone delete receipt conflict",
			)
		}
		return chainID, revision, nil
	}

	receiptChainID := ""
	receiptRevision := 0
	reservation, reserved, err := decodeFinalRevisionDeleteReservation(body)
	if err != nil {
		return "", 0, err
	}
	if reserved {
		receiptChainID = reservation.ChainID
		receiptRevision = reservation.Revision
	} else {
		metadata, decodeErr := decodeVersionsMetadata(body, c.cfg, "", true)
		if decodeErr != nil {
			return "", 0, fmt.Errorf(
				"airplan: invalid missing-marker delete receipt: %w", decodeErr,
			)
		}
		current := &metadata.Revisions[metadata.CurrentRevision-1]
		if !current.Deleted {
			return "", 0, errors.New(
				"airplan: missing-marker versions object is not a delete receipt",
			)
		}
		receiptChainID = metadata.ChainID
		receiptRevision = metadata.CurrentRevision
	}
	if chainID != "" && (chainID != receiptChainID || revision != receiptRevision) {
		return "", 0, errors.New(
			"airplan: local manifest and missing-marker delete receipt conflict",
		)
	}
	return receiptChainID, receiptRevision, nil
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

		RevisionChainID: res.revisionChainID,
		Revision:        res.revision,
		LatestRevision:  res.latestRevision,
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
