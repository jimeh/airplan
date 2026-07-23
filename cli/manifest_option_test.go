package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGlobalManifestSelectsLocalHistory(t *testing.T) {
	isolateEnv(t)
	path := filepath.Join(t.TempDir(), "custom.jsonl")
	record := uploadRecord(
		deleteDirA, "custom-manifest", "", time.Now().UTC(),
	)
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := executeCommand(
		t, "", "", "--manifest", path, "list", "--json",
	)
	if err != nil || stderr != "" ||
		!strings.Contains(stdout, "custom-manifest") {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestGlobalManifestRejectsLocalOnlyCommand(t *testing.T) {
	isolateEnv(t)
	_, _, err := executeCommand(
		t, "", "", "--manifest", "custom.jsonl", "config", "profiles",
	)
	if err == nil || !strings.Contains(err.Error(), "does not apply") {
		t.Fatalf("error = %v", err)
	}
}

func TestGlobalManifestRejectedByAirplanBackend(t *testing.T) {
	isolateEnv(t)
	config := filepath.Join(t.TempDir(), "config.toml")
	data := "backend = \"airplan\"\n" +
		"api_url = \"https://airplan.example\"\n" +
		"api_token = \"01234567890123456789012345678901\"\n"
	if err := os.WriteFile(config, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := executeCommand(t, "", "",
		"--manifest", "custom.jsonl", "list", "--config", config)
	if err == nil || !strings.Contains(err.Error(),
		"cannot be used with the airplan backend") {
		t.Fatalf("error = %v", err)
	}
}
