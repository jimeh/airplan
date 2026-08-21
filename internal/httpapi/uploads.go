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
	"unicode"
	"unicode/utf8"
)

const defaultMaxMetadataBytes = int64(256 << 10)

type spooledFile struct {
	path        string
	name        string
	contentType string
	size        int64
	file        *os.File
}

func (s *Server) parseDocumentUpload(
	requestReader *multipart.Reader,
) (DocumentUpload, func(), error) {
	var metadata DocumentMetadata
	parts, cleanup, err := s.parseDocumentMultipart(
		requestReader, &metadata,
		func() (documentMultipartSpec, error) {
			if err := validateDocumentMetadata(metadata); err != nil {
				return documentMultipartSpec{}, err
			}
			return documentSpec(
				metadata.Name,
				metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
				metadata.MaxPageSize,
				metadata.MaxTotalPageSize, metadata.MaxAssetSize,
				metadata.MaxTotalSize, metadata.Pages, metadata.Assets,
			)
		},
	)
	if err != nil {
		return DocumentUpload{}, func() {}, err
	}
	return DocumentUpload{
		Metadata: metadata, Document: parts.document.file,
		DocumentSize: parts.document.size,
		Pages:        parts.pages, Assets: parts.assets,
	}, cleanup, nil
}

func (s *Server) parseDocumentRevisionUpload(
	requestReader *multipart.Reader,
) (CreateDocumentRevisionUpload, func(), error) {
	var metadata CreateDocumentRevisionMetadata
	parts, cleanup, err := s.parseDocumentMultipart(
		requestReader, &metadata,
		func() (documentMultipartSpec, error) {
			if strings.TrimSpace(metadata.Target) == "" {
				return documentMultipartSpec{}, invalidRequest(
					"document revision target is required",
				)
			}
			if err := validateRevisionMetadata(metadata); err != nil {
				return documentMultipartSpec{}, err
			}
			return documentSpec(
				metadata.Name,
				metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
				metadata.MaxPageSize,
				metadata.MaxTotalPageSize, metadata.MaxAssetSize,
				metadata.MaxTotalSize, metadata.Pages, metadata.Assets,
			)
		},
	)
	if err != nil {
		return CreateDocumentRevisionUpload{}, func() {}, err
	}
	return CreateDocumentRevisionUpload{
		Metadata: metadata, Document: parts.document.file,
		DocumentSize: parts.document.size,
		Pages:        parts.pages, Assets: parts.assets,
	}, cleanup, nil
}

type documentMultipartSpec struct {
	name            string
	pageLimit       int64
	totalPageLimit  int64
	assetLimit      int64
	totalAssetLimit int64
	pages           []DocumentPageDescriptor
	assets          []DocumentAssetDescriptor
}

type parsedDocumentParts struct {
	document *spooledFile
	pages    []DocumentPage
	assets   []DocumentAsset
}

func documentSpec(
	name string, maxSize, maxPageSize, maxTotalPageSize, maxAssetSize,
	maxTotalSize int64, pages []DocumentPageDescriptor,
	assets []DocumentAssetDescriptor,
) (documentMultipartSpec, error) {
	pageLimit, err := requestedPageLimit(maxSize, maxPageSize)
	if err != nil {
		return documentMultipartSpec{}, err
	}
	return documentMultipartSpec{
		name: name, pageLimit: pageLimit,
		totalPageLimit: maxTotalPageSize, assetLimit: maxAssetSize,
		totalAssetLimit: maxTotalSize, pages: pages, assets: assets,
	}, nil
}

