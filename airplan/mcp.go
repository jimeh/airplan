package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jimeh/airplan/internal/serverlog"
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
	Source    string  `json:"source,omitempty" jsonschema:"Inventory source: manifest or storage."`
	NewerThan *string `json:"newer_than,omitempty" jsonschema:"Keep uploads at or after an age or date, such as 7d or 2026-07-01."`
	OlderThan *string `json:"older_than,omitempty" jsonschema:"Keep uploads before an age or date, such as 30d or 2026-07-01."`
	Limit     *int    `json:"limit,omitempty" jsonschema:"Keep only the N most recent matches, still ordered oldest first."`
	Kind      *string `json:"kind,omitempty" jsonschema:"Keep only document or collection uploads."`
	Slug      *string `json:"slug,omitempty" jsonschema:"Glob matched against document slugs; collections never match."`
	Protected *bool   `json:"protected,omitempty" jsonschema:"Keep only uploads whose purge-protection state matches this value."`
}

var errInvalidMCPListFilter = errors.New(
	"airplan: invalid list filter arguments",
)

type mcpListFilterError struct {
	cause error
}

func (e *mcpListFilterError) Error() string { return e.cause.Error() }
func (e *mcpListFilterError) Unwrap() []error {
	return []error{errInvalidMCPListFilter, e.cause}
}

func invalidMCPListFilter(err error) error {
	if err == nil {
		return nil
	}
	return &mcpListFilterError{cause: err}
}

// listFilter resolves the tool's selection arguments (SPEC.md §9). It shares
// the parser and the filter the CLI uses, so a listing selects the same
// uploads through either surface.
func (in mcpListInput) listFilter(now time.Time) (ListFilter, error) {
	filter := ListFilter{Limit: in.Limit, Protected: in.Protected}
	if in.NewerThan != nil {
		when, err := ParseTimeFilter(*in.NewerThan, now)
		if err != nil {
			return filter, invalidMCPListFilter(err)
		}
		filter.NewerThan = &when
	}
	if in.OlderThan != nil {
		when, err := ParseTimeFilter(*in.OlderThan, now)
		if err != nil {
			return filter, invalidMCPListFilter(err)
		}
		filter.OlderThan = &when
	}
	if in.Kind != nil {
		if strings.TrimSpace(*in.Kind) == "" {
			return filter, invalidMCPListFilter(
				errors.New("airplan: kind must not be empty"),
			)
		}
		filter.Kind = UploadKind(*in.Kind)
	}
	if in.Slug != nil {
		if *in.Slug == "" {
			return filter, invalidMCPListFilter(
				errors.New("airplan: slug must not be empty"),
			)
		}
		filter.Slug = *in.Slug
	}
	return filter, invalidMCPListFilter(filter.Validate())
}

type mcpListOutput struct {
	Source   string          `json:"source"`
	Manifest *ManifestList   `json:"manifest,omitempty"`
	Storage  *[]RemoteUpload `json:"storage,omitempty"`
}

type mcpTargetInput struct {
	URLOrKey string `json:"url_or_key" jsonschema:"Airplan public URL, upload directory, marker key, or declared object key."`
}

type mcpDeleteInput struct {
	URLOrKey string `json:"url_or_key" jsonschema:"Airplan public URL, upload directory, marker key, or declared object key."`
	Force    bool   `json:"force,omitempty" jsonschema:"Delete the upload even when it is purge-protected."`
}

