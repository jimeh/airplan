package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type trackedBundleReader struct {
	reader io.Reader
	reads  int
}

func (r *trackedBundleReader) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func TestValidateBundlePath(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"README.md", "docs/design.md", "images/flow chart.svg"} {
		if err := ValidateBundlePath(value); err != nil {
			t.Errorf("ValidateBundlePath(%q): %v", value, err)
		}
	}
	for _, value := range []string{"", "/absolute", "../outside", "docs/../x", "docs//x", `docs\x`, "docs/.airplan-secret"} {
		if err := ValidateBundlePath(value); err == nil {
			t.Errorf("ValidateBundlePath(%q) succeeded", value)
		}
	}
}

func TestRenderDocumentNamesLinksAndAssets(t *testing.T) {
	t.Parallel()
	asset := []byte("<svg>asset</svg>")
	bundle, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{Reader: strings.NewReader("# Entry\n\n[Design](docs/design.md?plain=1#details)\n"), Path: "README.md"},
		Pages: []PageInput{
			{Reader: strings.NewReader("# Design\n\n[Entry](../README.md)\n"), Path: "docs/design.md"},
			{Reader: strings.NewReader("package main\n"), Path: "src/main.go"},
			{Reader: strings.NewReader("package main\n"), Path: "src/main.h"},
		},
		Assets:        []AssetInput{{Reader: bytes.NewReader(asset), Path: "images/flow.svg", Size: int64(len(asset))}},
		RepositoryURL: "none",
	}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{IncludeSource: true, Repository: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Entrypoint != "readme.html" {
		t.Fatalf("entrypoint = %q", bundle.Entrypoint)
	}
	wantNames := []string{"readme.html", "docs/design.html", "src/main.go.html", "src/main.h.html"}
	for index, want := range wantNames {
		if got := bundle.Pages[index].PagePath; got != want {
			t.Errorf("page %d path = %q, want %q", index, got, want)
		}
	}
	if !bytes.Contains(bundle.Pages[0].HTML, []byte(`href="docs/design.html?plain=1#details"`)) {
		t.Errorf("entry link was not rewritten:\n%s", bundle.Pages[0].HTML)
	}
	if !bytes.Contains(bundle.Pages[1].HTML, []byte(`href="../readme.html"`)) {
		t.Errorf("nested link was not rewritten:\n%s", bundle.Pages[1].HTML)
	}
	if !bytes.Equal(asset, []byte("<svg>asset</svg>")) {
		t.Fatal("asset bytes changed")
	}
}

func TestRenderDocumentTextEntryRules(t *testing.T) {
	t.Parallel()

	t.Run("managed page rejected before child read", func(t *testing.T) {
		child := &trackedBundleReader{reader: strings.NewReader("# Child\n")}
		_, err := RenderDocument(context.Background(), DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("plain text\n"), Path: "notes.txt",
			},
			Pages:         []PageInput{{Reader: child, Path: "child.md"}},
			RepositoryURL: "none",
		}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}})
		if err == nil || !strings.Contains(err.Error(), "managed pages require a Markdown entry") {
			t.Fatalf("error = %v", err)
		}
		if child.reads != 0 {
			t.Fatalf("child reads = %d, want 0", child.reads)
		}
	})

	t.Run("asset accepted", func(t *testing.T) {
		asset := []byte("asset")
		bundle, err := RenderDocument(context.Background(), DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("plain text\n"), Path: "notes.txt",
			},
			Assets: []AssetInput{{
				Reader: bytes.NewReader(asset), Path: "evidence.bin",
				Size: int64(len(asset)),
			}},
			RepositoryURL: "none",
		}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}})
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Format != "txt" || len(bundle.Pages) != 1 || len(bundle.Assets) != 1 {
			t.Fatalf("bundle = %+v", bundle)
		}
	})
}

func TestRenderDocumentRejectsGeneratedCollision(t *testing.T) {
	t.Parallel()
	_, err := RenderDocument(context.Background(), DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("# Entry"), Path: "README.md"},
		Pages:         []PageInput{{Reader: strings.NewReader("source"), Path: "main.c"}},
		Assets:        []AssetInput{{Reader: bytes.NewReader(nil), Path: "main.c.html", Size: 0}},
		RepositoryURL: "none",
	}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}})
	if err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("error = %v", err)
	}
}

