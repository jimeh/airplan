package airplan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

	// Key is the page object key; delete tombstones reference the
	// upload by this key alone.
	Key string `json:"key"`

	SourceKey string `json:"source_key,omitempty"`
	URL       string `json:"url,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Title     string `json:"title,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`

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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal manifest record: %w", err)
	}
	line = append(line, '\n')

	lock := flock.New(path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock manifest: %w", err)
	}
	if !locked {
		return fmt.Errorf("lock manifest: %w", ctx.Err())
	}
	defer func() { _ = lock.Unlock() }()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}

	n, err := file.Write(line)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("write manifest: %w", err)
	}
	if n != len(line) {
		_ = file.Close()
		return fmt.Errorf("write manifest: %w", io.ErrShortWrite)
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

			var rec ManifestRecord
			if err := json.Unmarshal(line, &rec); err != nil {
				warnings = append(warnings,
					fmt.Sprintf("skipping malformed manifest line %d", lineNo))
			} else if rec.Type == "upload" {
				if rec.MarkerVersion != MarkerVersion {
					warnings = append(warnings, fmt.Sprintf(
						"skipping manifest line %d with unsupported marker_version %d",
						lineNo, rec.MarkerVersion,
					))
				} else {
					records = append(records, rec)
				}
			} else if rec.Type == "delete" {
				records = append(records, rec)
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
		URL:           res.URL,
		Bucket:        res.Bucket,
		Profile:       c.cfg.Profile,
		Title:         res.Title,
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

// ActiveUploads filters manifest records to upload entries without a
// matching delete tombstone, preserving file order.
func ActiveUploads(records []ManifestRecord) []ManifestRecord {
	deleted := make(map[string]bool)
	for _, r := range records {
		if r.Type == "delete" {
			deleted[r.Key] = true
		}
	}

	var out []ManifestRecord
	for _, r := range records {
		if r.Type == "upload" && r.MarkerVersion == MarkerVersion &&
			!deleted[r.Key] {
			out = append(out, r)
		}
	}
	return out
}
