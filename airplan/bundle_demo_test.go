package airplan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jimeh/go-golden"
)

var bundleDemoGoldens = golden.New(golden.WithSuffix(".html"))

func TestRenderBundleDemoGolden(t *testing.T) {
	root := filepath.Join("testdata", "bundle-demo")
	read := func(name string) []byte {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return body
	}

	entry := read("implementation-plan.md")
	design := read("docs/design.md")
	server := read("examples/server.go")
	flow := read("images/request-flow.svg")
	bundle, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: bytes.NewReader(entry), Path: "implementation-plan.md",
		},
		Pages: []PageInput{
			{Reader: bytes.NewReader(design), Path: "docs/design.md"},
			{Reader: bytes.NewReader(server), Path: "examples/server.go"},
		},
		Assets: []AssetInput{{
			Reader: bytes.NewReader(flow), Path: "images/request-flow.svg",
			Size: int64(len(flow)),
		}},
		RepositoryURL: "https://github.com/jimeh/airplan",
	}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{
		IncludeSource: true,
		Repository:    "https://github.com/jimeh/airplan",
	}})
	if err != nil {
		t.Fatal(err)
	}

	var inventory []string
	for _, page := range bundle.Pages {
		inventory = append(inventory, page.PagePath, page.SourcePath)
	}
	for _, asset := range bundle.Assets {
		inventory = append(inventory, asset.Path)
	}
	wantInventory := []string{
		"implementation-plan.html",
		"implementation-plan.md",
		"docs/design.html",
		"docs/design.md",
		"examples/server.go.html",
		"examples/server.go",
		"images/request-flow.svg",
	}
	if !reflect.DeepEqual(inventory, wantInventory) {
		t.Fatalf("rendered object inventory = %v, want %v", inventory, wantInventory)
	}

	goldenNames := map[string]string{
		"implementation-plan.html": "implementation_plan",
		"docs/design.html":         "design",
		"examples/server.go.html":  "server_go",
	}
	for _, page := range bundle.Pages {
		page := page
		name := goldenNames[page.PagePath]
		if name == "" {
			t.Fatalf("no golden name for rendered page %q", page.PagePath)
		}
		t.Run(name, func(t *testing.T) {
			want := bundleDemoGoldens.Do(t, page.HTML)
			if !bytes.Equal(page.HTML, want) {
				t.Errorf(
					"rendered output differs from %s "+
						"(set GOLDEN_UPDATE=1 to refresh)",
					bundleDemoGoldens.File(t),
				)
			}
		})
	}
}
