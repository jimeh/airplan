package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

// HTTPOperations adapts the public Airplan operation facade to the REST wire
// models. It contains conversions only; business rules remain in Client.
type HTTPOperations struct {
	Client        *Client
	ServerVersion string
}

func (o *HTTPOperations) client() (*Client, error) {
	if o == nil || o.Client == nil {
		return nil, errors.New("airplan: HTTP operation client is nil")
	}
	if o.Client.remote != nil {
		return nil, errors.New("airplan: HTTP server requires an s3 backend")
	}
	return o.Client, nil
}

// Capabilities returns safe server compatibility information.
func (o *HTTPOperations) Capabilities(
	context.Context,
) (httpapi.Capabilities, error) {
	version := o.ServerVersion
	if version == "" {
		version = "dev"
	}
	return httpapi.Capabilities{
		APIVersion: "v1", ServerVersion: version,
		Operations: []string{
			"upload_document", "create_document_revision",
			"upload_collection", "inspect_upload",
			"get_upload", "delete_upload", "protect_upload",
			"unprotect_upload", "list_manifest",
			"list_storage", "sync_manifest", "preview_purge",
			"execute_purge", "plan_document_upgrade",
			"execute_document_upgrade", "plan_bulk_upgrade",
			"execute_bulk_upgrade",
			"update_document",
		},
		UploadFormats: []httpapi.CapabilitiesUploadFormats{"md", "html", "txt"},
		Limits: httpapi.UploadLimits{
			DocumentBytes:        DefaultMaxInputSize,
			CollectionFileBytes:  DefaultMaxCollectionFileSize,
			CollectionTotalBytes: DefaultMaxCollectionTotalSize,
		},
		DocumentBundle: &httpapi.DocumentBundleCapabilities{
			ManagedPages: true, Assets: true, CanonicalRevisionRoute: true,
			MaxItems:              MaxDocumentItems,
			MaxPageBytes:          DefaultMaxInputSize,
			MaxTotalPageBytes:     DefaultMaxTotalPageSize,
			MaxGeneratedPageBytes: DefaultMaxGeneratedPageSize,
			MaxAssetBytes:         DefaultMaxAssetSize,
			MaxTotalAssetBytes:    DefaultMaxDocumentAssetTotalSize,
			MaxMetadataBytes:      256 << 10,
			MaxRequestBytes: DefaultMaxTotalPageSize +
				DefaultMaxDocumentAssetTotalSize + 16<<20,
		},
		MarkerVersions: supportedMarkerVersions(),
	}, nil
}

func supportedMarkerVersions() []int {
	versions := make([]int, 0, MarkerVersion)
	for version := 1; version <= MarkerVersion; version++ {
		if IsSupportedMarkerVersion(version) {
			versions = append(versions, version)
		}
	}
	return versions
}

func (o *HTTPOperations) UpdateDocument(
	ctx context.Context, upload httpapi.UpdateDocumentUpload,
) (httpapi.UpdateDocumentResult, error) {
	return o.CreateDocumentRevision(ctx, upload)
}

// CreateDocumentRevision invokes the canonical complete-document operation.
func (o *HTTPOperations) CreateDocumentRevision(
	ctx context.Context, upload httpapi.CreateDocumentRevisionUpload,
) (httpapi.DocumentRevisionResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.DocumentRevisionResult{}, err
	}
	repositoryURL := ""
	if upload.Metadata.RepositoryURL != "" {
		repositoryURL, err = hostedRepositoryURL(upload.Metadata.RepositoryURL)
		if err != nil {
			return httpapi.DocumentRevisionResult{}, apiOperationError(err)
		}
	}
	document := coreDocumentInput(
		upload.Metadata.Name, string(upload.Metadata.Format),
		upload.Metadata.Title, upload.Metadata.Slug, upload.Metadata.Lang,
		repositoryURL,
		upload.Metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
		upload.Metadata.MaxPageSize,
		upload.Metadata.MaxTotalPageSize, upload.Metadata.MaxAssetSize,
		upload.Metadata.MaxTotalSize, upload.Document, upload.Pages,
		upload.Assets,
	)
	document.maxGeneratedPageSize = httpapi.GeneratedPageLimit(ctx)
	result, err := client.CreateDocumentRevision(ctx, CreateDocumentRevisionInput{
		Target:   upload.Metadata.Target,
		Document: document,
	})
	if err != nil {
		return httpapi.DocumentRevisionResult{}, apiOperationError(err)
	}
	return wireDocumentRevisionResult(result), nil
}

