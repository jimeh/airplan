package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3filter"
	contract "github.com/jimeh/airplan/api"
	"github.com/jimeh/airplan/internal/httpapi/generated"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const (
	defaultDocumentBytes        = int64(10 << 20)
	defaultCollectionFileBytes  = int64(1 << 30)
	defaultCollectionTotalBytes = int64(2 << 30)
	defaultJSONBodyBytes        = int64(1 << 20)
	defaultBodyOverheadBytes    = int64(16 << 20)
	defaultCollectionFiles      = 100
)

// Options sets HTTP transport policy. Zero size values use conservative
// defaults; callers cannot disable a server hard limit.
type Options struct {
	Token                   string
	TempDir                 string
	OpenAPI                 []byte
	MaxRequestBodyBytes     int64
	MaxJSONBodyBytes        int64
	MaxDocumentBytes        int64
	MaxCollectionFileBytes  int64
	MaxCollectionTotalBytes int64
	MaxCollectionFiles      int
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
			s.auth.Wrap(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
	return requestIDMiddleware(secured), nil
}

func isMultipartUploadPath(path string) bool {
	return path == "/api/v1/uploads/documents" ||
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
	if !request.All && request.CreatedBefore.IsZero() && request.Slug == "" {
		return invalidRequest("purge preview requires a filter or all=true")
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
		options.MaxRequestBodyBytes = options.MaxCollectionTotalBytes +
			defaultBodyOverheadBytes
	}
}

func validateOptions(options Options) error {
	if options.MaxRequestBodyBytes <= 0 || options.MaxJSONBodyBytes <= 0 ||
		options.MaxDocumentBytes <= 0 ||
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
