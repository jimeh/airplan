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

func TestResolveServeOptionsDefaults(t *testing.T) {
	unsetServerEnvironment(t)
	opts := &serveOptions{}
	command := newServeCmdWithOptions(opts)
	if err := resolveServeOptions(command, opts); err != nil {
		t.Fatal(err)
	}
	if opts.listen != "127.0.0.1:8080" {
		t.Fatalf("listen = %q", opts.listen)
	}
	if opts.allowNonLoopback {
		t.Fatal("non-loopback acknowledgement enabled by default")
	}
	if opts.tokenFile != "" || len(opts.allowedOrigins) != 0 ||
		opts.tempDir != "" {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
}

func TestResolveServeOptionsEnvironment(t *testing.T) {
	unsetServerEnvironment(t)
	t.Setenv("AIRPLAN_SERVER_HOST", "2001:db8::1")
	t.Setenv("AIRPLAN_SERVER_PORT", "9090")
	t.Setenv("AIRPLAN_SERVER_ALLOW_NON_LOOPBACK", "true")
	t.Setenv("AIRPLAN_SERVER_TOKEN_FILE", "/run/secrets/airplan-token")
	t.Setenv(
		"AIRPLAN_SERVER_ALLOWED_ORIGINS",
		"https://one.example, https://two.example",
	)
	t.Setenv("AIRPLAN_SERVER_TEMP_DIR", "/var/tmp/airplan")

	opts := &serveOptions{}
	command := newServeCmdWithOptions(opts)
	if err := resolveServeOptions(command, opts); err != nil {
		t.Fatal(err)
	}
	if opts.listen != "[2001:db8::1]:9090" {
		t.Fatalf("listen = %q", opts.listen)
	}
	if !opts.allowNonLoopback {
		t.Fatal("non-loopback acknowledgement was not enabled")
	}
	if opts.tokenFile != "/run/secrets/airplan-token" {
		t.Fatalf("token file = %q", opts.tokenFile)
	}
	wantOrigins := []string{
		"https://one.example", "https://two.example",
	}
	if strings.Join(opts.allowedOrigins, "\n") !=
		strings.Join(wantOrigins, "\n") {
		t.Fatalf("origins = %#v", opts.allowedOrigins)
	}
	if opts.tempDir != "/var/tmp/airplan" {
		t.Fatalf("temp directory = %q", opts.tempDir)
	}
}

func TestResolveServeOptionsFlagPrecedence(t *testing.T) {
	unsetServerEnvironment(t)
	t.Setenv("AIRPLAN_SERVER_HOST", "0.0.0.0")
	t.Setenv("AIRPLAN_SERVER_PORT", "9090")
	t.Setenv("AIRPLAN_SERVER_ALLOW_NON_LOOPBACK", "true")
	t.Setenv("AIRPLAN_SERVER_TOKEN_FILE", "/environment/token")
	t.Setenv(
		"AIRPLAN_SERVER_ALLOWED_ORIGINS",
		"https://environment.example",
	)
	t.Setenv("AIRPLAN_SERVER_TEMP_DIR", "/environment/tmp")

	opts := &serveOptions{}
	command := newServeCmdWithOptions(opts)
	for name, value := range map[string]string{
		"listen":             "localhost:7070",
		"allow-non-loopback": "false",
		"token-file":         "/flag/token",
		"allowed-origin":     "https://flag.example",
		"temp-dir":           "/flag/tmp",
	} {
		if err := command.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	if err := resolveServeOptions(command, opts); err != nil {
		t.Fatal(err)
	}
	if opts.listen != "localhost:7070" || opts.allowNonLoopback ||
		opts.tokenFile != "/flag/token" ||
		opts.tempDir != "/flag/tmp" {
		t.Fatalf("resolved options = %+v", opts)
	}
	if len(opts.allowedOrigins) != 1 ||
		opts.allowedOrigins[0] != "https://flag.example" {
		t.Fatalf("origins = %#v", opts.allowedOrigins)
	}
}

func TestResolveServeOptionsHostAssembly(t *testing.T) {
	for _, test := range []struct {
		host string
		want string
	}{
		{host: "127.0.0.1", want: "127.0.0.1:8080"},
		{host: "localhost", want: "localhost:8080"},
		{host: "::1", want: "[::1]:8080"},
	} {
		t.Run(test.host, func(t *testing.T) {
			unsetServerEnvironment(t)
			t.Setenv("AIRPLAN_SERVER_HOST", test.host)
			opts := &serveOptions{}
			command := newServeCmdWithOptions(opts)
			if err := resolveServeOptions(command, opts); err != nil {
				t.Fatal(err)
			}
			if opts.listen != test.want {
				t.Fatalf("listen = %q, want %q", opts.listen, test.want)
			}
		})
	}
}

func TestResolveServeOptionsRejectsInvalidEnvironment(t *testing.T) {
	tests := []struct {
		name, environment, value string
	}{
		{name: "empty host", environment: "AIRPLAN_SERVER_HOST"},
		{name: "empty port", environment: "AIRPLAN_SERVER_PORT"},
		{
			name: "negative port", environment: "AIRPLAN_SERVER_PORT",
			value: "-1",
		},
		{
			name: "above maximum port", environment: "AIRPLAN_SERVER_PORT",
			value: "65536",
		},
		{
			name: "non-numeric port", environment: "AIRPLAN_SERVER_PORT",
			value: "http",
		},
		{
			name: "whitespace port", environment: "AIRPLAN_SERVER_PORT",
			value: " 8080",
		},
		{
			name:        "invalid boolean",
			environment: "AIRPLAN_SERVER_ALLOW_NON_LOOPBACK",
			value:       "1",
		},
		{
			name:        "empty origins",
			environment: "AIRPLAN_SERVER_ALLOWED_ORIGINS",
		},
		{
			name:        "empty origin entry",
			environment: "AIRPLAN_SERVER_ALLOWED_ORIGINS",
			value:       "https://one.example,,https://two.example",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unsetServerEnvironment(t)
			t.Setenv(test.environment, test.value)
			opts := &serveOptions{}
			command := newServeCmdWithOptions(opts)
			if err := resolveServeOptions(command, opts); err == nil {
				t.Fatal("resolution succeeded")
			}
		})
	}

	unsetServerEnvironment(t)
	t.Setenv("AIRPLAN_SERVER_PORT", "0")
	opts := &serveOptions{}
	command := newServeCmdWithOptions(opts)
	if err := resolveServeOptions(command, opts); err != nil {
		t.Fatal(err)
	}
	if opts.listen != "127.0.0.1:0" {
		t.Fatalf("port-zero listen = %q", opts.listen)
	}
}

func TestResolveServeOptionsTokenSourceConflict(t *testing.T) {
	unsetServerEnvironment(t)
	const token = "01234567890123456789012345678901"
	t.Setenv("AIRPLAN_SERVER_TOKEN_FILE", "/run/secrets/token")
	t.Setenv("AIRPLAN_SERVER_TOKEN", token)
	opts := &serveOptions{}
	command := newServeCmdWithOptions(opts)
	if err := resolveServeOptions(command, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := loadServerToken(
		opts.tokenFile, os.Getenv("AIRPLAN_SERVER_TOKEN"),
	); err == nil {
		t.Fatal("token source conflict was accepted")
	}
}

func unsetServerEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AIRPLAN_SERVER_HOST",
		"AIRPLAN_SERVER_PORT",
		"AIRPLAN_SERVER_ALLOW_NON_LOOPBACK",
		"AIRPLAN_SERVER_TOKEN",
		"AIRPLAN_SERVER_TOKEN_FILE",
		"AIRPLAN_SERVER_ALLOWED_ORIGINS",
		"AIRPLAN_SERVER_TEMP_DIR",
	} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
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
