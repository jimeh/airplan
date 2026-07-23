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
	MarkerVersion int                `json:"marker_version"`
	Page          *InspectedObject   `json:"page,omitempty"`
	Source        *InspectedObject   `json:"source,omitempty"`
	Files         []*InspectedObject `json:"files,omitempty"`
	// Warnings contains non-fatal URL assembly caveats for callers to report.
	Warnings []string `json:"warnings,omitempty"`

	// Error is set only for UploadInvalid and is stable for JSON output.
	Error MarkerErrorCode `json:"error,omitempty"`
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
	_ context.Context, upload RemoteUpload, markerBody []byte,
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
	inspection.CreatedAt = marker.CreatedAt
	inspection.Format = marker.Format
	inspection.Kind = marker.Kind
	inspection.Title = marker.Title
	inspection.Repo = marker.Repo
	inspection.MarkerVersion = marker.Version
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
				inspection.Source.ExpectedKnown = marker.Version == MarkerVersion
			}
		}
		fallback = fallback || sourceFallback
	}
	for _, object := range marker.Objects {
		if object.Role != MarkerRoleFile {
			continue
		}
		file, fileFallback := c.inspectedObject(dirPrefix+object.Name, byKey)
		file.ExpectedBytes = object.Bytes
		file.ExpectedKnown = true
		inspection.Files = append(inspection.Files, file)
		fallback = fallback || fileFallback
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
	for _, file := range inspection.Files {
		if !file.Exists || file.Bytes != file.ExpectedBytes {
			inspection.State = UploadIncomplete
		}
	}
	return inspection, nil
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
