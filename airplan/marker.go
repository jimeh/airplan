package airplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// MarkerFilename is the ownership marker basename (SPEC.md §5).
const MarkerFilename = ".airplan.json"

// MarkerSchema identifies airplan upload markers (SPEC.md §5).
const MarkerSchema = "airplan-upload"

// MarkerVersion is the only marker version defined by this spec.
const MarkerVersion = 1

// MaxMarkerSize is the maximum accepted marker body size.
const MaxMarkerSize = 64 * 1024

// MarkerErrorCode is a stable show --json validation error code.
type MarkerErrorCode string

// Stable invalid-marker error codes from SPEC.md §9.
const (
	MarkerErrorOversized          MarkerErrorCode = "oversized"
	MarkerErrorMalformedJSON      MarkerErrorCode = "malformed_json"
	MarkerErrorUnsupportedVersion MarkerErrorCode = "unsupported_version"
	MarkerErrorInvalidFields      MarkerErrorCode = "invalid_fields"
)

// MarkerValidationError reports invalid marker content. Request and storage
// failures use ordinary errors instead and must not become marker states.
type MarkerValidationError struct {
	Code MarkerErrorCode
	Err  error
}

func (e *MarkerValidationError) Error() string {
	return fmt.Sprintf("airplan: invalid ownership marker (%s): %v", e.Code, e.Err)
}

func (e *MarkerValidationError) Unwrap() error { return e.Err }

// MarkerCode returns the stable code carried by err, when err describes
// invalid marker content.
func MarkerCode(err error) (MarkerErrorCode, bool) {
	var markerErr *MarkerValidationError
	if !errors.As(err, &markerErr) {
		return "", false
	}
	return markerErr.Code, true
}

// UploadMarker is the versioned ownership record stored in every airplan
// upload directory (SPEC.md §5).
type UploadMarker struct {
	Schema    string    `json:"schema"`
	Version   int       `json:"version"`
	Directory string    `json:"directory"`
	CreatedAt time.Time `json:"created_at"`
	Format    string    `json:"format"`
	Page      string    `json:"page"`
	Source    string    `json:"source,omitempty"`
	Title     string    `json:"title,omitempty"`
}

type markerWire struct {
	Schema    *string `json:"schema"`
	Version   *int    `json:"version"`
	Directory *string `json:"directory"`
	CreatedAt *string `json:"created_at"`
	Format    *string `json:"format"`
	Page      *string `json:"page"`
	Source    *string `json:"source"`
	Title     *string `json:"title"`
}

// EncodeUploadMarker validates and encodes marker as UTF-8 JSON.
func EncodeUploadMarker(marker UploadMarker) ([]byte, error) {
	if err := validateUploadMarker(&marker, marker.Directory); err != nil {
		return nil, err
	}
	body, err := json.Marshal(marker)
	if err != nil {
		return nil, fmt.Errorf("airplan: encode ownership marker: %w", err)
	}
	if len(body) > MaxMarkerSize {
		return nil, markerInvalid(MarkerErrorOversized,
			fmt.Errorf("body is %d bytes; maximum is %d", len(body), MaxMarkerSize))
	}
	return body, nil
}

