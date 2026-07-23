package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpUploadDocumentInput struct {
	Content       string `json:"content" jsonschema:"The UTF-8 document text to upload."`
	Name          string `json:"name,omitempty" jsonschema:"Source filename used for format detection."`
	Format        string `json:"format,omitempty" jsonschema:"Input format: md, html, or txt."`
	Title         string `json:"title,omitempty"`
	Slug          string `json:"slug,omitempty"`
	Lang          string `json:"lang,omitempty"`
	RepositoryURL string `json:"repository_url,omitempty"`
	MaxSize       int64  `json:"max_size,omitempty"`
}

type mcpUploadFilesInput struct {
	Paths        []string `json:"paths" jsonschema:"Local file paths to upload as one collection."`
	Title        string   `json:"title,omitempty"`
	MaxSize      int64    `json:"max_size,omitempty"`
	MaxTotalSize int64    `json:"max_total_size,omitempty"`
}

type mcpListInput struct {
	Source string `json:"source,omitempty" jsonschema:"Inventory source: manifest or storage."`
}

type mcpListOutput struct {
	Source   string         `json:"source"`
	Manifest *ManifestList  `json:"manifest,omitempty"`
	Storage  []RemoteUpload `json:"storage,omitempty"`
}

type mcpTargetInput struct {
	URLOrKey string `json:"url_or_key" jsonschema:"Airplan public URL, upload directory, marker key, or declared object key."`
}

type mcpSyncInput struct {
	Apply       bool `json:"apply,omitempty" jsonschema:"Write manifest changes. Defaults to false (preview only)."`
	Prune       bool `json:"prune,omitempty"`
	Concurrency int  `json:"concurrency,omitempty"`
}

