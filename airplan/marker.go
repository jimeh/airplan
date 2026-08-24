package airplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jimeh/airplan/internal/pathrules"
)

// MarkerFilename is the ownership marker basename (SPEC.md §5).
const MarkerFilename = ".airplan.json"

// CollectionMarkerFilename is the collection ownership marker basename.
const CollectionMarkerFilename = ".airplan-collection.json"

// MarkerSchema identifies airplan upload markers (SPEC.md §5).
const MarkerSchema = "airplan-upload"

// MarkerVersion is the latest ownership marker version written by airplan.
const MarkerVersion = 6

// IsSupportedMarkerVersion reports whether version can be safely managed by
// this airplan release.
func IsSupportedMarkerVersion(version int) bool {
	return version >= 1 && version <= MarkerVersion
}

// MaxMarkerSize is the maximum accepted marker body size.
const MaxMarkerSize = 256 * 1024

// MaxCollectionFiles is the maximum number of files declared by a collection.
const MaxCollectionFiles = 100

// MaxDocumentItems is the maximum number of user-supplied entry, page, and
// asset items declared by a document marker.
const MaxDocumentItems = 100

// UploadKind identifies the shape and lifecycle rules for an upload.
type UploadKind string

// Supported modern marker upload kinds.
const (
	UploadKindDocument   UploadKind = "document"
	UploadKindCollection UploadKind = "collection"
)

// MarkerRole identifies the purpose of a declared upload object.
type MarkerRole string

// Supported modern marker object roles.
const (
	MarkerRolePage   MarkerRole = "page"
	MarkerRoleSource MarkerRole = "source"
	MarkerRoleFile   MarkerRole = "file"
	MarkerRoleDiff   MarkerRole = "diff"
	MarkerRoleAsset  MarkerRole = "asset"
)

// MarkerPage describes one managed source page in a document bundle. Pages
// are ordered and include the entry when Airplan rendered it.
type MarkerPage struct {
	Path   string `json:"path"`
	Page   string `json:"page"`
	Source string `json:"source,omitempty"`
	Format string `json:"format"`
	Title  string `json:"title,omitempty"`
	Lang   string `json:"lang"`
}

// RevisionDescriptor is the immutable chain identity carried by linked
// Markdown ownership markers. Mutable latest-navigation state lives in the
// separately replicated versions metadata object.
type RevisionDescriptor struct {
	ChainID     string `json:"chain_id"`
	Number      int    `json:"number"`
	PreviousURL string `json:"previous_url,omitempty"`
}

// MarkerObject declares one payload object owned by an upload marker.
type MarkerObject struct {
	Name        string     `json:"name"`
	Role        MarkerRole `json:"role"`
	Bytes       int64      `json:"bytes"`
	ContentType string     `json:"content_type"`
	SHA256      string     `json:"sha256,omitempty"`
}

func markerObjectForRole(marker *UploadMarker, role MarkerRole) (MarkerObject, bool) {
	if marker == nil {
		return MarkerObject{}, false
	}
	for _, object := range marker.Objects {
		if object.Role == role {
			return object, true
		}
	}
	return MarkerObject{}, false
}

// Producer identifies the Airplan release that produced a v4-or-newer marker.
type Producer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// RenderTemplate identifies the template used to generate a page. Custom
// template source and local paths are deliberately not persisted.
type RenderTemplate struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256,omitempty"`
}

// RenderRecipe is the reproducible rendering policy recorded by marker v4+
// and extended with theme identity by marker v5.
type RenderRecipe struct {
	Generation       int            `json:"generation"`
	Template         RenderTemplate `json:"template"`
	Indexable        bool           `json:"indexable"`
	NoExternalAssets bool           `json:"no_external_assets"`
	MermaidURL       string         `json:"mermaid_url,omitempty"`
	Themes           *ThemeRecipe   `json:"themes,omitempty"`
}

// RendererGeneration is incremented only when generated page capabilities or
// embedded assets require existing source-backed pages to be re-rendered.
const RendererGeneration = 29