func (s *Server) parseDocumentMultipart(
	requestReader *multipart.Reader, metadata any,
	describe func() (documentMultipartSpec, error),
) (parsedDocumentParts, func(), error) {
	reader, err := parseMultipartReader(requestReader)
	if err != nil {
		return parsedDocumentParts{}, func() {}, err
	}
	var spooled []*spooledFile
	cleanup := func() {
		for _, file := range spooled {
			cleanupSpooled(file)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			cleanup()
			panic(recovered)
		}
	}()
	fail := func(err error) (parsedDocumentParts, func(), error) {
		cleanup()
		return parsedDocumentParts{}, func() {}, err
	}

	part, err := nextRequiredPart(reader, "metadata")
	if err != nil {
		return fail(err)
	}
	if part.FileName() != "" {
		_ = part.Close()
		return fail(invalidRequest("metadata must be a non-file part"))
	}
	if err = decodeLimitedJSON(part, s.options.MaxMetadataBytes, metadata); err != nil {
		_ = part.Close()
		return fail(err)
	}
	_ = part.Close()
	spec, err := describe()
	if err != nil {
		return fail(err)
	}
	if err = validateDocumentDescriptors(spec, s.options.MaxDocumentItems); err != nil {
		return fail(err)
	}

	pageLimit := lowerLimit(s.options.MaxDocumentBytes, spec.pageLimit)
	totalPageLimit := lowerLimit(s.options.MaxTotalPageBytes, spec.totalPageLimit)
	assetLimit := lowerLimit(s.options.MaxAssetBytes, spec.assetLimit)
	totalAssetLimit := lowerLimit(s.options.MaxDocumentAssetTotalBytes, spec.totalAssetLimit)
	var declaredAssetTotal int64
	for _, descriptor := range spec.assets {
		if descriptor.Size > assetLimit {
			return fail(requestTooLarge(fmt.Sprintf(
				"asset %q exceeds the effective size limit", descriptor.Path,
			)))
		}
		if descriptor.Size > int64(^uint64(0)>>1)-declaredAssetTotal {
			return fail(invalidRequest("document asset total size is out of range"))
		}
		declaredAssetTotal += descriptor.Size
	}
	if declaredAssetTotal > totalAssetLimit {
		return fail(requestTooLarge(
			"document assets exceed the effective total size limit",
		))
	}

	part, err = nextRequiredPart(reader, "document")
	if err != nil {
		return fail(err)
	}
	document, err := s.spoolPart(part, pageLimit)
	_ = part.Close()
	if err != nil {
		return fail(err)
	}
	spooled = append(spooled, document)
	totalPages := document.size
	if totalPages > totalPageLimit {
		return fail(requestTooLarge("managed pages exceed the effective total size limit"))
	}

	result := parsedDocumentParts{document: document}
	for _, descriptor := range spec.pages {
		part, err = nextRequiredPart(reader, "pages")
		if err != nil {
			return fail(err)
		}
		file, spoolErr := s.spoolPart(part, pageLimit)
		_ = part.Close()
		if spoolErr != nil {
			return fail(spoolErr)
		}
		spooled = append(spooled, file)
		totalPages += file.size
		if totalPages > totalPageLimit {
			return fail(requestTooLarge(fmt.Sprintf(
				"managed page %q exceeds the effective total size limit",
				descriptor.Path,
			)))
		}
		if _, err = file.file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		result.pages = append(result.pages, DocumentPage{
			DocumentPageDescriptor: descriptor, Reader: file.file, Size: file.size,
		})
	}

	var totalAssets int64
	for _, descriptor := range spec.assets {
		part, err = nextRequiredPart(reader, "assets")
		if err != nil {
			return fail(err)
		}
		partLimit := assetLimit
		if descriptor.Size < partLimit {
			partLimit = descriptor.Size
		}
		file, spoolErr := s.spoolPart(part, partLimit)
		_ = part.Close()
		if spoolErr != nil {
			return fail(spoolErr)
		}
		spooled = append(spooled, file)
		if file.size != descriptor.Size {
			return fail(invalidRequest(fmt.Sprintf(
				"asset %q size does not match its descriptor", descriptor.Path,
			)))
		}
		totalAssets += file.size
		if totalAssets > totalAssetLimit {
			return fail(requestTooLarge(fmt.Sprintf(
				"asset %q exceeds the effective total size limit",
				descriptor.Path,
			)))
		}
		if _, err = file.file.Seek(0, io.SeekStart); err != nil {
			return fail(err)
		}
		result.assets = append(result.assets, DocumentAsset{
			DocumentAssetDescriptor: descriptor, Reader: file.file,
		})
	}

	extra, nextErr := reader.NextPart()
	if nextErr == nil {
		_ = extra.Close()
		return fail(invalidRequest("document multipart contains an extra part"))
	}
	if !errors.Is(nextErr, io.EOF) {
		return fail(invalidMultipart(nextErr))
	}
	if _, err = document.file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return result, cleanup, nil
}

func nextRequiredPart(reader *multipart.Reader, name string) (*multipart.Part, error) {
	part, err := reader.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, invalidRequest(fmt.Sprintf("document multipart is missing %s part", name))
	}
	if err != nil {
		return nil, invalidMultipart(err)
	}
	if part.FormName() != name {
		_ = part.Close()
		return nil, invalidRequest(fmt.Sprintf(
			"document multipart expected %s part, got %q", name, part.FormName(),
		))
	}
	return part, nil
}

