package airplan

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"

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
			"upload_document", "upload_collection", "inspect_upload",
			"get_upload", "delete_upload", "list_manifest",
			"list_storage", "sync_manifest", "preview_purge",
			"execute_purge",
		},
		UploadFormats: []httpapi.CapabilitiesUploadFormats{"md", "html", "txt"},
		Limits: httpapi.UploadLimits{
			DocumentBytes:        DefaultMaxInputSize,
			CollectionFileBytes:  DefaultMaxCollectionFileSize,
			CollectionTotalBytes: DefaultMaxCollectionTotalSize,
		},
		MarkerVersions: []int{1, 2, MarkerVersion},
	}, nil
}

// UploadDocument invokes the shared document operation.
func (o *HTTPOperations) UploadDocument(
	ctx context.Context, upload httpapi.DocumentUpload,
) (httpapi.UploadResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.UploadResult{}, err
	}
	result, err := client.Upload(ctx, Input{
		Reader: upload.Document, Name: upload.Metadata.Name,
		Format: string(upload.Metadata.Format), Title: upload.Metadata.Title,
		Slug: upload.Metadata.Slug, Lang: upload.Metadata.Lang,
		RepositoryURL: upload.Metadata.RepositoryURL,
		MaxSize:       upload.Metadata.MaxSize,
	})
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	return wireUploadResult(result, nil), nil
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
	result, err := client.UploadFiles(ctx, FilesInput{
		Files: files, Title: upload.Metadata.Title,
		RepositoryURL: upload.Metadata.RepositoryURL,
		MaxSize:       upload.Metadata.MaxSize,
		MaxTotalSize:  upload.Metadata.MaxTotalSize,
	})
	if err != nil {
		return httpapi.UploadResult{}, apiOperationError(err)
	}
	return wireUploadResult(&result.Result, result.Files), nil
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
		RepositoryURL: result.RepositoryURL,
		Warnings:      serverSafeWarnings(result.Warnings), Files: wireFiles,
	}
}

func serverSafeWarnings(warnings []string) []string {
	safe := append([]string(nil), warnings...)
	const templateFailure = " (note: the template also failed to load:"
	for index, warning := range safe {
		if start := strings.Index(warning, templateFailure); start >= 0 {
			safe[index] = warning[:start] +
				" (note: the configured template also failed to load)"
		}
	}
	return safe
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
		Warnings: append([]string(nil), result.Warnings...),
		Error:    string(result.Error),
	}
	if result.CreatedAt.IsZero() {
		wire.CreatedAt = nil
	}
	wire.Page = wireInspectedObject(result.Page)
	wire.Source = wireInspectedObject(result.Source)
	for _, file := range result.Files {
		wireFile := wireInspectedObject(file)
		if wireFile != nil {
			wire.Files = append(wire.Files, *wireFile)
		}
	}
	if wire.Files == nil {
		wire.Files = []httpapi.InspectedObject{}
	}
	return wire
}

