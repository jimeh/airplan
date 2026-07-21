package airplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MarkerFilename is the ownership marker basename (SPEC.md §5).
const MarkerFilename = ".airplan.json"

// CollectionMarkerFilename is the collection ownership marker basename.
const CollectionMarkerFilename = ".airplan-collection.json"

// MarkerSchema identifies airplan upload markers (SPEC.md §5).
const MarkerSchema = "airplan-upload"

// MarkerVersion is the latest ownership marker version written by airplan.
const MarkerVersion = 3

// IsSupportedMarkerVersion reports whether version can be safely managed by
// this airplan release.
func IsSupportedMarkerVersion(version int) bool {
	return version == 1 || version == 2 || version == MarkerVersion
}

// MaxMarkerSize is the maximum accepted marker body size.
const MaxMarkerSize = 64 * 1024

// MaxCollectionFiles is the maximum number of files declared by a collection.
const MaxCollectionFiles = 100

// UploadKind identifies the shape and lifecycle rules for an upload.
type UploadKind string

// Supported marker v3 upload kinds.
const (
	UploadKindDocument   UploadKind = "document"
	UploadKindCollection UploadKind = "collection"
)

// MarkerRole identifies the purpose of a declared upload object.
type MarkerRole string

// Supported marker v3 object roles.
const (
	MarkerRolePage   MarkerRole = "page"
	MarkerRoleSource MarkerRole = "source"
	MarkerRoleFile   MarkerRole = "file"
)

// MarkerObject declares one payload object owned by an upload marker.
type MarkerObject struct {
	Name        string     `json:"name"`
	Role        MarkerRole `json:"role"`
	Bytes       int64      `json:"bytes"`
	ContentType string     `json:"content_type"`
}

// MarkerFilenameForKind returns the exact marker basename for kind.
func MarkerFilenameForKind(kind UploadKind) (string, bool) {
	switch kind {
	case UploadKindDocument:
		return MarkerFilename, true
	case UploadKindCollection:
		return CollectionMarkerFilename, true
	default:
		return "", false
	}
}

// MarkerErrorCode is a stable show --json validation error code.
type MarkerErrorCode string

// Stable invalid-marker error codes from SPEC.md §9.
const (
	MarkerErrorOversized          MarkerErrorCode = "oversized"
	MarkerErrorMalformedJSON      MarkerErrorCode = "malformed_json"
	MarkerErrorUnsupportedVersion MarkerErrorCode = "unsupported_version"
	MarkerErrorInvalidFields      MarkerErrorCode = "invalid_fields"
	MarkerErrorConflictingMarkers MarkerErrorCode = "conflicting_markers"
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
	Schema    string         `json:"schema"`
	Version   int            `json:"version"`
	Directory string         `json:"directory"`
	CreatedAt time.Time      `json:"created_at"`
	Kind      UploadKind     `json:"kind"`
	Slug      string         `json:"slug,omitempty"`
	Format    string         `json:"format,omitempty"`
	Objects   []MarkerObject `json:"objects"`
	Title     string         `json:"title,omitempty"`
	Repo      string         `json:"repo,omitempty"`

	// Page, PageBytes, and Source are normalized compatibility views used by
	// callers that predate marker v3. They are never encoded in marker v3.
	Page      string `json:"-"`
	PageBytes int64  `json:"-"`
	Source    string `json:"-"`
}

type markerWire struct {
	Schema    *string         `json:"schema"`
	Version   *int            `json:"version"`
	Directory *string         `json:"directory"`
	CreatedAt *string         `json:"created_at"`
	Kind      *string         `json:"kind"`
	Slug      *string         `json:"slug"`
	Format    *string         `json:"format"`
	Objects   json.RawMessage `json:"objects"`
	Page      *string         `json:"page"`
	PageBytes json.RawMessage `json:"page_bytes"`
	Source    *string         `json:"source"`
	Title     *string         `json:"title"`
	Repo      json.RawMessage `json:"repo"`
}

type markerObjectWire struct {
	Name        *string `json:"name"`
	Role        *string `json:"role"`
	Bytes       *int64  `json:"bytes"`
	ContentType *string `json:"content_type"`
}

