package cli

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jimeh/airplan/airplan"
)

func TestTemplateCommandPrintsBuiltinTemplate(t *testing.T) {
	want := airplan.BuiltinTemplate()
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}

	var out bytes.Buffer
	cmd := newTemplateCmd()
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != want {
		t.Errorf("output length = %d, want %d", len(got), len(want))
	}
}

func TestTemplateCommandPrintsCollectionTemplate(t *testing.T) {
	var out bytes.Buffer
	cmd := newTemplateCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"collection"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := airplan.BuiltinCollectionTemplate()
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}
	if out.String() != want {
		t.Fatalf("collection template length = %d, want %d", out.Len(), len(want))
	}
}

func TestTemplateCommandOutputCanBeUsedAsCustomTemplate(t *testing.T) {
	var dumped bytes.Buffer
	cmd := newTemplateCmd()
	cmd.SetOut(&dumped)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, internal := range []string{
		"{{.CSS}}", "{{.JS}}", "airplan:shared-css",
		"airplan:page-css", "airplan:theme-init-js",
		"airplan:theme-js", "airplan:page-js", "airplan:theme-toggle",
	} {
		if strings.Contains(dumped.String(), internal) {
			t.Fatalf("dumped template contains internal marker %q", internal)
		}
	}
	assertDumpedSharedThemeAssets(t, dumped.String())

	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, dumped.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	tmpl, err := airplan.LoadTemplate(path)
	if err != nil {
		t.Fatal(err)
	}

	var rendered bytes.Buffer
	err = tmpl.Execute(&rendered, airplan.TemplateData{
		Title:        "Round trip",
		RenderedHTML: template.HTML("<h1>Round trip</h1>"),
		Format:       "md",
	})
	if err != nil {
		t.Fatalf("execute dumped template: %v", err)
	}
	if !strings.Contains(rendered.String(), "<h1>Round trip</h1>") ||
		!strings.Contains(rendered.String(), "--page-width: 54rem") {
		t.Fatal("dumped template did not render the built-in page")
	}
}

func TestCollectionTemplateCommandOutputCanBeUsedAsCustomTemplate(t *testing.T) {
	var dumped bytes.Buffer
	cmd := newTemplateCmd()
	cmd.SetOut(&dumped)
	cmd.SetArgs([]string{"collection"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, internal := range []string{
		"airplan:shared-css", "airplan:collection-css",
		"airplan:theme-init-js", "airplan:theme-js",
		"airplan:collection-js", "airplan:theme-toggle",
	} {
		if strings.Contains(dumped.String(), internal) {
			t.Fatalf("dumped collection template contains internal marker %q", internal)
		}
	}
	assertDumpedSharedThemeAssets(t, dumped.String())

	path := filepath.Join(t.TempDir(), "collection.html")
	if err := os.WriteFile(path, dumped.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	tmpl, err := airplan.LoadCollectionTemplate(path)
	if err != nil {
		t.Fatal(err)
	}

	var rendered bytes.Buffer
	err = tmpl.Execute(&rendered, airplan.CollectionTemplateData{
		Title: "Round trip",
		Files: []airplan.CollectionTemplateFile{{
			Name: "example.txt", Path: "./example.txt",
			ContentType: "text/plain", Bytes: 12, MediaKind: "file",
		}},
		TotalBytes: 12,
	})
	if err != nil {
		t.Fatalf("execute dumped collection template: %v", err)
	}
	if !strings.Contains(rendered.String(), "example.txt") ||
		!strings.Contains(rendered.String(), "--page-width: 54rem") {
		t.Fatal("dumped template did not render the built-in collection page")
	}
}

func assertDumpedSharedThemeAssets(t *testing.T, dumped string) {
	t.Helper()
	for _, sentinel := range []struct {
		name, value string
	}{
		{"theme-toggle markup", `role="group" aria-label="Color theme"`},
		{
			"early persisted-theme initialization",
			`const theme = localStorage.getItem('airplan-theme');`,
		},
		{
			"runtime theme behavior",
			`window.dispatchEvent(new CustomEvent('airplan:themechange'`,
		},
	} {
		if !strings.Contains(dumped, sentinel.value) {
			t.Errorf("dumped template missing %s sentinel %q",
				sentinel.name, sentinel.value)
		}
	}
}
