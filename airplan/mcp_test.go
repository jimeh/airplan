package airplan

import (
	"bytes"
	"context"
	"encoding/json"
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

func mcpStringPointer(value string) *string { return &value }

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
			uploadDocumentDescription := ""
			listSchema := ""
			for _, tool := range tools.Tools {
				hasUploadFiles = hasUploadFiles || tool.Name == "upload_files"
				if tool.Name == "upload_document" {
					uploadDocumentDescription = tool.Description
				}
				if tool.Name == "list_uploads" {
					encoded, marshalErr := json.Marshal(tool.InputSchema)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					listSchema = string(encoded)
				}
			}
			for _, field := range []string{
				"newer_than", "older_than", "limit", "kind", "slug",
			} {
				if !strings.Contains(listSchema, `"`+field+`"`) {
					t.Errorf("list_uploads schema missing %q: %s", field, listSchema)
				}
			}
			if hasUploadFiles != test.localFiles {
				t.Fatalf("upload_files present = %t", hasUploadFiles)
			}
			for _, feature := range []string{
				"Mermaid fences", "GitHub alerts", "responsive columns",
			} {
				if !strings.Contains(uploadDocumentDescription, feature) {
					t.Errorf(
						"upload_document description %q does not mention %q",
						uploadDocumentDescription, feature,
					)
				}
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

func TestMCPListUploadsFiltersManifestAndStorage(t *testing.T) {
	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	record := func(index int, kind UploadKind, slug string) ManifestRecord {
		dir := strings.Repeat(string(rune('a'+index)), 26)
		return ManifestRecord{
			Type: "upload", Time: when.Add(time.Duration(index) * time.Hour),
			Key: dir + "/plan.html", MarkerKey: dir + "/" + MarkerFilename,
			URL:    "https://plans.example.com/" + dir + "/plan.html",
			Bucket: "plans", Kind: string(kind), Slug: slug, Bytes: 4,
			MarkerVersion: MarkerVersion,
		}
	}
	records := []ManifestRecord{
		record(0, UploadKindDocument, "doc-old"),
		record(1, UploadKindCollection, ""),
		record(2, UploadKindDocument, "doc-new"),
	}
	storage := []RemoteUpload{
		{Dir: "old", Kind: UploadKindDocument, Slug: "doc-old", LastModified: when},
		{Dir: "collection", Kind: UploadKindCollection, LastModified: when.Add(time.Hour)},
		{Dir: "new", Kind: UploadKindDocument, Slug: "doc-new", LastModified: when.Add(2 * time.Hour)},
		{Dir: "conflict", Conflict: true, Slug: "doc-conflict", LastModified: when.Add(3 * time.Hour)},
	}
	transport := &mcpTestTransport{
		manifestResult: &ManifestList{Records: records}, remoteResult: storage,
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
	protocolClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := protocolClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	for _, source := range []string{"manifest", "storage"} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "list_uploads", Arguments: map[string]any{
				"source": source, "newer_than": when.Format(time.RFC3339),
				"older_than": when.Add(3 * time.Hour).Format(time.RFC3339),
				"kind":       "document", "slug": "doc-*", "limit": 1,
			},
		})
		if callErr != nil || result.IsError {
			t.Fatalf("%s result = %+v, error = %v", source, result, callErr)
		}
		encoded, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var output mcpListOutput
		if err := json.Unmarshal(encoded, &output); err != nil {
			t.Fatal(err)
		}
		if source == "manifest" {
			if output.Manifest == nil || len(output.Manifest.Records) != 1 ||
				output.Manifest.Records[0].Slug != "doc-new" {
				t.Fatalf("manifest output = %+v", output)
			}
		} else if len(output.Storage) != 1 || output.Storage[0].Dir != "new" {
			t.Fatalf("storage output = %+v", output)
		}
	}

	for _, source := range []string{"manifest", "storage"} {
		zero, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "list_uploads", Arguments: map[string]any{
				"source": source, "limit": 0,
			},
		})
		if err != nil || zero.IsError {
			t.Fatalf("%s zero limit result = %+v, error = %v", source, zero, err)
		}
		encoded, _ := json.Marshal(zero.StructuredContent)
		var zeroOutput mcpListOutput
		if err := json.Unmarshal(encoded, &zeroOutput); err != nil ||
			(zeroOutput.Manifest != nil && len(zeroOutput.Manifest.Records) != 0) ||
			len(zeroOutput.Storage) != 0 {
			t.Fatalf("%s zero limit output = %+v, error = %v", source, zeroOutput, err)
		}
	}

	beforeCalls := transport.listCalls
	for _, arguments := range []map[string]any{
		{"source": "storage", "limit": -1},
		{"source": "storage", "kind": "conflict"},
		{"source": "storage", "slug": "["},
		{"source": "storage", "older_than": "tomorrow"},
		{"source": "storage", "newer_than": ""},
		{"source": "storage", "older_than": ""},
		{"source": "storage", "kind": ""},
	} {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "list_uploads", Arguments: arguments,
		})
		if callErr != nil || !result.IsError {
			t.Fatalf("invalid arguments result = %+v, error = %v", result, callErr)
		}
	}
	if transport.listCalls != beforeCalls {
		t.Fatalf("invalid filters performed %d list calls", transport.listCalls-beforeCalls)
	}
}