type mcpPurgePreviewInput struct {
	Source        string     `json:"source,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	Slug          string     `json:"slug,omitempty"`
	All           bool       `json:"all,omitempty"`
	Concurrency   int        `json:"concurrency,omitempty"`
}

type mcpPurgeExecuteInput struct {
	UploadIDs []string `json:"upload_ids" jsonschema:"Exact upload_id values returned by preview_purge."`
}

// NewMCPServer builds the shared MCP tool server. LocalFiles controls whether
// upload_files is registered; hosted HTTP MCP never accepts server-local paths.
func NewMCPServer(
	client *Client, version string, localFiles bool,
) *mcp.Server {
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "airplan", Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "upload_document",
		Description: "Render and persist a UTF-8 document in Airplan, then " +
			"return its durable capability URL.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpUploadDocumentInput,
	) (*mcp.CallToolResult, Result, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		repository := input.RepositoryURL
		if !localFiles && repository == "" {
			repository = "none"
		}
		result, err := client.Upload(ctx, Input{
			Reader: strings.NewReader(input.Content), Name: input.Name,
			Format: input.Format, Title: input.Title, Slug: input.Slug,
			Lang: input.Lang, RepositoryURL: repository,
			MaxSize: mcpDocumentLimit(input.MaxSize, !localFiles),
		})
		if err != nil {
			return nil, Result{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return uploadToolContent(result), *result, nil
	})

	if localFiles {
		mcp.AddTool(server, &mcp.Tool{
			Name: "upload_files",
			Description: "Read the named local paths and persist them as one " +
				"Airplan collection. This is available only over local stdio.",
		}, func(ctx context.Context, _ *mcp.CallToolRequest,
			input mcpUploadFilesInput,
		) (*mcp.CallToolResult, FilesResult, error) {
			ctx, cancel := mcpOperationContext(ctx, client)
			defer cancel()
			files := make([]FileInput, 0, len(input.Paths))
			opened := make([]*os.File, 0, len(input.Paths))
			defer func() {
				for _, file := range opened {
					_ = file.Close()
				}
			}()
			for _, path := range input.Paths {
				file, err := os.Open(path)
				if err != nil {
					return nil, FilesResult{}, fmt.Errorf(
						"airplan: open collection file %q: %w", path, err,
					)
				}
				info, err := file.Stat()
				if err != nil || !info.Mode().IsRegular() {
					_ = file.Close()
					if err == nil {
						err = fmt.Errorf("not a regular file")
					}
					return nil, FilesResult{}, fmt.Errorf(
						"airplan: inspect collection file %q: %w", path, err,
					)
				}
				opened = append(opened, file)
				files = append(files, FileInput{
					Name: filepath.Base(path), Reader: file, Size: info.Size(),
				})
			}
			result, err := client.UploadFiles(ctx, FilesInput{
				Files: files, Title: input.Title,
				MaxSize: input.MaxSize, MaxTotalSize: input.MaxTotalSize,
			})
			if err != nil {
				return nil, FilesResult{}, mcpOperationError(err, false)
			}
			return uploadToolContent(&result.Result), *result, nil
		})
	}

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_uploads",
		Description: "List Airplan uploads from manifest history or directly " +
			"from marker-managed storage.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpListInput,
	) (*mcp.CallToolResult, mcpListOutput, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		source := input.Source
		if source == "" {
			source = string(UploadSourceManifest)
		}
		output := mcpListOutput{Source: source}
		switch UploadSource(source) {
		case UploadSourceManifest:
			listed, err := client.ListManifest(ctx, ListManifestOptions{
				Scope: ManifestScopeService,
			})
			if err != nil {
				return nil, output, mcpOperationError(err, !localFiles)
			}
			if !localFiles {
				listed.Warnings = serverSafeWarnings(listed.Warnings)
			}
			output.Manifest = listed
		case UploadSourceStorage:
			uploads, err := client.ListRemote(ctx)
			if err != nil {
				return nil, output, mcpOperationError(err, !localFiles)
			}
			if uploads == nil {
				uploads = []RemoteUpload{}
			}
			output.Storage = uploads
		default:
			return nil, output, fmt.Errorf(
				"airplan: source must be manifest or storage",
			)
		}
		return nil, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "inspect_upload",
		Description: "Validate and describe one marker-managed Airplan upload.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpTargetInput,
	) (*mcp.CallToolResult, UploadInspection, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.InspectUpload(ctx, input.URLOrKey)
		if err != nil {
			return nil, UploadInspection{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_upload",
		Description: "Permanently delete one explicit marker-managed upload. " +
			"Payload objects are removed first and the ownership marker last.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpTargetInput,
	) (*mcp.CallToolResult, DeleteResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.DeleteUpload(ctx, input.URLOrKey)
		if err != nil {
			return nil, DeleteResult{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "sync_manifest",
		Description: "Preview or apply storage-to-manifest reconciliation. " +
			"The default is a dry run; set apply=true to persist changes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpSyncInput,
	) (*mcp.CallToolResult, SyncManifestResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.SyncManifest(ctx, SyncManifestOptions{
			DryRun: !input.Apply, Prune: input.Prune,
			Concurrency: input.Concurrency,
		})
		if result == nil {
			return nil, SyncManifestResult{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
			for index := range result.Failures {
				result.Failures[index].Error = serverSafeItemError(
					result.Failures[index].Error,
				)
			}
		}
		return partialToolResult(mcpOperationError(err, !localFiles)), *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "preview_purge",
		Description: "Return explicit purge candidates without deleting " +
			"anything. Review upload_id values before execution.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpPurgePreviewInput,
	) (*mcp.CallToolResult, PurgePlan, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		source := UploadSource(input.Source)
		if source == "" {
			source = UploadSourceManifest
		}
		var createdBefore time.Time
		if input.CreatedBefore != nil {
			createdBefore = *input.CreatedBefore
		}
		result, err := client.PlanPurge(ctx, PurgePlanOptions{
			Source: source, CreatedBefore: createdBefore,
			Slug: input.Slug, All: input.All,
			Concurrency: input.Concurrency,
		})
		if err != nil {
			return nil, PurgePlan{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
			for index := range result.Candidates {
				result.Candidates[index].Warnings = serverSafeWarnings(
					result.Candidates[index].Warnings,
				)
			}
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "execute_purge",
		Description: "Permanently delete only the explicit upload_id values " +
			"reviewed from preview_purge. Each marker is revalidated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpPurgeExecuteInput,
	) (*mcp.CallToolResult, PurgeResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.Purge(ctx, PurgeRequest(input))
		if result == nil {
			return nil, PurgeResult{}, mcpOperationError(err, !localFiles)
		}
		if !localFiles {
			for index := range result.Items {
				result.Items[index].Error = serverSafeItemError(
					result.Items[index].Error,
				)
				if result.Items[index].Deleted != nil {
					result.Items[index].Deleted.Warnings = serverSafeWarnings(
						result.Items[index].Deleted.Warnings,
					)
				}
			}
		}
		return partialToolResult(mcpOperationError(err, !localFiles)), *result, nil
	})

	return server
}

func uploadToolContent(result *Result) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: result.URL},
		&mcp.ResourceLink{
			URI: result.URL, Name: result.Title,
			Description: "Uploaded Airplan page",
			MIMEType:    result.ContentType,
		},
	}}
}

func partialToolResult(err error) *mcp.CallToolResult {
	if err == nil {
		return nil
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func mcpOperationError(err error, hosted bool) error {
	if err == nil || !hosted {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return errors.New("airplan: the server operation timed out")
	case errors.Is(err, ErrInputTooLarge):
		return errors.New("airplan: the upload exceeds the effective size limit")
	case errors.Is(err, ErrBinaryInput), errors.Is(err, ErrInvalidUTF8),
		errors.Is(err, ErrEmptyInput):
		return errors.New("airplan: the request is not a valid document upload")
	default:
		return errors.New("airplan: the server could not complete the operation")
	}
}

func mcpDocumentLimit(requested int64, hosted bool) int64 {
	if !hosted {
		return requested
	}
	if requested > 0 && requested < DefaultMaxInputSize {
		return requested
	}
	return DefaultMaxInputSize
}

func mcpOperationContext(
	ctx context.Context, client *Client,
) (context.Context, context.CancelFunc) {
	if client != nil && client.cfg != nil && client.cfg.Timeout > 0 {
		return context.WithTimeout(ctx, client.cfg.Timeout)
	}
	return ctx, func() {}
}

// RunMCPStdio serves MCP frames over stdin/stdout until context cancellation.
func RunMCPStdio(ctx context.Context, client *Client, version string) error {
	return NewMCPServer(client, version, true).Run(ctx, &mcp.StdioTransport{})
}

// NewMCPHTTPHandler returns the current stateless Streamable HTTP MCP
// transport. Present Origin values must match the explicit allowlist.
func NewMCPHTTPHandler(
	client *Client, version string, allowedOrigins []string,
) (http.Handler, error) {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" ||
			u.User != nil || u.Path != "" || u.RawQuery != "" ||
			u.Fragment != "" {
			return nil, fmt.Errorf(
				"airplan: invalid allowed origin %q", origin,
			)
		}
		allowed[origin] = struct{}{}
	}
	server := NewMCPServer(client, version, false)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := r.Header.Values("Origin")
		if len(origins) > 1 {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if len(origins) == 1 {
			if _, ok := allowed[origins[0]]; !ok {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
	return limitMCPRequestBody(originHandler, mcpHTTPMaxRequestBytes), nil
}

const mcpHTTPMaxRequestBytes = 6*DefaultMaxInputSize + (1 << 20)

func limitMCPRequestBody(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > limit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		body := &mcpLimitedBody{
			body: r.Body, remaining: limit, limit: limit,
		}
		r.Body = body
		next.ServeHTTP(&mcpLimitResponseWriter{
			ResponseWriter: w, body: body,
		}, r)
	})
}

type mcpLimitedBody struct {
	body      io.ReadCloser
	remaining int64
	limit     int64
	exceeded  bool
}

func (b *mcpLimitedBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		var extra [1]byte
		n, err := b.body.Read(extra[:])
		if n > 0 {
			b.exceeded = true
			return 0, &http.MaxBytesError{Limit: b.limit}
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.body.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *mcpLimitedBody) Close() error { return b.body.Close() }

type mcpLimitResponseWriter struct {
	http.ResponseWriter
	body        *mcpLimitedBody
	wroteHeader bool
}

func (w *mcpLimitResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *mcpLimitResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if w.body.exceeded {
		status = http.StatusRequestEntityTooLarge
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *mcpLimitResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
