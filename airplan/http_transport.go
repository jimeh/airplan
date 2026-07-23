package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
		RepositoryURL: result.RepositoryURL,
		Warnings:      append([]string(nil), result.Warnings...),
	}}
	for _, file := range result.Files {
		core.Files = append(core.Files, FileResult{
			Name: file.Name, URL: file.URL, Key: file.Key,
			Bytes: file.Bytes, ContentType: file.ContentType,
		})
	}
	return core
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
			Slug: upload.Slug, Key: upload.Key, URL: upload.URL,
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
		Page:     coreInspectedObject(result.Page),
		Source:   coreInspectedObject(result.Source),
		Warnings: append([]string(nil), result.Warnings...),
		Error:    MarkerErrorCode(result.Error),
	}
	if result.CreatedAt != nil {
		core.CreatedAt = *result.CreatedAt
	}
	for _, file := range result.Files {
		file := file
		core.Files = append(core.Files, coreInspectedObject(&file))
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
		URLOrKey: target, Source: opts.Source,
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
	ctx context.Context, target string,
) (*DeleteResult, error) {
	result, err := t.client.DeleteUpload(ctx, httpapi.TargetRequest{
		URLOrKey: target,
	})
	if err != nil {
		return nil, transportError(err)
	}
	return &DeleteResult{
		Keys:    append([]string(nil), result.Keys...),
		PageKey: result.PageKey, MarkerKey: result.MarkerKey,
		Kind:     UploadKind(result.Kind),
		Warnings: append([]string(nil), result.Warnings...),
	}, nil
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
		Unchanged: result.Unchanged, Incomplete: result.Incomplete,
		Invalid: result.Invalid, Retained: result.Retained,
		Warnings: append([]string(nil), result.Warnings...),
	}
	for _, record := range result.AddedRecords {
		core.Added = append(core.Added, coreManifestRecord(record))
	}
	for _, record := range result.TombstoneRecords {
		core.Tombstoned = append(
			core.Tombstoned, coreManifestRecord(record),
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
		Concurrency: opts.Concurrency,
	}
	if !opts.CreatedBefore.IsZero() {
		request.CreatedBefore = &opts.CreatedBefore
	}
	result, err := t.client.PreviewPurge(ctx, request)
	if err != nil {
		return nil, transportError(err)
	}
	core := &PurgePlan{
		Invalid:  result.Invalid,
		Warnings: append([]string(nil), result.Warnings...),
	}
	for _, candidate := range result.Candidates {
		item := PurgeCandidate{
			UploadID: candidate.UploadID,
			Record:   coreManifestRecord(candidate.Record),
			Warnings: append([]string(nil), candidate.Warnings...),
		}
		if candidate.Inspection != nil {
			item.Inspection = coreInspection(*candidate.Inspection)
		}
		core.Candidates = append(core.Candidates, item)
	}
	return core, nil
}

func (t *httpTransport) Purge(
	ctx context.Context, req PurgeRequest,
) (*PurgeResult, error) {
	result, err := t.client.ExecutePurge(ctx, httpapi.PurgeRequest{
		UploadIds: req.UploadIDs,
	})
	if err != nil {
		return nil, transportError(err)
	}
	core := &PurgeResult{}
	failed := 0
	for _, item := range result.Items {
		coreItem := PurgeItemResult{
			UploadID: item.UploadID, Error: item.Error,
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
