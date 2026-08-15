package airplan

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// UploadState is the validated lifecycle state reported by show (SPEC.md §9).
type UploadState string

// Upload inspection states.
const (
	UploadComplete   UploadState = "complete"
	UploadIncomplete UploadState = "incomplete"
	UploadInvalid    UploadState = "invalid"
)

// InspectedObject describes one marker-declared payload object.
type InspectedObject struct {
	Key           string `json:"key"`
	URL           string `json:"url"`
	Exists        bool   `json:"exists"`
	Bytes         int64  `json:"bytes"`
	ExpectedBytes int64  `json:"expected_bytes,omitempty"`
	ExpectedKnown bool   `json:"expected_known,omitempty"`
}

// UploadInspection is the result of targeted remote marker inspection.
type UploadInspection struct {
	State     UploadState `json:"state"`
	Dir       string      `json:"dir"`
	MarkerKey string      `json:"marker_key"`
	Objects   int         `json:"objects"`
	Bytes     int64       `json:"bytes"`

	CreatedAt time.Time  `json:"created_at"`
	Format    string     `json:"format,omitempty"`
	Kind      UploadKind `json:"kind,omitempty"`
	Title     string     `json:"title,omitempty"`
	// Repo is the canonical repository URL declared by marker v2, or empty.
	Repo string `json:"repository_url,omitempty"`
	// MarkerVersion is the validated ownership marker version.
	MarkerVersion      int                `json:"marker_version"`
	ProducerVersion    string             `json:"producer_version,omitempty"`
	RendererGeneration int                `json:"renderer_version,omitempty"`
	Page               *InspectedObject   `json:"page,omitempty"`
	Source             *InspectedObject   `json:"source,omitempty"`
	Files              []*InspectedObject `json:"files,omitempty"`
	Diff               *InspectedObject   `json:"diff,omitempty"`
	RevisionChainID    string             `json:"revision_chain_id,omitempty"`
	Revision           int                `json:"revision,omitempty"`
	LatestRevision     int                `json:"latest_revision,omitempty"`
	LatestURL          string             `json:"latest_url,omitempty"`
	Versions           *VersionsMetadata  `json:"versions,omitempty"`
	RevisionError      string             `json:"revision_error,omitempty"`
	// Protected reports the purge-protection sentinel's presence in the
	// directory listing (SPEC.md §9). ProtectedAt falls back to the sentinel's
	// listing timestamp when its body cannot be read or decoded; ProtectReason
	// stays empty when that advisory metadata is unavailable.
	Protected     bool      `json:"protected,omitempty"`
	ProtectedAt   time.Time `json:"protected_at,omitzero"`
	ProtectReason string    `json:"protect_reason,omitempty"`
	// Warnings contains non-fatal URL assembly caveats for callers to report.
	Warnings []string `json:"warnings,omitempty"`

	// Error is set only for UploadInvalid and is stable for JSON output.
	Error MarkerErrorCode `json:"error,omitempty"`

	// marker and markerBytes retain the validated marker and the exact body
	// length that was fetched, so callers deriving declared totals use the
	// same inputs the upload writer had (SPEC.md §9). They stay unexported:
	// the marker's own fields are untrusted content that inspection results
	// deliberately do not republish.
	marker      *UploadMarker
	markerBytes int
}

// declaredTotalsFromInspection derives an upload's marker-declared totals from
// a validated inspection, using the same rule as the upload writer.
func declaredTotalsFromInspection(
	inspection *UploadInspection,
) declaredTotals {
	if inspection == nil || inspection.marker == nil {
		return declaredTotals{}
	}
	objects, total, ok := MarkerDeclaredTotals(
		*inspection.marker, inspection.markerBytes,
	)
	if !ok {
		return declaredTotals{}
	}
	return declaredTotals{objects: objects, bytes: total}
}

