package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"

	"github.com/jimeh/airplan/internal/httpapi/generated"
)

const maxClientJSONResponseBytes = int64(64 << 20)

// Client is the typed, non-retrying Airplan REST client foundation.
type Client struct {
	generated generated.ClientInterface
}

// NewClient constructs a typed client for one Airplan server URL.
func NewClient(
	baseURL string,
	token string,
	httpClient *http.Client,
) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Airplan API URL: %w", err)
	}
	if !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("airplan API URL must be an absolute HTTP(S) URL")
	}
	if token == "" {
		return nil, errors.New("airplan API token is empty")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	generatedClient, err := generated.NewClient(
		baseURL,
		generated.WithHTTPClient(httpClient),
		generated.WithRequestEditorFn(
			func(_ context.Context, request *http.Request) error {
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set(
					"Accept", "application/json, application/problem+json",
				)
				return nil
			},
		),
	)
	if err != nil {
		return nil, fmt.Errorf("construct generated Airplan client: %w", err)
	}
	return &Client{generated: generatedClient}, nil
}

// Health calls the unauthenticated process liveness endpoint.
func (c *Client) Health(ctx context.Context) (Health, error) {
	response, err := c.generated.Health(ctx)
	if err != nil {
		return Health{}, err
	}
	return decodeResponse[Health](response, http.StatusOK)
}

// OpenAPI returns the exact schema served by the remote process.
func (c *Client) OpenAPI(ctx context.Context) ([]byte, error) {
	response, err := c.generated.GetOpenAPI(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if err = responseProblem(response); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(response.Body, maxClientJSONResponseBytes))
}

// Capabilities returns safe compatibility information.
func (c *Client) Capabilities(ctx context.Context) (Capabilities, error) {
	response, err := c.generated.GetCapabilities(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	return decodeResponse[Capabilities](response, http.StatusOK)
}

// UploadDocument streams a multipart document upload without buffering the
// document in memory. The request is never retried automatically.
func (c *Client) UploadDocument(
	ctx context.Context,
	metadata DocumentMetadata,
	document io.Reader,
) (UploadResult, error) {
	if document == nil {
		return UploadResult{}, errors.New("airplan document reader is nil")
	}
	body, contentType := multipartBody(func(writer *multipart.Writer) error {
		if err := writeMetadataPart(writer, metadata); err != nil {
			return err
		}
		header := make(textproto.MIMEHeader)
		header.Set(
			"Content-Disposition",
			mime.FormatMediaType("form-data", map[string]string{
				"name":     "document",
				"filename": metadata.Name,
			}),
		)
		header.Set("Content-Type", "application/octet-stream")
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(part, document)
		return err
	})
	response, err := c.generated.UploadDocumentWithBody(ctx, contentType, body)
	if err != nil {
		return UploadResult{}, err
	}
	return decodeResponse[UploadResult](response, http.StatusCreated)
}

// UploadCollection streams collection members in caller order.
func (c *Client) UploadCollection(
	ctx context.Context,
	metadata CollectionMetadata,
	files []CollectionFile,
) (UploadResult, error) {
	if len(files) == 0 {
		return UploadResult{}, errors.New("airplan collection is empty")
	}
	body, contentType := multipartBody(func(writer *multipart.Writer) error {
		if err := writeMetadataPart(writer, metadata); err != nil {
			return err
		}
		for _, file := range files {
			if file.Reader == nil {
				return fmt.Errorf("airplan collection reader %q is nil", file.Name)
			}
			if _, err := file.Reader.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("seek collection input %q: %w", file.Name, err)
			}
			header := make(textproto.MIMEHeader)
			header.Set(
				"Content-Disposition",
				mime.FormatMediaType("form-data", map[string]string{
					"name":     "files",
					"filename": file.Name,
				}),
			)
			contentType := file.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			header.Set("Content-Type", contentType)
			part, err := writer.CreatePart(header)
			if err != nil {
				return err
			}
			if _, err = io.Copy(part, io.LimitReader(file.Reader, file.Size)); err != nil {
				return err
			}
		}
		return nil
	})
	response, err := c.generated.UploadCollectionWithBody(ctx, contentType, body)
	if err != nil {
		return UploadResult{}, err
	}
	return decodeResponse[UploadResult](response, http.StatusCreated)
}

// InspectUpload validates and describes one marker-managed upload.
func (c *Client) InspectUpload(
	ctx context.Context,
	request TargetRequest,
) (UploadInspection, error) {
	response, err := c.generated.InspectUpload(ctx, request)
	if err != nil {
		return UploadInspection{}, err
	}
	return decodeResponse[UploadInspection](response, http.StatusOK)
}

