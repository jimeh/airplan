package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3filter"
	contract "github.com/jimeh/airplan/api"
	"github.com/jimeh/airplan/internal/httpapi/generated"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const (
	defaultDocumentBytes        = int64(10 << 20)
	defaultTotalPageBytes       = int64(100 << 20)
	defaultGeneratedPageBytes   = int64(100 << 20)
	defaultAssetBytes           = int64(1 << 30)
	defaultDocumentAssetBytes   = int64(2 << 30)
	defaultCollectionFileBytes  = int64(1 << 30)
	defaultCollectionTotalBytes = int64(2 << 30)
	defaultJSONBodyBytes        = int64(1 << 20)
	defaultBodyOverheadBytes    = int64(16 << 20)
	defaultCollectionFiles      = 100
	defaultDocumentItems        = 100
)

// Options sets HTTP transport policy. Zero size values use conservative
// defaults; callers cannot disable a server hard limit.
type Options struct {
	Token                      string
	Logger                     *slog.Logger
	TempDir                    string
	OpenAPI                    []byte
	MaxRequestBodyBytes        int64
	MaxJSONBodyBytes           int64
	MaxDocumentBytes           int64
	MaxTotalPageBytes          int64
	MaxGeneratedPageBytes      int64
	MaxAssetBytes              int64
	MaxDocumentAssetTotalBytes int64
	MaxMetadataBytes           int64
	MaxDocumentItems           int
	MaxCollectionFileBytes     int64
	MaxCollectionTotalBytes    int64
	MaxCollectionFiles         int
}

// Server implements the OpenAPI-generated strict server interface around the
// shared Airplan operation service.
type Server struct {
	operations Operations
	auth       *BearerAuth
	options    Options
	openAPI    []byte
}

var _ generated.StrictServerInterface = (*Server)(nil)

// NewServer validates transport policy and constructs a REST adapter.
func NewServer(operations Operations, options Options) (*Server, error) {
	if operations == nil {
		return nil, errors.New("airplan HTTP API operations are nil")
	}
	auth, err := NewBearerAuth(options.Token)
	if err != nil {
		return nil, err
	}
	applyOptionDefaults(&options)
	if err = validateOptions(options); err != nil {
		return nil, err
	}
	if options.TempDir != "" {
		info, statErr := os.Stat(options.TempDir)
		if statErr != nil {
			return nil, fmt.Errorf("inspect HTTP upload temp dir: %w", statErr)
		}
		if !info.IsDir() {
			return nil, errors.New("airplan HTTP upload temp path is not a directory")
		}
	}
	schema := options.OpenAPI
	if schema == nil {
		schema = contract.OpenAPI()
	}
	return &Server{
		operations: operations,
		auth:       auth,
		options:    options,
		openAPI:    append([]byte(nil), schema...),
	}, nil
}

// NewHandler constructs the complete generated REST handler.
func NewHandler(operations Operations, options Options) (http.Handler, error) {
	server, err := NewServer(operations, options)
	if err != nil {
		return nil, err
	}
	return server.Handler()
}

// Handler registers the generated strict server and schema request validator,
// then adds authentication, size limits, and request IDs around it.
func (s *Server) Handler() (http.Handler, error) {
	spec, err := generated.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load generated OpenAPI schema: %w", err)
	}
	strict := generated.NewStrictHandlerWithOptions(
		s,
		nil,
		generated.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: func(
				w http.ResponseWriter, r *http.Request, err error,
			) {
				writeError(w, r, invalidRequest(err.Error()))
			},
			ResponseErrorHandlerFunc: func(
				w http.ResponseWriter, r *http.Request, err error,
			) {
				writeError(w, r, err)
			},
		},
	)
	routes := generated.Handler(strict)
	validatorOptions := func(excludeRequestBody bool) *nethttpmiddleware.Options {
		return &nethttpmiddleware.Options{
			DoNotValidateServers: true,
			Options: openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
				ExcludeRequestBody: excludeRequestBody,
			},
			ErrorHandlerWithOpts: func(
				_ context.Context, err error, w http.ResponseWriter,
				r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts,
			) {
				status := opts.StatusCode
				if status < 400 || status > 499 {
					status = http.StatusBadRequest
				}
				writeError(w, r, NewProblemError(
					status, "invalid_request", "Invalid request", err.Error(),
				))
			},
		}
	}
	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(
		spec, validatorOptions(false),
	)(routes)
	streamingValidator := nethttpmiddleware.OapiRequestValidatorWithOptions(
		spec, validatorOptions(true),
	)(routes)

	secured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && isMultipartUploadPath(r.URL.Path) {
				if err := validateMultipartContentType(r); err != nil {
					writeError(w, r, err)
					return
				}
				r.Body = http.MaxBytesReader(
					w, r.Body, s.options.MaxRequestBodyBytes,
				)
				streamingValidator.ServeHTTP(w, r)
				return
			} else if r.Method == http.MethodPost {
				r.Body = http.MaxBytesReader(
					w, r.Body, s.options.MaxJSONBodyBytes,
				)
			}
			validator.ServeHTTP(w, r)
		})
		if len(r.URL.Path) >= len("/api/v1/") &&
			r.URL.Path[:len("/api/v1/")] == "/api/v1/" {
			s.auth.WrapWithLogger(next, s.options.Logger, "rest").ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
	return requestIDMiddleware(secured), nil
}

