// Command checkopenapi verifies that the checked-in OpenAPI-generated Go
// client/server/models exactly match api/openapi.yaml.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	temp, err := os.CreateTemp("", "airplan-api-*.go")
	if err != nil {
		fatal(err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		fatal(err)
	}
	defer func() { _ = os.Remove(tempPath) }()
	configPath := filepath.Join(root, "api", "oapi-codegen.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		fatal(err)
	}
	config = bytes.Replace(
		config,
		[]byte("output: ../internal/httpapi/generated/api.gen.go"),
		[]byte("output: "+tempPath),
		1,
	)
	tempConfig, err := os.CreateTemp("", "airplan-oapi-*.yaml")
	if err != nil {
		fatal(err)
	}
	tempConfigPath := tempConfig.Name()
	defer func() { _ = os.Remove(tempConfigPath) }()
	if _, err := tempConfig.Write(config); err != nil {
		fatal(err)
	}
	if err := tempConfig.Close(); err != nil {
		fatal(err)
	}

	cmd := exec.Command("go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0",
		"--config", tempConfigPath,
		filepath.Join(root, "api", "openapi.yaml"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal(fmt.Errorf("generate OpenAPI code: %w", err))
	}
	want, err := os.ReadFile(tempPath)
	if err != nil {
		fatal(err)
	}
	generatedPath := filepath.Join(
		root, "internal", "httpapi", "generated", "api.gen.go",
	)
	got, err := os.ReadFile(generatedPath)
	if err != nil {
		fatal(err)
	}
	if !bytes.Equal(got, want) {
		fatal(fmt.Errorf(
			"%s is stale; run mise run generate", generatedPath,
		))
	}
}

func repositoryRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	return string(bytes.TrimSpace(out)), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "checkopenapi:", err)
	os.Exit(1)
}
