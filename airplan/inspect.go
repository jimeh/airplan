package airplan

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	Key           string
	URL           string
	Exists        bool
	Bytes         int64
	ExpectedBytes int64
	ExpectedKnown bool
}

// UploadInspection is the result of targeted remote marker inspection.
type UploadInspection struct {
	State     UploadState
	Dir       string
	MarkerKey string
	Objects   int
	Bytes     int64

	CreatedAt time.Time
	Format    string
	Kind      UploadKind
	Title     string
	// Repo is the canonical repository URL declared by marker v2, or empty.
	Repo string
	// MarkerVersion is the validated ownership marker version.
	MarkerVersion int
	Page          *InspectedObject
	Source        *InspectedObject
	Files         []*InspectedObject
	// Warnings contains non-fatal URL assembly caveats for callers to report.
	Warnings []string

	// Error is set only for UploadInvalid and is stable for JSON output.
	Error MarkerErrorCode
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
	key, err := KeyFromURLOrKey(c.cfg, urlOrKey)
	if err != nil {
		return nil, err
	}
	dirPrefix, err := uploadDirPrefix(key)
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
	var markerKeys []string
	for _, object := range objects {
		base := filepath.Base(object.Key)
		if base == MarkerFilename || base == CollectionMarkerFilename {
			markerKeys = append(markerKeys, object.Key)
		}
	}
	if len(markerKeys) == 0 {
		markerKeys = append(markerKeys, dirPrefix+MarkerFilename)
	}
	if len(markerKeys) > 1 {
		return &UploadInspection{
			State: UploadInvalid, Dir: dir,
			MarkerKey: markerKeys[0], Objects: len(objects),
			Error: MarkerErrorConflictingMarkers,
		}, nil
	}
	markerBody, err := c.st.getBytes(ctx, markerKeys[0], MaxMarkerSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf("airplan: ownership marker %q is missing",
				markerKeys[0])
		}
		return nil, err
	}
	resolved := &resolvedMarker{
		Key:      markerKeys[0],
		Basename: filepath.Base(markerKeys[0]), Body: markerBody,
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

func (c *Client) inspectListedUpload(
	ctx context.Context, upload RemoteUpload,
) (*UploadInspection, error) {
	if upload.Conflict {
		return &UploadInspection{
			State: UploadInvalid, Dir: upload.Dir,
			MarkerKey: upload.MarkerKey, Error: MarkerErrorConflictingMarkers,
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
		filepath.Base(upload.MarkerKey))
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
	dirPrefix := strings.TrimSuffix(upload.MarkerKey, filepath.Base(upload.MarkerKey))
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
				inspection.Source.ExpectedKnown = true
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
	if inspection.Source != nil && inspection.Source.ExpectedBytes > 0 && inspection.Source.Bytes != inspection.Source.ExpectedBytes {
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
		return fmt.Errorf(
			"airplan: show target %q must be the random directory or a direct child",
			key,
		)
	}
	return nil
}
