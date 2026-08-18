package airplan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// ManifestRecord is one line of the local upload manifest (SPEC.md
// §9). Exact JSON field names are part of the spec so conforming
// implementations can share a manifest. Readers ignore unknown fields
// and skip records with an unknown Type.
type ManifestRecord struct {
	// Type is "upload", "upgrade", "link", "delete", "protect", or "unprotect".
	Type string `json:"type"`

	// Time is the record time, RFC 3339 in UTC.
	Time time.Time `json:"time"`

	// Key is the page object key. Modern tombstones pair marker_key with
	// bucket; legacy tombstones identify uploads by Key alone.
	Key string `json:"key"`

	SourceKey string `json:"source_key,omitempty"`
	// MarkerKey is the full ownership marker key for managed records.
	MarkerKey string `json:"marker_key,omitempty"`
	URL       string `json:"url,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Profile   string `json:"profile,omitempty"`
	// Format is the marker-declared input format when known.
	Format string `json:"format,omitempty"`
	// Kind is document or collection for modern marker records.
	Kind string `json:"kind,omitempty"`
	// Slug is present only for document uploads.
	Slug  string `json:"slug,omitempty"`
	Title string `json:"title,omitempty"`
	// Repo is the canonical repository URL when known.
	Repo  string `json:"repo,omitempty"`
	Bytes int64  `json:"bytes,omitempty"`

	// Objects counts the ownership marker plus every object it declares, and
	// TotalBytes sums their declared sizes (SPEC.md §9). Both are absent on
	// records written before airplan recorded them and when the marker cannot
	// declare every counted size: v1, or v2 with a source. Bytes keeps its own
	// meaning, the primary page.
	Objects    int   `json:"objects,omitempty"`
	TotalBytes int64 `json:"total_bytes,omitempty"`
	// Reason is "deleted" or "remote_missing" for modern tombstones.
	Reason string `json:"reason,omitempty"`

	// MarkerVersion is the remote ownership-marker version written for an
	// upload. Delete records omit it.
	MarkerVersion int `json:"marker_version,omitempty"`
	// CreatedAt remains the upload's original creation time across upgrade
	// events, while Time records when the event was appended.
	CreatedAt       time.Time `json:"created_at,omitzero"`
	ProducerVersion string    `json:"producer_version,omitempty"`
	RendererVersion int       `json:"renderer_version,omitempty"`
	RevisionChainID string    `json:"revision_chain_id,omitempty"`
	Revision        int       `json:"revision,omitempty"`
	LatestRevision  int       `json:"latest_revision,omitempty"`

	// ProtectReason is the advisory purge-protection reason: written on
	// protect records and projected onto reduced upload records. It is a
	// distinct field because Reason is already a delete-tombstone enum.
	ProtectReason string `json:"protect_reason,omitempty"`

	// Protected and ProtectedAt are reduction-derived projections of the
	// local protection state (SPEC.md §9). Writers never emit them: every
	// writer builds fresh records, and ManifestUploads recomputes both on
	// reduced upload records.
	Protected   bool      `json:"protected,omitempty"`
	ProtectedAt time.Time `json:"protected_at,omitzero"`
}

// DefaultManifestPath returns the platform default manifest location
// (SPEC.md §9): $XDG_STATE_HOME/airplan/manifest.jsonl, defaulting to
// ~/.local/state on every platform except Windows, which uses
// %LocalAppData%.
func DefaultManifestPath() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "airplan", "manifest.jsonl"), nil
	}

	var base string
	if runtime.GOOS == "windows" {
		base = os.Getenv("LocalAppData")
		if base == "" {
			var err error
			base, err = os.UserConfigDir()
			if err != nil {
				return "", err
			}
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(base, "airplan", "manifest.jsonl"), nil
}

// appendManifestRecord appends one record to the manifest at path,
// creating the file and parent directory as needed. Concurrency rules
// per SPEC.md §9: the file is opened in append mode, the full line —
// trailing newline included — goes out in a single write, and the
// write is wrapped in an advisory file lock (gofrs/flock).
func appendManifestRecord(
	ctx context.Context, path string, rec ManifestRecord,
) error {
	return withManifestLock(ctx, path, func() error {
		return appendManifestRecordsUnlocked(path, []ManifestRecord{rec})
	})
}

func withManifestLock(ctx context.Context, path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	lock := flock.New(path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock manifest: %w", err)
	}
	if !locked {
		return fmt.Errorf("lock manifest: %w", ctx.Err())
	}
	defer func() { _ = lock.Unlock() }()
	return fn()
}

func appendManifestRecordsUnlocked(
	path string, records []ManifestRecord,
) error {
	if len(records) == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	for _, rec := range records {
		line, err := json.Marshal(rec)
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("marshal manifest record: %w", err)
		}
		line = append(line, '\n')
		n, err := file.Write(line)
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("write manifest: %w", err)
		}
		if n != len(line) {
			_ = file.Close()
			return fmt.Errorf("write manifest: %w", io.ErrShortWrite)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	return nil
}

// readManifest reads every record from the manifest at path. Torn or
// malformed lines are skipped, each producing a warning; records with
// an unknown type are skipped silently (forward compatibility). A
// missing file is not an error — it returns no records.
func readManifest(path string) ([]ManifestRecord, []string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open manifest: %w", err)
	}
	defer func() { _ = file.Close() }()

	const maxManifestLine = 10 * 1024 * 1024

	var (
		records  []ManifestRecord
		warnings []string
		lineNo   int
	)
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, oversized, readErr := readManifestLine(reader, maxManifestLine)
		if len(line) == 0 && !oversized && readErr == nil {
			return records, warnings,
				errors.New("read manifest: empty read without EOF")
		}
		if len(line) == 0 && !oversized && readErr == io.EOF {
			break
		}
		lineNo++
		if oversized {
			warnings = append(warnings,
				fmt.Sprintf("skipping oversized manifest line %d", lineNo))
		} else {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(bytes.TrimSpace(line)) == 0 {
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					return records, warnings,
						fmt.Errorf("read manifest: %w", readErr)
				}
				continue
			}

			var rec ManifestRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				warnings = append(warnings,
					fmt.Sprintf("skipping malformed manifest line %d", lineNo))
			} else if rec.Type == "upload" || rec.Type == "upgrade" || rec.Type == "link" {
				normalizeManifestRecord(&rec)
				if rec.MarkerVersion != 0 &&
					!IsSupportedMarkerVersion(rec.MarkerVersion) {
					warnings = append(warnings, fmt.Sprintf(
						"skipping manifest line %d with unsupported marker_version %d",
						lineNo, rec.MarkerVersion,
					))
				} else if err := validateManifestRecord(rec); err != nil {
					warnings = append(warnings, fmt.Sprintf(
						"skipping invalid manifest line %d: %s", lineNo, err,
					))
				} else if err := validateManifestInventoryEncoding(line, rec); err != nil {
					warnings = append(warnings, fmt.Sprintf(
						"skipping invalid manifest line %d: %s", lineNo, err,
					))
				} else {
					records = append(records, rec)
				}
			} else if rec.Type == "delete" || rec.Type == "protect" ||
				rec.Type == "unprotect" {
				normalizeManifestRecord(&rec)
				if err := validateManifestRecord(rec); err != nil {
					warnings = append(warnings, fmt.Sprintf(
						"skipping invalid manifest line %d: %s", lineNo, err,
					))
				} else {
					records = append(records, rec)
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return records, warnings, fmt.Errorf("read manifest: %w", readErr)
		}
	}

	return records, warnings, nil
}

// validateManifestInventoryEncoding distinguishes an absent optional pair
// from explicit JSON zeroes, which ordinary integer unmarshalling cannot do.
// The public response schema and manifest contract require positive values
// whenever either field is present.
func validateManifestInventoryEncoding(line []byte, rec ManifestRecord) error {
	var fields struct {
		Objects    json.RawMessage `json:"objects"`
		TotalBytes json.RawMessage `json:"total_bytes"`
	}
	if err := json.Unmarshal(line, &fields); err != nil {
		return err
	}
	objectsPresent := len(fields.Objects) != 0
	totalPresent := len(fields.TotalBytes) != 0
	if objectsPresent != totalPresent {
		return errors.New("objects and total_bytes must be set together")
	}
	if objectsPresent && (rec.Objects <= 0 || rec.TotalBytes <= 0) {
		return errors.New("objects and total_bytes must be positive when present")
	}
	return nil
}

func normalizeManifestRecord(rec *ManifestRecord) {
	if rec == nil {
		return
	}
	if rec.Kind == "" && path.Base(rec.MarkerKey) == CollectionMarkerFilename {
		rec.Kind = string(UploadKindCollection)
	}
	if rec.Type != "upload" && rec.Type != "upgrade" && rec.Type != "link" {
		return
	}
	if rec.Kind == "" {
		rec.Kind = string(UploadKindDocument)
	}
	if rec.Kind == string(UploadKindDocument) && rec.Slug == "" {
		if slug, ok := pageSlug(path.Base(rec.Key)); ok {
			rec.Slug = slug
		}
	}
}

func validateManifestRecord(rec ManifestRecord) error {
	if rec.Time.IsZero() {
		return errors.New("time is required")
	}
	if _, offset := rec.Time.Zone(); offset != 0 {
		return errors.New("time must be UTC")
	}
	if rec.Key == "" {
		return errors.New("key is required")
	}

	switch rec.Type {
	case "upload", "upgrade", "link":
		if rec.URL == "" {
			return errors.New("url is required")
		}
		if rec.Bucket == "" {
			return errors.New("bucket is required")
		}
		if rec.Bytes <= 0 {
			return errors.New("bytes must be positive")
		}
		if rec.Type == "upgrade" || rec.Type == "link" {
			if rec.MarkerVersion < 4 || !IsSupportedMarkerVersion(rec.MarkerVersion) {
				return fmt.Errorf("%s events require marker version 4 or newer", rec.Type)
			}
			if rec.CreatedAt.IsZero() {
				return fmt.Errorf("%s events require created_at", rec.Type)
			}
		}
		if !rec.CreatedAt.IsZero() {
			if _, offset := rec.CreatedAt.Zone(); offset != 0 {
				return errors.New("created_at must be UTC")
			}
		}
		if rec.RendererVersion < 0 {
			return errors.New("renderer_version must not be negative")
		}
		if rec.RevisionChainID != "" {
			if !isRandomDir(rec.RevisionChainID) || rec.Revision <= 0 ||
				(rec.LatestRevision != 0 && rec.LatestRevision < rec.Revision) {
				return errors.New("revision projection fields are invalid")
			}
		} else if rec.Revision != 0 || rec.LatestRevision != 0 {
			return errors.New("revision projection requires revision_chain_id")
		}
		if rec.Type == "link" && (rec.RevisionChainID == "" ||
			rec.Revision <= 0 || rec.LatestRevision < rec.Revision) {
			return errors.New("link events require complete revision projection fields")
		}
		// Both totals are optional, but they describe one measurement: a
		// negative or half-present pair is corrupt rather than absent, and
		// would otherwise render inconsistently forever, since backfill only
		// completes records missing both.
		if rec.Objects < 0 || rec.TotalBytes < 0 {
			return errors.New("objects and total_bytes must not be negative")
		}
		if (rec.Objects == 0) != (rec.TotalBytes == 0) {
			return errors.New("objects and total_bytes must be set together")
		}
		if rec.MarkerKey != "" {
			expected := markerKeyForManifestRecord(rec)
			if expected == "" || rec.MarkerKey != expected {
				return errors.New("marker_key must match the page directory")
			}
		}
		if rec.Format != "" && rec.Format != "md" &&
			rec.Format != "html" && rec.Format != "txt" {
			return fmt.Errorf("unsupported format %q", rec.Format)
		}
		if rec.Kind != "" && rec.Kind != string(UploadKindDocument) &&
			rec.Kind != string(UploadKindCollection) {
			return fmt.Errorf("unsupported kind %q", rec.Kind)
		}
		if rec.Kind == string(UploadKindCollection) && rec.Slug != "" {
			return errors.New("collection records must not declare slug")
		}
		if rec.Repo != "" {
			canonical, err := NormalizeRepositoryURL(rec.Repo)
			if err != nil || canonical != rec.Repo {
				return errors.New("repo must be a canonical repository URL")
			}
		}
	case "delete":
		if (rec.MarkerKey == "") != (rec.Bucket == "") {
			return errors.New("marker_key and bucket must be provided together")
		}
		if rec.MarkerKey != "" {
			expected := markerKeyForManifestRecord(rec)
			if expected == "" || rec.MarkerKey != expected {
				return errors.New("marker_key must match the page directory")
			}
		}
		if rec.Reason != "" && rec.Reason != "deleted" &&
			rec.Reason != "remote_missing" {
			return fmt.Errorf("unsupported delete reason %q", rec.Reason)
		}
		if rec.RevisionChainID != "" {
			if !isRandomDir(rec.RevisionChainID) || rec.Revision <= 0 ||
				rec.LatestRevision < 0 {
				return errors.New("delete revision projection fields are invalid")
			}
		} else if rec.Revision != 0 || rec.LatestRevision != 0 {
			return errors.New("delete revision projection requires revision_chain_id")
		}
	case "protect", "unprotect":
		// Protection applies to marker-managed identities only, so both
		// record types require the full (bucket, marker_key) identity.
		if rec.MarkerKey == "" || rec.Bucket == "" {
			return errors.New("marker_key and bucket are required")
		}
		expected := markerKeyForManifestRecord(rec)
		if expected == "" || rec.MarkerKey != expected {
			return errors.New("marker_key must match the page directory")
		}
		if rec.Type == "unprotect" && rec.ProtectReason != "" {
			return errors.New("unprotect records must not declare protect_reason")
		}
		if err := validateProtectReason(rec.ProtectReason); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported type %q", rec.Type)
	}

	return nil
}

// readManifestLine reads one logical line while retaining at most max
// bytes. Oversized lines are fully discarded so parsing can resume at
// the next record.
func readManifestLine(r *bufio.Reader, max int) ([]byte, bool, error) {
	line := make([]byte, 0, min(max, 64*1024))
	oversized := false
	for {
		fragment, err := r.ReadSlice('\n')
		if !oversized && len(line)+len(fragment) <= max {
			line = append(line, fragment...)
		} else {
			oversized = true
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, oversized, err
	}
}

// declaredTotals carries an upload's marker-declared object count and byte
// total from the writer that built the marker to the manifest record. A zero
// value records neither field when the marker cannot declare the full
// inventory, as with v1 and v2-with-source markers.
type declaredTotals struct {
	objects int
	bytes   int64
}

// markerDeclaredTotals derives the totals for a marker about to be written.
func markerDeclaredTotals(
	marker UploadMarker, markerBody []byte,
) declaredTotals {
	objects, total, ok := MarkerDeclaredTotals(marker, len(markerBody))
	if !ok {
		return declaredTotals{}
	}
	return declaredTotals{objects: objects, bytes: total}
}

func completeManifestProjection(
	cfg *Config, eventType string, eventTime time.Time, res *Result,
	declared declaredTotals, producer string, renderer int,
) ManifestRecord {
	return ManifestRecord{
		Type: eventType, Time: eventTime, CreatedAt: res.CreatedAt,
		Key: res.Key, SourceKey: res.SourceKey, MarkerKey: res.MarkerKey,
		URL: res.URL, Bucket: res.Bucket, Profile: cfg.Profile,
		Format: res.Format, Kind: res.Kind, Slug: res.Slug,
		Title: res.Title, Repo: res.RepositoryURL, Bytes: res.Bytes,
		Objects: declared.objects, TotalBytes: declared.bytes,
		MarkerVersion: res.MarkerVersion, ProducerVersion: producer,
		RendererVersion: renderer, RevisionChainID: res.RevisionChainID,
		Revision: res.Revision, LatestRevision: res.LatestRevision,
	}
}

// recordUpload appends an upload record for res, best-effort: manifest
// failures degrade to a warning on the result, never a failed upload
// (SPEC.md §9 — the manifest is convenience, not a source of truth).
func (c *Client) recordUpload(
	ctx context.Context, res *Result, declared declaredTotals,
) {
	if c.cfg.DisableManifest {
		return
	}

	path := c.cfg.ManifestPath
	if path == "" {
		var err error
		path, err = DefaultManifestPath()
		if err != nil {
			res.Warnings = append(res.Warnings,
				"manifest not recorded: "+err.Error())
			return
		}
	}

	renderer := 0
	if res.Kind == string(UploadKindCollection) ||
		(res.Kind == string(UploadKindDocument) && res.Format != "html") {
		renderer = RendererGeneration
	}
	rec := completeManifestProjection(c.cfg, "upload", res.CreatedAt, res,
		declared, producerVersion(c.cfg.ProducerVersion), renderer)
	if err := appendManifestRecord(ctx, path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"manifest not recorded: "+err.Error())
	}
}

func markerKeyForManifestRecord(rec ManifestRecord) string {
	dirPrefix, err := uploadDirPrefix(rec.Key)
	if err != nil {
		return ""
	}
	if rec.Kind == string(UploadKindCollection) {
		return dirPrefix + CollectionMarkerFilename
	}
	return dirPrefix + MarkerFilename
}

// ManifestRecordDir returns a record's 26-character random upload directory
// without key_prefix (SPEC.md §9), derived from its ownership marker key or,
// for records that omit one, from its page key. It returns "" when neither
// key contains a random directory segment, as in pre-marker legacy history.
func ManifestRecordDir(rec ManifestRecord) string {
	return uploadIDFromMarkerKey(manifestMarkerKey(rec))
}

// ManifestRecordKind returns a record's upload kind (SPEC.md §9). Conforming
// legacy history omits both kind and marker_version and is inferred as a
// document; managed records with a missing kind remain unknown.
func ManifestRecordKind(rec ManifestRecord) UploadKind {
	if rec.Kind != "" {
		return UploadKind(rec.Kind)
	}
	if rec.MarkerVersion == 0 {
		return UploadKindDocument
	}
	return ""
}

func manifestRecordHasAnnouncedRevision(rec ManifestRecord) bool {
	return rec.RevisionChainID != "" && rec.Revision > 0 &&
		rec.LatestRevision >= rec.Revision
}

type manifestRevisionChainKey struct {
	profile string
	bucket  string
	chainID string
}

func manifestRevisionChainIdentity(rec ManifestRecord) manifestRevisionChainKey {
	return manifestRevisionChainKey{
		profile: rec.Profile,
		bucket:  rec.Bucket,
		chainID: rec.RevisionChainID,
	}
}

func manifestDeletedRevisions(
	records []ManifestRecord,
) map[manifestRevisionChainKey]map[int]struct{} {
	deletedRevisions := make(map[manifestRevisionChainKey]map[int]struct{})
	for _, rec := range records {
		if rec.Type != "delete" || rec.RevisionChainID == "" || rec.Revision <= 0 {
			continue
		}
		chain := manifestRevisionChainIdentity(rec)
		deleted := deletedRevisions[chain]
		if deleted == nil {
			deleted = make(map[int]struct{})
			deletedRevisions[chain] = deleted
		}
		deleted[rec.Revision] = struct{}{}
	}
	return deletedRevisions
}

func manifestRevisionWasDeleted(
	deletedRevisions map[manifestRevisionChainKey]map[int]struct{},
	rec ManifestRecord,
) bool {
	if !manifestRecordHasAnnouncedRevision(rec) {
		return false
	}
	return manifestRevisionIdentityWasDeleted(
		deletedRevisions, rec.Profile, rec.Bucket,
		rec.RevisionChainID, rec.Revision,
	)
}

func manifestRevisionIdentityWasDeleted(
	deletedRevisions map[manifestRevisionChainKey]map[int]struct{},
	profile, bucket, chainID string, revision int,
) bool {
	if chainID == "" || revision <= 0 {
		return false
	}
	_, deleted := deletedRevisions[manifestRevisionChainKey{
		profile: profile, bucket: bucket, chainID: chainID,
	}][revision]
	return deleted
}

// ManifestRecordSlug returns a record's document slug (SPEC.md §9): the
// recorded value, or the slug derived from its page key for valid older
// records that omit one. Collections have no slug and return "".
func ManifestRecordSlug(rec ManifestRecord) string {
	if rec.Kind == string(UploadKindCollection) {
		return ""
	}
	if rec.Slug != "" {
		return rec.Slug
	}
	slug, _ := pageSlug(path.Base(rec.Key))
	return slug
}

// ReadManifest loads the manifest at path ("" = platform default),
// returning records in file order plus torn-line warnings (SPEC.md
// §9). A missing manifest yields no records and no error.
func ReadManifest(path string) ([]ManifestRecord, []string, error) {
	if path == "" {
		var err error
		path, err = DefaultManifestPath()
		if err != nil {
			return nil, nil, err
		}
	}
	return readManifest(path)
}

// ManifestUploads reduces manifest history chronologically so the latest
// upload or delete event wins for each identity. It includes legacy uploads
// recorded before ownership markers were introduced. Reduced upload records
// carry the derived Protected, ProtectedAt, and ProtectReason projection of
// the latest protect/unprotect event for their identity (SPEC.md §9).
func ManifestUploads(records []ManifestRecord) []ManifestRecord {
	type activeRecord struct {
		record ManifestRecord
		order  int
	}
	type protectionState struct {
		at     time.Time
		reason string
	}
	active := make(map[string]activeRecord)
	protection := make(map[string]protectionState)
	deletedRevisions := make(map[manifestRevisionChainKey]map[int]struct{})
	for index, rec := range records {
		switch rec.Type {
		case "upload", "upgrade", "link":
			if deleted := deletedRevisions[manifestRevisionChainIdentity(rec)]; rec.Revision > 0 && deleted != nil {
				if _, ok := deleted[rec.Revision]; ok {
					continue
				}
			}
			// Derived fields are recomputed below; stale values on a read
			// line never survive reduction.
			clearDerivedProtection(&rec)
			// Raw upgrade records retain the event time in ReadManifest. The
			// active upload projection keeps the original creation time so
			// upgrades do not change list ordering, age filters, or purge age.
			if (rec.Type == "upgrade" || rec.Type == "link") && !rec.CreatedAt.IsZero() {
				rec.Time = rec.CreatedAt
			}
			identity := manifestRecordIdentity(rec)
			order := index
			if rec.Type == "upgrade" || rec.Type == "link" {
				if previous, ok := active[identity]; ok {
					order = previous.order
				}
			}
			active[identity] = activeRecord{record: rec, order: order}
		case "delete":
			if rec.RevisionChainID != "" && rec.Revision > 0 {
				chain := manifestRevisionChainIdentity(rec)
				deleted := deletedRevisions[chain]
				if deleted == nil {
					deleted = make(map[int]struct{})
					deletedRevisions[chain] = deleted
				}
				deleted[rec.Revision] = struct{}{}
			}
			if rec.MarkerKey != "" && rec.Bucket != "" {
				identity := manifestRecordIdentity(rec)
				delete(active, identity)
				delete(protection, identity)
				continue
			}
			for identity, candidate := range active {
				if candidate.record.Key == rec.Key {
					delete(active, identity)
					delete(protection, identity)
				}
			}
		case "protect":
			protection[manifestRecordIdentity(rec)] = protectionState{
				at: rec.Time, reason: rec.ProtectReason,
			}
		case "unprotect":
			delete(protection, manifestRecordIdentity(rec))
		}
	}
	out := make([]activeRecord, 0, len(active))
	for _, record := range active {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	chainLatest := make(map[manifestRevisionChainKey]int)
	for _, record := range out {
		if rec := record.record; manifestRecordHasAnnouncedRevision(rec) {
			chain := manifestRevisionChainIdentity(rec)
			latest := rec.LatestRevision
			if latest > chainLatest[chain] {
				chainLatest[chain] = latest
			}
		}
	}
	for chain, latest := range chainLatest {
		for latest > 0 {
			if _, deleted := deletedRevisions[chain][latest]; !deleted {
				break
			}
			latest--
		}
		chainLatest[chain] = latest
	}
	uploads := make([]ManifestRecord, 0, len(out))
	for _, record := range out {
		rec := record.record
		if manifestRecordHasAnnouncedRevision(rec) {
			rec.LatestRevision = chainLatest[manifestRevisionChainIdentity(rec)]
		}
		// An upload never clears protection: a sync re-import of a still
		// protected directory must not silently drop it. Protection state
		// without an active upload is simply not surfaced.
		if state, ok := protection[manifestRecordIdentity(rec)]; ok {
			rec.Protected = true
			rec.ProtectedAt = state.at
			rec.ProtectReason = state.reason
		}
		uploads = append(uploads, rec)
	}
	return uploads
}

// clearDerivedProtection keeps protection state out of raw upload records.
// The projection is rebuilt exclusively from protect/unprotect events during
// manifest reduction (SPEC.md §9).
func clearDerivedProtection(rec *ManifestRecord) {
	if rec == nil {
		return
	}
	rec.Protected = false
	rec.ProtectedAt = time.Time{}
	rec.ProtectReason = ""
}

// sortManifestUploads orders reduced manifest uploads by record time
// ascending, tie-breaking on ownership marker key. This mirrors remote listing
// order (SPEC.md §9) so both views describe history the same way, including
// after sync appends imported records out of chronological order.
//
// ManifestUploads deliberately keeps its positional reduction order: callers
// such as delete reconciliation and profile inference depend on it. Listing
// paths sort the records they are about to return instead.
func sortManifestUploads(records []ManifestRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Time.Equal(records[j].Time) {
			return manifestMarkerKey(records[i]) <
				manifestMarkerKey(records[j])
		}
		return records[i].Time.Before(records[j].Time)
	})
}

// ActiveUploads returns marker-managed manifest uploads that have no matching
// delete tombstone. Legacy records remain available through ManifestUploads.
func ActiveUploads(records []ManifestRecord) []ManifestRecord {
	var out []ManifestRecord
	for _, r := range ManifestUploads(records) {
		if IsSupportedMarkerVersion(r.MarkerVersion) {
			out = append(out, r)
		}
	}
	return out
}

func manifestMarkerKey(rec ManifestRecord) string {
	if rec.MarkerKey != "" {
		return rec.MarkerKey
	}
	dirPrefix, err := uploadDirPrefix(rec.Key)
	if err != nil {
		return ""
	}
	if rec.Kind == string(UploadKindCollection) {
		return dirPrefix + CollectionMarkerFilename
	}
	return dirPrefix + MarkerFilename
}

func manifestRecordIdentity(rec ManifestRecord) string {
	markerKey := manifestMarkerKey(rec)
	if rec.Bucket != "" && markerKey != "" &&
		(rec.Type != "upload" || IsSupportedMarkerVersion(rec.MarkerVersion)) {
		return "managed\x00" + rec.Bucket + "\x00" + markerKey
	}
	return "legacy\x00" + rec.Key
}

// MatchingManifestUploads returns active manifest history entries naming
// target as their URL, directory, marker, page, or source object. URL query
// strings and fragments are ignored.
func MatchingManifestUploads(
	records []ManifestRecord, target string,
) []ManifestRecord {
	var matches []ManifestRecord
	for _, rec := range ManifestUploads(records) {
		if manifestUploadMatchesTarget(rec, target) {
			matches = append(matches, rec)
		}
	}
	return matches
}

func manifestUploadMatchesTarget(rec ManifestRecord, target string) bool {
	targetKey := strings.Trim(target, "/")
	isURL := strings.Contains(target, "://")
	if isURL {
		targetURL, err := url.Parse(target)
		if err != nil || !isHTTPURL(targetURL) {
			return false
		}
		recordURL, err := url.Parse(rec.URL)
		if err != nil || !isHTTPURL(recordURL) ||
			!strings.EqualFold(targetURL.Host, recordURL.Host) {
			return false
		}
		targetKey = strings.Trim(targetURL.Path, "/")
		if targetURL.Path == recordURL.Path {
			return true
		}
		if rec.Kind == string(UploadKindCollection) &&
			path.Dir(targetURL.Path) == path.Dir(recordURL.Path) &&
			path.Base(targetURL.Path) != "." &&
			path.Base(targetURL.Path) != ".." {
			return true
		}
	}

	candidates := []string{
		rec.Key,
		rec.SourceKey,
	}
	if dirPrefix, err := uploadDirPrefix(rec.Key); err == nil {
		candidates = append(candidates,
			strings.TrimSuffix(dirPrefix, "/"),
			manifestMarkerKey(rec),
		)
		if rec.Kind == string(UploadKindCollection) {
			rel := strings.TrimPrefix(targetKey, strings.TrimSuffix(dirPrefix, "/")+"/")
			if rel != targetKey && rel != "" && !strings.Contains(rel, "/") {
				return true
			}
		}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if targetKey == candidate ||
			(isURL && strings.HasSuffix(targetKey, "/"+candidate)) {
			return true
		}
	}
	return false
}

func isHTTPURL(value *url.URL) bool {
	return (value.Scheme == "http" || value.Scheme == "https") &&
		value.Host != ""
}