// UploadDocument invokes the shared document operation.
func (o *HTTPOperations) UploadDocument(
	ctx context.Context, upload httpapi.DocumentUpload,
) (httpapi.UploadResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UploadResult{}, err
	}
	repositoryURL, err := hostedRepositoryURL(upload.Metadata.RepositoryURL)
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	document := coreDocumentInput(
		upload.Metadata.Name, string(upload.Metadata.Format),
		upload.Metadata.Title, upload.Metadata.Slug, upload.Metadata.Lang,
		repositoryURL,
		upload.Metadata.MaxSize, //nolint:staticcheck // Protocol compatibility alias.
		upload.Metadata.MaxPageSize,
		upload.Metadata.MaxTotalPageSize, upload.Metadata.MaxAssetSize,
		upload.Metadata.MaxTotalSize, upload.Document, upload.Pages,
		upload.Assets,
	)
	document.maxGeneratedPageSize = httpapi.GeneratedPageLimit(ctx)
	result, err := client.UploadDocument(ctx, document)
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	return wireDocumentResult(result), nil
}

func coreDocumentInput(
	name, format, title, slug, lang, repositoryURL string,
	maxSize, maxPageSize, maxTotalPageSize, maxAssetSize, maxTotalSize int64,
	document io.Reader, pages []httpapi.DocumentPage,
	assets []httpapi.DocumentAsset,
) DocumentInput {
	if maxPageSize == 0 {
		maxPageSize = maxSize
	}
	in := DocumentInput{
		Entry: PageInput{Reader: document, Path: name, Format: format, Title: title, Lang: lang},
		Slug:  slug, Title: title, RepositoryURL: repositoryURL,
		MaxPageSize: maxPageSize, MaxTotalPageSize: maxTotalPageSize,
		MaxAssetSize: maxAssetSize, MaxTotalSize: maxTotalSize,
	}
	for _, page := range pages {
		in.Pages = append(in.Pages, PageInput{
			Reader: page.Reader, Path: page.Path, Format: string(page.Format),
			Title: page.Title, Lang: page.Lang,
		})
	}
	for _, asset := range assets {
		in.Assets = append(in.Assets, AssetInput{
			Reader: asset.Reader, Path: asset.Path, Size: asset.Size,
			ContentType: asset.ContentType,
		})
	}
	return in
}

// UploadCollection invokes the shared collection operation.
func (o *HTTPOperations) UploadCollection(
	ctx context.Context, upload httpapi.CollectionUpload,
) (httpapi.UploadResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UploadResult{}, err
	}
	files := make([]FileInput, 0, len(upload.Files))
	for _, file := range upload.Files {
		files = append(files, FileInput{
			Name: file.Name, ContentType: file.ContentType,
			Size: file.Size, Reader: file.Reader,
		})
	}
	repositoryURL, err := hostedRepositoryURL(upload.Metadata.RepositoryURL)
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	result, err := client.UploadFiles(ctx, FilesInput{
		Files: files, Title: upload.Metadata.Title,
		RepositoryURL: repositoryURL,
		MaxSize:       upload.Metadata.MaxSize,
		MaxTotalSize:  upload.Metadata.MaxTotalSize,
	})
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	return wireUploadResult(&result.Result, result.Files), nil
}

func (o *HTTPOperations) PlanDocumentUpgrade(
	ctx context.Context, request httpapi.UpgradePlanRequest,
) (httpapi.UpgradeDocumentPlan, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UpgradeDocumentPlan{}, err
	}
	plan, err := client.PlanUpgradeDocument(ctx, request.Target,
		UpgradeDocumentOptions{Force: request.Force})
	if err != nil {
		return httpapi.UpgradeDocumentPlan{}, apiOperationError(err)
	}
	return wireUpgradePlan(*plan), nil
}

func (o *HTTPOperations) ExecuteDocumentUpgrade(
	ctx context.Context, request httpapi.UpgradeDocumentPlan,
) (httpapi.UpgradeDocumentResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UpgradeDocumentResult{}, err
	}
	result, err := client.UpgradeDocument(ctx, coreUpgradePlan(request))
	if err != nil {
		return httpapi.UpgradeDocumentResult{}, apiOperationError(err)
	}
	return wireUpgradeResult(*result), nil
}

func (o *HTTPOperations) PlanBulkUpgrade(
	ctx context.Context, request httpapi.BulkUpgradeOptions,
) (httpapi.BulkUpgradePlan, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.BulkUpgradePlan{}, err
	}
	plan, err := client.PlanBulkUpgrade(ctx, BulkUpgradeOptions{Concurrency: request.Concurrency})
	if err != nil {
		return httpapi.BulkUpgradePlan{}, apiOperationError(err)
	}
	items := make([]httpapi.UpgradeDocumentPlan, len(plan.Items))
	for index, item := range plan.Items {
		items[index] = wireUpgradePlan(item)
	}
	counts := make(map[string]int, len(plan.Counts))
	for state, count := range plan.Counts {
		counts[string(state)] = count
	}
	return httpapi.BulkUpgradePlan{
		Items: items, Counts: counts,
		Warnings: serverSafeWarnings(plan.Warnings),
	}, nil
}