// EncodeUploadMarker validates and encodes marker as UTF-8 JSON.
func EncodeUploadMarker(marker UploadMarker) ([]byte, error) {
	markerName := MarkerFilename
	if marker.Version == MarkerVersion {
		var ok bool
		markerName, ok = MarkerFilenameForKind(marker.Kind)
		if !ok {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				fmt.Errorf("kind %q is unsupported", marker.Kind))
		}
	}
	if err := validateUploadMarker(&marker, marker.Directory, markerName); err != nil {
		return nil, err
	}
	var value any
	if marker.Version == MarkerVersion {
		value = struct {
			Schema    string         `json:"schema"`
			Version   int            `json:"version"`
			Directory string         `json:"directory"`
			CreatedAt time.Time      `json:"created_at"`
			Kind      UploadKind     `json:"kind"`
			Slug      string         `json:"slug,omitempty"`
			Format    string         `json:"format,omitempty"`
			Objects   []MarkerObject `json:"objects"`
			Title     string         `json:"title,omitempty"`
			Repo      string         `json:"repo,omitempty"`
		}{
			marker.Schema, marker.Version, marker.Directory, marker.CreatedAt,
			marker.Kind, marker.Slug, marker.Format, marker.Objects,
			marker.Title, marker.Repo,
		}
	} else {
		value = struct {
			Schema    string    `json:"schema"`
			Version   int       `json:"version"`
			Directory string    `json:"directory"`
			CreatedAt time.Time `json:"created_at"`
			Format    string    `json:"format"`
			Page      string    `json:"page"`
			PageBytes int64     `json:"page_bytes,omitempty"`
			Source    string    `json:"source,omitempty"`
			Title     string    `json:"title,omitempty"`
			Repo      string    `json:"repo,omitempty"`
		}{
			marker.Schema, marker.Version, marker.Directory, marker.CreatedAt,
			marker.Format, marker.Page, marker.PageBytes, marker.Source,
			marker.Title, marker.Repo,
		}
	}
	body, err := json.Marshal(value)
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
	return DecodeUploadMarkerForName(data, expectedDir, MarkerFilename)
}

// DecodeUploadMarkerForName strictly decodes and validates a marker against
// its containing directory and exact marker basename.
func DecodeUploadMarkerForName(
	data []byte, expectedDir, markerBasename string,
) (*UploadMarker, error) {
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
	if !IsSupportedMarkerVersion(*wire.Version) {
		return nil, markerInvalid(MarkerErrorUnsupportedVersion,
			fmt.Errorf("version %d is unsupported", *wire.Version))
	}
	if wire.Schema == nil || wire.Directory == nil || wire.CreatedAt == nil {
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
	}
	if wire.Title != nil {
		marker.Title = *wire.Title
	}
	if marker.Version == MarkerVersion {
		if wire.Kind == nil || len(wire.Objects) == 0 {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("one or more marker v3 fields are missing"))
		}
		if wire.Page != nil || len(wire.PageBytes) > 0 || wire.Source != nil {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("marker v3 must not use legacy object fields"))
		}
		marker.Kind = UploadKind(*wire.Kind)
		if wire.Slug != nil {
			marker.Slug = *wire.Slug
		}
		if wire.Format != nil {
			marker.Format = *wire.Format
		}
		objects, err := decodeMarkerObjects(wire.Objects)
		if err != nil {
			return nil, err
		}
		marker.Objects = objects
		if len(wire.Repo) > 0 {
			if err := json.Unmarshal(wire.Repo, &marker.Repo); err != nil {
				return nil, markerInvalid(MarkerErrorInvalidFields,
					fmt.Errorf("repo is not a string: %w", err))
			}
		}
	} else {
		if markerBasename != MarkerFilename {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				fmt.Errorf("marker versions 1 and 2 require basename %q",
					MarkerFilename))
		}
		if wire.Format == nil || wire.Page == nil {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("one or more legacy marker fields are missing"))
		}
		marker.Format = *wire.Format
		marker.Page = *wire.Page
		if wire.Source != nil {
			marker.Source = *wire.Source
		}
		if marker.Version == 2 {
			if len(wire.PageBytes) > 0 {
				if err := json.Unmarshal(wire.PageBytes, &marker.PageBytes); err != nil {
					return nil, markerInvalid(MarkerErrorInvalidFields,
						fmt.Errorf("page_bytes is not an integer: %w", err))
				}
			}
			if len(wire.Repo) > 0 {
				if err := json.Unmarshal(wire.Repo, &marker.Repo); err != nil {
					return nil, markerInvalid(MarkerErrorInvalidFields,
						fmt.Errorf("repo is not a string: %w", err))
				}
			}
		}
	}
	if err := validateUploadMarker(marker, expectedDir, markerBasename); err != nil {
		return nil, err
	}
	if marker.Version < MarkerVersion && wire.Source != nil && marker.Source == "" {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("source must be omitted when empty"))
	}
	if wire.Title != nil && marker.Title == "" {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("title must be omitted when empty"))
	}
	if marker.Version >= 2 && len(wire.Repo) > 0 && marker.Repo == "" {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			errors.New("repo must be omitted when empty"))
	}
	if marker.Version == MarkerVersion {
		if wire.Slug != nil && marker.Slug == "" {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("slug must be omitted when empty"))
		}
		if wire.Format != nil && marker.Format == "" {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("format must be omitted when empty"))
		}
		normalizeV3Compatibility(marker)
	} else {
		normalizeLegacyMarker(marker)
	}
	return marker, nil
}

