package api

import (
	"bytes"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEmbeddedOpenAPIMatchesSource(t *testing.T) {
	source, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(OpenAPI(), source) {
		t.Fatal("embedded OpenAPI does not match checked-in source")
	}
}

func TestOpenAPIHasExpectedVersionAndRoutes(t *testing.T) {
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(OpenAPI(), &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI != "3.0.3" {
		t.Fatalf("OpenAPI version = %q, want 3.0.3", document.OpenAPI)
	}
	want := []string{
		"/healthz",
		"/openapi.yaml",
		"/api/v1/capabilities",
		"/api/v1/uploads/documents",
		"/api/v1/uploads/collections",
		"/api/v1/uploads/inspect",
		"/api/v1/uploads/get",
		"/api/v1/uploads/delete",
		"/api/v1/uploads",
		"/api/v1/storage/uploads",
		"/api/v1/sync",
		"/api/v1/purge/preview",
		"/api/v1/purge",
	}
	for _, path := range want {
		if _, ok := document.Paths[path]; !ok {
			t.Errorf("OpenAPI is missing path %s", path)
		}
	}
}