type mcpProtectInput struct {
	URLOrKey string `json:"url_or_key" jsonschema:"Airplan public URL, upload directory, marker key, or declared object key."`
	Reason   string `json:"reason,omitempty" jsonschema:"Optional short note stored with the protection sentinel (at most 256 characters)."`
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

type mcpUpgradeDocumentInput struct {
	URLOrKey string `json:"url_or_key" jsonschema:"Airplan public URL or object key."`
	Apply    bool   `json:"apply,omitempty" jsonschema:"Apply the planned upgrade. Defaults to preview only."`
	Force    bool   `json:"force,omitempty"`
}

type mcpUpgradeDocumentsInput struct {
	Apply       bool                  `json:"apply,omitempty" jsonschema:"Apply exact preview items. Defaults to preview only."`
	Items       []UpgradeDocumentPlan `json:"items,omitempty" jsonschema:"Exact upgradeable items returned by a prior preview."`
	Concurrency int                   `json:"concurrency,omitempty"`
}

// MCPServerOptions configures an MCP server without changing its tool surface.
type MCPServerOptions struct {
	LocalFiles bool
	Logger     *slog.Logger
}

// NewMCPServer builds the shared MCP tool server. LocalFiles controls whether
// upload_files is registered; hosted HTTP MCP never accepts server-local paths.
func NewMCPServer(
	client *Client, version string, localFiles bool,
) *mcp.Server {
	return NewMCPServerWithOptions(client, version, MCPServerOptions{
		LocalFiles: localFiles,
	})
}

// NewMCPServerWithOptions builds the shared MCP server with optional
// observability. Logging is disabled when Logger is nil.
func NewMCPServerWithOptions(
	client *Client, version string, options MCPServerOptions,
) *mcp.Server {
	localFiles := options.LocalFiles
	if version == "" {
		version = "dev"
	}
	sdkLogger := serverlog.SafeMCPLogger(options.Logger)
	server := mcp.NewServer(&mcp.Implementation{
		Name: "airplan", Version: version,
	}, &mcp.ServerOptions{Logger: sdkLogger})
	if options.Logger != nil {
		server.AddReceivingMiddleware(mcpLoggingMiddleware(options.Logger))
	}

	mcp.AddTool(server, &mcp.Tool{
		Name: "upload_document",
		Description: "Render and persist a UTF-8 Markdown, HTML, or text " +
			"document, then return its durable capability URL. Markdown " +
			"supports GFM, highlighted code, Mermaid fences, GitHub alerts, " +
			"frontmatter, footnotes, and responsive columns; use them when " +
			"they improve clarity.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpUploadDocumentInput,
	) (*mcp.CallToolResult, Result, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		repository := input.RepositoryURL
		if !localFiles {
			var err error
			repository, err = hostedRepositoryURL(repository)
			if err != nil {
				return nil, Result{}, errors.New(
					"airplan: repository_url must be none or an explicit repository URL",
				)
			}
		}
		result, err := client.Upload(ctx, Input{
			Reader: strings.NewReader(input.Content), Name: input.Name,
			Format: input.Format, Title: input.Title, Slug: input.Slug,
			Lang: input.Lang, RepositoryURL: repository,
			MaxSize: mcpDocumentLimit(input.MaxSize, !localFiles),
		})
		if err != nil {
			return nil, Result{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
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
				return nil, FilesResult{}, mcpOperationError(
					ctx, err, false, options.Logger,
				)
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
		filter, err := input.listFilter(time.Now())
		if err != nil {
			return nil, output, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		switch UploadSource(source) {
		case UploadSourceManifest:
			listed, err := client.ListManifest(ctx, ListManifestOptions{
				Scope: ManifestScopeService,
			})
			if err != nil {
				return nil, output, mcpOperationError(
					ctx, err, !localFiles, options.Logger,
				)
			}
			if !localFiles {
				listed.Warnings = serverSafeWarnings(listed.Warnings)
				listed.Records = serverSafeManifestRecords(listed.Records)
			}
			listed.Records = filter.FilterManifestRecords(listed.Records)
			output.Manifest = listed
		case UploadSourceStorage:
			uploads, err := client.ListRemote(ctx)
			if err != nil {
				return nil, output, mcpOperationError(
					ctx, err, !localFiles, options.Logger,
				)
			}
			uploads = filter.FilterRemoteUploads(uploads)
			if uploads == nil {
				uploads = []RemoteUpload{}
			}
			output.Storage = &uploads
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
			return nil, UploadInspection{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "upgrade_document",
		Description: "Preview or apply an in-place renderer upgrade for one " +
			"source-backed Markdown upload. Defaults to preview; set apply=true " +
			"to preserve its URL and source while updating its page and marker.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpUpgradeDocumentInput,
	) (*mcp.CallToolResult, any, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		plan, err := client.PlanUpgradeDocument(ctx, input.URLOrKey,
			UpgradeDocumentOptions{Force: input.Force})
		if err != nil {
			return nil, nil, mcpOperationError(ctx, err, !localFiles, options.Logger)
		}
		if !input.Apply || plan.State != UpgradeStateUpgradeable {
			if !localFiles {
				plan.Profile = ""
			}
			return nil, *plan, nil
		}
		result, err := client.UpgradeDocument(ctx, *plan)
		if err != nil {
			return nil, nil, mcpOperationError(ctx, err, !localFiles, options.Logger)
		}
		return uploadToolContent(&result.Result), *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "upgrade_documents",
		Description: "Preview manifest-backed document upgrades, or apply only " +
			"the exact preview items supplied with apply=true.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpUpgradeDocumentsInput,
	) (*mcp.CallToolResult, any, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		if !input.Apply {
			result, err := client.PlanBulkUpgrade(ctx,
				BulkUpgradeOptions{Concurrency: input.Concurrency})
			if err != nil {
				return nil, nil, mcpOperationError(ctx, err, !localFiles, options.Logger)
			}
			if !localFiles {
				result.Warnings = serverSafeWarnings(result.Warnings)
				for index := range result.Items {
					result.Items[index].Profile = ""
				}
			}
			return nil, *result, nil
		}
		if len(input.Items) == 0 {
			return nil, nil, errors.New("airplan: apply requires exact preview items")
		}
		result, err := client.ExecuteBulkUpgrade(ctx, BulkUpgradeRequest{
			Items: input.Items, Concurrency: input.Concurrency,
		})
		if result == nil {
			return nil, nil, mcpOperationError(ctx, err, !localFiles, options.Logger)
		}
		if !localFiles {
			for index := range result.Items {
				result.Items[index].Plan.Profile = ""
				result.Items[index].Error = serverSafeItemError(result.Items[index].Error)
			}
		}
		return partialToolResult(mcpOperationError(ctx, err, !localFiles, options.Logger)), *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_upload",
		Description: "Permanently delete one explicit marker-managed upload. " +
			"Payload objects are removed first and the ownership marker last. " +
			"Purge-protected uploads are refused unless force is true.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpDeleteInput,
	) (*mcp.CallToolResult, DeleteResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.DeleteUploadWithOptions(
			ctx, input.URLOrKey, DeleteOptions{Force: input.Force},
		)
		if err != nil {
			return nil, DeleteResult{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "protect_upload",
		Description: "Mark one marker-managed upload as purge-protected so " +
			"bulk purge skips it and delete_upload requires force.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpProtectInput,
	) (*mcp.CallToolResult, ProtectionResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.ProtectUpload(ctx, input.URLOrKey, input.Reason)
		if err != nil {
			return nil, ProtectionResult{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
		}
		return nil, *result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "unprotect_upload",
		Description: "Remove purge protection from one marker-managed " +
			"upload so purge and delete_upload can remove it again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpTargetInput,
	) (*mcp.CallToolResult, ProtectionResult, error) {
		ctx, cancel := mcpOperationContext(ctx, client)
		defer cancel()
		result, err := client.UnprotectUpload(ctx, input.URLOrKey)
		if err != nil {
			return nil, ProtectionResult{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
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
			return nil, SyncManifestResult{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
			for index := range result.Failures {
				result.Failures[index].Error = serverSafeItemError(
					result.Failures[index].Error,
				)
			}
		}
		return partialToolResult(
			mcpOperationError(ctx, err, !localFiles, options.Logger),
		), *result, nil
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
			return nil, PurgePlan{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
		}
		if !localFiles {
			result.Warnings = serverSafeWarnings(result.Warnings)
			for index := range result.Candidates {
				result.Candidates[index].Warnings = serverSafeWarnings(
					result.Candidates[index].Warnings,
				)
			}
			for index := range result.Protected {
				result.Protected[index].Warnings = serverSafeWarnings(
					result.Protected[index].Warnings,
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
			return nil, PurgeResult{}, mcpOperationError(
				ctx, err, !localFiles, options.Logger,
			)
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
		return partialToolResult(
			mcpOperationError(ctx, err, !localFiles, options.Logger),
		), *result, nil
	})

	return server
}

func serverSafeManifestRecords(records []ManifestRecord) []ManifestRecord {
	safe := make([]ManifestRecord, len(records))
	copy(safe, records)
	for index := range safe {
		safe[index].Profile = ""
	}
	return safe
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

func mcpLoggingMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(
			ctx context.Context, method string, request mcp.Request,
		) (mcp.Result, error) {
			requestID := serverlog.RequestID(ctx)
			logger.Log(ctx, serverlog.LevelTrace, "mcp method started",
				"method", safeMCPMethod(method),
				"request_id", requestID,
			)
			started := time.Now()
			result, err := next(ctx, method, request)
			duration := time.Since(started)
			outcome := "success"
			errorClass := "none"
			if err != nil {
				outcome = "error"
				errorClass = "protocol"
			} else if toolResult, ok := result.(*mcp.CallToolResult); ok &&
				toolResult.IsError {
				outcome = "error"
				errorClass = "tool"
			}
			logger.Log(ctx, serverlog.LevelTrace, "mcp method completed",
				"method", safeMCPMethod(method),
				"outcome", outcome,
				"duration", duration,
				"request_id", requestID,
			)
			if method == "tools/call" {
				logger.DebugContext(ctx, "mcp tool completed",
					"tool", safeMCPToolName(request),
					"outcome", outcome,
					"error_class", errorClass,
					"duration", duration,
					"request_id", requestID,
				)
			}
			return result, err
		}
	}
}

func safeMCPMethod(method string) string {
	switch method {
	case "initialize", "notifications/initialized", "ping", "tools/list",
		"tools/call":
		return method
	default:
		return "unknown"
	}
}

func safeMCPToolName(request mcp.Request) string {
	call, ok := request.(*mcp.CallToolRequest)
	if !ok || call.Params == nil {
		return "unknown"
	}
	switch call.Params.Name {
	case "upload_document", "upload_files", "list_uploads", "inspect_upload",
		"delete_upload", "protect_upload", "unprotect_upload",
		"sync_manifest", "preview_purge", "execute_purge",
		"upgrade_document", "upgrade_documents":
		return call.Params.Name
	default:
		return "unknown"
	}
}

func mcpErrorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrInputTooLarge):
		return "input_too_large"
	case errors.Is(err, errInvalidMCPListFilter):
		return "invalid_list_filter"
	case errors.Is(err, ErrBinaryInput), errors.Is(err, ErrInvalidUTF8),
		errors.Is(err, ErrEmptyInput):
		return "invalid_input"
	default:
		return "operation"
	}
}

type hostedMCPError struct {
	public string
	cause  error
}

func (e *hostedMCPError) Error() string { return e.public }
func (e *hostedMCPError) Unwrap() error { return e.cause }

func mcpOperationError(
	ctx context.Context, err error, hosted bool, logger *slog.Logger,
) error {
	if err == nil || !hosted {
		return err
	}
	if logger != nil {
		logger.DebugContext(ctx, "mcp operation failed",
			"error_class", mcpErrorClass(err),
			"request_id", serverlog.RequestID(ctx),
		)
	}
	var protectedErr *UploadProtectedError
	public := "airplan: the server could not complete the operation"
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		public = "airplan: the server operation timed out"
	case errors.Is(err, ErrInputTooLarge):
		public = "airplan: the upload exceeds the effective size limit"
	case errors.Is(err, errInvalidMCPListFilter):
		public = errInvalidMCPListFilter.Error()
	case errors.Is(err, ErrBinaryInput), errors.Is(err, ErrInvalidUTF8),
		errors.Is(err, ErrEmptyInput):
		public = "airplan: the request is not a valid document upload"
	case errors.As(err, &protectedErr):
		public = "airplan: the upload is purge-protected; " +
			"unprotect it or delete with force"
	}
	return &hostedMCPError{public: public, cause: err}
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
	return NewMCPHTTPHandlerWithOptions(client, version, MCPHTTPOptions{
		AllowedOrigins: allowedOrigins,
	})
}

// MCPHTTPOptions configures the hosted Streamable HTTP MCP transport.
type MCPHTTPOptions struct {
	AllowedOrigins []string
	Logger         *slog.Logger
}

// NewMCPHTTPHandlerWithOptions returns the current stateless Streamable HTTP
// MCP transport with optional serve-only logging.
func NewMCPHTTPHandlerWithOptions(
	client *Client, version string, options MCPHTTPOptions,
) (http.Handler, error) {
	allowed := make(map[string]struct{}, len(options.AllowedOrigins))
	for _, origin := range options.AllowedOrigins {
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
	server := NewMCPServerWithOptions(client, version, MCPServerOptions{
		Logger: options.Logger,
	})
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless: true,
			Logger:    serverlog.SafeMCPLogger(options.Logger),
		},
	)
	originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := r.Header.Values("Origin")
		if len(origins) > 1 {
			logMCPRejection(r, options.Logger, "origin_duplicate")
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if len(origins) == 1 {
			if _, ok := allowed[origins[0]]; !ok {
				logMCPRejection(r, options.Logger, "origin_not_allowed")
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
	limited := limitMCPRequestBodyWithLogger(
		originHandler, mcpHTTPMaxRequestBytes, options.Logger,
	)
	return serverlog.RequestIDMiddleware(limited), nil
}

const mcpHTTPMaxRequestBytes = 6*DefaultMaxInputSize + (1 << 20)

func limitMCPRequestBody(next http.Handler, limit int64) http.Handler {
	return limitMCPRequestBodyWithLogger(next, limit, nil)
}

func limitMCPRequestBodyWithLogger(
	next http.Handler, limit int64, logger *slog.Logger,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > limit {
			logMCPRejection(r, logger, "body_limit")
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		body := &mcpLimitedBody{
			body: r.Body, remaining: limit, limit: limit,
		}
		r.Body = body
		writer := &mcpLimitResponseWriter{
			ResponseWriter: w, body: body,
		}
		next.ServeHTTP(writer, r)
		if body.exceeded {
			logMCPRejection(r, logger, "body_limit")
		}
	})
}

func logMCPRejection(r *http.Request, logger *slog.Logger, reason string) {
	if logger == nil {
		return
	}
	logger.DebugContext(r.Context(), "request rejected",
		"transport", "mcp",
		"reason", reason,
		"request_id", serverlog.RequestID(r.Context()),
	)
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
