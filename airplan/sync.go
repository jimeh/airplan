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
	// Enriched contains records already in local history that this run
	// completed with declared totals. They are appends of existing uploads,
	// never new ones, so they are counted apart from Added (SPEC.md §9).
	Enriched []ManifestRecord `json:"enriched_records"`
	// Tombstoned contains delete records planned or appended by this run.
	Tombstoned []ManifestRecord `json:"tombstone_records"`
	// Protection contains protect/unprotect records planned or appended to
	// mirror remote sentinel state from the listing snapshot (SPEC.md §9).
	Protection []ManifestRecord `json:"protection_records"`
	// Unchanged counts remote markers already active and complete in the
	// local manifest. A record selected for enrichment is reported as
	// enriched or deferred instead, never as unchanged.
	Unchanged int `json:"unchanged"`
	// Deferred counts active records whose declared totals could not be
	// completed this run, because their marker could not be fetched or is not
	// currently usable. Enrichment is metadata completion for uploads already
	// in history, so a deferred one warns and retries on a later run rather
	// than failing the sync (SPEC.md §9).
	Deferred int `json:"deferred"`
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
	}

	jobs := make([]syncJob, 0, len(remote)+len(active))
	for index := range remote {
		upload := remote[index]
		local, known := active[upload.MarkerKey]
		if !known {
			jobs = append(jobs, syncJob{
				kind: syncJobImport, markerKey: upload.MarkerKey,
				upload: &upload,
			})
			continue
		}
		// Protection reconciliation reuses the listing snapshot every job
		// already came from, so it costs no extra requests. Markers absent
		// from the snapshot belong to prune, which has its own
		// confirm-absence rules (SPEC.md §9).
		if upload.Protected != local.Protected {
			result.Protection = append(result.Protection,
				c.syncProtectionRecord(local, upload.Protected, ""))
		}
		// An already-active record still needs its marker inspected when it
		// predates declared totals, so history converges in one pass without
		// re-importing anything (SPEC.md §9). Such a record is reported by its
		// outcome, so it is not also counted as unchanged.
		if manifestRecordNeedsTotals(local) {
			record := local
			jobs = append(jobs, syncJob{
				kind: syncJobEnrich, markerKey: upload.MarkerKey,
				upload: &upload, local: &record,
			})
			continue
		}
		result.Unchanged++
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
				kind: syncJobPrune, markerKey: markerKey, local: &record,
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
				switch job.kind {
				case syncJobImport:
					jobResults[index] = c.syncImport(ctx, *job.upload)
				case syncJobEnrich:
					jobResults[index] = c.syncEnrich(
						ctx, *job.upload, *job.local,
					)
				case syncJobPrune:
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
		if item.protection != nil {
			result.Protection = append(result.Protection, *item.protection)
		}
		if item.tombstone != nil {
			result.Tombstoned = append(result.Tombstoned, *item.tombstone)
		}
		if item.retained {
			result.Retained++
		}
		if item.deferred {
			result.Deferred++
		}
		if item.failure != nil {
			result.Failures = append(result.Failures, *item.failure)
		}
		if item.warning != "" {
			result.Warnings = append(result.Warnings, item.warning)
		}
	}

	if !opts.DryRun && (len(result.Added) > 0 || len(result.Enriched) > 0 ||
		len(result.Tombstoned) > 0 || len(result.Protection) > 0) {
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
	if job.kind == syncJobPrune {
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
	inspection, err := c.inspectListedUpload(ctx, upload)
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
	declared := declaredTotalsFromInspection(inspection)
	record := ManifestRecord{
		Type: "upload", Time: inspection.CreatedAt.UTC(),
		CreatedAt: inspection.CreatedAt.UTC(),
		Key:       inspection.Page.Key, MarkerKey: inspection.MarkerKey,
		URL: inspection.Page.URL, Bucket: c.cfg.Bucket,
		Profile: c.cfg.Profile, Format: inspection.Format,
		Kind:  string(inspection.Kind),
		Title: inspection.Title, Repo: inspection.Repo,
		Bytes:      inspection.Page.Bytes,
		Objects:    declared.objects,
		TotalBytes: declared.bytes,

		MarkerVersion:   inspection.MarkerVersion,
		ProducerVersion: inspection.ProducerVersion,
		RendererVersion: inspection.RendererGeneration,
		RevisionChainID: inspection.RevisionChainID,
		Revision:        inspection.Revision,
		LatestRevision:  inspection.LatestRevision,
	}
	if inspection.Kind == UploadKindDocument {
		record.Slug, _ = pageSlug(path.Base(inspection.Page.Key))
	}
	if inspection.Source != nil {
		record.SourceKey = inspection.Source.Key
	}
	result := syncJobResult{upload: &record}
	if inspection.Protected {
		protection := c.syncProtectionRecord(
			record, true, inspection.ProtectReason,
		)
		if !inspection.ProtectedAt.IsZero() {
			protection.Time = inspection.ProtectedAt.UTC().
				Truncate(time.Second)
		}
		result.protection = &protection
	}
	if len(inspection.Warnings) > 0 {
		result.warning = inspection.Warnings[0]
	} else if inspection.RevisionError != "" {
		result.warning = inspection.RevisionError
	}
	return result
}

// manifestRecordNeedsTotals reports whether a record predates declared totals
// and could still gain them. Only records missing both fields qualify, so one
// pass converges. Only a marker that declares every object's size can supply
// them. Marker v3 and v4 always qualify; v2 qualifies only without a source, while
// v1 and v2-with-source are left alone rather than re-fetched by every run.
func manifestRecordNeedsTotals(rec ManifestRecord) bool {
	if rec.Objects != 0 || rec.TotalBytes != 0 {
		return false
	}
	switch rec.MarkerVersion {
	case 3, 4:
		return true
	case 2:
		return rec.SourceKey == ""
	default:
		return false
	}
}

// syncEnrich fills in declared totals for an active local record by inspecting
// its existing marker (SPEC.md §9). It appends an enriched copy of the record,
// preserving its original time and identity, so append-only history holds and
// latest-wins reduction collapses the pair.
func (c *Client) syncEnrich(
	ctx context.Context, upload RemoteUpload, record ManifestRecord,
) syncJobResult {
	inspection, err := c.inspectListedUpload(ctx, upload)
	if err != nil {
		return deferredEnrichment(upload.MarkerKey, err.Error())
	}
	// An incomplete or invalid marker leaves the fields absent rather than
	// guessing, and leaves the record itself untouched. These states describe
	// an upload already in local history, so they do not join the incomplete
	// and invalid counters, which report on import candidates.
	if inspection.State != UploadComplete {
		return deferredEnrichment(upload.MarkerKey,
			fmt.Sprintf("marker is %s", inspection.State))
	}
	declared := declaredTotalsFromInspection(inspection)
	if declared.objects == 0 {
		return deferredEnrichment(upload.MarkerKey,
			"marker declares no object sizes")
	}
	enriched := record
	enriched.Objects = declared.objects
	enriched.TotalBytes = declared.bytes
	clearDerivedProtection(&enriched)
	return syncJobResult{enriched: &enriched}
}

// deferredEnrichment reports one record whose declared totals could not be
// completed. It is a warning and a counter, never a sync failure: the upload
// is already in local history and only its metadata is missing, so failing the
// run would make an unreadable marker fail every later run too.
func deferredEnrichment(markerKey, reason string) syncJobResult {
	return syncJobResult{
		deferred: true,
		warning: fmt.Sprintf(
			"declared totals deferred for marker %q: %s", markerKey, reason,
		),
	}
}

// syncProtectionRecord builds one protect or unprotect manifest record for a
// scoped identity. Reason and remote timestamps are advisory: reconciliation
// for already-active records is presence-based and uses the sync time.
func (c *Client) syncProtectionRecord(
	record ManifestRecord, protected bool, reason string,
) ManifestRecord {
	recordType := "unprotect"
	if !protected {
		reason = ""
	} else {
		recordType = "protect"
	}
	return ManifestRecord{
		Type: recordType, Time: time.Now().UTC().Truncate(time.Second),
		Key: record.Key, MarkerKey: manifestMarkerKey(record),
		Bucket: c.cfg.Bucket, Profile: c.cfg.Profile,
		ProtectReason: reason,
	}
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
		// An enriched record re-states an upload that must still be active in
		// the manifest as just reread under the lock. A concurrent tombstone
		// removes its identity from that snapshot, so appending it here can
		// never resurrect a deleted upload; a concurrent enrichment leaves the
		// identity no longer missing its totals, so this run adds nothing.
		appendedEnriched := make([]ManifestRecord, 0, len(result.Enriched))
		for _, rec := range result.Enriched {
			markerKey := manifestMarkerKey(rec)
			existing, exists := active[markerKey]
			if !exists || !manifestRecordNeedsTotals(existing) {
				continue
			}
			// A concurrent re-upload or replacement under the same marker key
			// invalidates the inspection snapshot. Metadata-only edits retain
			// these identity fields and are safe to merge into.
			if !existing.Time.Equal(rec.Time) || existing.Key != rec.Key {
				continue
			}
			// Enrichment adds two fields; it must not carry the rest of the
			// pre-lock snapshot forward, or a record updated concurrently
			// would be reverted by latest-wins reduction.
			projected := existing
			projected.Objects = rec.Objects
			projected.TotalBytes = rec.TotalBytes
			appendable := projected
			clearDerivedProtection(&appendable)
			appendedEnriched = append(appendedEnriched, appendable)
			// Keep the reduced projection in memory. Protection fields are not
			// persisted on the upload line, but a protection reconciliation
			// planned in the same sync must still compare against them below.
			active[markerKey] = projected
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
		// Protection records are appended only for identities still active
		// after re-reduction, only while the local projection still
		// disagrees with the planned state, and never over a concurrent
		// protect/unprotect append: a fresher record for the identity means
		// the plan is stale even when it still "disagrees" (SPEC.md §9).
		appendedProtection := make([]ManifestRecord, 0,
			len(result.Protection))
		for _, rec := range result.Protection {
			record, exists := active[manifestMarkerKey(rec)]
			if !exists {
				continue
			}
			if record.Protected == (rec.Type == "protect") {
				continue
			}
			if hasConcurrentProtection(current, initialLen, rec) {
				continue
			}
			appendedProtection = append(appendedProtection, rec)
		}
		all := append([]ManifestRecord(nil), appendedUploads...)
		all = append(all, appendedEnriched...)
		all = append(all, appendedTombstones...)
		all = append(all, appendedProtection...)
		if err := appendManifestRecordsUnlocked(path, all); err != nil {
			return err
		}
		result.Added = appendedUploads
		result.Enriched = appendedEnriched
		result.Tombstoned = appendedTombstones
		result.Protection = appendedProtection
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

func hasConcurrentProtection(
	records []ManifestRecord, initialLen int, planned ManifestRecord,
) bool {
	if initialLen >= len(records) {
		return false
	}
	identity := manifestRecordIdentity(planned)
	for _, rec := range records[initialLen:] {
		if (rec.Type == "protect" || rec.Type == "unprotect") &&
			manifestRecordIdentity(rec) == identity {
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
	upload     *ManifestRecord
	enriched   *ManifestRecord
	protection *ManifestRecord
	tombstone  *ManifestRecord
	failure    *SyncFailure
	warning    string
	state      UploadState
	retained   bool
	deferred   bool
}

// syncJobKind selects what a sync worker does with one marker key.
type syncJobKind int

const (
	// syncJobImport validates a remote marker missing from local history.
	syncJobImport syncJobKind = iota
	// syncJobPrune confirms a local record's marker is really absent.
	syncJobPrune
	// syncJobEnrich fills in declared totals for an active local record that
	// predates them (SPEC.md §9).
	syncJobEnrich
)

type syncJob struct {
	kind      syncJobKind
	markerKey string
	upload    *RemoteUpload
	local     *ManifestRecord
}