func isMultipartUploadPath(path string) bool {
	return path == "/api/v1/uploads/documents" ||
		path == "/api/v1/uploads/documents/revisions" ||
		path == "/api/v1/uploads/documents/update" ||
		path == "/api/v1/uploads/collections"
}

// Multipart bodies intentionally bypass generic schema body validation because
// kin-openapi buffers them. The route validator still enforces method, path,
// security shape, and parameters; this adapter checks media type while the
// bounded streaming parser validates parts and metadata against the schema.
func validateMultipartContentType(r *http.Request) error {
	mediaType, parameters, err := mime.ParseMediaType(
		r.Header.Get("Content-Type"),
	)
	if err != nil || mediaType != "multipart/form-data" ||
		parameters["boundary"] == "" {
		return invalidRequest(
			"Content-Type must be multipart/form-data with a boundary",
		)
	}
	return nil
}

// GetCapabilities implements the generated operation.
func (s *Server) GetCapabilities(
	ctx context.Context, _ generated.GetCapabilitiesRequestObject,
) (generated.GetCapabilitiesResponseObject, error) {
	result, err := s.operations.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	result.Limits = generated.UploadLimits{
		DocumentBytes:        s.options.MaxDocumentBytes,
		CollectionFileBytes:  s.options.MaxCollectionFileBytes,
		CollectionTotalBytes: s.options.MaxCollectionTotalBytes,
	}
	result.DocumentBundle = &generated.DocumentBundleCapabilities{
		ManagedPages: true, Assets: true, CanonicalRevisionRoute: true,
		MaxItems:              s.options.MaxDocumentItems,
		MaxPageBytes:          s.options.MaxDocumentBytes,
		MaxTotalPageBytes:     s.options.MaxTotalPageBytes,
		MaxGeneratedPageBytes: s.options.MaxGeneratedPageBytes,
		MaxAssetBytes:         s.options.MaxAssetBytes,
		MaxTotalAssetBytes:    s.options.MaxDocumentAssetTotalBytes,
		MaxMetadataBytes:      s.options.MaxMetadataBytes,
		MaxRequestBytes:       s.options.MaxRequestBodyBytes,
	}
	return generated.GetCapabilities200JSONResponse(result), nil
}

