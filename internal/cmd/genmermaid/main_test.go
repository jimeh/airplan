package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesAndChecksPin(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "mermaid.json")
	outputPath := filepath.Join(dir, "mermaid_generated.go")
	manifest := `{"package":"mermaid","version":"11.16.0",` +
		`"released_at":"2026-06-25T11:30:40.280Z"}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(manifestPath, outputPath, false); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), `MermaidVersion = "11.16.0"`) {
		t.Fatalf("generated pin missing: %s", generated)
	}
	if err := run(manifestPath, outputPath, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(manifestPath, outputPath, true); err == nil {
		t.Fatal("check accepted stale output")
	}
}

func TestRunRejectsNonStableVersion(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "mermaid.json")
	manifest := `{"package":"mermaid","version":"11.17.0-rc.1",` +
		`"released_at":"2026-07-01T00:00:00Z"}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(manifestPath, filepath.Join(dir, "out.go"), false); err == nil {
		t.Fatal("prerelease version accepted")
	}
}