// InspectUpload fetches and validates one marker, lists its directory, and
// derives complete, incomplete, or invalid state (SPEC.md §9). Invalid marker
// content is a successful inspection result; request failures are errors.
func (c *Client) InspectUpload(
	ctx context.Context, urlOrKey string,
) (*UploadInspection, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.InspectUpload(ctx, urlOrKey)
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
	if err := validateInspectionTarget(key, dirPrefix); err != nil {
		return nil, err
	}

	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]
	objects, err := c.st.listKeys(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	documentMarkerKey := dirPrefix + MarkerFilename
	collectionMarkerKey := dirPrefix + CollectionMarkerFilename
	var documentMarker, collectionMarker bool
	for _, object := range objects {
		switch object.Key {
		case documentMarkerKey:
			documentMarker = true
		case collectionMarkerKey:
			collectionMarker = true
		}
	}
	markerKey := documentMarkerKey
	if !documentMarker && collectionMarker {
		markerKey = collectionMarkerKey
	}
	if documentMarker && collectionMarker {
		return &UploadInspection{
			State: UploadInvalid, Dir: dir,
			MarkerKey: documentMarkerKey, Objects: len(objects),
			Bytes: sumObjectBytes(objects),
			Error: MarkerErrorConflictingMarkers,
		}, nil
	}
	markerBody, err := c.st.getBytes(ctx, markerKey, MaxMarkerSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf("airplan: ownership marker %q is missing",
				markerKey)
		}
		return nil, err
	}
	resolved := &resolvedMarker{
		Key: markerKey, Basename: path.Base(markerKey), Body: markerBody,
	}

	return c.inspectUploadSnapshot(ctx, RemoteUpload{
		Dir:       dir,
		MarkerKey: resolved.Key,
		Objects:   len(objects),
		objects:   byObjectKey(objects),
	}, resolved.Body)
}

func byObjectKey(objects []objectInfo) map[string]objectInfo {
	byKey := make(map[string]objectInfo, len(objects))
	for _, object := range objects {
		byKey[object.Key] = object
	}
	return byKey
}

func sumObjectBytes(objects []objectInfo) int64 {
	var total int64
	for _, object := range objects {
		total += object.Size
	}
	return total
}

func (c *Client) inspectListedUpload(
	ctx context.Context, upload RemoteUpload,
) (*UploadInspection, error) {
	if upload.Conflict {
		return &UploadInspection{
			State: UploadInvalid, Dir: upload.Dir,
			MarkerKey: upload.MarkerKey, Objects: upload.Objects,
			Bytes: upload.Bytes, Error: MarkerErrorConflictingMarkers,
		}, nil
	}
	markerBody, err := c.st.getBytes(ctx, upload.MarkerKey, MaxMarkerSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf("airplan: ownership marker %q is missing",
				upload.MarkerKey)
		}
		return nil, err
	}
	return c.inspectUploadSnapshot(ctx, upload, markerBody)
}