func (o *HTTPOperations) ExecuteBulkUpgrade(
	ctx context.Context, request httpapi.BulkUpgradeRequest,
) (httpapi.BulkUpgradeResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.BulkUpgradeResult{}, err
	}
	items := make([]UpgradeDocumentPlan, len(request.Items))
	for index, item := range request.Items {
		items[index] = coreUpgradePlan(item)
	}
	result, err := client.ExecuteBulkUpgrade(ctx, BulkUpgradeRequest{Items: items, Concurrency: request.Concurrency})
	if err != nil && result == nil {
		return httpapi.BulkUpgradeResult{}, apiOperationError(err)
	}
	wireItems := make([]httpapi.BulkUpgradeItemResult, len(result.Items))
	for index, item := range result.Items {
		wireItems[index] = httpapi.BulkUpgradeItemResult{Plan: wireUpgradePlan(item.Plan), Error: serverSafeItemError(item.Error)}
		if item.Result != nil {
			result := wireUpgradeResult(*item.Result)
			wireItems[index].Result = &result
		}
	}
	return httpapi.BulkUpgradeResult{Items: wireItems, Upgraded: result.Upgraded, Failed: result.Failed}, nil
}

func wireUpgradePlan(plan UpgradeDocumentPlan) httpapi.UpgradeDocumentPlan {
	return httpapi.UpgradeDocumentPlan{
		Target: plan.Target, Bucket: plan.Bucket,
		State: httpapi.UpgradeState(plan.State), Reason: plan.Reason, URL: plan.URL,
		MarkerKey: plan.MarkerKey, PageKey: plan.PageKey, SourceKey: plan.SourceKey,
		CurrentMarkerVersion:      plan.CurrentMarkerVersion,
		CurrentProducerVersion:    plan.CurrentProducerVersion,
		CurrentRendererGeneration: plan.CurrentRendererGeneration,
		TargetMarkerVersion:       plan.TargetMarkerVersion,
		TargetProducerVersion:     plan.TargetProducerVersion,
		TargetRendererGeneration:  plan.TargetRendererGeneration,
		MarkerEtag:                plan.MarkerETag, PageEtag: plan.PageETag,
		SourceEtag:  plan.SourceETag,
		PageEtags:   cloneStringMap(plan.PageETags),
		SourceEtags: cloneStringMap(plan.SourceETags), Force: plan.Force,
	}
}

