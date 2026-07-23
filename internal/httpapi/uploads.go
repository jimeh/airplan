package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxMetadataBytes = int64(64 << 10)

type spooledFile struct {
	path        string
	name        string
	contentType string
	size        int64
	file        *os.File
}

func (s *Server) parseDocumentUpload(
	w http.ResponseWriter,
	r *http.Request,
) (DocumentUpload, func(), error) {
	reader, err := s.multipartReader(w, r)
	if err != nil {
		return DocumentUpload{}, func() {}, err
	}
	var metadata DocumentMetadata
	var gotMetadata bool
	var document *spooledFile
	cleanup := func() { cleanupSpooled(document) }

	for parts := 0; ; parts++ {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			cleanup()
			return DocumentUpload{}, func() {}, invalidMultipart(nextErr)
		}
		if parts >= 2 {
			_ = part.Close()
			cleanup()
			return DocumentUpload{}, func() {}, invalidRequest(
				"document upload contains too many parts",
			)
		}
		switch part.FormName() {
		case "metadata":
			if gotMetadata || part.FileName() != "" {
				_ = part.Close()
				cleanup()
				return DocumentUpload{}, func() {}, invalidRequest(
					"metadata must be one non-file part",
				)
			}
			if err = decodeLimitedJSON(part, maxMetadataBytes, &metadata); err != nil {
				_ = part.Close()
				cleanup()
				return DocumentUpload{}, func() {}, err
			}
			gotMetadata = true
		case "document":
			if document != nil {
				_ = part.Close()
				cleanup()
				return DocumentUpload{}, func() {}, invalidRequest(
					"document part is duplicated",
				)
			}
			document, err = s.spoolPart(part, s.options.MaxDocumentBytes)
			if err != nil {
				_ = part.Close()
				cleanup()
				return DocumentUpload{}, func() {}, err
			}
		default:
			_ = part.Close()
			cleanup()
			return DocumentUpload{}, func() {}, invalidRequest(
				"document upload contains an unknown part",
			)
		}
		_ = part.Close()
	}
	if !gotMetadata || document == nil {
		cleanup()
		return DocumentUpload{}, func() {}, invalidRequest(
			"document upload requires metadata and document parts",
		)
	}
	if err = validateDocumentMetadata(metadata); err != nil {
		cleanup()
		return DocumentUpload{}, func() {}, err
	}
	limit := lowerLimit(s.options.MaxDocumentBytes, metadata.MaxSize)
	if document.size > limit {
		cleanup()
		return DocumentUpload{}, func() {}, requestTooLarge(
			"document exceeds the effective size limit",
		)
	}
	if _, err = document.file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return DocumentUpload{}, func() {}, err
	}
	return DocumentUpload{
		Metadata: metadata,
		Document: document.file,
	}, cleanup, nil
}

func (s *Server) parseCollectionUpload(
	w http.ResponseWriter,
	r *http.Request,
) (CollectionUpload, func(), error) {
	reader, err := s.multipartReader(w, r)
	if err != nil {
		return CollectionUpload{}, func() {}, err
	}
	var metadata CollectionMetadata
	var gotMetadata bool
	var files []*spooledFile
	cleanup := func() {
		for _, file := range files {
			cleanupSpooled(file)
		}
	}
	seen := make(map[string]struct{})
	var total int64
	for parts := 0; ; parts++ {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			cleanup()
			return CollectionUpload{}, func() {}, invalidMultipart(nextErr)
		}
		if parts >= s.options.MaxCollectionFiles+1 {
			_ = part.Close()
			cleanup()
			return CollectionUpload{}, func() {}, invalidRequest(
				"collection contains too many parts",
			)
		}
		switch part.FormName() {
		case "metadata":
			if gotMetadata || part.FileName() != "" {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, invalidRequest(
					"metadata must be one non-file part",
				)
			}
			if err = decodeLimitedJSON(part, maxMetadataBytes, &metadata); err != nil {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, err
			}
			gotMetadata = true
		case "files":
			name, nameErr := multipartFilename(part)
			if nameErr != nil {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, nameErr
			}
			if _, duplicate := seen[name]; duplicate {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, invalidRequest(
					fmt.Sprintf("duplicate collection filename %q", name),
				)
			}
			file, spoolErr := s.spoolPart(
				part,
				s.options.MaxCollectionFileBytes,
			)
			if spoolErr != nil {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, spoolErr
			}
			file.name = name
			files = append(files, file)
			seen[name] = struct{}{}
			total += file.size
			if total > s.options.MaxCollectionTotalBytes {
				_ = part.Close()
				cleanup()
				return CollectionUpload{}, func() {}, requestTooLarge(
					"collection exceeds the server total size limit",
				)
			}
		default:
			_ = part.Close()
			cleanup()
			return CollectionUpload{}, func() {}, invalidRequest(
				"collection upload contains an unknown part",
			)
		}
		_ = part.Close()
	}
	if !gotMetadata || len(files) == 0 {
		cleanup()
		return CollectionUpload{}, func() {}, invalidRequest(
			"collection upload requires metadata and at least one file",
		)
	}
	if err = validateCollectionMetadata(metadata); err != nil {
		cleanup()
		return CollectionUpload{}, func() {}, err
	}
	fileLimit := lowerLimit(s.options.MaxCollectionFileBytes, metadata.MaxSize)
	totalLimit := lowerLimit(
		s.options.MaxCollectionTotalBytes,
		metadata.MaxTotalSize,
	)
	if total > totalLimit {
		cleanup()
		return CollectionUpload{}, func() {}, requestTooLarge(
			"collection exceeds the effective total size limit",
		)
	}
	result := CollectionUpload{Metadata: metadata}
	for _, file := range files {
		if file.size > fileLimit {
			cleanup()
			return CollectionUpload{}, func() {}, requestTooLarge(
				fmt.Sprintf("collection file %q exceeds the effective size limit", file.name),
			)
		}
		if _, err = file.file.Seek(0, io.SeekStart); err != nil {
			cleanup()
			return CollectionUpload{}, func() {}, err
		}
		result.Files = append(result.Files, CollectionFile{
			Name:        file.name,
			ContentType: file.contentType,
			Size:        file.size,
			Reader:      file.file,
		})
	}
	return result, cleanup, nil
}

