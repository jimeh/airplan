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
		"{{.CSS}}", "{{.JS}}", "airplan:page-css", "airplan:page-js",
	} {
		if strings.Contains(dumped.String(), internal) {
			t.Fatalf("dumped template contains internal marker %q", internal)
		}
	}

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
