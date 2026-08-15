package airplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// VersionsFilename is the mutable, replicated revision-chain index.
	VersionsFilename = ".airplan-versions.json"
	// DiffFilename is the immutable adjacent unified diff in revisions > 1.
	DiffFilename = ".airplan-changes.diff"
	// MaxVersionsMetadataSize bounds every replicated chain index.
	MaxVersionsMetadataSize = 64 * 1024
	// MaxDiffSize keeps pathological changes from publishing unbounded diffs.
	MaxDiffSize     = 32 * 1024 * 1024
	diffContentType = "text/plain; charset=utf-8"
)

// VersionsRevision describes one assigned document revision. Deleted entries
// are tombstones and deliberately omit capability URLs.
type VersionsRevision struct {
	Number    int       `json:"number"`
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	DiffURL   string    `json:"diff_url,omitempty"`
	Deleted   bool      `json:"deleted,omitempty"`
	DeletedAt time.Time `json:"deleted_at,omitzero"`
}

// VersionsMetadata is the separately versioned replicated chain index.
type VersionsMetadata struct {
	Schema               string             `json:"schema"`
	Version              int                `json:"version"`
	ChainID              string             `json:"chain_id"`
	CurrentRevision      int                `json:"current_revision"`
	LatestRevision       int                `json:"latest_revision"`
	LastAssignedRevision int                `json:"last_assigned_revision"`
	Revisions            []VersionsRevision `json:"revisions"`
}

// EncodeVersionsMetadata validates and deterministically encodes metadata.
// cfg and containingPageKey bind every live URL to this configured service and
// require current_revision to identify the containing upload.
func EncodeVersionsMetadata(
	metadata VersionsMetadata, cfg *Config, containingPageKey string,
) ([]byte, error) {
	return encodeVersionsMetadata(metadata, cfg, containingPageKey, false)
}

func encodeVersionsMetadata(
	metadata VersionsMetadata, cfg *Config, containingPageKey string,
	allowDeletedCurrent bool,
) ([]byte, error) {
	if err := validateVersionsMetadata(
		&metadata, cfg, containingPageKey, allowDeletedCurrent,
	); err != nil {
		return nil, err
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("airplan: encode versions metadata: %w", err)
	}
	if len(body) > MaxVersionsMetadataSize {
		return nil, fmt.Errorf("airplan: versions metadata is %d bytes; maximum is %d",
			len(body), MaxVersionsMetadataSize)
	}
	return body, nil
}

// DecodeVersionsMetadata strictly decodes and validates a replicated index.
func DecodeVersionsMetadata(
	data []byte, cfg *Config, containingPageKey string,
) (*VersionsMetadata, error) {
	return decodeVersionsMetadata(data, cfg, containingPageKey, false)
}

