package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
