package airplan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jimeh/go-golden"
)

func TestConfigSchemaCommitted(t *testing.T) {
	got, err := ConfigSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(got) {
		t.Fatal("ConfigSchema returned invalid JSON")
	}

	goldenPath := filepath.Join("..", "schema", "airplan.schema.json")
	if golden.Update() {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("schema differs from %s (set GOLDEN_UPDATE=1 to refresh)",
			goldenPath)
	}
}

func TestConfigSchemaShape(t *testing.T) {
	doc := configSchemaDoc(t)

	if got := doc["$id"]; got != configSchemaID {
		t.Fatalf("$id = %v, want %s", got, configSchemaID)
	}
	if got := doc["additionalProperties"]; got != false {
		t.Fatalf("root additionalProperties = %v, want false", got)
	}

	props := objectAt(t, doc, "properties")
	gotNames := keys(props)
	wantNames := []string{
		"access_key_id",
		"bucket",
		"collection_template",
		"default_profile",
		"endpoint",
		"indexable",
		"key_prefix",
		"mermaid_url",
		"no_external_assets",
		"no_source",
		"profiles",
		"public_base_url",
		"region",
		"repo",
		"secret_access_key",
		"template",
		"timeout",
	}
	if !slicesEqual(gotNames, wantNames) {
		t.Fatalf("root properties = %v, want %v", gotNames, wantNames)
	}

	profiles := objectAt(t, props, "profiles")
	additional := objectAt(t, profiles, "additionalProperties")
	if got := additional["$ref"]; got != "#/$defs/Settings" {
		t.Fatalf("profiles values $ref = %v, want #/$defs/Settings", got)
	}

	defs := objectAt(t, doc, "$defs")
	settings := objectAt(t, defs, "Settings")
	if got := settings["additionalProperties"]; got != false {
		t.Fatalf("Settings additionalProperties = %v, want false", got)
	}
}

func configSchemaDoc(t *testing.T) map[string]any {
	t.Helper()

	data, err := ConfigSchema()
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func objectAt(t *testing.T, obj map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := obj[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object", key, obj[key])
	}
	return value
}

func keys(obj map[string]any) []string {
	out := make([]string, 0, len(obj))
	for key := range obj {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