func TestRenderDocumentEnforcesItemBoundaryBeforeReading(t *testing.T) {
	t.Parallel()
	pages := make([]PageInput, MaxDocumentItems)
	_, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{Reader: strings.NewReader("# Entry"), Path: "entry.md"},
		Pages: pages, RepositoryURL: "none",
	}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}})
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("error = %v", err)
	}
}

func TestUploadDocumentRejectsAssetMutationBeforePublishingEntry(t *testing.T) {
	body := []byte("before")
	var putPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putPaths = append(putPaths, r.URL.Path)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/"+MarkerFilename) {
			copy(body, "after!")
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client, err := New(context.Background(), &Config{
		Endpoint: server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		DisableManifest: true, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UploadDocument(context.Background(), DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("# Entry"), Path: "entry.md"},
		Assets:        []AssetInput{{Reader: bytes.NewReader(body), Path: "evidence.bin", Size: int64(len(body))}},
		RepositoryURL: "none",
	})
	if err == nil || !strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("error = %v", err)
	}
	if len(putPaths) != 2 || !strings.HasSuffix(putPaths[0], "/"+MarkerFilename) ||
		!strings.HasSuffix(putPaths[1], "/entry.md") {
		t.Fatalf("PUT paths = %q, want marker and source only", putPaths)
	}
}

func TestUploadDocumentOmitsSourceBytesWithoutSourceObject(t *testing.T) {
	t.Run("authored HTML with asset", func(t *testing.T) {
		store := newUpgradeStore(t)
		client := store.client(t, "")
		asset := []byte("asset")
		result, err := client.UploadDocument(context.Background(), DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("<!doctype html><title>Page</title>"),
				Path:   "page.html",
			},
			Assets: []AssetInput{{
				Reader: bytes.NewReader(asset), Path: "asset.bin",
				Size: int64(len(asset)),
			}},
			RepositoryURL: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		assertPageResultOmitsSource(t, result.Pages)
		markerBody, ok := store.get(result.MarkerKey)
		if !ok {
			t.Fatal("marker was not uploaded")
		}
		marker, err := DecodeUploadMarker(markerBody, result.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(marker.Pages) != 0 || len(result.Assets) != 1 {
			t.Fatalf("marker pages = %+v, result assets = %+v", marker.Pages, result.Assets)
		}
	})

	t.Run("generated page under no-source", func(t *testing.T) {
		store := newUpgradeStore(t)
		client, err := New(context.Background(), &Config{
			Endpoint: store.server.URL, Bucket: "plans", AccessKeyID: "test",
			SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
			DisableManifest: true, NoSource: true, Repository: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.UploadDocument(context.Background(), DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("# Page\n"), Path: "page.md",
			},
			RepositoryURL: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		assertPageResultOmitsSource(t, result.Pages)
		if result.SourceKey != "" || result.SourceURL != "" {
			t.Fatalf("top-level source fields = %q, %q", result.SourceKey, result.SourceURL)
		}
	})
}

func assertPageResultOmitsSource(t *testing.T, pages []PageResult) {
	t.Helper()
	if len(pages) != 1 {
		t.Fatalf("pages = %+v", pages)
	}
	page := pages[0]
	if page.SourceKey != "" || page.SourceURL != "" || page.SourceBytes != 0 {
		t.Fatalf("source fields = %+v", page)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source_key", "source_url", "source_bytes"} {
		if bytes.Contains(body, []byte(`"`+field+`"`)) {
			t.Fatalf("JSON %s contains %q", body, field)
		}
	}
}

func TestCreateDocumentRevisionRejectsDetectedTextBeforeMutation(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Plan\n"), Path: "plan.md",
		},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	putsBefore := store.puts
	_, err = client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("plain replacement\n"), Path: "plan.txt",
			}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "supports Markdown input only") {
		t.Fatalf("error = %v", err)
	}
	if store.puts != putsBefore {
		t.Fatalf("PUT count = %d, want %d", store.puts, putsBefore)
	}

	asset := []byte("asset")
	_, err = client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{
				Entry: PageInput{
					Reader: strings.NewReader("plain replacement\n"), Path: "plan.txt",
				},
				Assets: []AssetInput{{
					Reader: bytes.NewReader(asset), Path: "asset.bin",
					Size: int64(len(asset)),
				}},
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "revision entry must use Markdown format") {
		t.Fatalf("bundle error = %v", err)
	}
	if store.puts != putsBefore {
		t.Fatalf("bundle PUT count = %d, want %d", store.puts, putsBefore)
	}
}

