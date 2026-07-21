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
	// Type is "upload" or "delete".
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
	Title  string `json:"title,omitempty"`
	// Repo is the canonical repository URL when known.
	Repo  string `json:"repo,omitempty"`
	Bytes int64  `json:"bytes,omitempty"`
	// Reason is "deleted" or "remote_missing" for modern tombstones.
	Reason string `json:"reason,omitempty"`

	// MarkerVersion is the remote ownership-marker version written for an
	// upload. Delete records omit it.
	MarkerVersion int `json:"marker_version,omitempty"`
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
			} else if rec.Type == "upload" {
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
				} else {
					records = append(records, rec)
				}
			} else if rec.Type == "delete" {
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
	case "upload":
		if rec.URL == "" {
			return errors.New("url is required")
		}
		if rec.Bucket == "" {
			return errors.New("bucket is required")
		}
		if rec.Bytes <= 0 {
			return errors.New("bytes must be positive")
		}
		if rec.MarkerKey != "" {
			expected := markerKeyForPage(rec.Key)
			if expected == "" || rec.MarkerKey != expected {
				return errors.New("marker_key must match the page directory")
			}
		}
		if rec.Format != "" && rec.Format != "md" &&
			rec.Format != "html" && rec.Format != "txt" {
			return fmt.Errorf("unsupported format %q", rec.Format)
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
			expected := markerKeyForPage(rec.Key)
			if expected == "" || rec.MarkerKey != expected {
				return errors.New("marker_key must match the page directory")
			}
		}
		if rec.Reason != "" && rec.Reason != "deleted" &&
			rec.Reason != "remote_missing" {
			return fmt.Errorf("unsupported delete reason %q", rec.Reason)
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

// recordUpload appends an upload record for res, best-effort: manifest
// failures degrade to a warning on the result, never a failed upload
// (SPEC.md §9 — the manifest is convenience, not a source of truth).
func (c *Client) recordUpload(ctx context.Context, res *Result) {
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

	rec := ManifestRecord{
		Type:          "upload",
		Time:          res.CreatedAt,
		Key:           res.Key,
		SourceKey:     res.SourceKey,
		MarkerKey:     res.MarkerKey,
		URL:           res.URL,
		Bucket:        res.Bucket,
		Profile:       c.cfg.Profile,
		Format:        res.Format,
		Title:         res.Title,
		Repo:          res.RepositoryURL,
		Bytes:         res.Bytes,
		MarkerVersion: res.MarkerVersion,
	}
	if err := appendManifestRecord(ctx, path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"manifest not recorded: "+err.Error())
	}
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
// recorded before ownership markers were introduced.
func ManifestUploads(records []ManifestRecord) []ManifestRecord {
	type activeRecord struct {
		record ManifestRecord
		order  int
	}
	active := make(map[string]activeRecord)
	for index, rec := range records {
		switch rec.Type {
		case "upload":
			active[manifestRecordIdentity(rec)] = activeRecord{rec, index}
		case "delete":
			if rec.MarkerKey != "" && rec.Bucket != "" {
				delete(active, manifestRecordIdentity(rec))
				continue
			}
			for identity, candidate := range active {
				if candidate.record.Key == rec.Key {
					delete(active, identity)
				}
			}
		}
	}
	out := make([]activeRecord, 0, len(active))
	for _, record := range active {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	uploads := make([]ManifestRecord, 0, len(out))
	for _, record := range out {
		uploads = append(uploads, record.record)
	}
	return uploads
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
	return dirPrefix + MarkerFilename
}

func manifestRecordIdentity(rec ManifestRecord) string {
	markerKey := manifestMarkerKey(rec)
	if rec.Bucket != "" && markerKey != "" &&
		(rec.Type == "delete" || IsSupportedMarkerVersion(rec.MarkerVersion)) {
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
	}

	candidates := []string{
		rec.Key,
		rec.SourceKey,
	}
	if dirPrefix, err := uploadDirPrefix(rec.Key); err == nil {
		candidates = append(candidates,
			strings.TrimSuffix(dirPrefix, "/"),
			dirPrefix+MarkerFilename,
		)
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
