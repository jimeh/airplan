package airplan

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DeleteResult describes a completed delete (SPEC.md §9).
type DeleteResult struct {
	// Keys are the object keys removed — page and siblings alike.
	Keys []string

	// PageKey is the .html page object's key when one was present;
	// it is what the manifest tombstone references.
	PageKey string

	// Warnings collects non-fatal issues (e.g. a manifest append
	// failure) for the caller to print to stderr.
	Warnings []string
}

// DeleteUpload removes an upload — every object under its random
// directory, so page and source go together — and appends a manifest
// tombstone (SPEC.md §9). The target may be a page URL, any object
// key from the upload, or the random directory itself, so it works on
// uploads made from other machines.
func (c *Client) DeleteUpload(
	ctx context.Context, urlOrKey string,
) (*DeleteResult, error) {
	key, err := KeyFromURLOrKey(c.cfg, urlOrKey)
	if err != nil {
		return nil, err
	}
	dir, err := uploadDirPrefix(key)
	if err != nil {
		return nil, err
	}

	objs, err := c.st.listKeys(ctx, dir)
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("airplan: no objects found under %q", dir)
	}

	res := &DeleteResult{}
	for _, o := range objs {
		res.Keys = append(res.Keys, o.Key)
		if strings.HasSuffix(o.Key, ".html") {
			res.PageKey = o.Key
		}
	}
	if res.PageKey == "" {
		// No page object (e.g. a previously half-deleted upload):
		// tombstone whatever key the caller referenced.
		res.PageKey = key
	}

	if err := c.st.deleteKeys(ctx, res.Keys); err != nil {
		return nil, err
	}

	c.recordDelete(res)
	return res, nil
}

// recordDelete appends a delete tombstone, best-effort like
// recordUpload: failures degrade to a warning, never a failed delete
// (the objects are already gone).
func (c *Client) recordDelete(res *DeleteResult) {
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

	rec := ManifestRecord{
		Type: "delete",
		Time: time.Now().UTC().Truncate(time.Second),
		Key:  res.PageKey,
	}
	if err := appendManifestRecord(path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"tombstone not recorded: "+err.Error())
	}
}
