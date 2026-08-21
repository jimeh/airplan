package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/jimeh/airplan/internal/httpapi"
)

type httpTransport struct {
	client     *httpapi.Client
	repository string
}

func newHTTPTransport(cfg *Config) (operationTransport, error) {
	if cfg.APIURL == "" || cfg.APIToken == "" {
		return nil, errors.New(
			"airplan: api_url and api_token are required for the airplan backend",
		)
	}
	client, err := httpapi.NewClient(cfg.APIURL, cfg.APIToken, &http.Client{})
	if err != nil {
		return nil, fmt.Errorf("airplan: construct HTTP transport: %w", err)
	}
	return &httpTransport{client: client, repository: cfg.Repository}, nil
}

func (t *httpTransport) Upload(
	ctx context.Context, in Input,
) (*Result, error) {
	repository := in.RepositoryURL
	if repository == "" {
		var err error
		repository, err = resolveRepository(ctx, t.repository, in.Name, "")
		if err != nil {
			return nil, err
		}
	}
	result, err := t.client.UploadDocument(ctx, httpapi.DocumentMetadata{
		Name: in.Name, Format: httpapi.DocumentMetadataFormat(in.Format),
		Title: in.Title, Slug: in.Slug,
		Lang: in.Lang, RepositoryURL: repository,
		MaxSize: portableUploadLimit(in.MaxSize),
	}, in.Reader)
	if err != nil {
		return nil, transportError(err)
	}
	core := coreUploadResult(result)
	return &core.Result, nil
}

func (t *httpTransport) UploadDocument(
	ctx context.Context, in DocumentInput,
) (*DocumentResult, error) {
	bundled := len(in.Pages) != 0 || len(in.Assets) != 0
	var capability *httpapi.DocumentBundleCapabilities
	var err error
	if bundled {
		capability, err = t.documentBundleCapabilities(ctx)
		if err != nil {
			return nil, err
		}
	}
	prepared := in
	if capability != nil {
		var cleanup func()
		prepared, cleanup, err = spoolHTTPDocumentPages(ctx, in, *capability)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}
	upload, err := t.wireDocumentUpload(ctx, prepared, !bundled)
	if err != nil {
		return nil, err
	}
	if capability != nil {
		if err = validateBundleCapability(*capability, upload); err != nil {
			return nil, err
		}
	}
	result, err := t.client.UploadDocumentBundle(ctx, upload)
	if err != nil {
		return nil, transportError(err)
	}
	document := coreDocumentResult(result)
	if len(document.Pages) == 0 {
		document.Pages = []PageResult{entryPageResult(in.Entry.Path, document.Result)}
	}
	return &document, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (t *httpTransport) wireDocumentUpload(
	ctx context.Context, in DocumentInput, legacy bool,
) (httpapi.DocumentUpload, error) {
	pages, assets, repository, err := t.wireDocumentParts(ctx, in)
	if err != nil {
		return httpapi.DocumentUpload{}, err
	}
	metadata := httpapi.DocumentMetadata{
		Name: in.Entry.Path, Format: httpapi.DocumentMetadataFormat(in.Entry.Format),
		Title: firstNonEmpty(in.Title, in.Entry.Title), Slug: in.Slug,
		Lang: in.Entry.Lang, RepositoryURL: repository,
	}
	if legacy {
		metadata.MaxSize = portableUploadLimit(in.MaxPageSize) //nolint:staticcheck // Deprecated server compatibility.
	} else {
		metadata.MaxPageSize = portableUploadLimit(in.MaxPageSize)
		metadata.MaxTotalPageSize = portableUploadLimit(in.MaxTotalPageSize)
		metadata.MaxAssetSize = portableUploadLimit(in.MaxAssetSize)
		metadata.MaxTotalSize = portableUploadLimit(in.MaxTotalSize)
	}
	for _, page := range pages {
		metadata.Pages = append(metadata.Pages, page.DocumentPageDescriptor)
	}
	for _, asset := range assets {
		metadata.Assets = append(metadata.Assets, asset.DocumentAssetDescriptor)
	}
	return httpapi.DocumentUpload{
		Metadata: metadata, Document: in.Entry.Reader,
		DocumentSize: measuredHTTPPageSize(in.Entry.Reader),
		Pages:        pages, Assets: assets,
	}, nil
}

func (t *httpTransport) wireDocumentRevisionUpload(
	ctx context.Context, in CreateDocumentRevisionInput, legacy bool,
) (httpapi.CreateDocumentRevisionUpload, error) {
	pages, assets, repository, err := t.wireDocumentParts(ctx, in.Document)
	if err != nil {
		return httpapi.CreateDocumentRevisionUpload{}, err
	}
	metadata := httpapi.CreateDocumentRevisionMetadata{
		Target: in.Target, Name: in.Document.Entry.Path,
		Title: firstNonEmpty(in.Document.Title, in.Document.Entry.Title),
	}
	if legacy {
		metadata.MaxSize = portableUploadLimit(in.Document.MaxPageSize) //nolint:staticcheck // Deprecated server compatibility.
	} else {
		metadata.Format = httpapi.CreateDocumentRevisionMetadataFormat(
			in.Document.Entry.Format,
		)
		metadata.Slug = in.Document.Slug
		metadata.Lang = in.Document.Entry.Lang
		metadata.RepositoryURL = repository
		metadata.MaxPageSize = portableUploadLimit(in.Document.MaxPageSize)
		metadata.MaxTotalPageSize = portableUploadLimit(
			in.Document.MaxTotalPageSize,
		)
		metadata.MaxAssetSize = portableUploadLimit(in.Document.MaxAssetSize)
		metadata.MaxTotalSize = portableUploadLimit(in.Document.MaxTotalSize)
		for _, page := range pages {
			metadata.Pages = append(metadata.Pages, page.DocumentPageDescriptor)
		}
		for _, asset := range assets {
			metadata.Assets = append(metadata.Assets, asset.DocumentAssetDescriptor)
		}
	}
	return httpapi.CreateDocumentRevisionUpload{
		Metadata: metadata, Document: in.Document.Entry.Reader,
		DocumentSize: measuredHTTPPageSize(in.Document.Entry.Reader),
		Pages:        pages, Assets: assets,
	}, nil
}

func (t *httpTransport) wireDocumentParts(
	ctx context.Context, in DocumentInput,
) ([]httpapi.DocumentPage, []httpapi.DocumentAsset, string, error) {
	repository := in.RepositoryURL
	if repository == "" {
		var err error
		repository, err = resolveRepository(ctx, t.repository, in.Entry.Path, "")
		if err != nil {
			return nil, nil, "", err
		}
	}
	pages := make([]httpapi.DocumentPage, 0, len(in.Pages))
	for _, page := range in.Pages {
		pages = append(pages, httpapi.DocumentPage{
			DocumentPageDescriptor: httpapi.DocumentPageDescriptor{
				Path:   page.Path,
				Format: httpapi.DocumentPageDescriptorFormat(page.Format),
				Title:  page.Title, Lang: page.Lang,
			},
			Reader: page.Reader, Size: measuredHTTPPageSize(page.Reader),
		})
	}
	assets := make([]httpapi.DocumentAsset, 0, len(in.Assets))
	for _, asset := range in.Assets {
		if asset.Reader == nil {
			return nil, nil, "", fmt.Errorf(
				"airplan: asset %q reader is nil", asset.Path,
			)
		}
		start, err := asset.Reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, nil, "", fmt.Errorf(
				"airplan: inspect asset %q reader: %w", asset.Path, err,
			)
		}
		assets = append(assets, httpapi.DocumentAsset{
			DocumentAssetDescriptor: httpapi.DocumentAssetDescriptor{
				Path: asset.Path, Size: asset.Size, ContentType: asset.ContentType,
			},
			Reader: asset.Reader, Start: start,
		})
	}
	return pages, assets, repository, nil
}

