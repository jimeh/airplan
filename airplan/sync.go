package airplan

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"sync"
	"time"
)

// SyncManifestOptions controls remote-to-local manifest reconciliation.
type SyncManifestOptions struct {
	// ManifestPath overrides the client's configured/default manifest path.
	ManifestPath string
	// Concurrency limits targeted remote requests. Zero uses the default.
	Concurrency int
	// Prune appends tombstones for scoped markers confirmed absent remotely.
	Prune bool
	// DryRun validates and plans reconciliation without writing the manifest.
	DryRun bool
}

// SyncFailure describes one remote item that could not be reconciled.
type SyncFailure struct {
	// MarkerKey is the remote item that could not be reconciled.
	MarkerKey string `json:"marker_key"`
	// Operation is "fetch" or "confirm_absence".
	Operation string `json:"operation"`
	// Error is the underlying request failure text.
	Error string `json:"error"`
}

// SyncManifestResult describes planned or applied manifest reconciliation.
type SyncManifestResult struct {
	// Added contains upload records planned or appended by this run.
	Added []ManifestRecord `json:"added_records"`
	// Enriched contains active upload records re-appended with declared
	// objects/total_bytes counts (SPEC.md §9). They are not new uploads
	// and are counted separately from Added.
	Enriched []ManifestRecord `json:"enriched_records"`
	// Tombstoned contains delete records planned or appended by this run.
	Tombstoned []ManifestRecord `json:"tombstone_records"`
	// Unchanged counts remote markers already active in the local manifest.
	Unchanged int `json:"unchanged"`
	// Incomplete counts valid markers whose declared payload is incomplete.
	Incomplete int `json:"incomplete"`
	// Invalid counts marker bodies that fail marker validation.
	Invalid int `json:"invalid"`
	// Retained counts local records kept after a listing inconsistency or error.
	Retained int `json:"retained"`
	// Failures contains per-item request failures in deterministic order.
	Failures []SyncFailure `json:"failures"`
	// Warnings contains manifest and public URL warnings.
	Warnings []string `json:"warnings,omitempty"`
}

