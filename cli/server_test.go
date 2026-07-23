package cli

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/jimeh/airplan/internal/httpapi"
	"github.com/jimeh/airplan/internal/serverlog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerAndMCPCommandsRegistered(t *testing.T) {
	root := newRootCmd()
	for _, name := range []string{"serve", "mcp"} {
		command, _, err := root.Find([]string{name})
		if err != nil || command == nil || command.Name() != name {
			t.Fatalf("find %s = %v, %v", name, command, err)
		}
		if command.Flag("manifest") == nil {
			t.Fatalf("%s does not inherit --manifest", name)
		}
	}
}

func TestLoadServerToken(t *testing.T) {
	const token = "01234567890123456789012345678901"
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadServerToken(path, "")
	if err != nil || got != token {
		t.Fatalf("token = %q, error = %v", got, err)
	}
	for _, test := range []struct {
		file, environment string
	}{
		{path, token},
		{"", "short"},
		{"", ""},
	} {
		if _, err := loadServerToken(test.file, test.environment); err == nil {
			t.Fatalf("loadServerToken(%q, %q) succeeded", test.file, test.environment)
		}
	}
}

func TestServeLogLevelPrecedence(t *testing.T) {
	t.Setenv("AIRPLAN_SERVER_LOG_LEVEL", "debug")
	command := newServeCmd()
	level, err := serveLogLevel(command, "")
	if err != nil || level != slog.LevelDebug {
		t.Fatalf("environment level = %v, %v", level, err)
	}
	if err := command.Flags().Set("log-level", "trace"); err != nil {
		t.Fatal(err)
	}
	level, err = serveLogLevel(command, "trace")
	if err != nil || level != serverlog.LevelTrace {
		t.Fatalf("flag level = %v, %v", level, err)
	}
}

func TestServeListenLineFollowsInfoThreshold(t *testing.T) {
	const want = "airplan: serving on http://127.0.0.1:8080\n"
	for _, test := range []struct {
		level slog.Level
		want  bool
	}{
		{level: slog.LevelError},
		{level: slog.LevelWarn},
		{level: slog.LevelInfo, want: true},
		{level: slog.LevelDebug, want: true},
		{level: serverlog.LevelTrace, want: true},
	} {
		var output bytes.Buffer
		logger := serverlog.New(&output, test.level)
		writeServeListen(&output, logger, "127.0.0.1:8080")
		if got := output.String(); (got == want) != test.want {
			t.Fatalf("level %v output = %q, want line = %t",
				test.level, got, test.want)
		}
	}
}

func TestLoopbackListenGuard(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:8080", "[::1]:8080", "localhost:8080",
	} {
		if !isLoopbackListen(address) {
			t.Errorf("%s was not accepted", address)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", ":8080", "bad"} {
		if isLoopbackListen(address) {
			t.Errorf("%s was accepted", address)
		}
	}
}

func TestMCPHelpDoesNotExposeServerCredentials(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"mcp", "--help"})
	var output strings.Builder
	root.SetOut(&output)
	root.SetErr(&output)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "token-file") {
		t.Fatalf("mcp help exposes server token flags: %s", output.String())
	}
}