func coreUpgradePlan(plan httpapi.UpgradeDocumentPlan) UpgradeDocumentPlan {
	return UpgradeDocumentPlan{
		Target: plan.Target, Bucket: plan.Bucket,
		State: UpgradeState(plan.State), Reason: plan.Reason, URL: plan.URL,
		MarkerKey: plan.MarkerKey, PageKey: plan.PageKey, SourceKey: plan.SourceKey,
		CurrentMarkerVersion:      plan.CurrentMarkerVersion,
		CurrentProducerVersion:    plan.CurrentProducerVersion,
		CurrentRendererGeneration: plan.CurrentRendererGeneration,
		TargetMarkerVersion:       plan.TargetMarkerVersion,
		TargetProducerVersion:     plan.TargetProducerVersion,
		TargetRendererGeneration:  plan.TargetRendererGeneration,
		MarkerETag:                plan.MarkerEtag, PageETag: plan.PageEtag,
		SourceETag:  plan.SourceEtag,
		PageETags:   cloneStringMap(plan.PageEtags),
		SourceETags: cloneStringMap(plan.SourceEtags), Force: plan.Force,
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func wireUpgradeResult(result UpgradeDocumentResult) httpapi.UpgradeDocumentResult {
	return httpapi.UpgradeDocumentResult{
		Result: wireUploadResult(&result.Result, nil),
		State:  httpapi.UpgradeState(result.State), Upgraded: result.Upgraded, Reason: result.Reason,
	}
}

func wireUploadResult(
	result *Result, files []FileResult,
) httpapi.UploadResult {
	wireFiles := make([]httpapi.FileResult, 0, len(files))
	for _, file := range files {
		wireFiles = append(wireFiles, httpapi.FileResult{
			Name: file.Name, URL: file.URL, Key: file.Key,
			Bytes: file.Bytes, ContentType: file.ContentType,
		})
	}
	return httpapi.UploadResult{
		ID: result.ID, Kind: httpapi.UploadResultKind(result.Kind),
		URL: result.URL, Key: result.Key,
		SourceURL: result.SourceURL, SourceKey: result.SourceKey,
		Bucket: result.Bucket, Bytes: result.Bytes,
		ContentType: result.ContentType, Title: result.Title,
		CreatedAt: result.CreatedAt, MarkerVersion: result.MarkerVersion,
		MarkerKey: result.MarkerKey, Format: result.Format, Slug: result.Slug,
		RepositoryURL:   result.RepositoryURL,
		RevisionChainID: result.RevisionChainID, Revision: result.Revision,
		LatestRevision: result.LatestRevision,
		Warnings:       serverSafeWarnings(result.Warnings), Files: wireFiles,
	}
}

func wireDocumentResult(result *DocumentResult) httpapi.UploadResult {
	wire := wireUploadResult(&result.Result, nil)
	for _, page := range result.Pages {
		wire.Pages = append(wire.Pages, httpapi.PageResult{
			Path: page.Path, Format: page.Format, Title: page.Title,
			URL: page.URL, Key: page.Key, SourceURL: page.SourceURL,
			SourceKey: page.SourceKey, Bytes: page.Bytes,
			SourceBytes: page.SourceBytes,
		})
	}
	for _, asset := range result.Assets {
		wire.Assets = append(wire.Assets, httpapi.AssetResult{
			Path: asset.Path, URL: asset.URL, Key: asset.Key,
			Bytes: asset.Bytes, ContentType: asset.ContentType,
		})
	}
	return wire
}

func wireDocumentRevisionResult(
	result *DocumentRevisionResult,
) httpapi.DocumentRevisionResult {
	document := wireDocumentResult(&result.DocumentResult)
	return httpapi.DocumentRevisionResult{
		ID: document.ID, Kind: httpapi.DocumentRevisionResultKind(document.Kind),
		URL: document.URL, Key: document.Key,
		SourceURL: document.SourceURL, SourceKey: document.SourceKey,
		Bucket: document.Bucket, Bytes: document.Bytes,
		ContentType: document.ContentType, Title: document.Title,
		CreatedAt: document.CreatedAt, MarkerVersion: document.MarkerVersion,
		MarkerKey: document.MarkerKey, Format: document.Format,
		Slug: document.Slug, RepositoryURL: document.RepositoryURL,
		RevisionChainID: document.RevisionChainID, Revision: document.Revision,
		LatestRevision: document.LatestRevision, Warnings: document.Warnings,
		Pages: document.Pages, Assets: document.Assets,
		PreviousURL: result.PreviousURL, DiffURL: result.DiffURL,
		Unchanged: result.Unchanged,
	}
}

var errInvalidHostedRepository = errors.New(
	"airplan: hosted repository must be none or an explicit repository URL",
)

func hostedRepositoryURL(repositoryURL string) (string, error) {
	repositoryURL = strings.TrimSpace(repositoryURL)
	if repositoryURL == "" || repositoryURL == "none" {
		return "none", nil
	}
	if repositoryURL == "auto" {
		return "", errInvalidHostedRepository
	}
	normalized, err := NormalizeRepositoryURL(repositoryURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errInvalidHostedRepository, err)
	}
	return normalized, nil
}

func serverSafeWarnings(warnings []string) []string {
	safe := make([]string, len(warnings))
	const templateFailure = " (note: the template also failed to load:"
	for index, warning := range warnings {
		if start := strings.Index(warning, templateFailure); start >= 0 {
			safe[index] = warning[:start] +
				" (note: the configured template also failed to load)"
			continue
		}
		safe[index] = "The server completed with a non-fatal warning."
	}
	return safe
}

func serverSafeItemError(message string) string {
	if message == "" {
		return ""
	}
	return "The server could not complete this item."
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

// InspectUpload invokes marker validation for one target.
func (o *HTTPOperations) InspectUpload(
	ctx context.Context, request httpapi.TargetRequest,
) (httpapi.UploadInspection, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UploadInspection{}, err
	}
	result, err := client.InspectUpload(ctx, request.URLOrKey)
	if err != nil {
		return httpapi.UploadInspection{}, apiOperationError(err)
	}
	return wireInspection(result), nil
}

func wireInspection(result *UploadInspection) httpapi.UploadInspection {
	createdAt := result.CreatedAt
	wire := httpapi.UploadInspection{
		State: httpapi.UploadInspectionState(result.State), ID: result.Dir,
		MarkerKey: result.MarkerKey, Objects: result.Objects,
		Bytes: result.Bytes, CreatedAt: &createdAt, Format: result.Format,
		Kind: httpapi.UploadInspectionKind(result.Kind), Title: result.Title,
		RepositoryURL: result.Repo, MarkerVersion: result.MarkerVersion,
		ProducerVersion: result.ProducerVersion,
		RendererVersion: result.RendererGeneration,
		RevisionChainID: result.RevisionChainID, Revision: result.Revision,
		LatestRevision: result.LatestRevision, LatestURL: result.LatestURL,
		RevisionError: result.RevisionError,
		Warnings:      serverSafeWarnings(result.Warnings),
		Error:         string(result.Error),
	}
	if result.CreatedAt.IsZero() {
		wire.CreatedAt = nil
	}
	wire.Protected = result.Protected
	if !result.ProtectedAt.IsZero() {
		protectedAt := result.ProtectedAt
		wire.ProtectedAt = &protectedAt
	}
	wire.ProtectReason = result.ProtectReason
	wire.Page = wireInspectedObject(result.Page)
	wire.Source = wireInspectedObject(result.Source)
	wire.Diff = wireInspectedObject(result.Diff)
	if result.Versions != nil {
		versions := wireVersionsMetadata(*result.Versions)
		wire.Versions = &versions
	}
	for _, file := range result.Files {
		wireFile := wireInspectedObject(file)
		if wireFile != nil {
			wire.Files = append(wire.Files, *wireFile)
		}
	}
	for _, page := range result.Pages {
		wirePage := httpapi.InspectedPage{
			Path: page.Path, Format: page.Format, Title: page.Title,
			Lang: page.Lang,
		}
		if object := wireInspectedObject(page.Page); object != nil {
			wirePage.Page = *object
		}
		wirePage.Source = wireInspectedObject(page.Source)
		wire.Pages = append(wire.Pages, wirePage)
	}
	for _, asset := range result.Assets {
		if wireAsset := wireInspectedObject(asset); wireAsset != nil {
			wire.Assets = append(wire.Assets, *wireAsset)
		}
	}
	if wire.Files == nil {
		wire.Files = []httpapi.InspectedObject{}
	}
	return wire
}

func wireVersionsMetadata(metadata VersionsMetadata) httpapi.VersionsMetadata {
	wire := httpapi.VersionsMetadata{
		Schema:  httpapi.VersionsMetadataSchema(metadata.Schema),
		Version: httpapi.VersionsMetadataVersion(metadata.Version),
		ChainID: metadata.ChainID, CurrentRevision: metadata.CurrentRevision,
		LatestRevision:       metadata.LatestRevision,
		LastAssignedRevision: metadata.LastAssignedRevision,
	}
	for _, revision := range metadata.Revisions {
		item := httpapi.VersionsRevision{
			Number: revision.Number, URL: revision.URL,
			DiffURL: revision.DiffURL, Deleted: revision.Deleted,
		}
		if !revision.CreatedAt.IsZero() {
			value := revision.CreatedAt
			item.CreatedAt = &value
		}
		if !revision.DeletedAt.IsZero() {
			value := revision.DeletedAt
			item.DeletedAt = &value
		}
		wire.Revisions = append(wire.Revisions, item)
	}
	return wire
}

func wireInspectedObject(object *InspectedObject) *httpapi.InspectedObject {
	if object == nil {
		return nil
	}
	return &httpapi.InspectedObject{
		Name: object.Name, Role: httpapi.InspectedObjectRole(object.Role),
		Key: object.Key, URL: object.URL, Exists: object.Exists,
		Bytes: object.Bytes, ExpectedBytes: object.ExpectedBytes,
		ExpectedKnown: object.ExpectedKnown,
		Sha256:        object.SHA256, ExpectedSha256: object.ExpectedSHA256,
	}
}

// GetUpload returns a marker-declared object without buffering its body.
func (o *HTTPOperations) GetUpload(
	ctx context.Context, request httpapi.GetUploadRequest,
) (httpapi.Download, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.Download{}, err
	}
	key, body, contentType, err := client.openUpload(
		ctx, request.URLOrKey, GetOptions{
			Source: request.Source, Diff: request.Diff,
			Page: request.Page, Asset: request.Asset,
		},
	)
	if err != nil {
		return httpapi.Download{}, apiOperationError(err)
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(key))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	return httpapi.Download{
		Body: body, Key: key,
		Filename: path.Base(key), ContentType: contentType,
	}, nil
}

