package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	contract "github.com/jimeh/airplan/api"
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

// Server is the typed REST adapter around the shared Airplan operation
// service. Use Handler to install request-ID middleware.
type Server struct {
	operations Operations
	auth       *BearerAuth
	options    Options
	openAPI    []byte
}

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

// NewHandler constructs the complete REST handler.
func NewHandler(operations Operations, options Options) (http.Handler, error) {
	server, err := NewServer(operations, options)
	if err != nil {
		return nil, err
	}
	return server.Handler(), nil
}

// Handler returns the REST server with request-ID propagation installed.
func (s *Server) Handler() http.Handler {
	return requestIDMiddleware(http.HandlerFunc(s.serveHTTP))
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		if r.Method != http.MethodGet {
			s.methodNotAllowed(w, r, http.MethodGet)
			return
		}
		writeJSON(w, http.StatusOK, Health{Status: "ok"})
	case "/openapi.yaml":
		if r.Method != http.MethodGet {
			s.methodNotAllowed(w, r, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.openAPI)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			s.auth.Wrap(http.HandlerFunc(s.serveAPI)).ServeHTTP(w, r)
			return
		}
		writeProblem(w, r, requestProblem(
			http.StatusNotFound,
			"not_found",
			"Not found",
			"The requested route does not exist.",
		))
	}
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/capabilities":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		result, err := s.operations.Capabilities(r.Context())
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/uploads/documents":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.uploadDocument(w, r)
	case "/api/v1/uploads/collections":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.uploadCollection(w, r)
	case "/api/v1/uploads/inspect":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var request TargetRequest
		if !s.decodeJSON(w, r, &request) ||
			!validateTarget(w, r, request.URLOrKey) {
			return
		}
		result, err := s.operations.InspectUpload(r.Context(), request)
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/uploads/get":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		s.getUpload(w, r)
	case "/api/v1/uploads/delete":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var request TargetRequest
		if !s.decodeJSON(w, r, &request) ||
			!validateTarget(w, r, request.URLOrKey) {
			return
		}
		result, err := s.operations.DeleteUpload(r.Context(), request)
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/uploads":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		result, err := s.operations.ListManifestUploads(r.Context())
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/storage/uploads":
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		result, err := s.operations.ListStorageUploads(r.Context())
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/sync":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var request SyncRequest
		if !s.decodeJSON(w, r, &request) ||
			!validateConcurrency(w, r, request.Concurrency) {
			return
		}
		result, err := s.operations.SyncManifest(r.Context(), request)
		result.Complete = len(result.Failures) == 0
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/purge/preview":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var request PurgePreviewRequest
		if !s.decodeJSON(w, r, &request) ||
			!validatePurgePreview(w, r, request) {
			return
		}
		result, err := s.operations.PreviewPurge(r.Context(), request)
		s.writeResult(w, r, http.StatusOK, result, err)
	case "/api/v1/purge":
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var request PurgeRequest
		if !s.decodeJSON(w, r, &request) ||
			!validatePurgeRequest(w, r, request) {
			return
		}
		result, err := s.operations.ExecutePurge(r.Context(), request)
		s.writeResult(w, r, http.StatusOK, result, err)
	default:
		writeProblem(w, r, requestProblem(
			http.StatusNotFound,
			"not_found",
			"Not found",
			"The requested API route does not exist.",
		))
	}
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	request, cleanup, err := s.parseDocumentUpload(w, r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer cleanup()
	result, err := s.operations.UploadDocument(r.Context(), request)
	s.writeResult(w, r, http.StatusCreated, result, err)
}

func (s *Server) uploadCollection(w http.ResponseWriter, r *http.Request) {
	request, cleanup, err := s.parseCollectionUpload(w, r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer cleanup()
	result, err := s.operations.UploadCollection(r.Context(), request)
	s.writeResult(w, r, http.StatusCreated, result, err)
}

func (s *Server) getUpload(w http.ResponseWriter, r *http.Request) {
	var request GetUploadRequest
	if !s.decodeJSON(w, r, &request) ||
		!validateTarget(w, r, request.URLOrKey) {
		return
	}
	download, err := s.operations.GetUpload(r.Context(), request)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if download.Body == nil {
		writeError(w, r, errors.New("operation returned a nil download body"))
		return
	}
	defer func() { _ = download.Body.Close() }()
	contentType := download.ContentType
	if _, _, parseErr := mime.ParseMediaType(contentType); parseErr != nil {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Airplan-Object-Key", download.Key)
	filename := safeDownloadFilename(download.Filename)
	w.Header().Set(
		"Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": filename}),
	)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, download.Body)
}

func safeDownloadFilename(filename string) string {
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) ||
		filename == "" {
		return "airplan-download"
	}
	return filename
}

func (s *Server) decodeJSON(
	w http.ResponseWriter,
	r *http.Request,
	destination any,
) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.options.MaxJSONBodyBytes)
	if err := decodeLimitedJSON(r.Body, s.options.MaxJSONBodyBytes, destination); err != nil {
		writeError(w, r, err)
		return false
	}
	return true
}

func (s *Server) writeResult(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	result any,
	err error,
) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requireMethod(
	w http.ResponseWriter,
	r *http.Request,
	method string,
) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeProblem(w, r, requestProblem(
		http.StatusMethodNotAllowed,
		"method_not_allowed",
		"Method not allowed",
		"The route does not support this HTTP method.",
	))
	return false
}

func (s *Server) methodNotAllowed(
	w http.ResponseWriter,
	r *http.Request,
	method string,
) {
	requireMethod(w, r, method)
}

func validateTarget(w http.ResponseWriter, r *http.Request, target string) bool {
	if target != "" {
		return true
	}
	writeError(w, r, invalidRequest("url_or_key is required"))
	return false
}

func validateConcurrency(
	w http.ResponseWriter,
	r *http.Request,
	concurrency int,
) bool {
	if concurrency == 0 || (concurrency >= 1 && concurrency <= 64) {
		return true
	}
	writeError(w, r, invalidRequest("concurrency must be between 1 and 64"))
	return false
}

func validatePurgePreview(
	w http.ResponseWriter,
	r *http.Request,
	request PurgePreviewRequest,
) bool {
	if request.Source != "manifest" && request.Source != "storage" {
		writeError(w, r, invalidRequest(
			"purge source must be manifest or storage",
		))
		return false
	}
	if !validateConcurrency(w, r, request.Concurrency) {
		return false
	}
	if !request.All && request.CreatedBefore == nil && request.Slug == "" {
		writeError(w, r, invalidRequest(
			"purge preview requires a filter or all=true",
		))
		return false
	}
	return true
}

func validatePurgeRequest(
	w http.ResponseWriter,
	r *http.Request,
	request PurgeRequest,
) bool {
	if len(request.UploadIDs) == 0 {
		writeError(w, r, invalidRequest("upload_ids must not be empty"))
		return false
	}
	seen := make(map[string]struct{}, len(request.UploadIDs))
	for _, uploadID := range request.UploadIDs {
		if uploadID == "" {
			writeError(w, r, invalidRequest("upload_ids must not contain empty IDs"))
			return false
		}
		if _, exists := seen[uploadID]; exists {
			writeError(w, r, invalidRequest("upload_ids must be unique"))
			return false
		}
		seen[uploadID] = struct{}{}
	}
	return true
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
