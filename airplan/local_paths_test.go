package airplan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPlanLocalPathsInfersUploadKindAndRoles(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		value := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(value), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(value, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return value
	}
	readme := write("README.md", "# Entry\n")
	index := write("index.md", "# Other entry\n")
	guide := write("docs/guide.md", "# Guide\n")
	source := write("examples/main.go", "package main\n")
	jsonFile := write("assets/status.json", `{"ok":true}`)
	htmlFile := write("site/help.html", "<html></html>")
	image := write("assets/screen.png", "not really an image")
	svg := write("assets/flow.svg", "<svg></svg>")

	plan, err := PlanLocalPaths(LocalPathPlanOptions{Paths: []string{
		guide, source, index, readme, jsonFile, htmlFile, image, svg,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != UploadKindDocument || plan.Entrypoint != readme {
		t.Fatalf("plan = %+v", plan)
	}
	if !reflect.DeepEqual(plan.PagePaths, []string{guide, source, index, jsonFile}) {
		t.Fatalf("pages = %v", plan.PagePaths)
	}
	if !reflect.DeepEqual(plan.AssetPaths, []string{htmlFile, image, svg}) {
		t.Fatalf("assets = %v", plan.AssetPaths)
	}

	plan, err = PlanLocalPaths(LocalPathPlanOptions{
		Paths:      []string{readme, jsonFile, image},
		AssetPaths: []string{jsonFile},
		PagePaths:  []string{image},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.PagePaths, []string{image}) ||
		!reflect.DeepEqual(plan.AssetPaths, []string{jsonFile}) {
		t.Fatalf("overridden plan = %+v", plan)
	}
}

func TestPlanLocalPathsHTMLAndCollectionDefaults(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		value := filepath.Join(root, name)
		if err := os.WriteFile(value, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return value
	}
	index := write("index.html", "<html></html>")
	help := write("help.html", "<html></html>")
	css := write("site.css", "body {}")
	first := write("main.go", "package main\n")
	second := write("main.h", "void main(void);\n")

	plan, err := PlanLocalPaths(LocalPathPlanOptions{Paths: []string{help, css, index}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != UploadKindDocument || plan.Entrypoint != index ||
		!reflect.DeepEqual(plan.AssetPaths, []string{help, css}) {
		t.Fatalf("HTML plan = %+v", plan)
	}

	plan, err = PlanLocalPaths(LocalPathPlanOptions{Paths: []string{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != UploadKindCollection ||
		!reflect.DeepEqual(plan.CollectionPaths, []string{first, second}) {
		t.Fatalf("collection plan = %+v", plan)
	}

	plan, err = PlanLocalPaths(LocalPathPlanOptions{Paths: []string{first}})
	if err != nil || plan.Kind != UploadKindDocument || plan.Entrypoint != first {
		t.Fatalf("single source plan = %+v, %v", plan, err)
	}
}

func TestPlanLocalPathsHandlesUTF8SniffBoundary(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name       string
		body       []byte
		collection bool
	}{
		{
			name: "complete rune across boundary",
			body: append(
				[]byte(strings.Repeat("a", localInputSniffSize-1)),
				[]byte("漢\n")...,
			),
		},
		{
			name: "malformed rune across boundary",
			body: append(
				[]byte(strings.Repeat("a", localInputSniffSize-1)),
				[]byte{0xe6, 0xff}...,
			),
			collection: true,
		},
		{
			name:       "nul byte",
			body:       []byte("text\x00more"),
			collection: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".dat")
			if err := os.WriteFile(path, tt.body, 0o600); err != nil {
				t.Fatal(err)
			}
			plan, err := PlanLocalPaths(LocalPathPlanOptions{Paths: []string{path}})
			if err != nil {
				t.Fatal(err)
			}
			got := plan.Kind == UploadKindCollection
			if got != tt.collection {
				t.Fatalf("collection = %v, want %v", got, tt.collection)
			}
		})
	}
}

func TestPlanLocalPathsEntrypointAndContainment(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "readme.md")
	page := filepath.Join(root, "page.md")
	outside := filepath.Join(t.TempDir(), "outside.md")
	for _, value := range []string{entry, page, outside} {
		if err := os.WriteFile(value, []byte("# Page\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := PlanLocalPaths(LocalPathPlanOptions{
		Paths:      []string{page},
		Entrypoint: entry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Entrypoint != entry || !reflect.DeepEqual(plan.PagePaths, []string{page}) {
		t.Fatalf("explicit entry plan = %+v", plan)
	}

	_, err = PlanLocalPaths(LocalPathPlanOptions{Paths: []string{entry, outside}})
	if err == nil || !strings.Contains(err.Error(), "outside the entry directory") {
		t.Fatalf("containment error = %v", err)
	}
}

func TestPlanLocalPathsRejectsConflictingIntent(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "README.md")
	if err := os.WriteFile(entry, []byte("# Entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, opts := range []LocalPathPlanOptions{
		{Paths: []string{entry}, ForceCollection: true, Entrypoint: entry},
		{Paths: []string{entry}, PagePaths: []string{entry}},
		{Paths: []string{entry}, PagePaths: []string{entry}, AssetPaths: []string{entry}},
	} {
		if _, err := PlanLocalPaths(opts); err == nil {
			t.Fatalf("PlanLocalPaths(%+v) succeeded", opts)
		}
	}
}