func TestServeHandlerAuthenticatesRESTAndHostedMCP(t *testing.T) {
	const token = "01234567890123456789012345678901"
	client, err := airplan.New(context.Background(), &airplan.Config{
		Backend:      airplan.BackendS3,
		ManifestPath: filepath.Join(t.TempDir(), "manifest.jsonl"),
		Repository:   "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	operations := &airplan.HTTPOperations{
		Client: client, ServerVersion: "test",
	}
	rest, err := httpapi.NewHandler(operations, httpapi.Options{
		Token: token,
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := httpapi.NewBearerAuth(token)
	if err != nil {
		t.Fatal(err)
	}
	var logs serveLogBuffer
	logger := serverlog.New(&logs, serverlog.LevelTrace)
	mcpHandler, err := airplan.NewMCPHTTPHandlerWithOptions(
		client, "test", airplan.MCPHTTPOptions{Logger: logger},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newServeHandler(
		rest, mcpHandler, auth, logger,
	))
	t.Cleanup(server.Close)

	restClient, err := httpapi.NewClient(server.URL, token, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := restClient.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("authenticated REST request: %v", err)
	}
	if capabilities.APIVersion != "v1" {
		t.Fatalf("REST API version = %q, want v1", capabilities.APIVersion)
	}

	protocolClient := mcp.NewClient(&mcp.Implementation{
		Name: "airplan-test", Version: "test",
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := protocolClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp",
		HTTPClient: &http.Client{Transport: serveBearerRoundTripper{
			base: server.Client().Transport, token: token,
		}},
		MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("authenticated MCP connect: %v", err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_uploads",
		Arguments: map[string]any{
			"source": "manifest",
		},
	})
	if err != nil {
		t.Fatalf("authenticated MCP tool call: %v", err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("authenticated MCP result = %+v", result)
	}
	if !regexp.MustCompile(
		`mcp tool completed.*request_id=[A-Za-z0-9._-]+`,
	).MatchString(logs.String()) {
		t.Fatalf("MCP tool log has no request ID: %s", logs.String())
	}
}

func TestServeHostedMCPAuthRejectsBeforeDownstream(t *testing.T) {
	const (
		token          = "01234567890123456789012345678901"
		headerSentinel = "private-auth-header-sentinel"
		bodySentinel   = "private-mcp-body-sentinel"
		pathSentinel   = "private-mcp-path-sentinel"
	)
	tests := []struct {
		name   string
		values []string
		reason string
	}{
		{name: "missing", reason: "missing"},
		{
			name:   "incorrect",
			values: []string{"Bearer " + headerSentinel},
			reason: "token_mismatch",
		},
		{
			name:   "malformed",
			values: []string{"Bearer  " + headerSentinel},
			reason: "malformed",
		},
		{
			name: "duplicate",
			values: []string{
				"Bearer " + token,
				"Bearer " + headerSentinel,
			},
			reason: "duplicate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
			auth, err := httpapi.NewBearerAuth(token)
			if err != nil {
				t.Fatal(err)
			}
			downstreamCalls := 0
			mcpHandler := http.HandlerFunc(func(
				http.ResponseWriter, *http.Request,
			) {
				downstreamCalls++
			})
			handler := newServeHandler(
				http.NotFoundHandler(), mcpHandler, auth, logger,
			)
			body := &serveRecordingBody{
				Reader: strings.NewReader(bodySentinel),
			}
			request := httptest.NewRequest(
				http.MethodPost, "/mcp?target="+pathSentinel, body,
			)
			request.Body = body
			for _, value := range test.values {
				request.Header.Add("Authorization", value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
			if downstreamCalls != 0 || body.reads != 0 {
				t.Fatalf("downstream calls = %d, body reads = %d",
					downstreamCalls, body.reads)
			}
			if recorder.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing generic bearer challenge")
			}
			output := logs.String()
			if !strings.Contains(output, "reason="+test.reason) {
				t.Fatalf("logs do not contain reason %q: %s",
					test.reason, output)
			}
			for _, sentinel := range []string{
				token, headerSentinel, bodySentinel, pathSentinel,
				"Authorization",
			} {
				if strings.Contains(output, sentinel) {
					t.Fatalf("logs contain sensitive sentinel %q: %s",
						sentinel, output)
				}
			}
			if strings.Contains(recorder.Body.String(), headerSentinel) {
				t.Fatalf("response leaked credential: %s", recorder.Body)
			}
		})
	}
}

type serveRecordingBody struct {
	io.Reader
	reads int
}

func (r *serveRecordingBody) Read(buffer []byte) (int, error) {
	r.reads++
	return r.Reader.Read(buffer)
}

func (r *serveRecordingBody) Close() error { return nil }

type serveLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *serveLogBuffer) Write(body []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(body)
}

func (b *serveLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type serveBearerRoundTripper struct {
	base  http.RoundTripper
	token string
}

func (r serveBearerRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+r.token)
	return r.base.RoundTrip(clone)
}