// UploadDocument implements the generated streaming multipart operation.
func (s *Server) UploadDocument(
	ctx context.Context, request generated.UploadDocumentRequestObject,
) (generated.UploadDocumentResponseObject, error) {
	upload, cleanup, err := s.parseDocumentUpload(request.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	result, err := s.operations.UploadDocument(ctx, upload)
	if err != nil {
		return nil, err
	}
	return generated.UploadDocument201JSONResponse(result), nil
}

// UploadCollection implements the generated streaming multipart operation.
func (s *Server) UploadCollection(
	ctx context.Context, request generated.UploadCollectionRequestObject,
) (generated.UploadCollectionResponseObject, error) {
	upload, cleanup, err := s.parseCollectionUpload(request.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	result, err := s.operations.UploadCollection(ctx, upload)
	if err != nil {
		return nil, err
	}
	return generated.UploadCollection201JSONResponse(result), nil
}

func (s *Server) UpdateDocument(
	ctx context.Context, request generated.UpdateDocumentRequestObject,
) (generated.UpdateDocumentResponseObject, error) {
	upload, cleanup, err := s.parseDocumentRevisionUpload(request.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	result, err := s.operations.CreateDocumentRevision(ctx, upload)
	if err != nil {
		return nil, err
	}
	return generated.UpdateDocument201JSONResponse(result), nil
}

// CreateDocumentRevision implements the canonical revision operation.
func (s *Server) CreateDocumentRevision(
	ctx context.Context, request generated.CreateDocumentRevisionRequestObject,
) (generated.CreateDocumentRevisionResponseObject, error) {
	upload, cleanup, err := s.parseDocumentRevisionUpload(request.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	result, err := s.operations.CreateDocumentRevision(ctx, upload)
	if err != nil {
		return nil, err
	}
	return generated.CreateDocumentRevision201JSONResponse(result), nil
}

func (s *Server) PlanDocumentUpgrade(
	ctx context.Context, request generated.PlanDocumentUpgradeRequestObject,
) (generated.PlanDocumentUpgradeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	result, err := s.operations.PlanDocumentUpgrade(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.PlanDocumentUpgrade200JSONResponse(result), nil
}

func (s *Server) ExecuteDocumentUpgrade(
	ctx context.Context, request generated.ExecuteDocumentUpgradeRequestObject,
) (generated.ExecuteDocumentUpgradeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	result, err := s.operations.ExecuteDocumentUpgrade(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.ExecuteDocumentUpgrade200JSONResponse(result), nil
}

func (s *Server) PlanBulkUpgrade(
	ctx context.Context, request generated.PlanBulkUpgradeRequestObject,
) (generated.PlanBulkUpgradeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	result, err := s.operations.PlanBulkUpgrade(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.PlanBulkUpgrade200JSONResponse(result), nil
}

func (s *Server) ExecuteBulkUpgrade(
	ctx context.Context, request generated.ExecuteBulkUpgradeRequestObject,
) (generated.ExecuteBulkUpgradeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	result, err := s.operations.ExecuteBulkUpgrade(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.ExecuteBulkUpgrade200JSONResponse(result), nil
}

// InspectUpload implements the generated operation.
func (s *Server) InspectUpload(
	ctx context.Context, request generated.InspectUploadRequestObject,
) (generated.InspectUploadResponseObject, error) {
	if request.Body == nil || request.Body.URLOrKey == "" {
		return nil, invalidRequest("url_or_key is required")
	}
	result, err := s.operations.InspectUpload(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.InspectUpload200JSONResponse(result), nil
}

// GetUpload implements the generated streaming response operation.
func (s *Server) GetUpload(
	ctx context.Context, request generated.GetUploadRequestObject,
) (generated.GetUploadResponseObject, error) {
	if request.Body == nil || request.Body.URLOrKey == "" {
		return nil, invalidRequest("url_or_key is required")
	}
	download, err := s.operations.GetUpload(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	if download.Body == nil {
		return nil, errors.New("operation returned a nil download body")
	}
	filename := safeDownloadFilename(download.Filename)
	disposition := mime.FormatMediaType(
		"attachment", map[string]string{"filename": filename},
	)
	key := download.Key
	return generated.GetUpload200ApplicationOctetStreamResponse{
		Body: download.Body,
		Headers: generated.GetUpload200ResponseHeaders{
			ContentType:        download.ContentType,
			ContentDisposition: disposition,
			XAirplanObjectKey:  key,
		},
	}, nil
}

// DeleteUpload implements the generated operation.
func (s *Server) DeleteUpload(
	ctx context.Context, request generated.DeleteUploadRequestObject,
) (generated.DeleteUploadResponseObject, error) {
	if request.Body == nil || request.Body.URLOrKey == "" {
		return nil, invalidRequest("url_or_key is required")
	}
	result, err := s.operations.DeleteUpload(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.DeleteUpload200JSONResponse(result), nil
}

// ProtectUpload implements the generated operation.
func (s *Server) ProtectUpload(
	ctx context.Context, request generated.ProtectUploadRequestObject,
) (generated.ProtectUploadResponseObject, error) {
	if request.Body == nil || request.Body.URLOrKey == "" {
		return nil, invalidRequest("url_or_key is required")
	}
	result, err := s.operations.ProtectUpload(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.ProtectUpload200JSONResponse(result), nil
}

// UnprotectUpload implements the generated operation.
func (s *Server) UnprotectUpload(
	ctx context.Context, request generated.UnprotectUploadRequestObject,
) (generated.UnprotectUploadResponseObject, error) {
	if request.Body == nil || request.Body.URLOrKey == "" {
		return nil, invalidRequest("url_or_key is required")
	}
	result, err := s.operations.UnprotectUpload(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.UnprotectUpload200JSONResponse(result), nil
}

// ListManifestUploads implements the generated operation.
func (s *Server) ListManifestUploads(
	ctx context.Context, _ generated.ListManifestUploadsRequestObject,
) (generated.ListManifestUploadsResponseObject, error) {
	result, err := s.operations.ListManifestUploads(ctx)
	if err != nil {
		return nil, err
	}
	return generated.ListManifestUploads200JSONResponse(result), nil
}

// ListStorageUploads implements the generated operation.
func (s *Server) ListStorageUploads(
	ctx context.Context, _ generated.ListStorageUploadsRequestObject,
) (generated.ListStorageUploadsResponseObject, error) {
	result, err := s.operations.ListStorageUploads(ctx)
	if err != nil {
		return nil, err
	}
	return generated.ListStorageUploads200JSONResponse(result), nil
}

// SyncManifest implements the generated operation.
func (s *Server) SyncManifest(
	ctx context.Context, request generated.SyncManifestRequestObject,
) (generated.SyncManifestResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	result, err := s.operations.SyncManifest(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	result.Complete = len(result.Failures) == 0
	return generated.SyncManifest200JSONResponse(result), nil
}

// PreviewPurge implements the generated operation.
func (s *Server) PreviewPurge(
	ctx context.Context, request generated.PreviewPurgeRequestObject,
) (generated.PreviewPurgeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	if err := validatePurgePreview(*request.Body); err != nil {
		return nil, err
	}
	result, err := s.operations.PreviewPurge(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.PreviewPurge200JSONResponse(result), nil
}

// ExecutePurge implements the generated operation.
func (s *Server) ExecutePurge(
	ctx context.Context, request generated.ExecutePurgeRequestObject,
) (generated.ExecutePurgeResponseObject, error) {
	if request.Body == nil {
		return nil, invalidRequest("request body is required")
	}
	if err := validatePurgeRequest(*request.Body); err != nil {
		return nil, err
	}
	result, err := s.operations.ExecutePurge(ctx, *request.Body)
	if err != nil {
		return nil, err
	}
	return generated.ExecutePurge200JSONResponse(result), nil
}

// Health implements the generated public liveness operation.
func (s *Server) Health(
	context.Context, generated.HealthRequestObject,
) (generated.HealthResponseObject, error) {
	return generated.Health200JSONResponse(Health{Status: "ok"}), nil
}

// GetOpenAPI implements the generated public schema operation.
func (s *Server) GetOpenAPI(
	context.Context, generated.GetOpenAPIRequestObject,
) (generated.GetOpenAPIResponseObject, error) {
	return generated.GetOpenAPI200ApplicationYamlResponse{
		Body: bytes.NewReader(s.openAPI), ContentLength: int64(len(s.openAPI)),
	}, nil
}

func safeDownloadFilename(filename string) string {
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) ||
		filename == "" {
		return "airplan-download"
	}
	return filename
}

func validatePurgePreview(request PurgePreviewRequest) error {
	if request.Source != "manifest" && request.Source != "storage" {
		return invalidRequest("purge source must be manifest or storage")
	}
	if request.Concurrency != 0 &&
		(request.Concurrency < 1 || request.Concurrency > 64) {
		return invalidRequest("concurrency must be between 1 and 64")
	}
	if !request.All &&
		(request.CreatedBefore == nil || request.CreatedBefore.IsZero()) &&
		request.Slug == "" {
		return invalidRequest("purge preview requires a filter or all=true")
	}
	if request.Slug != "" {
		if _, err := path.Match(request.Slug, ""); err != nil {
			return invalidRequest("purge slug must be a valid glob pattern")
		}
	}
	return nil
}

func validatePurgeRequest(request PurgeRequest) error {
	if len(request.UploadIds) == 0 {
		return invalidRequest("upload_ids must not be empty")
	}
	seen := make(map[string]struct{}, len(request.UploadIds))
	for _, uploadID := range request.UploadIds {
		if uploadID == "" {
			return invalidRequest("upload_ids must not contain empty IDs")
		}
		if _, exists := seen[uploadID]; exists {
			return invalidRequest("upload_ids must be unique")
		}
		seen[uploadID] = struct{}{}
	}
	return nil
}

func applyOptionDefaults(options *Options) {
	if options.MaxDocumentBytes == 0 {
		options.MaxDocumentBytes = defaultDocumentBytes
	}
	if options.MaxTotalPageBytes == 0 {
		options.MaxTotalPageBytes = defaultTotalPageBytes
	}
	if options.MaxGeneratedPageBytes == 0 {
		options.MaxGeneratedPageBytes = defaultGeneratedPageBytes
	}
	if options.MaxAssetBytes == 0 {
		options.MaxAssetBytes = defaultAssetBytes
	}
	if options.MaxDocumentAssetTotalBytes == 0 {
		options.MaxDocumentAssetTotalBytes = defaultDocumentAssetBytes
	}
	if options.MaxMetadataBytes == 0 {
		options.MaxMetadataBytes = defaultMaxMetadataBytes
	}
	if options.MaxDocumentItems == 0 {
		options.MaxDocumentItems = defaultDocumentItems
	}
	if options.MaxCollectionFileBytes == 0 {
		options.MaxCollectionFileBytes = defaultCollectionFileBytes
	}
	if options.MaxCollectionTotalBytes == 0 {
		options.MaxCollectionTotalBytes = defaultCollectionTotalBytes
	}
	if options.MaxJSONBodyBytes == 0 {
		options.MaxJSONBodyBytes = defaultJSONBodyBytes
	}
	if options.MaxCollectionFiles == 0 {
		options.MaxCollectionFiles = defaultCollectionFiles
	}
	if options.MaxRequestBodyBytes == 0 {
		documentEnvelope := options.MaxTotalPageBytes +
			options.MaxDocumentAssetTotalBytes
		payload := max(options.MaxCollectionTotalBytes, documentEnvelope)
		options.MaxRequestBodyBytes = payload + defaultBodyOverheadBytes
	}
}

func validateOptions(options Options) error {
	if options.MaxRequestBodyBytes <= 0 || options.MaxJSONBodyBytes <= 0 ||
		options.MaxDocumentBytes <= 0 ||
		options.MaxTotalPageBytes <= 0 ||
		options.MaxGeneratedPageBytes <= 0 || options.MaxAssetBytes <= 0 ||
		options.MaxDocumentAssetTotalBytes <= 0 ||
		options.MaxMetadataBytes <= 0 || options.MaxDocumentItems <= 0 ||
		options.MaxCollectionFileBytes <= 0 ||
		options.MaxCollectionTotalBytes <= 0 ||
		options.MaxCollectionFiles <= 0 {
		return errors.New("airplan HTTP API limits must be positive")
	}
	if options.MaxCollectionFileBytes > options.MaxCollectionTotalBytes {
		return errors.New(
			"airplan collection file limit exceeds collection total limit",
		)
	}
	if options.MaxDocumentBytes > options.MaxTotalPageBytes {
		return errors.New("airplan document page limit exceeds document page total limit")
	}
	if options.MaxAssetBytes > options.MaxDocumentAssetTotalBytes {
		return errors.New("airplan document asset limit exceeds document asset total limit")
	}
	if options.MaxDocumentBytes > defaultDocumentBytes ||
		options.MaxTotalPageBytes > defaultTotalPageBytes ||
		options.MaxAssetBytes > defaultAssetBytes ||
		options.MaxDocumentAssetTotalBytes > defaultDocumentAssetBytes ||
		options.MaxDocumentItems > defaultDocumentItems {
		return errors.New("airplan HTTP document limits exceed core limits")
	}
	if options.MaxGeneratedPageBytes != defaultGeneratedPageBytes {
		return errors.New("airplan HTTP generated page limit must match the core limit")
	}
	if options.MaxMetadataBytes != defaultMaxMetadataBytes {
		return errors.New("airplan HTTP metadata limit must match the protocol limit")
	}
	documentEnvelope := options.MaxTotalPageBytes + options.MaxDocumentAssetTotalBytes
	minimumPayload := max(options.MaxCollectionTotalBytes, documentEnvelope)
	if options.MaxRequestBodyBytes < minimumPayload+defaultBodyOverheadBytes {
		return errors.New("airplan HTTP request limit cannot contain advertised upload limits")
	}
	return nil
}

func parseMultipartReader(
	reader *multipart.Reader,
) (*multipart.Reader, error) {
	if reader == nil {
		return nil, invalidRequest("multipart request body is required")
	}
	return reader, nil
}