func TestMCPListFilterSelectionBoundaries(t *testing.T) {
	threshold := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	records := []ManifestRecord{
		{Time: threshold.Add(-time.Second), Kind: "document", Slug: "before"},
		{Time: threshold, Kind: "document", Slug: "exact"},
		{Time: threshold.Add(time.Second), Kind: "collection", Slug: ""},
	}
	remote := []RemoteUpload{
		{Dir: "before", LastModified: threshold.Add(-time.Second), Kind: UploadKindDocument, Slug: "before"},
		{Dir: "exact", LastModified: threshold, Kind: UploadKindDocument, Slug: "exact"},
		{Dir: "after", LastModified: threshold.Add(time.Second), Kind: UploadKindCollection},
		{Dir: "conflict", LastModified: threshold.Add(2 * time.Second), Conflict: true, Slug: "exact"},
	}
	newer, err := parseMCPListFilters(mcpListInput{
		NewerThan: mcpStringPointer(threshold.Format(time.RFC3339)),
	}, threshold)
	if err != nil {
		t.Fatal(err)
	}
	if got := selectMCPManifestRecords(records, newer); len(got) != 2 ||
		got[0].Slug != "exact" {
		t.Fatalf("newer manifest = %+v", got)
	}
	if got := selectMCPRemoteUploads(remote, newer); len(got) != 3 ||
		got[0].Dir != "exact" {
		t.Fatalf("newer storage = %+v", got)
	}
	older, err := parseMCPListFilters(mcpListInput{
		OlderThan: mcpStringPointer(threshold.Format(time.RFC3339)),
	}, threshold)
	if err != nil {
		t.Fatal(err)
	}
	if got := selectMCPManifestRecords(records, older); len(got) != 1 ||
		got[0].Slug != "before" {
		t.Fatalf("older manifest = %+v", got)
	}
	if got := selectMCPRemoteUploads(remote, older); len(got) != 1 ||
		got[0].Dir != "before" {
		t.Fatalf("older storage = %+v", got)
	}
	slug, err := parseMCPListFilters(mcpListInput{
		Slug: mcpStringPointer("*"),
	}, threshold)
	if err != nil {
		t.Fatal(err)
	}
	if got := selectMCPManifestRecords(records, slug); len(got) != 2 {
		t.Fatalf("slug manifest = %+v", got)
	}
	if got := selectMCPRemoteUploads(remote, slug); len(got) != 2 {
		t.Fatalf("slug storage = %+v", got)
	}
}