func (t *httpTransport) documentBundleCapabilities(
	ctx context.Context,
) (*httpapi.DocumentBundleCapabilities, error) {
	capability, err := t.revisionCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	if capability == nil || !capability.ManagedPages || !capability.Assets {
		return nil, documentBundleUpgradeError()
	}
	return capability, nil
}

func (t *httpTransport) revisionCapabilities(
	ctx context.Context,
) (*httpapi.DocumentBundleCapabilities, error) {
	capabilities, err := t.client.Capabilities(ctx)
	if err != nil {
		return nil, transportError(err)
	}
	return capabilities.DocumentBundle, nil
}

func documentBundleUpgradeError() error {
	return errors.New(
		"airplan: configured server does not support document bundles; upgrade the server",
	)
}

func validateBundleCapability(
	capability httpapi.DocumentBundleCapabilities, upload httpapi.DocumentUpload,
) error {
	return validateBundleCapabilityValues(
		capability, upload.Metadata, upload.DocumentSize, upload.Pages,
		upload.Assets,
	)
}

func validateRevisionBundleCapability(
	capability httpapi.DocumentBundleCapabilities,
	upload httpapi.CreateDocumentRevisionUpload,
) error {
	return validateBundleCapabilityValues(
		capability, upload.Metadata, upload.DocumentSize, upload.Pages,
		upload.Assets,
	)
}