func TestCreateDocumentRevisionRendersCompleteBundleFromSpooledSources(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	asset := []byte("asset-one")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Plan\n\n[Details](docs/details.md)\n"),
			Path:   "plan.md",
		},
		Pages: []PageInput{{
			Reader: strings.NewReader("# Details\n\nFirst.\n"),
			Path:   "docs/details.md",
		}},
		Assets: []AssetInput{{
			Reader: bytes.NewReader(asset), Path: "images/evidence.bin",
			Size: int64(len(asset)),
		}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	asset = []byte("asset-two")
	second, err := client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{
				Entry: PageInput{
					Reader: strings.NewReader("# Plan\n\n[Details](docs/details.md)\n\nRevised.\n"),
					Path:   "plan.md",
				},
				Pages: []PageInput{{
					Reader: strings.NewReader("# Details\n\nSecond.\n"),
					Path:   "docs/details.md",
				}},
				Assets: []AssetInput{{
					Reader: bytes.NewReader(asset), Path: "images/evidence.bin",
					Size: int64(len(asset)),
				}},
				RepositoryURL: "none",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != 2 || len(second.Pages) != 2 ||
		len(second.Assets) != 1 || second.Pages[1].Path != "docs/details.md" {
		t.Fatalf("revision result = %+v", second)
	}
	markerBody, ok := store.get(second.MarkerKey)
	if !ok {
		t.Fatalf("marker %q was not uploaded", second.MarkerKey)
	}
	marker, err := DecodeUploadMarker(markerBody, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	dirPrefix := second.ID + "/"
	for _, object := range marker.Objects {
		body, exists := store.get(dirPrefix + object.Name)
		if !exists || int64(len(body)) != object.Bytes ||
			contentSHA256(body) != object.SHA256 {
			t.Fatalf("object %q = %d bytes, exists %t; marker = %+v",
				object.Name, len(body), exists, object)
		}
	}
}

func TestMarkerV6BundleRoundTrip(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: markerTestDir, CreatedAt: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
		Kind: UploadKindDocument, Slug: "readme", Format: "md", Title: "Entry",
		Producer:   Producer{Name: "airplan", Version: "0.11.0"},
		Render:     documentRenderRecipe(&Config{ThemeBundle: defaultThemeBundle()}, ""),
		Entrypoint: "readme.html",
		Pages: []MarkerPage{
			{Path: "README.md", Page: "readme.html", Source: "readme.md", Format: "md", Title: "Entry", Lang: ""},
			{Path: "src/main.c", Page: "src/main.c.html", Source: "src/main.c", Format: "txt", Title: "main.c", Lang: "c"},
		},
		Objects: []MarkerObject{
			{Name: "readme.html", Role: MarkerRolePage, Bytes: 1, ContentType: pageContentType, SHA256: digest},
			{Name: "readme.md", Role: MarkerRoleSource, Bytes: 1, ContentType: sourceContentType, SHA256: digest},
			{Name: "src/main.c.html", Role: MarkerRolePage, Bytes: 1, ContentType: pageContentType, SHA256: digest},
			{Name: "src/main.c", Role: MarkerRoleSource, Bytes: 1, ContentType: textContentType, SHA256: digest},
			{Name: "images/flow.svg", Role: MarkerRoleAsset, Bytes: 1, ContentType: "image/svg+xml", SHA256: digest},
		},
	}
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeUploadMarker(body, markerTestDir)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Page != marker.Entrypoint || decoded.Source != marker.Pages[0].Source || len(decoded.Pages) != 2 {
		t.Fatalf("decoded compatibility view = %+v", decoded)
	}
}

func TestMarkerV6RequiresPersistedPageLanguage(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("a", 64)
	body := []byte(fmt.Sprintf(`{"schema":"airplan-upload","version":6,"directory":"%s","created_at":"2026-08-19T12:00:00Z","kind":"document","slug":"plan","format":"md","objects":[{"name":"plan.html","role":"page","bytes":1,"content_type":"text/html","sha256":"%s"}],"entrypoint":"plan.html","pages":[{"path":"plan.md","page":"plan.html","format":"md"}]}`, markerTestDir, digest))
	_, err := DecodeUploadMarker(body, markerTestDir)
	if err == nil || !strings.Contains(err.Error(), "required fields") {
		t.Fatalf("DecodeUploadMarker error = %v", err)
	}
}