func (s *Server) multipartReader(
	w http.ResponseWriter,
	r *http.Request,
) (*multipart.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" ||
		params["boundary"] == "" {
		return nil, invalidRequest(
			"Content-Type must be multipart/form-data with a boundary",
		)
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.options.MaxRequestBodyBytes)
	return multipart.NewReader(r.Body, params["boundary"]), nil
}

func (s *Server) spoolPart(
	part *multipart.Part,
	limit int64,
) (*spooledFile, error) {
	file, err := os.CreateTemp(s.options.TempDir, "airplan-upload-*")
	if err != nil {
		return nil, fmt.Errorf("create upload temporary file: %w", err)
	}
	spooled := &spooledFile{
		path:        file.Name(),
		contentType: part.Header.Get("Content-Type"),
		file:        file,
	}
	if chmodErr := file.Chmod(0o600); chmodErr != nil {
		cleanupSpooled(spooled)
		return nil, fmt.Errorf("secure upload temporary file: %w", chmodErr)
	}
	written, err := io.Copy(file, io.LimitReader(part, limit+1))
	spooled.size = written
	if err != nil {
		cleanupSpooled(spooled)
		return nil, invalidMultipart(err)
	}
	if written > limit {
		cleanupSpooled(spooled)
		return nil, requestTooLarge("multipart part exceeds the server size limit")
	}
	if err = file.Sync(); err != nil {
		cleanupSpooled(spooled)
		return nil, fmt.Errorf("flush upload temporary file: %w", err)
	}
	return spooled, nil
}

func cleanupSpooled(file *spooledFile) {
	if file == nil {
		return
	}
	if file.file != nil {
		_ = file.file.Close()
	}
	if file.path != "" {
		_ = os.Remove(file.path)
	}
}

func decodeLimitedJSON(reader io.Reader, limit int64, dst any) error {
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return requestTooLarge("request JSON exceeds the size limit")
		}
		return invalidRequest("request JSON is invalid")
	}
	if limited.N <= 0 {
		return requestTooLarge("request JSON exceeds the size limit")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalidRequest("request body must contain one JSON value")
	}
	return nil
}

func safeUploadName(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') ||
		strings.ContainsAny(name, `/\\`) || name == "." || name == ".." ||
		filepath.Base(name) != name {
		return "", invalidRequest("collection filename is unsafe")
	}
	return name, nil
}

func multipartFilename(part *multipart.Part) (string, error) {
	_, parameters, err := mime.ParseMediaType(
		part.Header.Get("Content-Disposition"),
	)
	if err != nil {
		return "", invalidRequest("collection part disposition is invalid")
	}
	return safeUploadName(parameters["filename"])
}

func validateDocumentMetadata(metadata DocumentMetadata) error {
	if metadata.Name == "" {
		return invalidRequest("document metadata name is required")
	}
	if metadata.MaxSize < 0 {
		return invalidRequest("document max_size must be positive")
	}
	switch metadata.Format {
	case "", "md", "html", "txt":
		return nil
	default:
		return invalidRequest("document format must be md, html, or txt")
	}
}

func validateCollectionMetadata(metadata CollectionMetadata) error {
	if metadata.MaxSize < 0 || metadata.MaxTotalSize < 0 {
		return invalidRequest("collection size limits must be positive")
	}
	return nil
}

func lowerLimit(server, requested int64) int64 {
	if requested > 0 && requested < server {
		return requested
	}
	return server
}

func invalidMultipart(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return requestTooLarge("multipart request exceeds the total size limit")
	}
	return invalidRequest("multipart request is malformed: " + err.Error())
}

func invalidRequest(detail string) error {
	return NewProblemError(
		http.StatusBadRequest,
		"invalid_request",
		"Invalid request",
		detail,
	)
}

func requestTooLarge(detail string) error {
	return NewProblemError(
		http.StatusRequestEntityTooLarge,
		"request_too_large",
		"Request too large",
		detail,
	)
}