func validateBundleCapabilityValues(
	capability httpapi.DocumentBundleCapabilities, metadata any,
	documentSize int64, pages []httpapi.DocumentPage,
	assets []httpapi.DocumentAsset,
) error {
	if !capability.ManagedPages || !capability.Assets || capability.MaxItems <= 0 ||
		capability.MaxPageBytes <= 0 || capability.MaxTotalPageBytes <= 0 ||
		capability.MaxGeneratedPageBytes <= 0 ||
		capability.MaxAssetBytes <= 0 || capability.MaxTotalAssetBytes <= 0 ||
		capability.MaxMetadataBytes <= 0 || capability.MaxRequestBytes <= 0 {
		return documentBundleUpgradeError()
	}
	if len(pages)+len(assets)+1 > capability.MaxItems {
		return fmt.Errorf(
			"airplan: document has %d items; server maximum is %d",
			len(pages)+len(assets)+1, capability.MaxItems,
		)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("airplan: encode document metadata: %w", err)
	}
	if int64(len(encoded)+1) > capability.MaxMetadataBytes {
		return fmt.Errorf(
			"airplan: document metadata exceeds server limit of %d bytes",
			capability.MaxMetadataBytes,
		)
	}
	if documentSize < 0 || documentSize > capability.MaxPageBytes {
		return fmt.Errorf(
			"airplan: document entry exceeds server page limit of %d bytes",
			capability.MaxPageBytes,
		)
	}
	pageTotal := documentSize
	for _, page := range pages {
		if page.Size < 0 || page.Size > capability.MaxPageBytes {
			return fmt.Errorf(
				"airplan: page %q exceeds server limit of %d bytes",
				page.Path, capability.MaxPageBytes,
			)
		}
		if page.Size > mathMaxInt64-pageTotal {
			return errors.New("airplan: managed page total size is out of range")
		}
		pageTotal += page.Size
	}
	if pageTotal > capability.MaxTotalPageBytes {
		return fmt.Errorf(
			"airplan: managed pages exceed server limit of %d bytes",
			capability.MaxTotalPageBytes,
		)
	}
	var assetTotal int64
	for _, asset := range assets {
		if asset.Size < 0 {
			return fmt.Errorf("airplan: asset %q has invalid size", asset.Path)
		}
		if asset.Size > capability.MaxAssetBytes {
			return fmt.Errorf(
				"airplan: asset %q exceeds server limit of %d bytes",
				asset.Path, capability.MaxAssetBytes,
			)
		}
		if asset.Size > mathMaxInt64-assetTotal {
			return errors.New("airplan: document asset total size is out of range")
		}
		assetTotal += asset.Size
	}
	if assetTotal > capability.MaxTotalAssetBytes {
		return fmt.Errorf(
			"airplan: document assets exceed server limit of %d bytes",
			capability.MaxTotalAssetBytes,
		)
	}
	metadataBytes := int64(len(encoded) + 1)
	if pageTotal > mathMaxInt64-assetTotal ||
		pageTotal+assetTotal > mathMaxInt64-metadataBytes {
		return errors.New("airplan: document request size is out of range")
	}
	payloadBytes := pageTotal + assetTotal + metadataBytes
	// MIME headers repeat descriptor paths and add one boundary per part. Twice
	// the bounded metadata plus 1 MiB is a conservative envelope for 100 parts.
	overhead := int64(1<<20) + 2*metadataBytes
	if payloadBytes > mathMaxInt64-overhead ||
		payloadBytes+overhead > capability.MaxRequestBytes {
		return fmt.Errorf(
			"airplan: document multipart exceeds server request limit of %d bytes",
			capability.MaxRequestBytes,
		)
	}
	return nil
}

func entryPageResult(path string, result Result) PageResult {
	return PageResult{
		Path: path, Format: result.Format, Title: result.Title,
		URL: result.URL, Key: result.Key, SourceURL: result.SourceURL,
		SourceKey: result.SourceKey, Bytes: result.Bytes,
	}
}

type measuredHTTPPage struct {
	file *os.File
	size int64
}

func (p *measuredHTTPPage) Read(buffer []byte) (int, error) {
	return p.file.Read(buffer)
}

func spoolHTTPDocumentPages(
	ctx context.Context, in DocumentInput,
	capability httpapi.DocumentBundleCapabilities,
) (DocumentInput, func(), error) {
	if capability.MaxPageBytes <= 0 || capability.MaxTotalPageBytes <= 0 ||
		capability.MaxGeneratedPageBytes <= 0 {
		return DocumentInput{}, func() {}, documentBundleUpgradeError()
	}
	pageLimit := effectiveLimit(in.MaxPageSize, DefaultMaxInputSize)
	if pageLimit == 0 || capability.MaxPageBytes < pageLimit {
		pageLimit = capability.MaxPageBytes
	}
	totalLimit := effectiveLimit(in.MaxTotalPageSize, DefaultMaxTotalPageSize)
	if totalLimit == 0 || capability.MaxTotalPageBytes < totalLimit {
		totalLimit = capability.MaxTotalPageBytes
	}
	var files []*os.File
	cleanup := func() {
		for _, file := range files {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
		}
	}
	spool := func(page PageInput) (PageInput, int64, error) {
		if page.Reader == nil {
			return PageInput{}, 0, errors.New("airplan: document page reader is nil")
		}
		if err := ctx.Err(); err != nil {
			return PageInput{}, 0, err
		}
		file, err := os.CreateTemp("", "airplan-http-page-*")
		if err != nil {
			return PageInput{}, 0, fmt.Errorf("airplan: create page temporary file: %w", err)
		}
		files = append(files, file)
		if err = file.Chmod(0o600); err != nil {
			return PageInput{}, 0, fmt.Errorf("airplan: secure page temporary file: %w", err)
		}
		size, err := io.Copy(file, io.LimitReader(page.Reader, pageLimit+1))
		if err != nil {
			return PageInput{}, 0, fmt.Errorf("airplan: spool page %q: %w", page.Path, err)
		}
		if size > pageLimit {
			return PageInput{}, 0, fmt.Errorf(
				"airplan: page %q exceeds server limit of %d bytes: %w",
				page.Path, pageLimit, ErrInputTooLarge,
			)
		}
		if err = ctx.Err(); err != nil {
			return PageInput{}, 0, err
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return PageInput{}, 0, fmt.Errorf("airplan: rewind page %q: %w", page.Path, err)
		}
		page.Reader = &measuredHTTPPage{file: file, size: size}
		return page, size, nil
	}

	prepared := in
	entry, total, err := spool(in.Entry)
	if err != nil {
		cleanup()
		return DocumentInput{}, func() {}, err
	}
	prepared.Entry = entry
	if total > totalLimit {
		cleanup()
		return DocumentInput{}, func() {}, fmt.Errorf(
			"airplan: document entry exceeds server total managed source limit of %d bytes: %w",
			totalLimit, ErrInputTooLarge,
		)
	}
	prepared.Pages = make([]PageInput, len(in.Pages))
	for index, page := range in.Pages {
		preparedPage, pageSize, pageErr := spool(page)
		if pageErr != nil {
			cleanup()
			return DocumentInput{}, func() {}, pageErr
		}
		prepared.Pages[index] = preparedPage
		if pageSize > mathMaxInt64-total {
			cleanup()
			return DocumentInput{}, func() {}, errors.New(
				"airplan: managed page total size is out of range",
			)
		}
		total += pageSize
		if total > totalLimit {
			cleanup()
			return DocumentInput{}, func() {}, fmt.Errorf(
				"airplan: page %q exceeds server total managed source limit of %d bytes: %w",
				page.Path, totalLimit, ErrInputTooLarge,
			)
		}
	}
	return prepared, cleanup, nil
}