// DeleteUpload permanently removes one marker-managed upload.
func (o *HTTPOperations) DeleteUpload(
	ctx context.Context, request httpapi.DeleteRequest,
) (httpapi.DeleteResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.DeleteResult{}, err
	}
	result, err := client.DeleteUploadWithOptions(
		ctx, request.URLOrKey, DeleteOptions{Force: request.Force},
	)
	if err != nil {
		return httpapi.DeleteResult{}, apiOperationError(err)
	}
	return wireDeleteResult(result), nil
}

// ProtectUpload marks one marker-managed upload as purge-protected.
func (o *HTTPOperations) ProtectUpload(
	ctx context.Context, request httpapi.ProtectRequest,
) (httpapi.ProtectionResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.ProtectionResult{}, err
	}
	result, err := client.ProtectUpload(ctx, request.URLOrKey, request.Reason)
	if err != nil {
		return httpapi.ProtectionResult{}, apiOperationError(err)
	}
	return wireProtectionResult(result), nil
}

// UnprotectUpload removes purge protection from one marker-managed upload.
func (o *HTTPOperations) UnprotectUpload(
	ctx context.Context, request httpapi.TargetRequest,
) (httpapi.ProtectionResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.ProtectionResult{}, err
	}
	result, err := client.UnprotectUpload(ctx, request.URLOrKey)
	if err != nil {
		return httpapi.ProtectionResult{}, apiOperationError(err)
	}
	return wireProtectionResult(result), nil
}

func wireProtectionResult(result *ProtectionResult) httpapi.ProtectionResult {
	wire := httpapi.ProtectionResult{
		ID: result.ID, MarkerKey: result.MarkerKey,
		SentinelKey: result.SentinelKey, PageKey: result.PageKey,
		Kind:      httpapi.ProtectionResultKind(result.Kind),
		Protected: result.Protected, Reason: result.Reason,
		Warnings: serverSafeWarnings(result.Warnings),
	}
	if !result.ProtectedAt.IsZero() {
		protectedAt := result.ProtectedAt
		wire.ProtectedAt = &protectedAt
	}
	return wire
}

