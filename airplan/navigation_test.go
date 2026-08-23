package airplan

import (
	"html/template"
	"reflect"
	"strings"
	"testing"
)

func TestBuildDocumentPageNavigation(t *testing.T) {
	t.Parallel()

	pages := []DocumentTemplatePage{
		{Path: "README.md", Title: "Bundle overview", URL: "../../README.html"},
		{Path: "guides/getting-started.md", Title: "Getting started", URL: "../../guides/getting-started.html"},
		{Path: "notes.md", Title: "Release notes", URL: "../../notes.html"},
		{Path: "guides/reference/api.md", Title: "API", URL: "../../guides/reference/api.html"},
		{Path: "guides/architecture.md", Title: "Architecture", URL: "../../guides/architecture.html"},
		{Path: "examples/server.go", Title: "server.go", URL: "server.go.html", Current: true},
	}

	want := []DocumentTemplateNavigationItem{
		{Name: "README.md", Path: "README.md", Title: "Bundle overview", URL: "../../README.html"},
		{
			Name: "guides", Path: "guides", IsDirectory: true,
			Children: []DocumentTemplateNavigationItem{
				{Name: "getting-started.md", Path: "guides/getting-started.md", Title: "Getting started", URL: "../../guides/getting-started.html"},
				{
					Name: "reference", Path: "guides/reference", IsDirectory: true,
					Children: []DocumentTemplateNavigationItem{
						{Name: "api.md", Path: "guides/reference/api.md", Title: "API", URL: "../../guides/reference/api.html"},
					},
				},
				{Name: "architecture.md", Path: "guides/architecture.md", Title: "Architecture", URL: "../../guides/architecture.html"},
			},
		},
		{Name: "notes.md", Path: "notes.md", Title: "Release notes", URL: "../../notes.html"},
		{
			Name: "examples", Path: "examples", Current: true, IsDirectory: true,
			Children: []DocumentTemplateNavigationItem{
				{Name: "server.go", Path: "examples/server.go", Title: "server.go", URL: "server.go.html", Current: true},
			},
		},
	}

	if got := buildDocumentPageNavigation(pages); !reflect.DeepEqual(got, want) {
		t.Fatalf("navigation = %#v\nwant %#v", got, want)
	}
}

func TestCustomTemplateReceivesFlatPagesAndGroupedNavigation(t *testing.T) {
	t.Parallel()

	tmpl := template.Must(template.New("navigation").Parse(
		`{{range .Pages}}{{.Path}};{{end}}|` +
			`{{range .PageNavigation}}` +
			`{{if .IsDirectory}}dir={{.Name}}/{{range .Children}}{{.Name}},{{end}};` +
			`{{else}}page={{.Name}};{{end}}{{end}}|` +
			`{{range .CurrentPageBreadcrumbs}}{{.}};{{end}}`,
	))
	pages := []DocumentTemplatePage{
		{Path: "README.md", Title: "Bundle overview", URL: "README.html"},
		{Path: "guides/getting-started.md", Title: "Getting started", URL: "guides/getting-started.html", Current: true},
		{Path: "guides/architecture.md", Title: "Architecture", URL: "guides/architecture.html"},
	}
	out, err := RenderMarkdown([]byte("# Bundle overview\n"), RenderOptions{
		Title: "Bundle overview", Template: tmpl, Pages: pages,
		CurrentPage: pages[1], Entrypoint: "README.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "README.md;guides/getting-started.md;guides/architecture.md;|" +
		"page=README.md;dir=guides/getting-started.md,architecture.md,;|" +
		"guides;getting-started.md;"
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("custom navigation data = %q, want %q", got, want)
	}
}