func wireInspectedObject(object *InspectedObject) *httpapi.InspectedObject {
	if object == nil {
		return nil
	}
	return &httpapi.InspectedObject{
		Key: object.Key, URL: object.URL, Exists: object.Exists,
		Bytes: object.Bytes, ExpectedBytes: object.ExpectedBytes,
		ExpectedKnown: object.ExpectedKnown,
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
			Source: request.Source,
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
	ctx context.Context, request httpapi.TargetRequest,
) (httpapi.DeleteResult, error) {
	client, err := o.client()
	if err != nil {
		return httpapi.DeleteResult{}, err
	}
	result, err := client.DeleteUpload(ctx, request.URLOrKey)
	if err != nil {
		return httpapi.DeleteResult{}, apiOperationError(err)
	}
	return wireDeleteResult(result), nil
}

func wireDeleteResult(result *DeleteResult) httpapi.DeleteResult {
	return httpapi.DeleteResult{
		ID:   uploadIDFromMarkerKey(result.MarkerKey),
		Keys: append([]string(nil), result.Keys...), PageKey: result.PageKey,
		MarkerKey: result.MarkerKey,
		Kind:      httpapi.DeleteResultKind(result.Kind),
		Warnings:  append([]string(nil), result.Warnings...),
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
		Records: records, Warnings: append([]string(nil), result.Warnings...),
	}, nil
}

func wireManifestRecord(record ManifestRecord) httpapi.ManifestRecord {
	return httpapi.ManifestRecord{
		Type: httpapi.ManifestRecordType(record.Type),
		Time: record.Time, Key: record.Key,
		SourceKey: record.SourceKey, MarkerKey: record.MarkerKey,
		URL: record.URL, Bucket: record.Bucket, Format: record.Format,
		Kind: httpapi.ManifestRecordKind(record.Kind),
		Slug: record.Slug, Title: record.Title,
		RepositoryURL: record.Repo, Bytes: record.Bytes,
		Reason: record.Reason, MarkerVersion: record.MarkerVersion,
	}
}

func coreManifestRecord(record httpapi.ManifestRecord) ManifestRecord {
	return ManifestRecord{
		Type: string(record.Type), Time: record.Time, Key: record.Key,
		SourceKey: record.SourceKey, MarkerKey: record.MarkerKey,
		URL: record.URL, Bucket: record.Bucket, Format: record.Format,
		Kind: string(record.Kind), Slug: record.Slug, Title: record.Title,
		Repo: record.RepositoryURL, Bytes: record.Bytes,
		Reason: record.Reason, MarkerVersion: record.MarkerVersion,
	}
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
			Slug: upload.Slug, Key: upload.Key, URL: upload.URL,
			Keys:    append([]string(nil), upload.Keys...),
			Objects: upload.Objects, Bytes: upload.Bytes,
			LastModified: upload.LastModified,
		})
	}
	return httpapi.StorageList{Uploads: uploads}, nil
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
		Unchanged: result.Unchanged, Incomplete: result.Incomplete,
		Invalid: result.Invalid, Retained: result.Retained,
		Warnings: append([]string(nil), result.Warnings...),
		Complete: len(result.Failures) == 0,
	}
	for _, record := range result.Added {
		wire.AddedRecords = append(
			wire.AddedRecords, wireManifestRecord(record),
		)
	}
	for _, record := range result.Tombstoned {
		wire.TombstoneRecords = append(
			wire.TombstoneRecords, wireManifestRecord(record),
		)
	}
	for _, failure := range result.Failures {
		wire.Failures = append(wire.Failures, httpapi.SyncFailure{
			MarkerKey: failure.MarkerKey,
			Operation: httpapi.SyncFailureOperation(failure.Operation),
			Error:     failure.Error,
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
	result, err := client.PlanPurge(ctx, PurgePlanOptions{
		Source:        UploadSource(request.Source),
		CreatedBefore: request.CreatedBefore,
		Slug:          request.Slug, All: request.All,
		Concurrency: request.Concurrency,
	})
	if err != nil {
		return httpapi.PurgePreview{}, apiOperationError(err)
	}
	wire := httpapi.PurgePreview{
		Invalid:  result.Invalid,
		Warnings: append([]string(nil), result.Warnings...),
	}
	for _, candidate := range result.Candidates {
		item := httpapi.PurgeCandidate{
			UploadID: candidate.UploadID,
			Record:   wireManifestRecord(candidate.Record),
			Warnings: append([]string(nil), candidate.Warnings...),
		}
		if candidate.Inspection != nil {
			inspection := wireInspection(candidate.Inspection)
			item.Inspection = &inspection
		}
		wire.Candidates = append(wire.Candidates, item)
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
		UploadIDs: request.UploadIds,
	})
	if result == nil {
		return httpapi.PurgeResult{}, apiOperationError(purgeErr)
	}
	wire := httpapi.PurgeResult{}
	for _, item := range result.Items {
		wireItem := httpapi.PurgeItemResult{
			UploadID: item.UploadID, Error: item.Error,
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
	if errors.Is(err, ErrBinaryInput) || errors.Is(err, ErrInvalidUTF8) ||
		errors.Is(err, ErrEmptyInput) ||
		strings.Contains(err.Error(), "invalid") ||
		strings.Contains(err.Error(), "requires") {
		return httpapi.NewProblemError(
			http.StatusUnprocessableEntity, "invalid_upload",
			"Invalid upload", "The request does not describe a valid Airplan upload.",
		)
	}
	if strings.Contains(err.Error(), "missing") ||
		strings.Contains(err.Error(), "not found") {
		return httpapi.NewProblemError(
			http.StatusNotFound, "upload_not_found", "Upload not found",
			"The marker-managed upload could not be found.",
		)
	}
	return err
}