func measuredHTTPPageSize(reader io.Reader) int64 {
	if page, ok := reader.(*measuredHTTPPage); ok {
		return page.size
	}
	return -1
}

func (t *httpTransport) UploadFiles(
	ctx context.Context, in FilesInput,
) (*FilesResult, error) {
	repository := t.repository
	if in.RepositoryURL != "" {
		repository = in.RepositoryURL
	}
	prepared, title, repository, _, err := prepareCollection(
		ctx, in, repository,
	)
	if err != nil {
		return nil, err
	}
	files := make([]httpapi.CollectionFile, 0, len(prepared))
	for _, file := range prepared {
		files = append(files, httpapi.CollectionFile{
			Name: file.Name, Reader: file.Reader, Size: file.Size,
			ContentType: file.ContentType,
		})
	}
	result, err := t.client.UploadCollection(ctx, httpapi.CollectionMetadata{
		Title: title, RepositoryURL: repository,
		MaxSize:      portableUploadLimit(in.MaxSize),
		MaxTotalSize: portableUploadLimit(in.MaxTotalSize),
	}, files)
	if err != nil {
		return nil, transportError(err)
	}
	core := coreUploadResult(result)
	return &core, nil
}

func (t *httpTransport) UpdateDocument(
	ctx context.Context, input UpdateDocumentInput,
) (*UpdateDocumentResult, error) {
	revision, err := t.CreateDocumentRevision(ctx, CreateDocumentRevisionInput{
		Target: input.Target,
		Document: DocumentInput{
			Entry: PageInput{
				Reader: input.Input.Reader, Path: input.Input.Name,
				Format: input.Input.Format, Title: input.Input.Title,
				Lang: input.Input.Lang,
			},
			Slug: input.Input.Slug, Title: input.Input.Title,
			MaxPageSize:   input.Input.MaxSize,
			RepositoryURL: input.Input.RepositoryURL,
		},
	})
	if err != nil {
		return nil, err
	}
	return &UpdateDocumentResult{
		Result: revision.Result, PreviousURL: revision.PreviousURL,
		DiffURL: revision.DiffURL, Unchanged: revision.Unchanged,
	}, nil
}

func (t *httpTransport) CreateDocumentRevision(
	ctx context.Context, in CreateDocumentRevisionInput,
) (*DocumentRevisionResult, error) {
	capability, err := t.revisionCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	bundled := len(in.Document.Pages) != 0 || len(in.Document.Assets) != 0
	if bundled && capability == nil {
		return nil, documentBundleUpgradeError()
	}
	prepared := in
	if capability != nil {
		var cleanup func()
		prepared.Document, cleanup, err = spoolHTTPDocumentPages(
			ctx, in.Document, *capability,
		)
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}
	upload, err := t.wireDocumentRevisionUpload(ctx, prepared, capability == nil)
	if err != nil {
		return nil, err
	}
	if capability != nil {
		if err = validateRevisionBundleCapability(*capability, upload); err != nil {
			return nil, err
		}
	}
	var result httpapi.DocumentRevisionResult
	if capability != nil && capability.CanonicalRevisionRoute {
		result, err = t.client.CreateDocumentRevision(ctx, upload)
	} else {
		result, err = t.client.UpdateDocumentBundle(ctx, upload)
	}
	if err != nil {
		return nil, transportError(err)
	}
	return coreDocumentRevisionResult(result, in.Document.Entry.Path), nil
}

func (t *httpTransport) PlanUpgradeDocument(
	ctx context.Context, target string, opts UpgradeDocumentOptions,
) (*UpgradeDocumentPlan, error) {
	result, err := t.client.PlanDocumentUpgrade(ctx, httpapi.UpgradePlanRequest{Target: target, Force: opts.Force})
	if err != nil {
		return nil, transportError(err)
	}
	core := coreUpgradePlan(result)
	return &core, nil
}

