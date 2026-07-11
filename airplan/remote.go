package airplan

import (
	"context"
	"sort"
	"strings"
	"time"
)

// RemoteUpload describes one marker directory discovered by a live bucket
// listing (SPEC.md §9). Its fields describe storage occupancy only; marker
// content has not been fetched or trusted.
type RemoteUpload struct {
	// Dir is the 26-character random directory without key_prefix.
	Dir string

	// MarkerKey is the marker's full storage key, including key_prefix.
	MarkerKey string

	// Slug is inferred only when exactly one valid direct-child HTML page
	// filename exists. It is a display hint, not trusted marker data.
	Slug string

	// Keys contains every object key recursively beneath the directory.
	Keys []string

	// Objects and Bytes describe all Keys, including the marker and extras.
	Objects int
	Bytes   int64

	// LastModified is the marker object's storage timestamp.
	LastModified time.Time
}

type remoteGroup struct {
	dir       string
	marker    *objectInfo
	keys      []string
	bytes     int64
	pageSlugs []string
}

// ListRemote discovers exact marker-key candidates under key_prefix using
// LIST operations only. It never fetches markers or heads payload objects.
func (c *Client) ListRemote(ctx context.Context) ([]RemoteUpload, error) {
	prefix := strings.Trim(c.cfg.KeyPrefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	objects, err := c.st.listKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}

	groups := make(map[string]*remoteGroup)
	for _, obj := range objects {
		if !strings.HasPrefix(obj.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, prefix)
		segments := strings.Split(rest, "/")
		if len(segments) < 2 || !isRandomDir(segments[0]) {
			continue
		}

		dir := segments[0]
		group := groups[dir]
		if group == nil {
			group = &remoteGroup{
				dir: dir,
			}
			groups[dir] = group
		}
		group.keys = append(group.keys, obj.Key)
		group.bytes += obj.Size

		if len(segments) != 2 {
			continue
		}
		switch segments[1] {
		case MarkerFilename:
			marker := obj
			group.marker = &marker
		default:
			if slug, ok := pageSlug(segments[1]); ok {
				group.pageSlugs = append(group.pageSlugs, slug)
			}
		}
	}

	uploads := make([]RemoteUpload, 0, len(groups))
	for _, group := range groups {
		if group.marker == nil {
			continue
		}
		sort.Strings(group.keys)
		upload := RemoteUpload{
			Dir:          group.dir,
			MarkerKey:    group.marker.Key,
			Keys:         group.keys,
			Objects:      len(group.keys),
			Bytes:        group.bytes,
			LastModified: group.marker.LastModified.UTC(),
		}
		if len(group.pageSlugs) == 1 {
			upload.Slug = group.pageSlugs[0]
		}
		uploads = append(uploads, upload)
	}

	sort.Slice(uploads, func(i, j int) bool {
		if uploads[i].LastModified.Equal(uploads[j].LastModified) {
			return uploads[i].MarkerKey < uploads[j].MarkerKey
		}
		return uploads[i].LastModified.Before(uploads[j].LastModified)
	})
	return uploads, nil
}