func decodeVersionsMetadata(
	data []byte, cfg *Config, containingPageKey string,
	allowDeletedCurrent bool,
) (*VersionsMetadata, error) {
	if len(data) > MaxVersionsMetadataSize {
		return nil, fmt.Errorf("airplan: versions metadata exceeds %d bytes",
			MaxVersionsMetadataSize)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("airplan: versions metadata is not valid UTF-8")
	}
	if err := validateJSONNames(data); err != nil {
		return nil, fmt.Errorf("airplan: versions metadata is malformed: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var metadata VersionsMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("airplan: versions metadata is malformed: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("airplan: versions metadata has trailing data")
	}
	if err := validateVersionsMetadata(
		&metadata, cfg, containingPageKey, allowDeletedCurrent,
	); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func validateVersionsMetadata(
	metadata *VersionsMetadata, cfg *Config, containingPageKey string,
	allowDeletedCurrent bool,
) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("airplan: invalid versions metadata: "+format, args...)
	}
	if metadata.Schema != "airplan-versions" || metadata.Version != 1 {
		return invalid("unsupported schema or version")
	}
	if !isRandomDir(metadata.ChainID) {
		return invalid("chain_id is not a lowercase 128-bit base32 value")
	}
	if len(metadata.Revisions) < 2 {
		return invalid("at least two assigned revisions are required")
	}
	seenCurrent := false
	latest := 0
	previous := 0
	for index := range metadata.Revisions {
		revision := &metadata.Revisions[index]
		if index == 0 && revision.Number != 1 {
			return invalid("the first assigned revision must be 1")
		}
		if revision.Number != index+1 {
			return invalid("revision numbers must include every assigned number in order")
		}
		previous = revision.Number
		if revision.Number > metadata.LastAssignedRevision {
			return invalid("revision %d exceeds last_assigned_revision", revision.Number)
		}
		if revision.Deleted {
			if revision.URL != "" || revision.DiffURL != "" ||
				revision.CreatedAt != (time.Time{}) || revision.DeletedAt.IsZero() ||
				!isUTCSecond(revision.DeletedAt) {
				return invalid("revision %d tombstone fields are invalid", revision.Number)
			}
			if revision.Number == metadata.CurrentRevision {
				if !allowDeletedCurrent {
					return invalid("current_revision does not name a live entry")
				}
				seenCurrent = true
			}
			continue
		}
		if revision.URL == "" || revision.CreatedAt.IsZero() ||
			!isUTCSecond(revision.CreatedAt) {
			return invalid("revision %d live fields are invalid", revision.Number)
		}
		pageKey, err := validateVersionsURL(cfg, revision.URL, false)
		if err != nil {
			return invalid("revision %d URL: %v", revision.Number, err)
		}
		if revision.Number == 1 {
			if revision.DiffURL != "" {
				return invalid("revision 1 must not declare diff_url")
			}
		} else {
			diffKey, err := validateVersionsURL(cfg, revision.DiffURL, true)
			if err != nil {
				return invalid("revision %d diff URL: %v", revision.Number, err)
			}
			if path.Dir(diffKey) != path.Dir(pageKey) {
				return invalid("revision %d diff URL is outside its upload directory",
					revision.Number)
			}
		}
		if revision.Number == metadata.CurrentRevision {
			seenCurrent = true
			if containingPageKey != "" && pageKey != containingPageKey {
				return invalid("current_revision does not identify the containing upload")
			}
		}
		latest = revision.Number
	}
	if !seenCurrent {
		return invalid("current_revision does not name a live entry")
	}
	if latest == 0 || metadata.LatestRevision != latest {
		return invalid("latest_revision does not name the greatest live entry")
	}
	if metadata.LastAssignedRevision != previous {
		return invalid("last_assigned_revision does not equal the assigned high-water mark")
	}
	return nil
}

func validateVersionsURL(cfg *Config, raw string, diff bool) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && !configuredHTTPBase(cfg, u)) || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("must be an absolute canonical HTTPS URL")
	}
	key, err := KeyFromURLOrKey(cfg, raw)
	if err != nil {
		return "", err
	}
	canonical, _, err := PublicURL(cfg, key)
	if err != nil {
		return "", err
	}
	if raw != canonical {
		return "", errors.New("must use the canonical public URL encoding")
	}
	dirPrefix, err := uploadDirPrefixForKeyPrefix(key, cfg.KeyPrefix)
	if err != nil || path.Dir(key)+"/" != dirPrefix {
		return "", errors.New("must identify a direct child of an Airplan upload directory")
	}
	if diff {
		if path.Base(key) != DiffFilename {
			return "", fmt.Errorf("must end in %q", DiffFilename)
		}
	} else if !strings.HasSuffix(path.Base(key), ".html") {
		return "", errors.New("must identify an Airplan HTML page")
	}
	return key, nil
}

func configuredHTTPBase(cfg *Config, candidate *url.URL) bool {
	if cfg == nil || candidate == nil || candidate.Scheme != "http" {
		return false
	}
	for _, raw := range []string{cfg.PublicBaseURL, cfg.Endpoint} {
		base, err := url.Parse(raw)
		if err == nil && base.Scheme == "http" &&
			strings.EqualFold(base.Host, candidate.Host) {
			return true
		}
	}
	return false
}

func isUTCSecond(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0 && value.Nanosecond() == 0
}
