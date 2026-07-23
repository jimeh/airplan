package airplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, ManifestPath: t.TempDir() + "/manifest.jsonl",
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewMCPHTTPHandler(
		client, "test", []string{"https://agent.example"},
	)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
	))
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