func (t *httpTransport) UpgradeDocument(
	ctx context.Context, plan UpgradeDocumentPlan,
) (*UpgradeDocumentResult, error) {
	result, err := t.client.ExecuteDocumentUpgrade(ctx, wireUpgradePlan(plan))
	if err != nil {
		return nil, transportError(err)
	}
	coreUpload := coreUploadResult(result.Result)
	return &UpgradeDocumentResult{
		Result: coreUpload.Result,
		State:  UpgradeState(result.State), Upgraded: result.Upgraded, Reason: result.Reason,
	}, nil
}

func (t *httpTransport) PlanBulkUpgrade(
	ctx context.Context, opts BulkUpgradeOptions,
) (*BulkUpgradePlan, error) {
	result, err := t.client.PlanBulkUpgrade(ctx, httpapi.BulkUpgradeOptions{Concurrency: opts.Concurrency})
	if err != nil {
		return nil, transportError(err)
	}
	items := make([]UpgradeDocumentPlan, len(result.Items))
	for index, item := range result.Items {
		items[index] = coreUpgradePlan(item)
	}
	counts := make(map[UpgradeState]int, len(result.Counts))
	for state, count := range result.Counts {
		counts[UpgradeState(state)] = count
	}
	return &BulkUpgradePlan{Items: items, Counts: counts, Warnings: append([]string(nil), result.Warnings...)}, nil
}

func (t *httpTransport) ExecuteBulkUpgrade(
	ctx context.Context, req BulkUpgradeRequest,
) (*BulkUpgradeResult, error) {
	items := make([]httpapi.UpgradeDocumentPlan, len(req.Items))
	for index, item := range req.Items {
		items[index] = wireUpgradePlan(item)
	}
	result, err := t.client.ExecuteBulkUpgrade(ctx, httpapi.BulkUpgradeRequest{Items: items, Concurrency: req.Concurrency})
	if err != nil {
		return nil, transportError(err)
	}
	core := &BulkUpgradeResult{Items: make([]BulkUpgradeItemResult, len(result.Items)), Upgraded: result.Upgraded, Failed: result.Failed}
	for index, item := range result.Items {
		core.Items[index] = BulkUpgradeItemResult{Plan: coreUpgradePlan(item.Plan), Error: item.Error}
		if item.Result != nil {
			upload := coreUploadResult(item.Result.Result)
			core.Items[index].Result = &UpgradeDocumentResult{Result: upload.Result, State: UpgradeState(item.Result.State), Upgraded: item.Result.Upgraded, Reason: item.Result.Reason}
		}
	}
	return core, nil
}

func portableUploadLimit(limit int64) int64 {
	if limit < 0 {
		return 0
	}
	return limit
}

func coreUploadResult(result httpapi.UploadResult) FilesResult {
	core := FilesResult{Result: Result{
		ID: result.ID, Kind: string(result.Kind), URL: result.URL, Key: result.Key,
		SourceURL: result.SourceURL, SourceKey: result.SourceKey,
		Bucket: result.Bucket, Bytes: result.Bytes,
		ContentType: result.ContentType, Title: result.Title,
		CreatedAt: result.CreatedAt, MarkerVersion: result.MarkerVersion,
		MarkerKey: result.MarkerKey, Format: result.Format, Slug: result.Slug,
		RepositoryURL:   result.RepositoryURL,
		RevisionChainID: result.RevisionChainID, Revision: result.Revision,
		LatestRevision: result.LatestRevision,
		Warnings:       append([]string(nil), result.Warnings...),
	}}
	for _, file := range result.Files {
		core.Files = append(core.Files, FileResult{
			Name: file.Name, URL: file.URL, Key: file.Key,
			Bytes: file.Bytes, ContentType: file.ContentType,
		})
	}
	return core
}

func coreDocumentResult(result httpapi.UploadResult) DocumentResult {
	base := coreUploadResult(result)
	document := DocumentResult{Result: base.Result}
	for _, page := range result.Pages {
		document.Pages = append(document.Pages, PageResult{
			Path: page.Path, Format: page.Format, Title: page.Title,
			URL: page.URL, Key: page.Key, SourceURL: page.SourceURL,
			SourceKey: page.SourceKey, Bytes: page.Bytes,
			SourceBytes: page.SourceBytes,
		})
	}
	for _, asset := range result.Assets {
		document.Assets = append(document.Assets, AssetResult{
			Path: asset.Path, URL: asset.URL, Key: asset.Key,
			Bytes: asset.Bytes, ContentType: asset.ContentType,
		})
	}
	return document
}