func wireDeleteResult(result *DeleteResult) httpapi.DeleteResult {
	kind := result.Kind
	if kind == "" {
		kind = UploadKindDocument
		if path.Base(result.MarkerKey) == CollectionMarkerFilename {
			kind = UploadKindCollection
		}
	}
	return httpapi.DeleteResult{
		ID:   uploadIDFromMarkerKey(result.MarkerKey),
		Keys: nonNilStrings(result.Keys), PageKey: result.PageKey,
		MarkerKey: result.MarkerKey,
		Kind:      httpapi.DeleteResultKind(kind),
		Warnings:  serverSafeWarnings(result.Warnings),
	}
}

// ListManifestUploads returns service-scoped shared manifest records.
func (o *HTTPOperations) ListManifestUploads(
	ctx context.Context,
) (httpapi.ManifestList, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.ManifestList{}, err
	}
	result, err := client.ListManifest(ctx, ListManifestOptions{
		Scope: ManifestScopeService,
	})
	if err != nil {
		return httpapi.ManifestList{}, apiOperationError(err)
	}
	records := make([]httpapi.ManifestRecord, 0, len(result.Records))
	for _, record := range result.Records {
		records = append(records, wireManifestRecord(record))
	}
	return httpapi.ManifestList{
		Records: records, Warnings: serverSafeWarnings(result.Warnings),
	}, nil
}

func wireManifestRecord(record ManifestRecord) httpapi.ManifestRecord {
	wire := httpapi.ManifestRecord{
		Type: httpapi.ManifestRecordType(record.Type),
		Time: record.Time, Key: record.Key,
		SourceKey: record.SourceKey, MarkerKey: record.MarkerKey,
		URL: record.URL, Bucket: record.Bucket, Format: record.Format,
		Kind: httpapi.ManifestRecordKind(record.Kind),
		Slug: record.Slug, Title: record.Title,
		RepositoryURL: record.Repo, Bytes: record.Bytes,
		Objects: record.Objects, TotalBytes: record.TotalBytes,
		Reason: record.Reason, MarkerVersion: record.MarkerVersion,
		ProducerVersion: record.ProducerVersion,
		RendererVersion: record.RendererVersion,
		RevisionChainID: record.RevisionChainID, Revision: record.Revision,
		LatestRevision: record.LatestRevision,
		ProtectReason:  safeProtectReason(record.ProtectReason),
		Protected:      record.Protected,
	}
	if !record.CreatedAt.IsZero() {
		createdAt := record.CreatedAt
		wire.CreatedAt = &createdAt
	}
	if !record.ProtectedAt.IsZero() {
		protectedAt := record.ProtectedAt
		wire.ProtectedAt = &protectedAt
	}
	return wire
}

func coreManifestRecord(record httpapi.ManifestRecord) ManifestRecord {
	core := ManifestRecord{
		Type: string(record.Type), Time: record.Time, Key: record.Key,
		SourceKey: record.SourceKey, MarkerKey: record.MarkerKey,
		URL: record.URL, Bucket: record.Bucket, Format: record.Format,
		Kind: string(record.Kind), Slug: record.Slug, Title: record.Title,
		Repo: record.RepositoryURL, Bytes: record.Bytes,
		Objects: record.Objects, TotalBytes: record.TotalBytes,
		Reason: record.Reason, MarkerVersion: record.MarkerVersion,
		ProducerVersion: record.ProducerVersion,
		RendererVersion: record.RendererVersion,
		RevisionChainID: record.RevisionChainID, Revision: record.Revision,
		LatestRevision: record.LatestRevision,
		ProtectReason:  safeProtectReason(record.ProtectReason),
		Protected:      record.Protected,
	}
	if record.CreatedAt != nil {
		core.CreatedAt = *record.CreatedAt
	}
	if record.ProtectedAt != nil {
		core.ProtectedAt = *record.ProtectedAt
	}
	return core
}

// ListStorageUploads returns a direct marker-prefix storage snapshot.
func (o *HTTPOperations) ListStorageUploads(
	ctx context.Context,
) (httpapi.StorageList, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.StorageList{}, err
	}
	result, err := client.ListRemote(ctx)
	if err != nil {
		return httpapi.StorageList{}, apiOperationError(err)
	}
	uploads := make([]httpapi.RemoteUpload, 0, len(result))
	for _, upload := range result {
		uploads = append(uploads, httpapi.RemoteUpload{
			ID: upload.Dir, MarkerKey: upload.MarkerKey,
			Kind: httpapi.RemoteUploadKind(upload.Kind), Conflict: upload.Conflict,
			Protected: upload.Protected,
			Slug:      upload.Slug, Key: upload.Key, URL: upload.URL,
			Keys:    nonNilStrings(upload.Keys),
			Objects: upload.Objects, Bytes: upload.Bytes,
			LastModified: upload.LastModified,
		})
	}
	return httpapi.StorageList{Uploads: uploads, Warnings: []string{}}, nil
}