func validateUploadMarker(
	marker *UploadMarker, expectedDir, markerBasename string,
) error {
	invalid := func(format string, args ...any) error {
		return markerInvalid(MarkerErrorInvalidFields, fmt.Errorf(format, args...))
	}
	if marker.Schema != MarkerSchema {
		return invalid("schema must be %q", MarkerSchema)
	}
	if !IsSupportedMarkerVersion(marker.Version) {
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
	if markerBasename != MarkerFilename &&
		markerBasename != CollectionMarkerFilename {
		return invalid("marker basename %q is unsupported", markerBasename)
	}
	switch marker.Version {
	case 1:
		if marker.PageBytes != 0 || marker.Repo != "" {
			return invalid("version 1 must not declare page_bytes or repo")
		}
	case 2:
		if marker.PageBytes <= 0 {
			return invalid("page_bytes must be positive")
		}
	}
	if marker.Repo != "" {
		canonical, err := NormalizeRepositoryURL(marker.Repo)
		if err != nil || canonical != marker.Repo {
			return invalid("repo %q is not a canonical repository URL",
				marker.Repo)
		}
	}
	_, offset := marker.CreatedAt.Zone()
	if offset != 0 {
		return invalid("created_at is not UTC")
	}

	if marker.Version == MarkerVersion {
		return validateMarkerV3(marker, markerBasename, invalid)
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

func validateMarkerV3(
	marker *UploadMarker,
	markerBasename string,
	invalid func(string, ...any) error,
) error {
	wantBasename, ok := MarkerFilenameForKind(marker.Kind)
	if !ok {
		return invalid("kind %q is unsupported", marker.Kind)
	}
	if markerBasename != wantBasename {
		return invalid("kind %q requires marker basename %q",
			marker.Kind, wantBasename)
	}
	if len(marker.Objects) == 0 {
		return invalid("objects must not be empty")
	}

	names := make(map[string]struct{}, len(marker.Objects))
	var page, source *MarkerObject
	files := 0
	for i := range marker.Objects {
		object := &marker.Objects[i]
		if !validMarkerObjectName(object.Name) {
			return invalid("object name %q is not a safe direct basename",
				object.Name)
		}
		if _, exists := names[object.Name]; exists {
			return invalid("object name %q is duplicated", object.Name)
		}
		names[object.Name] = struct{}{}
		if !validNormalizedContentType(object.ContentType) {
			return invalid("object %q content type %q is not normalized",
				object.Name, object.ContentType)
		}
		switch object.Role {
		case MarkerRolePage:
			if page != nil {
				return invalid("objects must contain exactly one page")
			}
			page = object
			if object.Bytes <= 0 {
				return invalid("page %q bytes must be positive", object.Name)
			}
			if !isHTMLContentType(object.ContentType) {
				return invalid("page %q must have an HTML content type",
					object.Name)
			}
		case MarkerRoleSource:
			if source != nil {
				return invalid("objects must contain at most one source")
			}
			source = object
			if object.Bytes <= 0 {
				return invalid("source %q bytes must be positive", object.Name)
			}
		case MarkerRoleFile:
			files++
			if object.Bytes < 0 {
				return invalid("file %q bytes must not be negative", object.Name)
			}
		default:
			return invalid("object %q role %q is unsupported",
				object.Name, object.Role)
		}
	}
	if page == nil {
		return invalid("objects must contain exactly one page")
	}

	switch marker.Kind {
	case UploadKindDocument:
		return validateDocumentMarkerV3(marker, page, source, files, invalid)
	case UploadKindCollection:
		return validateCollectionMarkerV3(marker, page, source, files, invalid)
	default:
		return invalid("kind %q is unsupported", marker.Kind)
	}
}

func validateDocumentMarkerV3(
	marker *UploadMarker,
	page, source *MarkerObject,
	files int,
	invalid func(string, ...any) error,
) error {
	if marker.Slug == "" || !validSlug(marker.Slug) {
		return invalid("slug %q is not valid", marker.Slug)
	}
	if page.Name != marker.Slug+".html" {
		return invalid("page %q does not match slug %q",
			page.Name, marker.Slug)
	}
	if files != 0 {
		return invalid("document markers must not declare file objects")
	}
	switch marker.Format {
	case "md":
		if source != nil && source.Name != marker.Slug+".md" {
			return invalid("markdown source %q does not match page %q",
				source.Name, page.Name)
		}
	case "html":
		if source != nil {
			return invalid("HTML markers must not declare a source")
		}
	case "txt":
		if source != nil && !validTextSource(source.Name, marker.Slug) {
			return invalid("text source %q does not match page %q",
				source.Name, page.Name)
		}
	default:
		return invalid("format %q is unsupported", marker.Format)
	}
	return nil
}

func validateCollectionMarkerV3(
	marker *UploadMarker,
	page, source *MarkerObject,
	files int,
	invalid func(string, ...any) error,
) error {
	if marker.Slug != "" {
		return invalid("collection markers must not declare a slug")
	}
	if marker.Format != "" {
		return invalid("collection markers must not declare a format")
	}
	if page.Name != "index.html" {
		return invalid("collection page must be %q", "index.html")
	}
	if source != nil {
		return invalid("collection markers must not declare a source")
	}
	if files == 0 {
		return invalid("collection markers must declare at least one file")
	}
	if files > MaxCollectionFiles {
		return invalid("collection declares %d files; maximum is %d",
			files, MaxCollectionFiles)
	}
	return nil
}

func decodeMarkerObjects(data json.RawMessage) ([]MarkerObject, error) {
	var wires []markerObjectWire
	if err := json.Unmarshal(data, &wires); err != nil {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			fmt.Errorf("objects is not an array: %w", err))
	}
	objects := make([]MarkerObject, len(wires))
	for i, wire := range wires {
		if wire.Name == nil || wire.Role == nil || wire.Bytes == nil ||
			wire.ContentType == nil {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				fmt.Errorf("object %d is missing one or more required fields", i))
		}
		objects[i] = MarkerObject{
			Name: *wire.Name, Role: MarkerRole(*wire.Role), Bytes: *wire.Bytes,
			ContentType: *wire.ContentType,
		}
	}
	return objects, nil
}

func normalizeV3Compatibility(marker *UploadMarker) {
	for _, object := range marker.Objects {
		switch object.Role {
		case MarkerRolePage:
			marker.Page = object.Name
			marker.PageBytes = object.Bytes
		case MarkerRoleSource:
			marker.Source = object.Name
		}
	}
}

func normalizeLegacyMarker(marker *UploadMarker) {
	marker.Kind = UploadKindDocument
	marker.Slug, _ = pageSlug(marker.Page)
	marker.Objects = []MarkerObject{{
		Name: marker.Page, Role: MarkerRolePage, Bytes: marker.PageBytes,
		ContentType: "text/html; charset=utf-8",
	}}
	if marker.Source != "" {
		contentType := "text/plain; charset=utf-8"
		if marker.Format == "md" {
			contentType = "text/markdown; charset=utf-8"
		}
		marker.Objects = append(marker.Objects, MarkerObject{
			Name: marker.Source, Role: MarkerRoleSource,
			ContentType: contentType,
		})
	}
}

func validMarkerObjectName(name string) bool {
	if !utf8.ValidString(name) || name == "" || name == "." || name == ".." ||
		name == MarkerFilename || name == CollectionMarkerFilename ||
		strings.ContainsAny(name, `/\\`) {
		return false
	}
	for _, r := range name {
		if r == 0 || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validNormalizedContentType(value string) bool {
	mediaType, params, err := mime.ParseMediaType(value)
	return err == nil && value == mime.FormatMediaType(mediaType, params)
}

func isHTMLContentType(value string) bool {
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil || mediaType != "text/html" {
		return false
	}
	charset, ok := params["charset"]
	return !ok || strings.EqualFold(charset, "utf-8")
}

func validSlug(slug string) bool {
	if len(slug) == 0 || len(slug) > 64 {
		return false
	}
	for _, r := range slug {
		if !isSlugChar(r) {
			return false
		}
	}
	return true
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