func coreDocumentRevisionResult(
	result httpapi.DocumentRevisionResult, entryPath string,
) *DocumentRevisionResult {
	upload := httpapi.UploadResult{
		ID: result.ID, Kind: httpapi.UploadResultKind(result.Kind),
		URL: result.URL, Key: result.Key, SourceURL: result.SourceURL,
		SourceKey: result.SourceKey, Bucket: result.Bucket, Bytes: result.Bytes,
		ContentType: result.ContentType, Title: result.Title,
		CreatedAt: result.CreatedAt, MarkerVersion: result.MarkerVersion,
		MarkerKey: result.MarkerKey, Format: result.Format, Slug: result.Slug,
		RepositoryURL:   result.RepositoryURL,
		RevisionChainID: result.RevisionChainID, Revision: result.Revision,
		LatestRevision: result.LatestRevision, Warnings: result.Warnings,
		Pages: result.Pages, Assets: result.Assets,
	}
	document := coreDocumentResult(upload)
	if len(document.Pages) == 0 {
		document.Pages = []PageResult{entryPageResult(entryPath, document.Result)}
	}
	return &DocumentRevisionResult{
		DocumentResult: document, PreviousURL: result.PreviousURL,
		DiffURL: result.DiffURL, Unchanged: result.Unchanged,
	}
}

func (t *httpTransport) ListManifest(
	ctx context.Context, _ ListManifestOptions,
) (*ManifestList, error) {
	result, err := t.client.ListManifestUploads(ctx)
	if err != nil {
		return nil, transportError(err)
	}
	records := make([]ManifestRecord, 0, len(result.Records))
	for _, record := range result.Records {
		records = append(records, coreManifestRecord(record))
	}
	return &ManifestList{
		Records: records, Warnings: append([]string(nil), result.Warnings...),
	}, nil
}

func (t *httpTransport) ListRemote(
	ctx context.Context,
) ([]RemoteUpload, error) {
	result, err := t.client.ListStorageUploads(ctx)
	if err != nil {
		return nil, transportError(err)
	}
	uploads := make([]RemoteUpload, 0, len(result.Uploads))
	for _, upload := range result.Uploads {
		uploads = append(uploads, RemoteUpload{
			Dir: upload.ID, MarkerKey: upload.MarkerKey,
			Kind: UploadKind(upload.Kind), Conflict: upload.Conflict,
			Protected: upload.Protected,
			Slug:      upload.Slug, Key: upload.Key, URL: upload.URL,
			Keys:    append([]string(nil), upload.Keys...),
			Objects: upload.Objects, Bytes: upload.Bytes,
			LastModified: upload.LastModified,
		})
	}
	return uploads, nil
}

func (t *httpTransport) InspectUpload(
	ctx context.Context, target string,
) (*UploadInspection, error) {
	result, err := t.client.InspectUpload(ctx, httpapi.TargetRequest{
		URLOrKey: target,
	})
	if err != nil {
		return nil, transportError(err)
	}
	return coreInspection(result), nil
}

func coreInspection(result httpapi.UploadInspection) *UploadInspection {
	core := &UploadInspection{
		State: UploadState(result.State), Dir: result.ID,
		MarkerKey: result.MarkerKey, Objects: result.Objects,
		Bytes: result.Bytes, Format: result.Format,
		Kind: UploadKind(result.Kind), Title: result.Title,
		Repo: result.RepositoryURL, MarkerVersion: result.MarkerVersion,
		ProducerVersion:    result.ProducerVersion,
		RendererGeneration: result.RendererVersion,
		RevisionChainID:    result.RevisionChainID, Revision: result.Revision,
		LatestRevision: result.LatestRevision, LatestURL: result.LatestURL,
		RevisionError: result.RevisionError,
		Page:          coreInspectedObject(result.Page),
		Source:        coreInspectedObject(result.Source),
		Diff:          coreInspectedObject(result.Diff),
		Protected:     result.Protected,
		ProtectReason: safeProtectReason(result.ProtectReason),
		Warnings:      append([]string(nil), result.Warnings...),
		Error:         MarkerErrorCode(result.Error),
	}
	if result.Versions != nil {
		versions := coreVersionsMetadata(*result.Versions)
		core.Versions = &versions
	}
	if result.CreatedAt != nil {
		core.CreatedAt = *result.CreatedAt
	}
	if result.ProtectedAt != nil {
		core.ProtectedAt = *result.ProtectedAt
	}
	for _, file := range result.Files {
		file := file
		core.Files = append(core.Files, coreInspectedObject(&file))
	}
	for _, page := range result.Pages {
		pageObject := page.Page
		core.Pages = append(core.Pages, InspectedPage{
			Path: page.Path, Format: page.Format, Title: page.Title,
			Lang: page.Lang, Page: coreInspectedObject(&pageObject),
			Source: coreInspectedObject(page.Source),
		})
	}
	for _, asset := range result.Assets {
		asset := asset
		core.Assets = append(core.Assets, coreInspectedObject(&asset))
	}
	return core
}

func coreVersionsMetadata(metadata httpapi.VersionsMetadata) VersionsMetadata {
	core := VersionsMetadata{
		Schema: string(metadata.Schema), Version: int(metadata.Version),
		ChainID: metadata.ChainID, CurrentRevision: metadata.CurrentRevision,
		LatestRevision:       metadata.LatestRevision,
		LastAssignedRevision: metadata.LastAssignedRevision,
	}
	for _, revision := range metadata.Revisions {
		item := VersionsRevision{
			Number: revision.Number, URL: revision.URL,
			DiffURL: revision.DiffURL, Deleted: revision.Deleted,
		}
		if revision.CreatedAt != nil {
			item.CreatedAt = *revision.CreatedAt
		}
		if revision.DeletedAt != nil {
			item.DeletedAt = *revision.DeletedAt
		}
		core.Revisions = append(core.Revisions, item)
	}
	return core
}