// SyncManifest invokes the shared reconciliation operation.
func (o *HTTPOperations) SyncManifest(
	ctx context.Context, request httpapi.SyncRequest,
) (httpapi.SyncResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.SyncResult{}, err
	}
	result, syncErr := client.SyncManifest(ctx, SyncManifestOptions{
		Prune: request.Prune, DryRun: request.DryRun,
		Concurrency: request.Concurrency,
	})
	if result == nil {
		return httpapi.SyncResult{}, apiOperationError(syncErr)
	}
	wire := httpapi.SyncResult{
		Unchanged: result.Unchanged, Deferred: result.Deferred,
		Incomplete: result.Incomplete,
		Invalid:    result.Invalid, Retained: result.Retained,
		Warnings:          serverSafeWarnings(result.Warnings),
		Complete:          len(result.Failures) == 0,
		AddedRecords:      []httpapi.ManifestRecord{},
		EnrichedRecords:   []httpapi.ManifestRecord{},
		TombstoneRecords:  []httpapi.ManifestRecord{},
		ProtectionRecords: []httpapi.ManifestRecord{},
		Failures:          []httpapi.SyncFailure{},
	}
	for _, record := range result.Added {
		wire.AddedRecords = append(
			wire.AddedRecords, wireManifestRecord(record),
		)
	}
	for _, record := range result.Enriched {
		wire.EnrichedRecords = append(
			wire.EnrichedRecords, wireManifestRecord(record),
		)
	}
	for _, record := range result.Tombstoned {
		wire.TombstoneRecords = append(
			wire.TombstoneRecords, wireManifestRecord(record),
		)
	}
	for _, record := range result.Protection {
		wire.ProtectionRecords = append(
			wire.ProtectionRecords, wireManifestRecord(record),
		)
	}
	for _, failure := range result.Failures {
		wire.Failures = append(wire.Failures, httpapi.SyncFailure{
			MarkerKey: failure.MarkerKey,
			Operation: httpapi.SyncFailureOperation(failure.Operation),
			Error:     serverSafeItemError(failure.Error),
		})
	}
	// Per-item sync failures are a successful partial result on the wire.
	if result.Failures != nil {
		return wire, nil
	}
	return wire, apiOperationError(syncErr)
}

// PreviewPurge returns explicit deletion candidates without mutation.
func (o *HTTPOperations) PreviewPurge(
	ctx context.Context, request httpapi.PurgePreviewRequest,
) (httpapi.PurgePreview, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.PurgePreview{}, err
	}
	var createdBefore time.Time
	if request.CreatedBefore != nil {
		createdBefore = *request.CreatedBefore
	}
	result, err := client.PlanPurge(ctx, PurgePlanOptions{
		Source:        UploadSource(request.Source),
		CreatedBefore: createdBefore,
		Slug:          request.Slug, All: request.All,
		Concurrency: request.Concurrency, IncludeVersioned: request.IncludeVersioned,
	})
	if err != nil {
		return httpapi.PurgePreview{}, apiOperationError(err)
	}
	wire := httpapi.PurgePreview{
		Invalid:    result.Invalid,
		Warnings:   serverSafeWarnings(result.Warnings),
		Candidates: []httpapi.PurgeCandidate{},
		Protected:  []httpapi.PurgeCandidate{},
		Versioned:  []httpapi.PurgeCandidate{},
	}
	wireCandidate := func(candidate PurgeCandidate) httpapi.PurgeCandidate {
		item := httpapi.PurgeCandidate{
			UploadID: candidate.UploadID,
			Record:   wireManifestRecord(candidate.Record),
			Warnings: serverSafeWarnings(candidate.Warnings),
		}
		if candidate.Inspection != nil {
			inspection := wireInspection(candidate.Inspection)
			item.Inspection = &inspection
		}
		return item
	}
	for _, candidate := range result.Candidates {
		wire.Candidates = append(wire.Candidates, wireCandidate(candidate))
	}
	for _, candidate := range result.Protected {
		wire.Protected = append(wire.Protected, wireCandidate(candidate))
	}
	for _, candidate := range result.Versioned {
		wire.Versioned = append(wire.Versioned, wireCandidate(candidate))
	}
	return wire, nil
}