// SyncManifest reconciles the selected remote marker inventory into a local
// append-only manifest. It returns successfully validated progress alongside
// an aggregate error when individual targeted requests fail.
func (c *Client) SyncManifest(
	ctx context.Context, opts SyncManifestOptions,
) (*SyncManifestResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		if opts.ManifestPath != "" {
			return nil, errors.New(
				"airplan: manifest path cannot be selected for the airplan backend",
			)
		}
		return c.remote.SyncManifest(ctx, opts)
	}
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}
	concurrency, err := ValidateRemoteConcurrency(opts.Concurrency)
	if err != nil {
		return nil, err
	}
	if c.cfg.DisableManifest {
		return nil, errors.New("airplan: local manifest is disabled")
	}
	manifestPath := opts.ManifestPath
	if manifestPath == "" {
		manifestPath = c.cfg.ManifestPath
	}
	if manifestPath == "" {
		manifestPath, err = DefaultManifestPath()
		if err != nil {
			return nil, err
		}
	}

	records, warnings, err := readManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	result := &SyncManifestResult{Warnings: append([]string(nil), warnings...)}
	active := scopedActiveUploads(records, c.cfg)
	remote, err := c.ListRemote(ctx)
	if err != nil {
		return nil, err
	}
	remoteByMarker := make(map[string]RemoteUpload, len(remote))
	for _, upload := range remote {
		remoteByMarker[upload.MarkerKey] = upload
		if _, ok := active[upload.MarkerKey]; ok {
			result.Unchanged++
		}
	}

	jobs := make([]syncJob, 0, len(remote)+len(active))
	for index := range remote {
		upload := remote[index]
		record, ok := active[upload.MarkerKey]
		if !ok {
			jobs = append(jobs, syncJob{
				markerKey: upload.MarkerKey, upload: &upload,
			})
			continue
		}
		// Backfill declared objects/total_bytes for active records that
		// predate those fields (SPEC.md §9). Only records missing both
		// fields qualify, so backfill converges once a marker inspects
		// complete and fully declares its inventory; markers that never
		// do are re-inspected on later syncs and left untouched.
		if record.Objects == 0 && record.TotalBytes == 0 {
			jobs = append(jobs, syncJob{
				markerKey: upload.MarkerKey, upload: &upload,
				local: &record, enrich: true,
			})
		}
	}
	if opts.Prune {
		markerKeys := make([]string, 0, len(active))
		for markerKey := range active {
			markerKeys = append(markerKeys, markerKey)
		}
		sort.Strings(markerKeys)
		for _, markerKey := range markerKeys {
			if _, ok := remoteByMarker[markerKey]; ok {
				continue
			}
			record := active[markerKey]
			jobs = append(jobs, syncJob{
				markerKey: markerKey, local: &record,
			})
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].markerKey < jobs[j].markerKey
	})

	jobResults := make([]syncJobResult, len(jobs))
	jobCh := make(chan int)
	var wg sync.WaitGroup
	for range min(concurrency, len(jobs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobCh {
				job := jobs[index]
				switch {
				case job.enrich:
					jobResults[index] = c.syncEnrich(
						ctx, *job.upload, *job.local,
					)
				case job.upload != nil:
					jobResults[index] = c.syncImport(ctx, *job.upload)
				default:
					jobResults[index] = c.syncPrune(ctx, *job.local)
				}
			}
		}()
	}
	scheduling := true
	for index := range jobs {
		if !scheduling {
			jobResults[index].failure = syncFailureForContext(jobs[index], ctx.Err())
			continue
		}
		select {
		case jobCh <- index:
		case <-ctx.Done():
			jobResults[index].failure = syncFailureForContext(jobs[index], ctx.Err())
			scheduling = false
		}
	}
	close(jobCh)
	wg.Wait()

	for _, item := range jobResults {
		switch item.state {
		case UploadIncomplete:
			result.Incomplete++
		case UploadInvalid:
			result.Invalid++
		}
		if item.upload != nil {
			result.Added = append(result.Added, *item.upload)
		}
		if item.enriched != nil {
			result.Enriched = append(result.Enriched, *item.enriched)
		}
		if item.tombstone != nil {
			result.Tombstoned = append(result.Tombstoned, *item.tombstone)
		}
		if item.retained {
			result.Retained++
		}
		if item.failure != nil {
			result.Failures = append(result.Failures, *item.failure)
		}
		if item.warning != "" {
			result.Warnings = append(result.Warnings, item.warning)
		}
	}

	if !opts.DryRun && (len(result.Added) > 0 ||
		len(result.Enriched) > 0 || len(result.Tombstoned) > 0) {
		initialLen := len(records)
		if err := c.commitSyncManifest(
			ctx, manifestPath, initialLen, result,
		); err != nil {
			return nil, err
		}
	}
	if len(result.Failures) > 0 {
		return result, fmt.Errorf("airplan: sync incomplete: %d remote request(s) failed",
			len(result.Failures))
	}
	return result, nil
}

func syncFailureForContext(job syncJob, err error) *SyncFailure {
	operation := "fetch"
	if job.local != nil && !job.enrich {
		operation = "confirm_absence"
	}
	return &SyncFailure{
		MarkerKey: job.markerKey, Operation: operation, Error: err.Error(),
	}
}

func scopedActiveUploads(
	records []ManifestRecord, cfg *Config,
) map[string]ManifestRecord {
	active := make(map[string]ManifestRecord)
	for _, rec := range ActiveUploads(records) {
		markerKey := manifestMarkerKey(rec)
		if rec.Profile == cfg.Profile && rec.Bucket == cfg.Bucket &&
			markerKey != "" && KeyMatchesPrefix(markerKey, cfg.KeyPrefix) {
			active[markerKey] = rec
		}
	}
	return active
}

