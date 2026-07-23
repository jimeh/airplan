package airplan

import (
	"context"
	"fmt"
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
		repository := input.RepositoryURL
		if !localFiles && repository == "" {
			repository = "none"
		}
		result, err := client.Upload(ctx, Input{
			Reader: strings.NewReader(input.Content), Name: input.Name,
			Format: input.Format, Title: input.Title, Slug: input.Slug,
			Lang: input.Lang, RepositoryURL: repository,
			MaxSize: input.MaxSize,
		})
		if err != nil {
			return nil, Result{}, err
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
				return nil, FilesResult{}, err
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
				return nil, output, err
			}
			output.Manifest = listed
		case UploadSourceStorage:
			uploads, err := client.ListRemote(ctx)
			if err != nil {
				return nil, output, err
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
		result, err := client.InspectUpload(ctx, input.URLOrKey)
		if err != nil {
			return nil, UploadInspection{}, err
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
		result, err := client.DeleteUpload(ctx, input.URLOrKey)
		if err != nil {
			return nil, DeleteResult{}, err
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
		result, err := client.SyncManifest(ctx, SyncManifestOptions{
			DryRun: !input.Apply, Prune: input.Prune,
			Concurrency: input.Concurrency,
		})
		if result == nil {
			return nil, SyncManifestResult{}, err
		}
		return nil, *result, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "preview_purge",
		Description: "Return explicit purge candidates without deleting " +
			"anything. Review upload_id values before execution.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest,
		input mcpPurgePreviewInput,
	) (*mcp.CallToolResult, PurgePlan, error) {
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
			return nil, PurgePlan{}, err
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
		result, err := client.Purge(ctx, PurgeRequest(input))
		if result == nil {
			return nil, PurgeResult{}, err
		}
		return nil, *result, err
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	}), nil
}