func TestMCPListFilterExplicitEmptyValues(t *testing.T) {
	for _, field := range []string{"newer_than", "older_than", "kind"} {
		t.Run(field, func(t *testing.T) {
			var input mcpListInput
			if err := json.Unmarshal([]byte(`{"`+field+`":""}`), &input); err != nil {
				t.Fatal(err)
			}
			if _, err := parseMCPListFilters(input, time.Now()); err == nil {
				t.Fatalf("explicit empty %s was treated as omitted", field)
			}
		})
	}

	var explicit mcpListInput
	if err := json.Unmarshal([]byte(`{"slug":""}`), &explicit); err != nil {
		t.Fatal(err)
	}
	filters, err := parseMCPListFilters(explicit, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	records := []ManifestRecord{
		{Kind: "document", Slug: ""},
		{Kind: "document", Slug: "plan"},
		{Kind: "collection", Slug: ""},
	}
	selected := selectMCPManifestRecords(records, filters)
	if len(selected) != 1 || selected[0].Kind != "document" ||
		selected[0].Slug != "" {
		t.Fatalf("explicit empty slug selected %+v", selected)
	}
	remote := []RemoteUpload{
		{Kind: UploadKindDocument, Slug: ""},
		{Kind: UploadKindDocument, Slug: "plan"},
		{Kind: UploadKindCollection},
	}
	if selectedRemote := selectMCPRemoteUploads(remote, filters); len(selectedRemote) != 1 || selectedRemote[0].Kind != UploadKindDocument ||
		selectedRemote[0].Slug != "" {
		t.Fatalf("explicit empty slug selected storage %+v", selectedRemote)
	}

	omitted, err := parseMCPListFilters(mcpListInput{}, time.Now())
	if err != nil || len(selectMCPManifestRecords(records, omitted)) != len(records) ||
		len(selectMCPRemoteUploads(remote, omitted)) != len(remote) {
		t.Fatalf("omitted filters changed selection: %+v, %v", omitted, err)
	}
}

func TestHostedMCPListFilterErrorsAreSanitizedBeforeListing(t *testing.T) {
	const sentinel = "private-filter-value-sentinel"
	transport := &mcpTestTransport{}
	client := &Client{cfg: &Config{}, remote: transport}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := NewMCPServerWithOptions(client, "test", MCPServerOptions{
		LocalFiles: false,
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
		Name: "list_uploads", Arguments: map[string]any{
			"source": "storage", "newer_than": sentinel,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result = %+v, want tool error", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	visible := string(encoded)
	if strings.Contains(visible, sentinel) {
		t.Fatalf("hosted result exposed request value: %s", visible)
	}
	if !strings.Contains(visible, "server could not complete the operation") {
		t.Fatalf("hosted result = %s, want sanitized operation error", visible)
	}
	if transport.listCalls != 0 {
		t.Fatalf("invalid filter performed %d list calls", transport.listCalls)
	}

	localInput := mcpListInput{NewerThan: mcpStringPointer(sentinel)}
	if _, err := parseMCPListFilters(localInput, time.Now()); err == nil ||
		!strings.Contains(err.Error(), sentinel) {
		t.Fatalf("local filter error = %v, want detailed request value", err)
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
		"error_class=tool",
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

func TestMCPLogsSDKLevelFailuresWithSafeClasses(t *testing.T) {
	const unknownToolSentinel = "private-unknown-tool-sentinel"
	client := &Client{cfg: &Config{}, remote: &mcpTestTransport{}}
	var logs bytes.Buffer
	logger := serverlog.New(&logs, slog.LevelDebug)
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

	if _, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: unknownToolSentinel,
	}); err == nil {
		t.Fatal("unknown tool call succeeded")
	}
	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "upload_document",
		Arguments: map[string]any{
			"content": 123,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatalf("invalid argument result = %+v", invalid)
	}

	output := logs.String()
	for _, want := range []string{
		"tool=unknown outcome=error error_class=protocol",
		"tool=upload_document outcome=error error_class=tool",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs do not contain %q: %s", want, output)
		}
	}
	if strings.Contains(output, unknownToolSentinel) {
		t.Fatalf("unknown tool name leaked: %s", output)
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
	uploadResult   *Result
	uploadErr      error
	syncResult     *SyncManifestResult
	syncErr        error
	purgeResult    *PurgeResult
	purgeErr       error
	manifestResult *ManifestList
	remoteResult   []RemoteUpload
	listCalls      int
}

func (t *mcpTestTransport) ListManifest(
	context.Context, ListManifestOptions,
) (*ManifestList, error) {
	t.listCalls++
	if t.manifestResult == nil {
		return &ManifestList{}, nil
	}
	return t.manifestResult, nil
}

func (t *mcpTestTransport) ListRemote(context.Context) ([]RemoteUpload, error) {
	t.listCalls++
	return t.remoteResult, nil
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
