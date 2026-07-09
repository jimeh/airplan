package airplan

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"
)

// RemoteUpload describes one upload discovered from a live bucket
// listing (SPEC.md §9). Bytes and LastModified describe the page
// object.
type RemoteUpload struct {
	Dir          string
	PageKey      string
	Keys         []string
	Bytes        int64
	LastModified time.Time
}

type remoteGroup struct {
	dir     string
	keys    []string
	pages   []objectInfo
	invalid bool
}

// ListRemote discovers airplan uploads from the configured bucket and
// key_prefix. It only recognizes the precise key shape from SPEC.md
// §9 so unrelated shared-bucket objects are never returned.
func (c *Client) ListRemote(ctx context.Context) ([]RemoteUpload, error) {
	prefix := strings.Trim(c.cfg.KeyPrefix, "/")
	if prefix != "" {
		prefix += "/"
	}

	objs, err := c.st.listKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}

	groups := map[string]*remoteGroup{}
	for _, obj := range objs {
		if !strings.HasPrefix(obj.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(obj.Key, prefix)
		segs := strings.Split(rest, "/")
		if len(segs) == 0 || !isRandomDir(segs[0]) {
			continue
		}

		dir := BuildKey(prefix, segs[0], "")
		group := groups[dir]
		if group == nil {
			group = &remoteGroup{dir: dir}
			groups[dir] = group
		}

		if len(segs) != 2 || segs[1] == "" {
			group.invalid = true
			continue
		}

		group.keys = append(group.keys, obj.Key)
		if path.Ext(segs[1]) == ".html" {
			group.pages = append(group.pages, obj)
		}
	}

	uploads := make([]RemoteUpload, 0, len(groups))
	for _, group := range groups {
		if group.invalid || len(group.pages) == 0 {
			continue
		}
		sort.Slice(group.pages, func(i, j int) bool {
			return group.pages[i].Key < group.pages[j].Key
		})
		sort.Strings(group.keys)

		page := group.pages[0]
		uploads = append(uploads, RemoteUpload{
			Dir:          group.dir,
			PageKey:      page.Key,
			Keys:         group.keys,
			Bytes:        page.Size,
			LastModified: page.LastModified,
		})
	}

	sort.Slice(uploads, func(i, j int) bool {
		if uploads[i].LastModified.Equal(uploads[j].LastModified) {
			return uploads[i].PageKey < uploads[j].PageKey
		}
		return uploads[i].LastModified.Before(uploads[j].LastModified)
	})
	return uploads, nil
}

// RemoteTitle fetches the title metadata for a remote page object.
func (c *Client) RemoteTitle(ctx context.Context, pageKey string) (string, error) {
	return c.st.headTitle(ctx, pageKey)
}
