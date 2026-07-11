package airplan

import (
	"context"
	"errors"
	"fmt"
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
	Key    string
	URL    string
	Exists bool
	Bytes  int64
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
	Title     string
	Page      *InspectedObject
	Source    *InspectedObject

	// Error is set only for UploadInvalid and is stable for JSON output.
	Error MarkerErrorCode
}

// InspectUpload fetches and validates one marker, lists its directory, and
// derives complete, incomplete, or invalid state (SPEC.md §9). Invalid marker
// content is a successful inspection result; request failures are errors.
func (c *Client) InspectUpload(
	ctx context.Context, urlOrKey string,
) (*UploadInspection, error) {
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
	markerKey := dirPrefix + MarkerFilename

	objects, err := c.st.listKeys(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	markerBody, err := c.st.getBytes(ctx, markerKey, MaxMarkerSize)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf("airplan: ownership marker %q is missing",
				markerKey)
		}
		return nil, err
	}

	inspection := &UploadInspection{
		Dir:       dir,
		MarkerKey: markerKey,
		Objects:   len(objects),
	}
	byKey := make(map[string]objectInfo, len(objects))
	markerListed := false
	for _, object := range objects {
		inspection.Bytes += object.Size
		byKey[object.Key] = object
		if object.Key == markerKey {
			markerListed = true
		}
	}
	// Reconcile an immediately-created marker that was fetched but did not
	// appear in the preceding directory listing.
	if !markerListed {
		inspection.Objects++
		inspection.Bytes += int64(len(markerBody))
	}

	marker, err := DecodeUploadMarker(markerBody, dir)
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
	inspection.Title = marker.Title
	inspection.Page = c.inspectedObject(dirPrefix+marker.Page, byKey)
	if marker.Source != "" {
		inspection.Source = c.inspectedObject(dirPrefix+marker.Source, byKey)
	}
	inspection.State = UploadComplete
	if !inspection.Page.Exists ||
		(inspection.Source != nil && !inspection.Source.Exists) {
		inspection.State = UploadIncomplete
	}
	return inspection, nil
}

func (c *Client) inspectedObject(
	key string, objects map[string]objectInfo,
) *InspectedObject {
	object, exists := objects[key]
	url, _ := PublicURL(c.cfg, key)
	return &InspectedObject{
		Key:    key,
		URL:    url,
		Exists: exists,
		Bytes:  object.Size,
	}
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