// ExecutePurge deletes only explicit reviewed IDs.
func (o *HTTPOperations) ExecutePurge(
	ctx context.Context, request httpapi.PurgeRequest,
) (httpapi.PurgeResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.PurgeResult{}, err
	}
	result, purgeErr := client.Purge(ctx, PurgeRequest{
		UploadIDs: request.UploadIds, IncludeVersioned: request.IncludeVersioned,
	})
	if result == nil {
		return httpapi.PurgeResult{}, apiOperationError(purgeErr)
	}
	wire := httpapi.PurgeResult{Items: []httpapi.PurgeItemResult{}}
	for _, item := range result.Items {
		wireItem := httpapi.PurgeItemResult{
			UploadID: item.UploadID, Protected: item.Protected,
			Versioned: item.Versioned,
			Error:     serverSafeItemError(item.Error),
		}
		if item.Deleted != nil {
			deleted := wireDeleteResult(item.Deleted)
			wireItem.Deleted = &deleted
		}
		wire.Items = append(wire.Items, wireItem)
	}
	// Partial failures are carried per item on the successful response.
	return wire, nil
}

func apiOperationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return httpapi.NewProblemError(
			http.StatusRequestTimeout, "request_timeout", "Request timeout",
			"The operation was cancelled or exceeded its deadline.",
		)
	}
	if errors.Is(err, ErrInputTooLarge) {
		return httpapi.NewProblemError(
			http.StatusRequestEntityTooLarge, "input_too_large",
			"Input too large", "The upload exceeds the effective size limit.",
		)
	}
	if errors.Is(err, ErrRevisionHistoryFull) {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "revision_history_full",
			"Revision history full",
			"The revision chain cannot accept another revision.",
		)
	}
	if errors.Is(err, ErrBinaryInput) || errors.Is(err, ErrInvalidUTF8) ||
		errors.Is(err, ErrEmptyInput) ||
		errors.Is(err, errInvalidHostedRepository) {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_upload",
			"Invalid upload", "The request does not describe a valid Airplan upload.",
		)
	}
	var invalidDocument *InvalidDocumentInputError
	if errors.As(err, &invalidDocument) {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_upload",
			"Invalid upload", "The request does not describe a valid Airplan upload.",
		)
	}
	if errors.Is(err, errInvalidTarget) {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_target",
			"Invalid upload target",
			"The target is not a valid marker-managed Airplan upload.",
		)
	}
	var updateRefusal *updateRefusalError
	if errors.As(err, &updateRefusal) {
		switch updateRefusal.kind {
		case updateRefusalMissing:
			return httpapi.NewProblemError(
				http.StatusNotFound, "upload_not_found", "Upload not found",
				"The marker-managed upload could not be found.",
			)
		case updateRefusalInvalidUpload:
			return httpapi.NewProblemError(
				http.StatusUnprocessableEntity, "invalid_upload", "Invalid upload",
				"The revision chain cannot be safely reconciled.",
			)
		default:
			return httpapi.NewProblemError(
				http.StatusUnprocessableEntity, "invalid_target", "Invalid upload target",
				"The document is not eligible for a linked revision update.",
			)
		}
	}
	var refusal *upgradeRefusalError
	if errors.As(err, &refusal) {
		switch refusal.state {
		case UpgradeStateMissing:
			return httpapi.NewProblemError(
				http.StatusNotFound, "upload_not_found", "Upload not found",
				"The marker-managed upload could not be found.",
			)
		case UpgradeStateInvalid:
			return httpapi.NewProblemError(
				http.StatusUnprocessableEntity, "invalid_upload", "Invalid upload",
				"The ownership marker is invalid.",
			)
		case UpgradeStateIneligible:
			return httpapi.NewProblemError(
				http.StatusUnprocessableEntity, "invalid_target",
				"Invalid upload target", "The document is not eligible for upgrade.",
			)
		}
	}
	var revisionConflict *revisionAppendConflictError
	if errors.As(err, &revisionConflict) {
		return httpapi.NewProblemError(
			http.StatusConflict, "revision_conflict", "Revision conflict",
			"Another writer appended first; retry from the newly resolved latest revision.",
		)
	}
	if errors.Is(err, ErrConflict) {
		return httpapi.NewProblemError(
			http.StatusConflict, "upgrade_conflict", "Upgrade conflict",
			"The upload changed after it was planned; create a new upgrade plan.",
		)
	}
	if errors.Is(err, errInvalidProtectReason) {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_protect_reason",
			"Invalid protection reason",
			"The protection reason is not valid UTF-8, is too long, or contains control characters.",
		)
	}
	var protectedErr *UploadProtectedError
	if errors.As(err, &protectedErr) {
		problem := httpapi.NewProblemError(
			http.StatusConflict, "upload_protected", "Upload protected",
			"The upload is purge-protected; unprotect it or force deletion.",
		)
		// The advisory reason is already bounded and control-free (SPEC.md
		// §5), so it is safe to carry to the client for parity with the
		// direct S3 error text.
		problem.Problem.ProtectReason = protectedErr.Reason
		return problem
	}
	if _, ok := MarkerCode(err); ok {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_upload",
			"Invalid upload", "The ownership marker is invalid.",
		)
	}
	if errors.Is(err, errObjectNotFound) ||
		errors.Is(err, errOwnershipMarkerMissing) {
		return httpapi.NewProblemError(
			http.StatusNotFound, "upload_not_found", "Upload not found",
			"The marker-managed upload could not be found.",
		)
	}
	return err
}