// DecodeUploadMarker strictly decodes and validates a marker for expectedDir.
// Unknown fields are ignored, but duplicate names anywhere in the JSON value
// are rejected (SPEC.md §5).
func DecodeUploadMarker(data []byte, expectedDir string) (*UploadMarker, error) {
	if len(data) > MaxMarkerSize {
		return nil, markerInvalid(MarkerErrorOversized,
			fmt.Errorf("body is larger than %d bytes", MaxMarkerSize))
	}
	if !utf8.Valid(data) {
		return nil, markerInvalid(MarkerErrorMalformedJSON,
			errors.New("body is not valid UTF-8"))
	}
	if err := validateJSONNames(data); err != nil {
		return nil, markerInvalid(MarkerErrorMalformedJSON, err)
	}

	var wire markerWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, markerInvalid(MarkerErrorMalformedJSON, err)
	}
	if wire.Version == nil {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("missing required field version"))
	}
	if *wire.Version != MarkerVersion {
		return nil, markerInvalid(MarkerErrorUnsupportedVersion,
			fmt.Errorf("version %d is unsupported", *wire.Version))
	}
	if wire.Schema == nil || wire.Directory == nil || wire.CreatedAt == nil ||
		wire.Format == nil || wire.Page == nil {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("one or more required fields are missing"))
	}

	createdAt, err := time.Parse(time.RFC3339, *wire.CreatedAt)
	if err != nil {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			fmt.Errorf("created_at is not RFC 3339: %w", err))
	}
	_, offset := createdAt.Zone()
	if offset != 0 {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("created_at is not UTC"))
	}

	marker := &UploadMarker{
		Schema:    *wire.Schema,
		Version:   *wire.Version,
		Directory: *wire.Directory,
		CreatedAt: createdAt.UTC(),
		Format:    *wire.Format,
		Page:      *wire.Page,
	}
	if wire.Source != nil {
		marker.Source = *wire.Source
	}
	if wire.Title != nil {
		marker.Title = *wire.Title
	}
	if err := validateUploadMarker(marker, expectedDir); err != nil {
		return nil, err
	}
	if wire.Source != nil && marker.Source == "" {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("source must be omitted when empty"))
	}
	if wire.Title != nil && marker.Title == "" {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("title must be omitted when empty"))
	}
	return marker, nil
}

func validateUploadMarker(marker *UploadMarker, expectedDir string) error {
	invalid := func(format string, args ...any) error {
		return markerInvalid(MarkerErrorInvalidFields, fmt.Errorf(format, args...))
	}
	if marker.Schema != MarkerSchema {
		return invalid("schema must be %q", MarkerSchema)
	}
	if marker.Version != MarkerVersion {
		return markerInvalid(MarkerErrorUnsupportedVersion,
			fmt.Errorf("version %d is unsupported", marker.Version))
	}
	if !isRandomDir(marker.Directory) || marker.Directory != expectedDir {
		return invalid("directory %q does not match containing directory %q",
			marker.Directory, expectedDir)
	}
	if marker.CreatedAt.IsZero() {
		return invalid("created_at is required")
	}
	_, offset := marker.CreatedAt.Zone()
	if offset != 0 {
		return invalid("created_at is not UTC")
	}

	slug, ok := pageSlug(marker.Page)
	if !ok {
		return invalid("page %q is not a valid page basename", marker.Page)
	}
	switch marker.Format {
	case "md":
		if marker.Source != "" && marker.Source != slug+".md" {
			return invalid("markdown source %q does not match page %q",
				marker.Source, marker.Page)
		}
	case "html":
		if marker.Source != "" {
			return invalid("HTML markers must not declare a source")
		}
	case "txt":
		if marker.Source != "" && !validTextSource(marker.Source, slug) {
			return invalid("text source %q does not match page %q",
				marker.Source, marker.Page)
		}
	default:
		return invalid("format %q is unsupported", marker.Format)
	}
	return nil
}

func markerInvalid(code MarkerErrorCode, err error) error {
	return &MarkerValidationError{Code: code, Err: err}
}

func pageSlug(name string) (string, bool) {
	if !strings.HasSuffix(name, ".html") {
		return "", false
	}
	slug := strings.TrimSuffix(name, ".html")
	if len(slug) == 0 || len(slug) > 64 {
		return "", false
	}
	for _, r := range slug {
		if !isSlugChar(r) {
			return "", false
		}
	}
	return slug, true
}

func validTextSource(name, slug string) bool {
	prefix := slug + "."
	if !strings.HasPrefix(name, prefix) || len(name) == len(prefix) {
		return false
	}
	ext := strings.TrimPrefix(name, prefix)
	if ext == "html" || ext == "htm" {
		return false
	}
	for _, r := range ext {
		if r < 'a' || r > 'z' {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func validateJSONNames(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := validateJSONValue(dec); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func validateJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object field name is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = struct{}{}
			if err := validateJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		for dec.More() {
			if err := validateJSONValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	return nil
}
