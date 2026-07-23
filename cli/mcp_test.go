package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStdioSubcommandWithOfficialClient(t *testing.T) {
	const token = "01234567890123456789012345678901"
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/uploads" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": []any{}, "warnings": []any{},
		})
	}))
	t.Cleanup(server.Close)

	for _, test := range []struct {
		name   string
		config string
	}{
		{name: "s3", config: "backend = \"s3\"\ntimeout = \"100ms\"\n"},
		{
			name: "airplan",
			config: fmt.Sprintf(
				"backend = \"airplan\"\napi_url = %q\napi_token = %q\n"+
					"timeout = \"100ms\"\n",
				server.URL, token,
			),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			configDir := t.TempDir()
			configPath := filepath.Join(configDir, "config.toml")
			if err := os.WriteFile(
				configPath, []byte(test.config), 0o600,
			); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(
				os.Args[0], "-test.run=TestMCPStdioHelperProcess",
			)
			cmd.Env = mcpHelperEnvironment(
				"AIRPLAN_MCP_HELPER=1",
				"AIRPLAN_CONFIG="+configPath,
				"AIRPLAN_PROFILE=",
				"AIRPLAN_MANIFEST="+filepath.Join(t.TempDir(), "manifest.jsonl"),
				"AWS_EC2_METADATA_DISABLED=true",
			)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			protocolClient := mcp.NewClient(&mcp.Implementation{
				Name: "airplan-test", Version: "test",
			}, nil)
			session, err := protocolClient.Connect(
				ctx, &mcp.CommandTransport{Command: cmd}, nil,
			)
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
		})
	}
}

func mcpHelperEnvironment(overrides ...string) []string {
	names := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		name, _, _ := strings.Cut(override, "=")
		names[name] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := names[name]; !replaced {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("AIRPLAN_MCP_HELPER") != "1" {
		return
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"mcp"})
	if err := cmd.Execute(); err != nil {
		message := err.Error()
		if !strings.HasPrefix(message, "airplan:") {
			message = "airplan: " + message
		}
		fmt.Fprintln(os.Stderr, message)
		os.Exit(1)
	}
	os.Exit(0)
}