// GetUpload opens one streaming marker-declared object response. The caller
// must close Download.Body.
func (c *Client) GetUpload(
	ctx context.Context,
	request GetUploadRequest,
) (Download, error) {
	response, err := c.generated.GetUpload(ctx, request)
	if err != nil {
		return Download{}, err
	}
	if err = responseProblem(response); err != nil {
		_ = response.Body.Close()
		return Download{}, err
	}
	filename := "airplan-download"
	if _, params, parseErr := mime.ParseMediaType(
		response.Header.Get("Content-Disposition"),
	); parseErr == nil && params["filename"] != "" {
		filename = params["filename"]
	}
	return Download{
		Body:        response.Body,
		Key:         response.Header.Get("X-Airplan-Object-Key"),
		Filename:    filename,
		ContentType: response.Header.Get("Content-Type"),
	}, nil
}

// DeleteUpload permanently deletes one marker-managed upload.
func (c *Client) DeleteUpload(
	ctx context.Context,
	request TargetRequest,
) (DeleteResult, error) {
	response, err := c.generated.DeleteUpload(ctx, request)
	if err != nil {
		return DeleteResult{}, err
	}
	return decodeResponse[DeleteResult](response, http.StatusOK)
}

// ListManifestUploads lists scoped server manifest history.
func (c *Client) ListManifestUploads(ctx context.Context) (ManifestList, error) {
	response, err := c.generated.ListManifestUploads(ctx)
	if err != nil {
		return ManifestList{}, err
	}
	return decodeResponse[ManifestList](response, http.StatusOK)
}

// ListStorageUploads lists direct storage candidates.
func (c *Client) ListStorageUploads(ctx context.Context) (StorageList, error) {
	response, err := c.generated.ListStorageUploads(ctx)
	if err != nil {
		return StorageList{}, err
	}
	return decodeResponse[StorageList](response, http.StatusOK)
}

// SyncManifest plans or applies storage-to-manifest reconciliation.
func (c *Client) SyncManifest(
	ctx context.Context,
	request SyncRequest,
) (SyncResult, error) {
	response, err := c.generated.SyncManifest(ctx, request)
	if err != nil {
		return SyncResult{}, err
	}
	return decodeResponse[SyncResult](response, http.StatusOK)
}

// PreviewPurge returns explicit candidates without deleting them.
func (c *Client) PreviewPurge(
	ctx context.Context,
	request PurgePreviewRequest,
) (PurgePreview, error) {
	response, err := c.generated.PreviewPurge(ctx, request)
	if err != nil {
		return PurgePreview{}, err
	}
	return decodeResponse[PurgePreview](response, http.StatusOK)
}

// ExecutePurge permanently deletes explicit reviewed upload IDs.
func (c *Client) ExecutePurge(
	ctx context.Context,
	request PurgeRequest,
) (PurgeResult, error) {
	response, err := c.generated.ExecutePurge(ctx, request)
	if err != nil {
		return PurgeResult{}, err
	}
	return decodeResponse[PurgeResult](response, http.StatusOK)
}

func decodeResponse[T any](response *http.Response, status int) (T, error) {
	var result T
	defer func() { _ = response.Body.Close() }()
	if err := responseProblem(response); err != nil {
		return result, err
	}
	if response.StatusCode != status {
		return result, fmt.Errorf("airplan API returned HTTP %d", response.StatusCode)
	}
	err := json.NewDecoder(io.LimitReader(
		response.Body,
		maxClientJSONResponseBytes,
	)).Decode(&result)
	return result, err
}

func responseProblem(response *http.Response) error {
	if response.StatusCode < 400 {
		return nil
	}
	var problem Problem
	err := json.NewDecoder(io.LimitReader(
		response.Body,
		maxClientJSONResponseBytes,
	)).Decode(&problem)
	if err != nil || problem.Status == 0 || problem.Code == "" {
		return fmt.Errorf("airplan API returned HTTP %d", response.StatusCode)
	}
	return &ProblemError{Problem: problem}
}

func multipartBody(write func(*multipart.Writer) error) (io.Reader, string) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	go func() {
		err := write(multipartWriter)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()
	return reader, contentType
}

func writeMetadataPart(writer *multipart.Writer, metadata any) error {
	header := make(textproto.MIMEHeader)
	header.Set(
		"Content-Disposition",
		mime.FormatMediaType("form-data", map[string]string{"name": "metadata"}),
	)
	header.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	return json.NewEncoder(part).Encode(metadata)
}