// MarkerDeclaredTotals returns the object count and byte total an upload
// declares (SPEC.md §9): its ownership marker plus every object the marker
// lists, with markerBytes the exact serialized marker body written to storage.
//
// These are declared values, never a storage listing, so the same upload
// reports the same totals whether they were recorded when it was uploaded or
// derived later from its marker by sync. Marker v3 and newer declare every object;
// marker v2 declares the page and qualifies only when it has no source. Marker
// v1, unsupported versions, and v2-with-source report ok false because their
// totals would be a guess.
func MarkerDeclaredTotals(
	marker UploadMarker, markerBytes int,
) (objects int, total int64, ok bool) {
	if markerBytes <= 0 {
		return 0, 0, false
	}
	if marker.Version == 2 {
		if marker.Source != "" || marker.PageBytes <= 0 {
			return 0, 0, false
		}
		total, ok := addDeclaredBytes(int64(markerBytes), marker.PageBytes)
		if !ok {
			return 0, 0, false
		}
		return 2, total, true
	}
	if marker.Version != 3 && marker.Version != 4 && marker.Version != 5 &&
		marker.Version != 6 {
		return 0, 0, false
	}
	objects = 1
	total = int64(markerBytes)
	for _, object := range marker.Objects {
		var added bool
		total, added = addDeclaredBytes(total, object.Bytes)
		if !added {
			return 0, 0, false
		}
		objects++
	}
	return objects, total, true
}

func addDeclaredBytes(total, value int64) (int64, bool) {
	if total <= 0 || value < 0 || total > math.MaxInt64-value {
		return 0, false
	}
	return total + value, true
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
	Schema     string              `json:"schema"`
	Version    int                 `json:"version"`
	Directory  string              `json:"directory"`
	CreatedAt  time.Time           `json:"created_at"`
	Kind       UploadKind          `json:"kind"`
	Slug       string              `json:"slug,omitempty"`
	Format     string              `json:"format,omitempty"`
	Objects    []MarkerObject      `json:"objects"`
	Title      string              `json:"title,omitempty"`
	Repo       string              `json:"repo,omitempty"`
	Producer   Producer            `json:"producer"`
	Render     *RenderRecipe       `json:"render,omitempty"`
	Revision   *RevisionDescriptor `json:"revision,omitempty"`
	Entrypoint string              `json:"entrypoint,omitempty"`
	Pages      []MarkerPage        `json:"pages,omitempty"`

	// Page, PageBytes, PageSHA256, and Source are normalized compatibility views
	// used by callers that predate marker v3. They are never encoded separately
	// in modern markers.
	Page       string `json:"-"`
	PageBytes  int64  `json:"-"`
	PageSHA256 string `json:"-"`
	Source     string `json:"-"`
}

type markerWire struct {
	Schema     *string         `json:"schema"`
	Version    *int            `json:"version"`
	Directory  *string         `json:"directory"`
	CreatedAt  *string         `json:"created_at"`
	Kind       *string         `json:"kind"`
	Slug       *string         `json:"slug"`
	Format     *string         `json:"format"`
	Objects    json.RawMessage `json:"objects"`
	Page       *string         `json:"page"`
	PageBytes  json.RawMessage `json:"page_bytes"`
	Source     *string         `json:"source"`
	Title      *string         `json:"title"`
	Repo       json.RawMessage `json:"repo"`
	Producer   json.RawMessage `json:"producer"`
	Render     json.RawMessage `json:"render"`
	Revision   json.RawMessage `json:"revision"`
	Entrypoint *string         `json:"entrypoint"`
	Pages      json.RawMessage `json:"pages"`
}

type markerObjectWire struct {
	Name        *string         `json:"name"`
	Role        *string         `json:"role"`
	Bytes       *int64          `json:"bytes"`
	ContentType *string         `json:"content_type"`
	SHA256      json.RawMessage `json:"sha256"`
}

type markerPageWire struct {
	Path   *string `json:"path"`
	Page   *string `json:"page"`
	Source *string `json:"source"`
	Format *string `json:"format"`
	Title  *string `json:"title"`
	Lang   *string `json:"lang"`
}

