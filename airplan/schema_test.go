package airplan

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
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
		"api_token",
		"api_url",
		"backend",
		"bucket",
		"collection_template",
		"dark_theme",
		"default_profile",
		"endpoint",
		"indexable",
		"key_prefix",
		"light_theme",
		"mermaid_url",
		"no_external_assets",
		"no_source",
		"profiles",
		"public_base_url",
		"region",
		"repo",
		"secret_access_key",
		"template",
		"themes",
		"timeout",
	}
	if !slicesEqual(gotNames, wantNames) {
		t.Fatalf("root properties = %v, want %v", gotNames, wantNames)
	}
	backend := objectAt(t, props, "backend")
	if got := stringSliceAt(t, backend, "enum"); !slicesEqual(got, []string{"s3", "airplan"}) {
		t.Fatalf("backend enum = %v, want [s3 airplan]", got)
	}
	if got := backend["default"]; got != "s3" {
		t.Fatalf("backend default = %v, want s3", got)
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

	themes := objectAt(t, props, "themes")
	propertyNames := objectAt(t, themes, "propertyNames")
	if got := propertyNames["pattern"]; got != themeIDPattern.String() {
		t.Fatalf("themes propertyNames pattern = %v, want %s", got, themeIDPattern)
	}
	if got := propertyNames["maxLength"]; got != float64(maxThemeIDLength) {
		t.Fatalf("themes propertyNames maxLength = %v, want %d", got, maxThemeIDLength)
	}
	reserved := stringSliceAt(t, objectAt(t, propertyNames, "not"), "enum")
	wantReserved := make([]string, 0, len(builtinThemes()))
	for _, theme := range builtinThemes() {
		wantReserved = append(wantReserved, theme.ID)
	}
	if !slicesEqual(reserved, wantReserved) {
		t.Fatalf("themes reserved IDs = %v, want %v", reserved, wantReserved)
	}

	themeConfig := objectAt(t, defs, "ThemeConfig")
	syntax := objectAt(t, objectAt(t, themeConfig, "properties"), "syntax")
	gotSyntax := stringSliceAt(t, syntax, "enum")
	styleNames := append([]string(nil), styles.Names()...)
	sort.Strings(styleNames)
	wantSyntax := make([]string, 2, len(styleNames)+2)
	wantSyntax[0] = ""
	wantSyntax[1] = "derived"
	for _, name := range styleNames {
		wantSyntax = append(wantSyntax, "chroma:"+name)
	}
	if !slicesEqual(gotSyntax, wantSyntax) {
		t.Fatalf("theme syntax enum = %v, want %v", gotSyntax, wantSyntax)
	}
	for _, value := range gotSyntax[2:] {
		if !registeredChromaStyle(value[len("chroma:"):]) {
			t.Fatalf("schema syntax %q is not accepted at runtime", value)
		}
	}
}

func stringSliceAt(t *testing.T, obj map[string]any, key string) []string {
	t.Helper()

	values, ok := obj[key].([]any)
	if !ok {
		t.Fatalf("%s is %T, want array", key, obj[key])
	}
	out := make([]string, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s[%d] is %T, want string", key, i, value)
		}
		out[i] = text
	}
	return out
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