func (c *Client) inspectUploadSnapshot(
	ctx context.Context, upload RemoteUpload, markerBody []byte,
) (*UploadInspection, error) {
	inspection := &UploadInspection{
		Dir: upload.Dir, MarkerKey: upload.MarkerKey,
		Objects: len(upload.objects),
	}
	byKey := upload.objects
	markerListed := false
	for _, object := range byKey {
		inspection.Bytes += object.Size
		if object.Key == upload.MarkerKey {
			markerListed = true
		}
	}
	if !markerListed {
		inspection.Objects++
		inspection.Bytes += int64(len(markerBody))
	}
	c.inspectProtection(ctx, inspection, byKey)

	marker, err := DecodeUploadMarkerForName(markerBody, upload.Dir,
		path.Base(upload.MarkerKey))
	if err != nil {
		code, ok := MarkerCode(err)
		if !ok {
			return nil, err
		}
		inspection.State = UploadInvalid
		inspection.Error = code
		return inspection, nil
	}
	inspection.marker = marker
	inspection.markerBytes = len(markerBody)
	inspection.CreatedAt = marker.CreatedAt
	inspection.Format = marker.Format
	inspection.Kind = marker.Kind
	inspection.Title = marker.Title
	inspection.Repo = marker.Repo
	inspection.MarkerVersion = marker.Version
	inspection.ProducerVersion = marker.Producer.Version
	if marker.Render != nil {
		inspection.RendererGeneration = marker.Render.Generation
	}
	if marker.Revision != nil {
		inspection.RevisionChainID = marker.Revision.ChainID
		inspection.Revision = marker.Revision.Number
	}
	dirPrefix := strings.TrimSuffix(upload.MarkerKey, path.Base(upload.MarkerKey))
	var fallback bool
	inspection.Page, fallback = c.inspectedObject(dirPrefix+marker.Page, byKey)
	inspection.Page.ExpectedBytes = marker.PageBytes
	inspection.Page.ExpectedKnown = marker.Version >= 2
	if marker.Source != "" {
		var sourceFallback bool
		inspection.Source, sourceFallback = c.inspectedObject(
			dirPrefix+marker.Source, byKey,
		)
		for _, object := range marker.Objects {
			if object.Role == MarkerRoleSource {
				inspection.Source.ExpectedBytes = object.Bytes
				inspection.Source.ExpectedKnown = marker.Version >= 3
			}
		}
		fallback = fallback || sourceFallback
	}
	for _, object := range marker.Objects {
		if object.Role == MarkerRoleDiff {
			var diffFallback bool
			inspection.Diff, diffFallback = c.inspectedObject(dirPrefix+object.Name, byKey)
			inspection.Diff.ExpectedBytes = object.Bytes
			inspection.Diff.ExpectedKnown = true
			fallback = fallback || diffFallback
			continue
		}
		if object.Role != MarkerRoleFile {
			continue
		}
		file, fileFallback := c.inspectedObject(dirPrefix+object.Name, byKey)
		file.ExpectedBytes = object.Bytes
		file.ExpectedKnown = true
		inspection.Files = append(inspection.Files, file)
		fallback = fallback || fileFallback
	}
	if marker.Revision != nil {
		body, metadataErr := c.st.getBytes(ctx, dirPrefix+VersionsFilename,
			MaxVersionsMetadataSize)
		if metadataErr != nil {
			inspection.RevisionError = "versions metadata is unavailable"
		} else {
			metadata, decodeErr := DecodeVersionsMetadata(body, c.cfg,
				dirPrefix+marker.Page)
			if decodeErr != nil || metadata.ChainID != marker.Revision.ChainID ||
				metadata.CurrentRevision != marker.Revision.Number {
				inspection.RevisionError = "versions metadata is invalid"
			} else {
				inspection.Versions = metadata
				inspection.LatestRevision = metadata.LatestRevision
				if latest := liveVersionsRevision(metadata, metadata.LatestRevision); latest != nil {
					inspection.LatestURL = latest.URL
				}
			}
		}
	}
	if fallback {
		inspection.Warnings = append(inspection.Warnings,
			PublicURLFallbackWarning)
	}
	inspection.State = UploadComplete
	if !inspection.Page.Exists ||
		(inspection.Source != nil && !inspection.Source.Exists) ||
		(marker.Version >= 2 && inspection.Page.Bytes != marker.PageBytes) {
		inspection.State = UploadIncomplete
	}
	if inspection.Source != nil && inspection.Source.ExpectedKnown &&
		inspection.Source.Bytes != inspection.Source.ExpectedBytes {
		inspection.State = UploadIncomplete
	}
	if inspection.Diff != nil &&
		(!inspection.Diff.Exists || inspection.Diff.Bytes != inspection.Diff.ExpectedBytes) {
		inspection.State = UploadIncomplete
	}
	for _, file := range inspection.Files {
		if !file.Exists || file.Bytes != file.ExpectedBytes {
			inspection.State = UploadIncomplete
		}
	}
	return inspection, nil
}

// inspectProtection derives protection state from the same listing snapshot
// the inspection already holds. Presence is authoritative (SPEC.md §5); the
// sentinel body only supplies advisory metadata, so any read or decode
// failure keeps the listing's protected state and timestamp.
func (c *Client) inspectProtection(
	ctx context.Context,
	inspection *UploadInspection,
	objects map[string]objectInfo,
) {
	dirPrefix := strings.TrimSuffix(
		inspection.MarkerKey, path.Base(inspection.MarkerKey),
	)
	sentinel, ok := objects[dirPrefix+ProtectedFilename]
	if !ok {
		return
	}
	inspection.Protected = true
	inspection.ProtectedAt = sentinel.LastModified.UTC()
	body, err := c.st.getBytes(ctx, sentinel.Key, MaxMarkerSize)
	if err != nil {
		return
	}
	createdAt, reason := decodeProtectionSentinel(body)
	if !createdAt.IsZero() {
		inspection.ProtectedAt = createdAt
	}
	inspection.ProtectReason = reason
}

func (c *Client) inspectedObject(
	key string, objects map[string]objectInfo,
) (*InspectedObject, bool) {
	object, exists := objects[key]
	url, fallback, _ := PublicURL(c.cfg, key)
	return &InspectedObject{
		Key:    key,
		URL:    url,
		Exists: exists,
		Bytes:  object.Size,
	}, fallback
}

func validateInspectionTarget(key, dirPrefix string) error {
	dirKey := strings.TrimSuffix(dirPrefix, "/")
	if key == dirKey {
		return nil
	}
	rel := strings.TrimPrefix(key, dirPrefix)
	if rel == key || rel == "" || strings.Contains(rel, "/") {
		return invalidTargetf(
			"airplan: show target %q must be the random directory or a direct child",
			key,
		)
	}
	return nil
}