func coreInspectedObject(object *httpapi.InspectedObject) *InspectedObject {
	if object == nil {
		return nil
	}
	return &InspectedObject{
		Key: object.Key, URL: object.URL, Exists: object.Exists,
		Bytes: object.Bytes, ExpectedBytes: object.ExpectedBytes,
		ExpectedKnown: object.ExpectedKnown,
	}
}

func (t *httpTransport) InspectRemoteUploads(
	ctx context.Context, uploads []RemoteUpload, concurrency int,
) ([]RemoteInspectionResult, error) {
	limit, err := ValidateRemoteConcurrency(concurrency)
	if err != nil {
		return nil, err
	}
	results := make([]RemoteInspectionResult, len(uploads))
	if len(uploads) == 0 {
		return results, nil
	}
	sem := make(chan struct{}, min(limit, len(uploads)))
	var wg sync.WaitGroup
	for index := range uploads {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[index] = RemoteInspectionResult{
					Upload: uploads[index], Err: ctx.Err(),
				}
				return
			}
			inspection, inspectErr := t.InspectUpload(
				ctx, uploads[index].MarkerKey,
			)
			results[index] = RemoteInspectionResult{
				Upload: uploads[index], Inspection: inspection,
				Err: inspectErr,
			}
		}(index)
	}
	wg.Wait()
	return results, nil
}

func (t *httpTransport) GetUploadTo(
	ctx context.Context, target string, opts GetOptions, dst io.Writer,
) (string, error) {
	download, err := t.client.GetUpload(ctx, httpapi.GetUploadRequest{
		URLOrKey: target, Source: opts.Source, Diff: opts.Diff,
	})
	if err != nil {
		return "", transportError(err)
	}
	defer func() { _ = download.Body.Close() }()
	if _, err := io.Copy(dst, download.Body); err != nil {
		return "", fmt.Errorf("airplan: stream server download: %w", err)
	}
	return download.Key, nil
}

func (t *httpTransport) GetUpload(
	ctx context.Context, target string, opts GetOptions,
) (*GetResult, error) {
	var body bytes.Buffer
	key, err := t.GetUploadTo(ctx, target, opts, &body)
	if err != nil {
		return nil, err
	}
	return &GetResult{Key: key, Body: body.Bytes()}, nil
}

func (t *httpTransport) DeleteUpload(
	ctx context.Context, target string, opts DeleteOptions,
) (*DeleteResult, error) {
	result, err := t.client.DeleteUpload(ctx, httpapi.DeleteRequest{
		URLOrKey: target, Force: opts.Force,
	})
	if err != nil {
		// Reconstruct the typed protection failure, including the advisory
		// reason the problem carries, so purge skip handling and the CLI
		// --force hint match the direct S3 backend.
		var problem *httpapi.ProblemError
		if errors.As(err, &problem) &&
			problem.Problem.Code == "upload_protected" {
			reason := problem.Problem.ProtectReason
			if validateProtectReason(reason) != nil {
				reason = ""
			}
			return nil, &UploadProtectedError{Target: target, Reason: reason}
		}
		return nil, transportError(err)
	}
	return &DeleteResult{
		Keys:    append([]string(nil), result.Keys...),
		PageKey: result.PageKey, MarkerKey: result.MarkerKey,
		Kind:     UploadKind(result.Kind),
		Warnings: append([]string(nil), result.Warnings...),
	}, nil
}

func (t *httpTransport) ProtectUpload(
	ctx context.Context, target, reason string,
) (*ProtectionResult, error) {
	result, err := t.client.ProtectUpload(ctx, httpapi.ProtectRequest{
		URLOrKey: target, Reason: reason,
	})
	if err != nil {
		return nil, transportError(err)
	}
	return coreProtectionResult(result), nil
}

func (t *httpTransport) UnprotectUpload(
	ctx context.Context, target string,
) (*ProtectionResult, error) {
	result, err := t.client.UnprotectUpload(ctx, httpapi.TargetRequest{
		URLOrKey: target,
	})
	if err != nil {
		return nil, transportError(err)
	}
	return coreProtectionResult(result), nil
}

func coreProtectionResult(result httpapi.ProtectionResult) *ProtectionResult {
	core := &ProtectionResult{
		ID: result.ID, MarkerKey: result.MarkerKey,
		SentinelKey: result.SentinelKey, PageKey: result.PageKey,
		Kind: UploadKind(result.Kind), Protected: result.Protected,
		Reason:   safeProtectReason(result.Reason),
		Warnings: append([]string(nil), result.Warnings...),
	}
	if result.ProtectedAt != nil {
		core.ProtectedAt = *result.ProtectedAt
	}
	return core
}