// EncodeUploadMarker validates and encodes marker as UTF-8 JSON.
func EncodeUploadMarker(marker UploadMarker) ([]byte, error) {
	if marker.Version == 4 && marker.Render != nil {
		render := *marker.Render
		render.Themes = nil
		marker.Render = &render
	}
	markerName := MarkerFilename
	if marker.Version >= 3 {
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
	if marker.Version >= 3 {
		producer := marker.Producer
		render := marker.Render
		if marker.Version < 4 {
			// producer and render were unknown extension fields in v3. Keep
			// v3 encoding compatible rather than assigning v4 meaning to them.
			producer = Producer{}
			render = nil
		}
		objects := append([]MarkerObject(nil), marker.Objects...)
		if marker.Version < 4 {
			for index := range objects {
				objects[index].SHA256 = ""
			}
		}
		value = struct {
			Schema     string              `json:"schema"`
			Version    int                 `json:"version"`
			Directory  string              `json:"directory"`
			CreatedAt  time.Time           `json:"created_at"`
			Kind       UploadKind          `json:"kind"`
			Slug       string              `json:"slug,omitempty"`
			Format     string              `json:"format,omitempty"`
			Objects    []MarkerObject      `json:"objects"`
			Title      string              `json:"title,omitempty"`
			Repo       string              `json:"repo,omitempty"`
			Producer   Producer            `json:"producer,omitzero"`
			Render     *RenderRecipe       `json:"render,omitempty"`
			Revision   *RevisionDescriptor `json:"revision,omitempty"`
			Entrypoint string              `json:"entrypoint,omitempty"`
			Pages      []MarkerPage        `json:"pages,omitempty"`
		}{
			marker.Schema, marker.Version, marker.Directory, marker.CreatedAt,
			marker.Kind, marker.Slug, marker.Format, objects,
			marker.Title, marker.Repo, producer, render, marker.Revision,
			marker.Entrypoint, marker.Pages,
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
	if marker.Version >= 3 {
		if wire.Kind == nil || len(wire.Objects) == 0 {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("one or more modern marker fields are missing"))
		}
		if wire.Page != nil || len(wire.PageBytes) > 0 || wire.Source != nil {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("modern markers must not use legacy object fields"))
		}
		marker.Kind = UploadKind(*wire.Kind)
		if wire.Slug != nil {
			marker.Slug = *wire.Slug
		}
		if wire.Format != nil {
			marker.Format = *wire.Format
		}
		objects, err := decodeMarkerObjects(wire.Objects, marker.Version)
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
		if marker.Version >= 4 && len(wire.Producer) > 0 {
			if err := json.Unmarshal(wire.Producer, &marker.Producer); err != nil {
				return nil, markerInvalid(MarkerErrorInvalidFields,
					fmt.Errorf("producer is invalid: %w", err))
			}
		}
		if marker.Version >= 4 && len(wire.Render) > 0 {
			var recipe RenderRecipe
			if err := json.Unmarshal(wire.Render, &recipe); err != nil {
				return nil, markerInvalid(MarkerErrorInvalidFields,
					fmt.Errorf("render is invalid: %w", err))
			}
			marker.Render = &recipe
		}
		if marker.Version >= 4 && len(wire.Revision) > 0 {
			var revision RevisionDescriptor
			if err := json.Unmarshal(wire.Revision, &revision); err != nil {
				return nil, markerInvalid(MarkerErrorInvalidFields,
					fmt.Errorf("revision is invalid: %w", err))
			}
			marker.Revision = &revision
		}
		if marker.Version >= 6 {
			if wire.Entrypoint != nil {
				marker.Entrypoint = *wire.Entrypoint
			}
			if len(wire.Pages) > 0 {
				marker.Pages, err = decodeMarkerPages(wire.Pages)
				if err != nil {
					return nil, err
				}
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
		if marker.Version == 1 &&
			(len(wire.PageBytes) > 0 || len(wire.Repo) > 0) {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				errors.New("version 1 must not declare page_bytes or repo"))
		}
	}
	if err := validateUploadMarker(marker, expectedDir, markerBasename); err != nil {
		return nil, err
	}
	if marker.Version < 3 && wire.Source != nil && marker.Source == "" {
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
	if marker.Version >= 3 {
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

	if marker.Version >= 3 {
		if err := validateModernMarker(marker, markerBasename, invalid); err != nil {
			return err
		}
		if marker.Version >= 4 {
			return validateMarkerV4Plus(marker, invalid)
		}
		return nil
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

func validateModernMarker(
	marker *UploadMarker,
	markerBasename string,
	invalid func(string, ...any) error,
) error {
	if marker.Version < 6 && (marker.Entrypoint != "" || len(marker.Pages) != 0) {
		return invalid("marker versions before 6 must not declare entrypoint or pages")
	}
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
	var page, source, diff *MarkerObject
	pages := 0
	sources := 0
	assets := 0
	files := 0
	for i := range marker.Objects {
		object := &marker.Objects[i]
		validName := validMarkerObjectName(object.Name)
		if marker.Version >= 6 {
			switch marker.Kind {
			case UploadKindCollection:
				validName = object.Role == MarkerRolePage &&
					validMarkerObjectName(object.Name)
				if object.Role == MarkerRoleFile {
					validName = validateCollectionName(object.Name) == nil
				}
			default:
				validName = validMarkerObjectPath(object.Name) ||
					object.Name == DiffFilename
			}
		}
		if !validName {
			return invalid("object name %q is not a safe relative path",
				object.Name)
		}
		if marker.Version >= 4 && strings.HasPrefix(strings.ToLower(object.Name), ".airplan-") &&
			(object.Role != MarkerRoleDiff || object.Name != DiffFilename) {
			return invalid("object name %q uses the reserved .airplan- prefix",
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
		if object.SHA256 != "" && !validSHA256(object.SHA256) {
			return invalid("object %q sha256 must be 64 lowercase hex characters",
				object.Name)
		}
		if marker.Version >= 4 && marker.Version < 6 && object.Role != MarkerRolePage &&
			object.SHA256 != "" {
			return invalid("object %q sha256 is only valid for the page", object.Name)
		}
		switch object.Role {
		case MarkerRolePage:
			if marker.Version < 6 && page != nil {
				return invalid("objects must contain exactly one page")
			}
			if page == nil {
				page = object
			}
			pages++
			if object.Bytes <= 0 {
				return invalid("page %q bytes must be positive", object.Name)
			}
			if !isHTMLContentType(object.ContentType) {
				return invalid("page %q must have an HTML content type",
					object.Name)
			}
		case MarkerRoleSource:
			if marker.Version < 6 && source != nil {
				return invalid("objects must contain at most one source")
			}
			if source == nil {
				source = object
			}
			sources++
			if object.Bytes <= 0 {
				return invalid("source %q bytes must be positive", object.Name)
			}
		case MarkerRoleFile:
			files++
			if object.Bytes < 0 {
				return invalid("file %q bytes must not be negative", object.Name)
			}
		case MarkerRoleDiff:
			if diff != nil {
				return invalid("objects must contain at most one diff")
			}
			diff = object
			if object.Name != DiffFilename {
				return invalid("diff object must be %q", DiffFilename)
			}
			if object.Bytes <= 0 {
				return invalid("diff %q bytes must be positive", object.Name)
			}
			if object.ContentType != diffContentType {
				return invalid("diff %q must have content type %q",
					object.Name, diffContentType)
			}
		case MarkerRoleAsset:
			if marker.Version < 6 || marker.Kind != UploadKindDocument {
				return invalid("object %q role %q is unsupported", object.Name, object.Role)
			}
			assets++
			if object.Bytes < 0 {
				return invalid("asset %q bytes must not be negative", object.Name)
			}
		default:
			return invalid("object %q role %q is unsupported",
				object.Name, object.Role)
		}
	}
	if page == nil {
		return invalid("objects must contain exactly one page")
	}
	if marker.Version >= 4 && page.SHA256 == "" {
		return invalid("page %q sha256 is required", page.Name)
	}
	if marker.Version >= 6 {
		for _, object := range marker.Objects {
			if object.SHA256 == "" {
				return invalid("object %q sha256 is required", object.Name)
			}
		}
	}

	switch marker.Kind {
	case UploadKindDocument:
		if marker.Version >= 6 {
			return validateDocumentMarkerV6(marker, pages, sources, assets, files, diff, invalid)
		}
		return validateDocumentMarkerV3(marker, page, source, diff, files, invalid)
	case UploadKindCollection:
		if pages != 1 {
			return invalid("collection markers must contain exactly one page")
		}
		if marker.Version >= 6 && (marker.Entrypoint != "" || len(marker.Pages) != 0) {
			return invalid("collection markers must not declare entrypoint or pages")
		}
		if diff != nil {
			return invalid("collection markers must not declare a diff")
		}
		return validateCollectionMarkerV3(marker, page, source, files, invalid)
	default:
		return invalid("kind %q is unsupported", marker.Kind)
	}
}

func validateMarkerV4Plus(
	marker *UploadMarker,
	invalid func(string, ...any) error,
) error {
	if marker.Producer.Name != "airplan" || marker.Producer.Version == "" {
		return invalid("producer must identify a non-empty airplan version")
	}
	if strings.TrimSpace(marker.Producer.Version) != marker.Producer.Version {
		return invalid("producer version must not contain surrounding whitespace")
	}
	if marker.Revision != nil {
		revision := marker.Revision
		if !isRandomDir(revision.ChainID) {
			return invalid("revision chain_id is not a lowercase 128-bit base32 value")
		}
		if revision.Number <= 0 {
			return invalid("revision number must be positive")
		}
		if revision.Number == 1 && revision.PreviousURL != "" {
			return invalid("revision 1 must not declare previous_url")
		}
		if revision.Number > 1 && revision.PreviousURL == "" {
			return invalid("revision %d must declare previous_url", revision.Number)
		}
		if revision.PreviousURL != "" {
			previous, err := url.Parse(revision.PreviousURL)
			if err != nil || (previous.Scheme != "https" && previous.Scheme != "http") || previous.Host == "" ||
				previous.User != nil || previous.RawQuery != "" || previous.Fragment != "" ||
				!strings.HasSuffix(previous.Path, ".html") {
				return invalid("revision previous_url must be an absolute HTTP(S) HTML URL")
			}
		}
		if marker.Kind != UploadKindDocument || marker.Format != "md" || !markerDeclaresSource(marker) {
			return invalid("only source-backed Markdown documents may declare a revision")
		}
	}
	// Authored HTML is the only document payload Airplan does not render.
	if marker.Kind == UploadKindDocument && marker.Format == "html" {
		if marker.Render != nil {
			return invalid("authored HTML must not declare a render recipe")
		}
		return nil
	}
	if marker.Render == nil {
		return invalid("render recipe is required for generated pages")
	}
	render := marker.Render
	if render.Generation <= 0 {
		return invalid("render generation must be positive")
	}
	switch render.Template.Kind {
	case "builtin", "builtin_collection":
		if render.Template.SHA256 != "" {
			return invalid("built-in templates must not declare sha256")
		}
	case "custom", "custom_collection":
		if len(render.Template.SHA256) != 64 {
			return invalid("custom template sha256 must be 64 lowercase hex characters")
		}
		for _, char := range render.Template.SHA256 {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return invalid("custom template sha256 must be 64 lowercase hex characters")
			}
		}
	default:
		return invalid("render template kind %q is unsupported", render.Template.Kind)
	}
	if marker.Kind == UploadKindDocument {
		if render.Template.Kind != "builtin" && render.Template.Kind != "custom" {
			return invalid("document render template kind %q is invalid",
				render.Template.Kind)
		}
	} else if render.Template.Kind != "builtin_collection" &&
		render.Template.Kind != "custom_collection" {
		return invalid("collection render template kind %q is invalid",
			render.Template.Kind)
	}
	if marker.Kind == UploadKindCollection && render.MermaidURL != "" {
		return invalid("collection render recipes must not declare mermaid_url")
	}
	if render.MermaidURL != "" {
		if err := validateMermaidURL(render.MermaidURL); err != nil {
			return invalid("render mermaid_url is invalid: %v", err)
		}
	}
	if marker.Version == 4 {
		if render.Themes != nil {
			return invalid("version 4 render recipes must not declare themes")
		}
		return nil
	}
	if render.Themes == nil {
		return invalid("themes render recipe is required")
	}
	if !validThemeID(render.Themes.DefaultLight) {
		return invalid("themes default_light must be a lowercase ASCII slug up to %d characters", maxThemeIDLength)
	}
	if !validThemeID(render.Themes.DefaultDark) {
		return invalid("themes default_dark must be a lowercase ASCII slug up to %d characters", maxThemeIDLength)
	}
	if !validSHA256(render.Themes.CatalogSHA256) {
		return invalid("themes catalog_sha256 must be 64 lowercase hex characters")
	}
	return nil
}

func markerDeclaresSource(marker *UploadMarker) bool {
	if marker == nil {
		return false
	}
	if marker.Source != "" {
		return true
	}
	for _, object := range marker.Objects {
		if object.Role == MarkerRoleSource {
			return true
		}
	}
	return false
}

func validateDocumentMarkerV3(
	marker *UploadMarker,
	page, source, diff *MarkerObject,
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
	if marker.Version < 4 && diff != nil {
		return invalid("marker versions before 4 must not declare a diff")
	}
	if marker.Revision == nil && diff != nil {
		return invalid("standalone documents must not declare a diff")
	}
	if marker.Revision != nil {
		if marker.Revision.Number == 1 && diff != nil {
			return invalid("revision 1 must not declare a diff")
		}
		if marker.Revision.Number > 1 && diff == nil {
			return invalid("revision %d must declare a diff", marker.Revision.Number)
		}
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

func validateDocumentMarkerV6(
	marker *UploadMarker,
	pageCount, sourceCount, assetCount, files int,
	diff *MarkerObject,
	invalid func(string, ...any) error,
) error {
	if marker.Slug == "" || !validSlug(marker.Slug) {
		return invalid("slug %q is not valid", marker.Slug)
	}
	if files != 0 {
		return invalid("document markers must not declare file objects")
	}
	if marker.Entrypoint == "" {
		return invalid("entrypoint is required")
	}
	objects := make(map[string]MarkerObject, len(marker.Objects))
	folded := make(map[string]string, len(marker.Objects))
	for _, object := range marker.Objects {
		objects[object.Name] = object
		fold := strings.ToLower(object.Name)
		if previous, ok := folded[fold]; ok && previous != object.Name {
			return invalid("object names %q and %q collide when case-folded", previous, object.Name)
		}
		for previousFold, previous := range folded {
			if strings.HasPrefix(fold, previousFold+"/") ||
				strings.HasPrefix(previousFold, fold+"/") {
				return invalid("object names %q and %q conflict as file and directory", previous, object.Name)
			}
		}
		folded[fold] = object.Name
	}
	entry, ok := objects[marker.Entrypoint]
	if !ok || entry.Role != MarkerRolePage {
		return invalid("entrypoint %q must identify a page object", marker.Entrypoint)
	}
	if marker.Entrypoint != marker.Slug+".html" {
		return invalid("entrypoint %q does not match slug %q", marker.Entrypoint, marker.Slug)
	}
	switch marker.Format {
	case "html":
		if pageCount != 1 {
			return invalid("authored HTML documents must contain exactly one page object")
		}
		if len(marker.Pages) != 0 || sourceCount != 0 {
			return invalid("authored HTML documents must not declare managed pages or sources")
		}
	case "txt":
		if pageCount != 1 || len(marker.Pages) != 1 {
			return invalid("text documents must contain exactly one managed page")
		}
	case "md":
		if pageCount != len(marker.Pages) {
			return invalid("pages descriptors and page objects do not match")
		}
	default:
		return invalid("format %q is unsupported", marker.Format)
	}
	if marker.Format != "html" {
		if len(marker.Pages) == 0 || marker.Pages[0].Page != marker.Entrypoint {
			return invalid("the first managed page must be the entrypoint")
		}
		seenPaths := make(map[string]string, len(marker.Pages))
		seenPages := make(map[string]struct{}, len(marker.Pages))
		seenSources := make(map[string]struct{}, len(marker.Pages))
		for index, descriptor := range marker.Pages {
			if !validMarkerObjectPath(descriptor.Path) {
				return invalid("page %d path %q is not a safe relative path", index, descriptor.Path)
			}
			fold := strings.ToLower(descriptor.Path)
			if previous, exists := seenPaths[fold]; exists {
				return invalid("page paths %q and %q collide when case-folded", previous, descriptor.Path)
			}
			seenPaths[fold] = descriptor.Path
			var format Format
			switch descriptor.Format {
			case "md":
				format = FormatMarkdown
			case "txt":
				format = FormatText
			default:
				return invalid("page %q format %q is unsupported", descriptor.Path, descriptor.Format)
			}
			expectedPage := managedPageObjectName(descriptor.Path, format)
			expectedSource := descriptor.Path
			if index == 0 {
				expectedPage = marker.Entrypoint
				expectedSource = entrySourceObjectName(marker.Slug, descriptor.Path, format)
			}
			if descriptor.Page != expectedPage {
				return invalid("page descriptor %q page %q does not match generated path %q", descriptor.Path, descriptor.Page, expectedPage)
			}
			page, exists := objects[descriptor.Page]
			if !exists || page.Role != MarkerRolePage {
				return invalid("page descriptor %q does not identify a page object", descriptor.Path)
			}
			if _, exists := seenPages[descriptor.Page]; exists {
				return invalid("page object %q is used by multiple descriptors", descriptor.Page)
			}
			seenPages[descriptor.Page] = struct{}{}
			if descriptor.Source != "" {
				if descriptor.Source != expectedSource {
					return invalid("page descriptor %q source %q does not match generated path %q", descriptor.Path, descriptor.Source, expectedSource)
				}
				source, exists := objects[descriptor.Source]
				if !exists || source.Role != MarkerRoleSource {
					return invalid("page descriptor %q does not identify a source object", descriptor.Path)
				}
				if _, exists := seenSources[descriptor.Source]; exists {
					return invalid("source object %q is used by multiple descriptors", descriptor.Source)
				}
				seenSources[descriptor.Source] = struct{}{}
			}
		}
		if len(seenSources) != sourceCount {
			return invalid("source objects and page descriptors do not match")
		}
		if marker.Format != marker.Pages[0].Format {
			return invalid("entry format %q does not match top-level format %q", marker.Pages[0].Format, marker.Format)
		}
	}
	if pageCount+assetCount > MaxDocumentItems {
		return invalid("document declares %d items; maximum is %d", pageCount+assetCount, MaxDocumentItems)
	}
	if marker.Revision == nil && diff != nil {
		return invalid("standalone documents must not declare a diff")
	}
	if marker.Revision != nil {
		if marker.Revision.Number == 1 && diff != nil {
			return invalid("revision 1 must not declare a diff")
		}
		if marker.Revision.Number > 1 && diff == nil {
			return invalid("revision %d must declare a diff", marker.Revision.Number)
		}
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

func decodeMarkerObjects(data json.RawMessage, version int) ([]MarkerObject, error) {
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
		if version >= 4 && len(wire.SHA256) > 0 {
			var digest *string
			if err := json.Unmarshal(wire.SHA256, &digest); err != nil || digest == nil {
				return nil, markerInvalid(MarkerErrorInvalidFields,
					fmt.Errorf("object %d sha256 must be a string", i))
			}
			objects[i].SHA256 = *digest
		}
	}
	return objects, nil
}

func decodeMarkerPages(data json.RawMessage) ([]MarkerPage, error) {
	var wires []markerPageWire
	if err := json.Unmarshal(data, &wires); err != nil {
		return nil, markerInvalid(MarkerErrorInvalidFields,
			fmt.Errorf("pages is not an array: %w", err))
	}
	pages := make([]MarkerPage, len(wires))
	for index, wire := range wires {
		if wire.Path == nil || wire.Page == nil || wire.Format == nil || wire.Lang == nil {
			return nil, markerInvalid(MarkerErrorInvalidFields,
				fmt.Errorf("page %d is missing one or more required fields", index))
		}
		pages[index] = MarkerPage{
			Path: *wire.Path, Page: *wire.Page, Format: *wire.Format, Lang: *wire.Lang,
		}
		if wire.Source != nil {
			pages[index].Source = *wire.Source
		}
		if wire.Title != nil {
			pages[index].Title = *wire.Title
		}
	}
	return pages, nil
}

func normalizeV3Compatibility(marker *UploadMarker) {
	if marker.Version >= 6 && marker.Kind == UploadKindDocument {
		marker.Page = marker.Entrypoint
		for _, object := range marker.Objects {
			if object.Name == marker.Entrypoint {
				marker.PageBytes = object.Bytes
				marker.PageSHA256 = object.SHA256
				break
			}
		}
		if len(marker.Pages) > 0 {
			marker.Source = marker.Pages[0].Source
		}
		return
	}
	for _, object := range marker.Objects {
		switch object.Role {
		case MarkerRolePage:
			marker.Page = object.Name
			marker.PageBytes = object.Bytes
			marker.PageSHA256 = object.SHA256
		case MarkerRoleSource:
			marker.Source = object.Name
		}
	}
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
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
		name == ProtectedFilename ||
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

func validMarkerObjectPath(name string) bool {
	if !utf8.ValidString(name) || name == "" || strings.HasPrefix(name, "/") ||
		strings.ContainsRune(name, '\\') {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		lowerSegment := strings.ToLower(segment)
		if segment == "" || segment == "." || segment == ".." ||
			strings.HasPrefix(lowerSegment, ".airplan-") || lowerSegment == MarkerFilename ||
			lowerSegment == CollectionMarkerFilename || lowerSegment == ProtectedFilename ||
			!pathrules.PortableSegment(segment) {
			return false
		}
		for _, r := range segment {
			if r == 0 || unicode.IsControl(r) {
				return false
			}
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
