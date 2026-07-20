package airplan

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// GetOptions selects which declared object GetUpload fetches (SPEC.md §9).
type GetOptions struct {
	// Source selects the marker-declared source object instead of the page. It
	// is valid only when the target is the random directory.
	Source bool
}

// GetResult is one fetched marker-managed object (SPEC.md §9).
type GetResult struct {
	// Key is the full storage key fetched.
	Key  string
	Body []byte
}

// GetUpload validates the upload's ownership marker and fetches one declared
// object without modifying remote storage or local history (SPEC.md §9).
func (c *Client) GetUpload(
	ctx context.Context, urlOrKey string, opts GetOptions,
) (*GetResult, error) {
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
			return nil, fmt.Errorf(
				"airplan: ownership marker %q is missing", markerKey,
			)
		}
		return nil, err
	}
	marker, err := DecodeUploadMarker(markerBody, dir)
	if err != nil {
		return nil, err
	}

	objectKey, err := resolveGetTarget(
		key, dirPrefix, markerKey, marker, opts,
	)
	if err != nil {
		return nil, err
	}
	if objectKey == markerKey {
		return &GetResult{Key: markerKey, Body: markerBody}, nil
	}

	body, err := c.st.getBytes(ctx, objectKey, 0)
	if err != nil {
		if errors.Is(err, errObjectNotFound) {
			return nil, fmt.Errorf(
				"airplan: upload object %q is missing", objectKey,
			)
		}
		return nil, err
	}
	return &GetResult{Key: objectKey, Body: body}, nil
}

func resolveGetTarget(
	key, dirPrefix, markerKey string,
	marker *UploadMarker,
	opts GetOptions,
) (string, error) {
	dirKey := strings.TrimSuffix(dirPrefix, "/")
	pageKey := dirPrefix + marker.Page
	sourceKey := ""
	if marker.Source != "" {
		sourceKey = dirPrefix + marker.Source
	}

	if key == dirKey {
		if !opts.Source {
			return pageKey, nil
		}
		if sourceKey == "" {
			return "", errors.New("airplan: upload declares no source object")
		}
		return sourceKey, nil
	}
	if key == markerKey || key == pageKey ||
		(sourceKey != "" && key == sourceKey) {
		if opts.Source {
			return "", errors.New(
				"airplan: --source cannot be used with an explicit child target",
			)
		}
		return key, nil
	}
	return "", fmt.Errorf(
		"airplan: get target %q is not the directory, marker, or declared payload",
		key,
	)
}
