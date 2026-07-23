package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
	"github.com/jimeh/airplan/internal/serverlog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolSurfaceAndManifestList(t *testing.T) {
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, ManifestPath: t.TempDir() + "/manifest.jsonl",
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		localFiles bool
		wantTools  int
	}{
		{name: "stdio", localFiles: true, wantTools: 8},
		{name: "hosted HTTP", localFiles: false, wantTools: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewMCPServer(client, "test", test.localFiles)
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = serverSession.Close() }()
			protocolClient := mcp.NewClient(&mcp.Implementation{
				Name: "test", Version: "test",
			}, nil)
			session, err := protocolClient.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()
			tools, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.Tools) != test.wantTools {
				t.Fatalf("tools = %d, want %d", len(tools.Tools), test.wantTools)
			}
			hasUploadFiles := false
			for _, tool := range tools.Tools {
				hasUploadFiles = hasUploadFiles || tool.Name == "upload_files"
			}
			if hasUploadFiles != test.localFiles {
				t.Fatalf("upload_files present = %t", hasUploadFiles)
			}
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "list_uploads",
				Arguments: map[string]any{"source": "manifest"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || len(result.Content) == 0 {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestMCPHTTPOriginGuard(t *testing.T) {
	const originSentinel = "https://private-origin-sentinel.example"
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, ManifestPath: t.TempDir() + "/manifest.jsonl",
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	handler, err := NewMCPHTTPHandlerWithOptions(
		client, "test", MCPHTTPOptions{
			AllowedOrigins: []string{"https://agent.example"},
			Logger:         serverlog.New(&logs, slog.LevelDebug),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
	))
	request.Header.Set("Origin", originSentinel)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if !strings.Contains(logs.String(), "reason=origin_not_allowed") {
		t.Fatalf("origin rejection was not logged: %s", logs.String())
	}
	if strings.Contains(logs.String(), originSentinel) {
		t.Fatalf("origin value leaked: %s", logs.String())
	}
}

func TestMCPStreamableHTTPWithOfficialClient(t *testing.T) {
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, ManifestPath: t.TempDir() + "/manifest.jsonl",
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	mcpHandler, err := NewMCPHTTPHandler(client, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	auth, err := httpapi.NewBearerAuth(token)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(auth.Wrap(mcpHandler))
	t.Cleanup(server.Close)
	protocolClient := mcp.NewClient(&mcp.Implementation{
		Name: "airplan-test", Version: "test",
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := protocolClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL,
		HTTPClient: &http.Client{Transport: bearerRoundTripper{
			base: http.DefaultTransport, token: token,
		}},
		MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_uploads",
		Arguments: map[string]any{
			"source": "manifest",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCPPartialResultsRetainStructuredContent(t *testing.T) {
	transport := &mcpTestTransport{
		syncResult: &SyncManifestResult{Unchanged: 2},
		syncErr:    errors.New("one marker failed"),
		purgeResult: &PurgeResult{Items: []PurgeItemResult{
			{UploadID: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Error: "delete failed"},
		}},
		purgeErr: errors.New("one purge failed"),
	}
	client := &Client{cfg: &Config{}, remote: transport}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := NewMCPServer(client, "test", true)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	protocolClient := mcp.NewClient(&mcp.Implementation{
		Name: "test", Version: "test",
	}, nil)
	session, err := protocolClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for _, call := range []*mcp.CallToolParams{
		{Name: "sync_manifest", Arguments: map[string]any{}},
		{Name: "execute_purge", Arguments: map[string]any{
			"upload_ids": []string{"aaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}},
	} {
		result, err := session.CallTool(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError || result.StructuredContent == nil {
			t.Fatalf("%s result = %+v", call.Name, result)
		}
	}
}

func TestHostedMCPHidesServerPaths(t *testing.T) {
	const privatePath = "/private/airplan/template.html"
	for _, test := range []struct {
		name      string
		result    *Result
		uploadErr error
	}{
		{
			name:      "error",
			uploadErr: errors.New("read template " + privatePath + ": denied"),
		},
		{
			name: "warning",
			result: &Result{
				URL: "https://plans.example/upload", Title: "upload",
				Warnings: []string{
					"custom template ignored for HTML input — HTML input is used as-is " +
						"(note: the template also failed to load: read " + privatePath + ")",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{cfg: &Config{}, remote: &mcpTestTransport{
				uploadResult: test.result, uploadErr: test.uploadErr,
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server := NewMCPServer(client, "test", false)
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = serverSession.Close() }()
			protocolClient := mcp.NewClient(&mcp.Implementation{
				Name: "test", Version: "test",
			}, nil)
			session, err := protocolClient.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "upload_document", Arguments: map[string]any{
					"content": "<html></html>", "format": "html",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(fmt.Sprint(result), privatePath) {
				t.Fatalf("server path leaked: %+v", result)
			}
		})
	}
}

func TestHostedMCPLogsSafeToolOutcome(t *testing.T) {
	const (
		contentSentinel  = "private-upload-content-sentinel"
		urlSentinel      = "private-url-sentinel.example"
		pathSentinel     = "private/path/sentinel.md"
		errorSentinel    = "private-s3-response-sentinel"
		endpointSentinel = "https://private-s3-endpoint.example"
		bucketSentinel   = "private-bucket-sentinel"
		fsSentinel       = "/private/server/path/sentinel"
	)
	client := &Client{cfg: &Config{}, remote: &mcpTestTransport{
		uploadErr: fmt.Errorf(
			"%s %s %s %s: %w",
			errorSentinel,
			endpointSentinel,
			bucketSentinel,
			fsSentinel,
			ErrInputTooLarge,
		),
	}}
	var logs bytes.Buffer
	logger := serverlog.New(&logs, serverlog.LevelTrace)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := NewMCPServerWithOptions(client, "test", MCPServerOptions{
		Logger: logger,
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	protocolClient := mcp.NewClient(&mcp.Implementation{
		Name: "test", Version: "test",
	}, nil)
	session, err := protocolClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "upload_document",
		Arguments: map[string]any{
			"content":        contentSentinel,
			"name":           pathSentinel,
			"repository_url": "https://" + urlSentinel + "/owner/repo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result = %+v, want tool error", result)
	}
	output := logs.String()
	for _, want := range []string{
		"mcp tool completed",
		"tool=upload_document",
		"outcome=error",
		"error_class=input_too_large",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs do not contain %q: %s", want, output)
		}
	}
	for _, sentinel := range []string{
		contentSentinel, urlSentinel, pathSentinel, errorSentinel,
		endpointSentinel, bucketSentinel, fsSentinel,
	} {
		if strings.Contains(output, sentinel) {
			t.Fatalf("logs contain sensitive sentinel %q: %s",
				sentinel, output)
		}
	}
}

func TestMCPRequestBodyLimit(t *testing.T) {
	reader := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := limitMCPRequestBody(reader, 8)
	for _, test := range []struct {
		name    string
		chunked bool
	}{
		{name: "fixed length"},
		{name: "chunked", chunked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost, "/mcp", strings.NewReader("123456789"),
			)
			if test.chunked {
				request.ContentLength = -1
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413", recorder.Code)
			}
		})
	}
}

func TestMCPRequestBodyLimitLogsWithoutBody(t *testing.T) {
	const bodySentinel = "private-body-sentinel"
	var logs bytes.Buffer
	logger := serverlog.New(&logs, slog.LevelDebug)
	handler := serverlog.RequestIDMiddleware(limitMCPRequestBodyWithLogger(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("oversized request reached downstream handler")
		}),
		8,
		logger,
	))
	request := httptest.NewRequest(
		http.MethodPost, "/mcp", strings.NewReader(bodySentinel),
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if !strings.Contains(logs.String(), "reason=body_limit") {
		t.Fatalf("body limit rejection was not logged: %s", logs.String())
	}
	if strings.Contains(logs.String(), bodySentinel) {
		t.Fatalf("request body leaked: %s", logs.String())
	}
}

func TestMCPHostedDocumentLimitAndPerCallTimeout(t *testing.T) {
	for _, test := range []struct {
		requested int64
		hosted    bool
		want      int64
	}{
		{requested: -1, hosted: true, want: DefaultMaxInputSize},
		{requested: DefaultMaxInputSize + 1, hosted: true, want: DefaultMaxInputSize},
		{requested: 1024, hosted: true, want: 1024},
		{requested: -1, hosted: false, want: -1},
	} {
		if got := mcpDocumentLimit(test.requested, test.hosted); got != test.want {
			t.Errorf("mcpDocumentLimit(%d, %t) = %d, want %d",
				test.requested, test.hosted, got, test.want)
		}
	}

	base := context.Background()
	client := &Client{cfg: &Config{Timeout: time.Second}}
	ctx, cancel := mcpOperationContext(base, client)
	defer cancel()
	if _, ok := base.Deadline(); ok {
		t.Fatal("base session context unexpectedly has a deadline")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("tool operation context has no configured deadline")
	}
}

type mcpTestTransport struct {
	operationTransport
	uploadResult *Result
	uploadErr    error
	syncResult   *SyncManifestResult
	syncErr      error
	purgeResult  *PurgeResult
	purgeErr     error
}

func (t *mcpTestTransport) Upload(context.Context, Input) (*Result, error) {
	return t.uploadResult, t.uploadErr
}

func (t *mcpTestTransport) SyncManifest(
	context.Context, SyncManifestOptions,
) (*SyncManifestResult, error) {
	return t.syncResult, t.syncErr
}

func (t *mcpTestTransport) Purge(
	context.Context, PurgeRequest,
) (*PurgeResult, error) {
	return t.purgeResult, t.purgeErr
}

type bearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (r bearerRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+r.token)
	return r.base.RoundTrip(clone)
}
