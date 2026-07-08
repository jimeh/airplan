package airplan

import (
	"errors"
	"time"
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
}

// DefaultManifestPath returns the platform default manifest location
// (SPEC.md §9): $XDG_STATE_HOME/airplan/manifest.jsonl, defaulting to
// ~/.local/state on every platform except Windows, which uses
// %LocalAppData%.
func DefaultManifestPath() (string, error) {
	return "", errors.New("airplan: DefaultManifestPath not implemented")
}

// appendManifestRecord appends one record to the manifest at path,
// creating the file and parent directory as needed. Concurrency rules
// per SPEC.md §9: the file is opened in append mode, the full line —
// trailing newline included — goes out in a single write, and the
// write is wrapped in an advisory file lock (gofrs/flock).
func appendManifestRecord(path string, rec ManifestRecord) error {
	return errors.New("airplan: appendManifestRecord not implemented")
}

// readManifest reads every record from the manifest at path. Torn or
// malformed lines are skipped, each producing a warning; records with
// an unknown type are skipped silently (forward compatibility). A
// missing file is not an error — it returns no records.
//
//nolint:unused // exercised by the manifest packet tests; phase-3 commands consume it.
func readManifest(path string) ([]ManifestRecord, []string, error) {
	return nil, nil, errors.New("airplan: readManifest not implemented")
}

// recordUpload appends an upload record for res, best-effort: manifest
// failures degrade to a warning on the result, never a failed upload
// (SPEC.md §9 — the manifest is convenience, not a source of truth).
func (c *Client) recordUpload(res *Result) {
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
		Type:      "upload",
		Time:      time.Now().UTC().Truncate(time.Second),
		Key:       res.Key,
		SourceKey: res.SourceKey,
		URL:       res.URL,
		Bucket:    res.Bucket,
		Profile:   c.cfg.Profile,
		Title:     res.Title,
		Bytes:     res.Bytes,
	}
	if err := appendManifestRecord(path, rec); err != nil {
		res.Warnings = append(res.Warnings,
			"manifest not recorded: "+err.Error())
	}
}