func (s *Server) parseCollectionUpload(
	requestReader *multipart.Reader,
) (CollectionUpload, func(), error) {
	reader, err := parseMultipartReader(requestReader)
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
	defer func() {
		if recovered := recover(); recovered != nil {
			cleanup()
			panic(recovered)
		}
	}()
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
			if err = decodeLimitedJSON(part, s.options.MaxMetadataBytes, &metadata); err != nil {
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
	if utf8.RuneCountInString(metadata.Name) > 255 {
		return invalidRequest("document metadata name is too long")
	}
	if err := validateDocumentLimits(
		metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
		metadata.MaxPageSize, metadata.MaxTotalPageSize,
		metadata.MaxAssetSize, metadata.MaxTotalSize,
	); err != nil {
		return err
	}
	switch metadata.Format {
	case "", "md", "html", "txt":
		return nil
	default:
		return invalidRequest("document format must be md, html, or txt")
	}
}

func validateRevisionMetadata(metadata CreateDocumentRevisionMetadata) error {
	if utf8.RuneCountInString(metadata.Name) > 255 {
		return invalidRequest("document metadata name is too long")
	}
	if err := validateDocumentLimits(
		metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
		metadata.MaxPageSize, metadata.MaxTotalPageSize,
		metadata.MaxAssetSize, metadata.MaxTotalSize,
	); err != nil {
		return err
	}
	switch metadata.Format {
	case "", "md", "html", "txt":
		return nil
	default:
		return invalidRequest("document format must be md, html, or txt")
	}
}

func validateDocumentLimits(
	maxSize, maxPageSize, maxTotalPageSize, maxAssetSize, maxTotalSize int64,
) error {
	if maxSize < 0 || maxPageSize < 0 || maxTotalPageSize < 0 ||
		maxAssetSize < 0 || maxTotalSize < 0 {
		return invalidRequest("document size limits must be positive")
	}
	_, err := requestedPageLimit(maxSize, maxPageSize)
	return err
}

func requestedPageLimit(maxSize, maxPageSize int64) (int64, error) {
	if maxSize > 0 && maxPageSize > 0 && maxSize != maxPageSize {
		return 0, invalidRequest("max_size and max_page_size must match when both are set")
	}
	if maxPageSize > 0 {
		return maxPageSize, nil
	}
	return maxSize, nil
}

func validateDocumentDescriptors(spec documentMultipartSpec, maxItems int) error {
	if len(spec.pages)+len(spec.assets)+1 > maxItems {
		return invalidRequest(fmt.Sprintf(
			"document contains too many items; maximum is %d", maxItems,
		))
	}
	if len(spec.pages)+len(spec.assets) > 0 {
		if _, err := safeUploadName(spec.name); err != nil {
			return invalidRequest("bundled document name must be a safe filename")
		}
	}
	seen := make(map[string]string, len(spec.pages)+len(spec.assets)+1)
	if spec.name != "" {
		seen[strings.ToLower(spec.name)] = spec.name
	}
	for _, descriptor := range spec.pages {
		if err := validateLogicalPath(descriptor.Path); err != nil {
			return err
		}
		if descriptor.Format != "" && !descriptor.Format.Valid() {
			return invalidRequest(fmt.Sprintf(
				"managed page %q format must be md or txt", descriptor.Path,
			))
		}
		if err := recordLogicalPath(seen, descriptor.Path); err != nil {
			return err
		}
	}
	for _, descriptor := range spec.assets {
		if err := validateLogicalPath(descriptor.Path); err != nil {
			return err
		}
		if descriptor.Size < 0 {
			return invalidRequest(fmt.Sprintf(
				"asset %q size must not be negative", descriptor.Path,
			))
		}
		if strings.ContainsAny(descriptor.ContentType, "\r\n\x00") {
			return invalidRequest(fmt.Sprintf(
				"asset %q content type is invalid", descriptor.Path,
			))
		}
		if err := recordLogicalPath(seen, descriptor.Path); err != nil {
			return err
		}
	}
	return nil
}

func validateLogicalPath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return invalidRequest(fmt.Sprintf("bundle path %q is invalid", value))
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return invalidRequest(fmt.Sprintf("bundle path %q is invalid", value))
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." ||
			strings.HasPrefix(strings.ToLower(segment), ".airplan-") {
			return invalidRequest(fmt.Sprintf("bundle path %q is invalid", value))
		}
	}
	return nil
}

func recordLogicalPath(seen map[string]string, value string) error {
	folded := strings.ToLower(value)
	if previous, ok := seen[folded]; ok {
		return invalidRequest(fmt.Sprintf(
			"bundle path %q conflicts with %q", value, previous,
		))
	}
	seen[folded] = value
	return nil
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