func (c *Client) syncImport(
	ctx context.Context, upload RemoteUpload,
) syncJobResult {
	inspection, markerBody, err := c.inspectListedUploadBody(ctx, upload)
	if err != nil {
		return syncJobResult{failure: &SyncFailure{
			MarkerKey: upload.MarkerKey, Operation: "fetch", Error: err.Error(),
		}}
	}
	if inspection.State != UploadComplete {
		return syncJobResult{state: inspection.State}
	}
	if inspection.Page.Bytes <= 0 {
		return syncJobResult{state: UploadIncomplete}
	}
	record := ManifestRecord{
		Type: "upload", Time: inspection.CreatedAt.UTC(),
		Key: inspection.Page.Key, MarkerKey: inspection.MarkerKey,
		URL: inspection.Page.URL, Bucket: c.cfg.Bucket,
		Profile: c.cfg.Profile, Format: inspection.Format,
		Kind:  string(inspection.Kind),
		Title: inspection.Title, Repo: inspection.Repo,
		Bytes: inspection.Page.Bytes, MarkerVersion: inspection.MarkerVersion,
	}
	if objects, total, ok := declaredInspectionTotals(
		inspection, len(markerBody),
	); ok {
		record.Objects, record.TotalBytes = objects, total
	}
	if inspection.Kind == UploadKindDocument {
		record.Slug, _ = pageSlug(path.Base(inspection.Page.Key))
	}
	if inspection.Source != nil {
		record.SourceKey = inspection.Source.Key
	}
	result := syncJobResult{upload: &record}
	if len(inspection.Warnings) > 0 {
		result.warning = inspection.Warnings[0]
	}
	return result
}

// declaredInspectionTotals derives the marker-declared inventory from a
// complete inspection: the marker plus its page, optional source, and
// files (SPEC.md §9). The counts are recorded only when the marker
// fully determines every counted object's bytes — marker v1 never
// declares page bytes and v1/v2 never declare source bytes, so those
// markers report ok=false and both fields stay absent rather than
// persisting observed listing sizes labelled as declared. Interrupted
// remnants and unrecognized extras are deliberately excluded, keeping
// the values identical to a fresh upload of the same content.
func declaredInspectionTotals(
	inspection *UploadInspection, markerBytes int,
) (int, int64, bool) {
	if !inspection.Page.ExpectedKnown {
		return 0, 0, false
	}
	objects := 2 // the marker and its page
	total := int64(markerBytes) + inspection.Page.ExpectedBytes
	if inspection.Source != nil {
		if !inspection.Source.ExpectedKnown {
			return 0, 0, false
		}
		objects++
		total += inspection.Source.ExpectedBytes
	}
	for _, file := range inspection.Files {
		objects++
		total += file.ExpectedBytes
	}
	return objects, total, true
}

// syncEnrich re-appends one active record with declared inventory
// counts (SPEC.md §9). The enriched record carries the original time
// and identity, so latest-wins reduction collapses it in place. A
// non-complete inspection leaves the record untouched rather than
// guessing.
func (c *Client) syncEnrich(
	ctx context.Context, upload RemoteUpload, record ManifestRecord,
) syncJobResult {
	inspection, markerBody, err := c.inspectListedUploadBody(ctx, upload)
	if err != nil {
		return syncJobResult{failure: &SyncFailure{
			MarkerKey: upload.MarkerKey, Operation: "fetch",
			Error: err.Error(),
		}}
	}
	if inspection.State != UploadComplete {
		return syncJobResult{}
	}
	objects, total, ok := declaredInspectionTotals(
		inspection, len(markerBody),
	)
	if !ok {
		return syncJobResult{}
	}
	enriched := record
	enriched.Objects, enriched.TotalBytes = objects, total
	return syncJobResult{enriched: &enriched}
}

func (c *Client) syncPrune(
	ctx context.Context, record ManifestRecord,
) syncJobResult {
	markerKey := manifestMarkerKey(record)
	_, err := c.st.getBytes(ctx, markerKey, MaxMarkerSize)
	if err == nil {
		return syncJobResult{
			retained: true,
			warning: fmt.Sprintf(
				"marker %q was absent from listing but exists remotely; retained",
				markerKey),
		}
	}
	if !errors.Is(err, errObjectNotFound) {
		return syncJobResult{
			retained: true,
			failure: &SyncFailure{
				MarkerKey: markerKey,
				Operation: "confirm_absence", Error: err.Error(),
			},
		}
	}
	tombstone := ManifestRecord{
		Type: "delete", Time: time.Now().UTC().Truncate(time.Second),
		Key: record.Key, MarkerKey: markerKey, Bucket: c.cfg.Bucket,
		Profile: c.cfg.Profile, Reason: "remote_missing", Kind: record.Kind,
	}
	return syncJobResult{tombstone: &tombstone}
}

