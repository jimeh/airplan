package airplan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultRemoteConcurrency is the default targeted-request limit used by
// remote inspection operations.
const DefaultRemoteConcurrency = 8

// MaxRemoteConcurrency is the largest supported targeted-request limit.
const MaxRemoteConcurrency = 64

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

	objects map[string]objectInfo
}

type remoteGroup struct {
	dir       string
	marker    *objectInfo
	keys      []string
	objects   map[string]objectInfo
	bytes     int64
	pageSlugs []string
}

// ListRemote discovers exact marker-key candidates under key_prefix using
// LIST operations only. It never fetches markers or heads payload objects.
func (c *Client) ListRemote(ctx context.Context) ([]RemoteUpload, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
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
				dir:     dir,
				objects: make(map[string]objectInfo),
			}
			groups[dir] = group
		}
		group.keys = append(group.keys, obj.Key)
		group.objects[obj.Key] = obj
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
			objects:      group.objects,
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

// ValidateRemoteConcurrency validates a targeted remote request limit. Zero
// selects DefaultRemoteConcurrency for library callers.
func ValidateRemoteConcurrency(concurrency int) (int, error) {
	if concurrency == 0 {
		return DefaultRemoteConcurrency, nil
	}
	if concurrency < 1 || concurrency > MaxRemoteConcurrency {
		return 0, fmt.Errorf("airplan: concurrency must be between 1 and %d",
			MaxRemoteConcurrency)
	}
	return concurrency, nil
}

// RemoteInspectionResult pairs a listed upload with its targeted marker
// inspection. Err describes a request failure; invalid marker content is an
// inspection state instead.
type RemoteInspectionResult struct {
	// Upload is the untrusted LIST-derived remote candidate.
	Upload RemoteUpload
	// Inspection is non-nil when marker inspection completed.
	Inspection *UploadInspection
	// Err is a targeted request failure; invalid content uses Inspection.State.
	Err error
}

// InspectRemoteUploads fetches and validates listed markers with bounded
// concurrency. Results preserve input order and reuse the listing snapshot.
func (c *Client) InspectRemoteUploads(
	ctx context.Context, uploads []RemoteUpload, concurrency int,
) ([]RemoteInspectionResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	limit, err := ValidateRemoteConcurrency(concurrency)
	if err != nil {
		return nil, err
	}
	results := make([]RemoteInspectionResult, len(uploads))
	if len(uploads) == 0 {
		return results, nil
	}
	type job struct {
		index  int
		upload RemoteUpload
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	for range min(limit, len(uploads)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				inspection, inspectErr := c.inspectListedUpload(
					ctx, job.upload,
				)
				results[job.index] = RemoteInspectionResult{
					Upload: job.upload, Inspection: inspection, Err: inspectErr,
				}
			}
		}()
	}
	for index, upload := range uploads {
		select {
		case jobs <- job{index: index, upload: upload}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return results, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return results, nil
}