func (t *httpTransport) SyncManifest(
	ctx context.Context, opts SyncManifestOptions,
) (*SyncManifestResult, error) {
	result, err := t.client.SyncManifest(ctx, httpapi.SyncRequest{
		Prune: opts.Prune, DryRun: opts.DryRun,
		Concurrency: opts.Concurrency,
	})
	if err != nil {
		return nil, transportError(err)
	}
	core := &SyncManifestResult{
		Unchanged: result.Unchanged, Deferred: result.Deferred,
		Incomplete: result.Incomplete,
		Invalid:    result.Invalid, Retained: result.Retained,
		Warnings: append([]string(nil), result.Warnings...),
	}
	for _, record := range result.AddedRecords {
		core.Added = append(core.Added, coreManifestRecord(record))
	}
	for _, record := range result.EnrichedRecords {
		core.Enriched = append(core.Enriched, coreManifestRecord(record))
	}
	for _, record := range result.TombstoneRecords {
		core.Tombstoned = append(
			core.Tombstoned, coreManifestRecord(record),
		)
	}
	for _, record := range result.ProtectionRecords {
		core.Protection = append(
			core.Protection, coreManifestRecord(record),
		)
	}
	for _, failure := range result.Failures {
		core.Failures = append(core.Failures, SyncFailure{
			MarkerKey: failure.MarkerKey, Operation: string(failure.Operation),
			Error: failure.Error,
		})
	}
	if len(core.Failures) > 0 {
		return core, fmt.Errorf(
			"airplan: sync incomplete: %d remote request(s) failed",
			len(core.Failures),
		)
	}
	return core, nil
}

func (t *httpTransport) PlanPurge(
	ctx context.Context, opts PurgePlanOptions,
) (*PurgePlan, error) {
	request := httpapi.PurgePreviewRequest{
		Source: httpapi.PurgePreviewRequestSource(opts.Source),
		Slug:   opts.Slug, All: opts.All,
		Concurrency: opts.Concurrency, IncludeVersioned: opts.IncludeVersioned,
	}
	if !opts.CreatedBefore.IsZero() {
		request.CreatedBefore = &opts.CreatedBefore
	}
	result, err := t.client.PreviewPurge(ctx, request)
	if err != nil {
		return nil, transportError(err)
	}
	core := &PurgePlan{
		Candidates: []PurgeCandidate{},
		Protected:  []PurgeCandidate{},
		Versioned:  []PurgeCandidate{},
		Invalid:    result.Invalid,
		Warnings:   append([]string(nil), result.Warnings...),
	}
	coreCandidate := func(candidate httpapi.PurgeCandidate) PurgeCandidate {
		item := PurgeCandidate{
			UploadID: candidate.UploadID,
			Record:   coreManifestRecord(candidate.Record),
			Warnings: append([]string(nil), candidate.Warnings...),
		}
		if candidate.Inspection != nil {
			item.Inspection = coreInspection(*candidate.Inspection)
		}
		return item
	}
	for _, candidate := range result.Candidates {
		core.Candidates = append(core.Candidates, coreCandidate(candidate))
	}
	for _, candidate := range result.Protected {
		core.Protected = append(core.Protected, coreCandidate(candidate))
	}
	for _, candidate := range result.Versioned {
		core.Versioned = append(core.Versioned, coreCandidate(candidate))
	}
	return core, nil
}

func (t *httpTransport) Purge(
	ctx context.Context, req PurgeRequest,
) (*PurgeResult, error) {
	result, err := t.client.ExecutePurge(ctx, httpapi.PurgeRequest{
		UploadIds: req.UploadIDs, IncludeVersioned: req.IncludeVersioned,
	})
	if err != nil {
		return nil, transportError(err)
	}
	core := &PurgeResult{}
	failed := 0
	for _, item := range result.Items {
		coreItem := PurgeItemResult{
			UploadID: item.UploadID, Protected: item.Protected,
			Versioned: item.Versioned,
			Error:     item.Error,
		}
		if item.Deleted != nil {
			coreItem.Deleted = &DeleteResult{
				Keys:      append([]string(nil), item.Deleted.Keys...),
				PageKey:   item.Deleted.PageKey,
				MarkerKey: item.Deleted.MarkerKey,
				Kind:      UploadKind(item.Deleted.Kind),
				Warnings:  append([]string(nil), item.Deleted.Warnings...),
			}
		}
		if item.Error != "" {
			failed++
		}
		core.Items = append(core.Items, coreItem)
	}
	if failed > 0 {
		return core, fmt.Errorf(
			"airplan: purge failed: %d upload(s) failed", failed,
		)
	}
	return core, nil
}

func transportError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var problem *httpapi.ProblemError
	if errors.As(err, &problem) {
		switch problem.Problem.Code {
		case "upgrade_conflict", "revision_conflict":
			return fmt.Errorf(
				"%w: server reported a conditional mutation conflict: %w",
				ErrConflict, err,
			)
		case "input_too_large", "request_too_large":
			return fmt.Errorf("%w: %w", ErrInputTooLarge, err)
		case "revision_history_full":
			return fmt.Errorf("%w: %w", ErrRevisionHistoryFull, err)
		}
	}
	return fmt.Errorf("airplan: server: %w", err)
}

// APIURL returns the resolved server URL for diagnostics that do not expose
// credentials.
func (c *Client) APIURL() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return c.cfg.APIURL
}

// Backend reports the selected product backend.
func (c *Client) Backend() Backend {
	if c == nil || c.cfg == nil {
		return ""
	}
	return c.cfg.EffectiveBackend()
}