func (c *Client) commitSyncManifest(
	ctx context.Context, path string, initialLen int,
	result *SyncManifestResult,
) error {
	return withManifestLock(ctx, path, func() error {
		current, warnings, err := readManifest(path)
		if err != nil {
			return err
		}
		result.Warnings = appendUniqueStrings(result.Warnings, warnings...)
		active := scopedActiveUploads(current, c.cfg)
		appendedUploads := make([]ManifestRecord, 0, len(result.Added))
		for _, rec := range result.Added {
			markerKey := manifestMarkerKey(rec)
			if _, exists := active[markerKey]; exists {
				continue
			}
			if hasConcurrentDelete(current, initialLen, rec) {
				continue
			}
			appendedUploads = append(appendedUploads, rec)
			active[markerKey] = rec
		}
		appendedEnriched := make([]ManifestRecord, 0, len(result.Enriched))
		for _, rec := range result.Enriched {
			// Re-check under the lock: a concurrently tombstoned
			// identity is never resurrected, and a concurrently
			// enriched or replaced record is not enriched twice.
			existing, exists := active[manifestMarkerKey(rec)]
			if !exists {
				continue
			}
			if existing.Objects != 0 || existing.TotalBytes != 0 {
				continue
			}
			// A concurrent re-upload of the same identity supersedes the
			// stale inspection; leave it for the next sync pass.
			if !existing.Time.Equal(rec.Time) || existing.Key != rec.Key {
				continue
			}
			appendedEnriched = append(appendedEnriched, rec)
			active[manifestMarkerKey(rec)] = rec
		}
		appendedTombstones := make([]ManifestRecord, 0,
			len(result.Tombstoned))
		for _, rec := range result.Tombstoned {
			if _, exists := active[manifestMarkerKey(rec)]; !exists {
				continue
			}
			if hasConcurrentUpload(current, initialLen, rec) {
				continue
			}
			appendedTombstones = append(appendedTombstones, rec)
			delete(active, manifestMarkerKey(rec))
		}
		all := append(append([]ManifestRecord(nil), appendedUploads...),
			appendedEnriched...)
		all = append(all, appendedTombstones...)
		if err := appendManifestRecordsUnlocked(path, all); err != nil {
			return err
		}
		result.Added = appendedUploads
		result.Enriched = appendedEnriched
		result.Tombstoned = appendedTombstones
		return nil
	})
}

func appendUniqueStrings(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		existing = append(existing, value)
		seen[value] = struct{}{}
	}
	return existing
}

func hasConcurrentUpload(
	records []ManifestRecord, initialLen int, tombstone ManifestRecord,
) bool {
	if initialLen >= len(records) {
		return false
	}
	identity := manifestRecordIdentity(tombstone)
	for _, rec := range records[initialLen:] {
		if rec.Type == "upload" && manifestRecordIdentity(rec) == identity {
			return true
		}
	}
	return false
}

func hasConcurrentDelete(
	records []ManifestRecord, initialLen int, upload ManifestRecord,
) bool {
	if initialLen >= len(records) {
		return false
	}
	identity := manifestRecordIdentity(upload)
	for _, rec := range records[initialLen:] {
		if rec.Type != "delete" {
			continue
		}
		if rec.MarkerKey != "" && rec.Bucket != "" {
			if manifestRecordIdentity(rec) == identity {
				return true
			}
		} else if rec.Key == upload.Key {
			return true
		}
	}
	return false
}

type syncJobResult struct {
	upload    *ManifestRecord
	enriched  *ManifestRecord
	tombstone *ManifestRecord
	failure   *SyncFailure
	warning   string
	state     UploadState
	retained  bool
}

type syncJob struct {
	markerKey string
	upload    *RemoteUpload
	local     *ManifestRecord
	// enrich marks a declared-inventory backfill job: upload and local
	// are both set, and only objects/total_bytes may change.
	enrich bool
}
