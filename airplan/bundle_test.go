package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type trackedBundleReader struct {
	reader io.Reader
	reads  int
}

type unreadableAssetReader struct {
	reads int
	seeks int
}

type failSecondHashReader struct {
	reader     *bytes.Reader
	startSeeks int
	failErr    error
}

func (r *failSecondHashReader) Read(p []byte) (int, error) {
	if r.startSeeks >= 3 {
		return 0, r.failErr
	}
	return r.reader.Read(p)
}

func (r *failSecondHashReader) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekStart {
		r.startSeeks++
	}
	return r.reader.Seek(offset, whence)
}

func (r *unreadableAssetReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("unexpected read")
}

func (r *unreadableAssetReader) Seek(int64, int) (int64, error) {
	r.seeks++
	return 0, errors.New("unexpected seek")
}

func oversizedUnreadableDocument(reader *unreadableAssetReader) DocumentInput {
	assets := make([]AssetInput, MaxDocumentItems)
	for index := range assets {
		assets[index] = AssetInput{Reader: reader, Path: fmt.Sprintf("asset-%d.bin", index), Size: 1}
	}
	return DocumentInput{
		Entry:  PageInput{Reader: strings.NewReader("# Entry"), Path: "entry.md"},
		Assets: assets, RepositoryURL: "none",
	}
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
	for _, value := range []string{"", "/absolute", "../outside", "docs/../x", "docs//x", `docs\x`, "docs/.airplan-secret", "docs/.AIRPLAN-secret", ".airplan.json", ".AIRPLAN.JSON", "CON", "aux.txt", "trailing.", "trailing "} {
		if err := ValidateBundlePath(value); err == nil {
			t.Errorf("ValidateBundlePath(%q) succeeded", value)
		} else {
			var invalid *InvalidDocumentInputError
			if !errors.As(err, &invalid) {
				t.Errorf("ValidateBundlePath(%q) error type = %T", value, err)
			}
		}
	}
}

func TestRenderDocumentRejectsNonPortableMaterializationPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input DocumentInput
	}{
		{
			name: "nested entry",
			input: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Plan\n"), Path: "docs/plan.md",
			}},
		},
		{
			name: "generated page equals source",
			input: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Plan\n"), Path: "guide.html", Format: "md",
			}},
		},
		{
			name: "file directory prefix",
			input: DocumentInput{
				Entry: PageInput{Reader: strings.NewReader("# Plan\n"), Path: "plan.md"},
				Assets: []AssetInput{
					{Reader: bytes.NewReader(nil), Path: "images", Size: 0},
					{Reader: bytes.NewReader(nil), Path: "images/icon.svg", Size: 0},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := RenderDocument(context.Background(), test.input,
				DocumentRenderOptions{RenderInputOptions: RenderInputOptions{
					Repository: "none",
				}})
			var invalid *InvalidDocumentInputError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v (%T), want InvalidDocumentInputError", err, err)
			}
		})
	}
}

func TestRenderDocumentNamesLinksAndAssets(t *testing.T) {
	t.Parallel()
	asset := []byte("<svg>asset</svg>")
	bundle, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{Reader: strings.NewReader("# Entry\n\n[Design](docs/design.md?plain=1#details)\n\n[Absolute](/docs/design.md)\n"), Path: "README.md"},
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
	if !bytes.Contains(bundle.Pages[0].HTML, []byte(`href="/docs/design.md"`)) {
		t.Errorf("root-relative link was rewritten:\n%s", bundle.Pages[0].HTML)
	}
	if !bytes.Contains(bundle.Pages[1].HTML, []byte(`href="../readme.html"`)) {
		t.Errorf("nested link was not rewritten:\n%s", bundle.Pages[1].HTML)
	}
	if !bytes.Equal(asset, []byte("<svg>asset</svg>")) {
		t.Fatal("asset bytes changed")
	}
}

func TestRenderDocumentProjectsRevisionChangesPerPage(t *testing.T) {
	report := "# airplan revisions: 2 -> 3\n# README.md\n--- revision-2/README.md\n+++ revision-3/README.md\n@@ -1 +1 @@\n-old\n+new\n# server.go\n--- revision-2/server.go\n+++ revision-3/server.go\n@@ -1 +1 @@\n-old\n+new\n"
	bundle, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{Reader: strings.NewReader("# New\n"), Path: "README.md"},
		Pages: []PageInput{
			{Reader: strings.NewReader("# Stable\n"), Path: "docs/stable.md"},
			{Reader: strings.NewReader("package main\n"), Path: "server.go"},
		},
	}, DocumentRenderOptions{
		RenderInputOptions: RenderInputOptions{IncludeSource: true},
		Revision:           3, RevisionCount: 3, PreviousRevision: 2,
		RevisionChainID: strings.Repeat("a", 26), VersionsPath: VersionsFilename,
		DiffPath:     DiffFilename,
		ChangedPages: map[string]bool{"README.md": true, "server.go": true},
		PageDiffs: map[string]string{
			"README.md": "# README.md\n--- revision-2/README.md\n+++ revision-3/README.md\n",
			"server.go": "# server.go\n--- revision-2/server.go\n+++ revision-3/server.go\n",
		},
		CompleteDiffText: report,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(bundle.Pages[0].HTML, []byte(`data-view="changes"`)) ||
		!bytes.Contains(bundle.Pages[0].HTML, []byte(`class="content all-changes-view"`)) ||
		bytes.Contains(bundle.Pages[0].HTML, []byte(`Changes to server.go`)) {
		t.Fatal("entry does not contain distinct page-local and complete changes")
	}
	if bytes.Contains(bundle.Pages[1].HTML, []byte(`data-view="changes"`)) ||
		bytes.Contains(bundle.Pages[1].HTML, []byte(`class="content all-changes-view"`)) {
		t.Fatal("unchanged child exposes changes UI")
	}
	if !bytes.Contains(bundle.Pages[2].HTML, []byte(`>Code</span>`)) ||
		!bytes.Contains(bundle.Pages[2].HTML, []byte(`data-view="changes"`)) ||
		bytes.Contains(bundle.Pages[2].HTML, []byte(`class="content all-changes-view"`)) {
		t.Fatal("changed source page mode projection is incorrect")
	}
	for _, page := range bundle.Pages {
		if !bytes.Contains(page.HTML, []byte(`name="airplan-page-path"`)) ||
			!bytes.Contains(page.HTML, []byte(`name="airplan-revision-chain"`)) {
			t.Fatalf("page %q lacks immutable revision identity", page.Path)
		}
	}
}

func TestCreateDocumentRevisionProjectsFourPageChanges(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	firstAsset := []byte("first asset")
	secondAsset := []byte("second asset")
	input := func(revised bool) DocumentInput {
		entry, server, asset := "# Plan\n\nOriginal.\n", "package main\n", firstAsset
		pages := []PageInput{
			{Reader: strings.NewReader("# Stable\n"), Path: "stable.md"},
			{Reader: strings.NewReader(server), Path: "airplan guide.go", Lang: "Go"},
			{Reader: strings.NewReader("# Removed\n"), Path: "removed.md"},
		}
		if revised {
			entry, server, asset = "# Plan\n\nRevised.\n", "package main\n\nfunc main() {}\n", secondAsset
			pages = pages[:2]
			pages[1].Reader = strings.NewReader(server)
		}
		return DocumentInput{
			Entry: PageInput{Reader: strings.NewReader(entry), Path: "README.md"},
			Pages: pages,
			Assets: []AssetInput{{
				Reader: bytes.NewReader(asset), Path: "assets/evidence.bin",
				Size: int64(len(asset)), ContentType: "application/octet-stream",
			}},
			RepositoryURL: "none",
		}
	}

	first, err := client.UploadDocument(context.Background(), input(false))
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateDocumentRevision(
		context.Background(),
		CreateDocumentRevisionInput{Target: first.URL, Document: input(true)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Pages) != 3 {
		t.Fatalf("revision pages = %+v", second.Pages)
	}

	entry, entryOK := store.get(second.Pages[0].Key)
	stable, stableOK := store.get(second.Pages[1].Key)
	server, serverOK := store.get(second.Pages[2].Key)
	diff, diffOK := store.get(second.ID + "/" + DiffFilename)
	if !entryOK || !stableOK || !serverOK || !diffOK {
		t.Fatal("revision page or diff object is missing")
	}
	if !bytes.Contains(entry, []byte(`data-view="changes"`)) ||
		!bytes.Contains(entry, []byte(`class="content all-changes-view"`)) ||
		!bytes.Contains(entry, []byte(`Changes to README.md`)) {
		t.Fatal("changed entry lacks its local Changes mode or complete report")
	}
	if bytes.Contains(stable, []byte(`data-view="changes"`)) ||
		bytes.Contains(stable, []byte(`class="content all-changes-view"`)) {
		t.Fatal("unchanged Markdown page exposes changes UI")
	}
	if !bytes.Contains(server, []byte(`>Code</span>`)) ||
		!bytes.Contains(server, []byte(`data-view="changes"`)) ||
		!bytes.Contains(server, []byte(`Changes to airplan guide.go`)) ||
		bytes.Contains(server, []byte(`class="content all-changes-view"`)) {
		t.Fatal("changed source page lacks Code/Changes or embeds the complete report")
	}
	for _, want := range []string{
		"# airplan page order\n", `# airplan page: "README.md"` + "\n",
		`# airplan asset: "assets/evidence.bin"` + "\n",
		`# airplan page: "removed.md"` + "\npage removed:",
		`# airplan page: "airplan guide.go"` + "\n",
	} {
		if !bytes.Contains(diff, []byte(want)) {
			t.Fatalf("complete diff lacks %q:\n%s", want, diff)
		}
	}
	if bytes.Contains(diff, []byte(`# airplan page: "stable.md"`+"\n")) {
		t.Fatalf("complete diff includes unchanged page:\n%s", diff)
	}
}

func TestUploadDocumentHonorsLowerGeneratedPageLimitBeforeMutation(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	_, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Generated page\n"), Path: "plan.md",
		},
		RepositoryURL:        "none",
		maxGeneratedPageSize: 1,
	})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("error = %v, want ErrInputTooLarge", err)
	}
	if store.puts != 0 {
		t.Fatalf("storage PUTs = %d, want 0", store.puts)
	}
}

func TestRelativeObjectURLPrefixesSchemeLikeFirstSegment(t *testing.T) {
	t.Parallel()
	if got := relativeObjectURL("entry.html", "notes:2026.html"); got != "./notes:2026.html" && got != "./notes%3A2026.html" {
		t.Fatalf("relativeObjectURL = %q", got)
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
		var invalid *InvalidDocumentInputError
		if !errors.As(err, &invalid) {
			t.Fatalf("error type = %T, want InvalidDocumentInputError", err)
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

func TestRenderDocumentHTMLPageCombinationIsInvalidInput(t *testing.T) {
	t.Parallel()
	_, err := RenderDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("<!doctype html><title>Entry</title>"),
			Path:   "entry.html",
		},
		Pages: []PageInput{{
			Reader: strings.NewReader("# Child\n"), Path: "child.md",
		}},
		RepositoryURL: "none",
	}, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{
		Repository: "none",
	}})
	if err == nil || !strings.Contains(
		err.Error(), "authored HTML entries cannot declare managed pages",
	) {
		t.Fatalf("error = %v", err)
	}
	var invalid *InvalidDocumentInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("error type = %T, want InvalidDocumentInputError", err)
	}
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
	reader := &unreadableAssetReader{}
	_, err := RenderDocument(context.Background(), oversizedUnreadableDocument(reader),
		DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}})
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("error = %v", err)
	}
	if reader.reads != 0 || reader.seeks != 0 {
		t.Fatalf("asset reader calls = %d reads, %d seeks", reader.reads, reader.seeks)
	}
}

func TestUploadDocumentEnforcesItemBoundaryBeforeReading(t *testing.T) {
	reader := &unreadableAssetReader{}
	store := newUpgradeStore(t)
	_, err := store.client(t, "").UploadDocument(
		context.Background(), oversizedUnreadableDocument(reader),
	)
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("error = %v", err)
	}
	if reader.reads != 0 || reader.seeks != 0 {
		t.Fatalf("asset reader calls = %d reads, %d seeks", reader.reads, reader.seeks)
	}
}

func TestMaterializeDocumentEnforcesItemBoundaryBeforeReading(t *testing.T) {
	reader := &unreadableAssetReader{}
	_, err := MaterializeDocument(
		context.Background(), oversizedUnreadableDocument(reader),
		DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Repository: "none"}},
		filepath.Join(t.TempDir(), "output"),
	)
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("error = %v", err)
	}
	if reader.reads != 0 || reader.seeks != 0 {
		t.Fatalf("asset reader calls = %d reads, %d seeks", reader.reads, reader.seeks)
	}
}

func TestCreateDocumentRevisionEnforcesItemBoundaryBeforeDispatchOrPreparation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	t.Cleanup(server.Close)
	client, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &unreadableAssetReader{}
	_, err = client.CreateDocumentRevision(context.Background(), CreateDocumentRevisionInput{
		Target:   "https://plans.example.com/target/index.html",
		Document: oversizedUnreadableDocument(reader),
	})
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("error = %v", err)
	}
	if requests != 0 || reader.reads != 0 || reader.seeks != 0 {
		t.Fatalf("requests = %d, asset reader calls = %d reads, %d seeks", requests, reader.reads, reader.seeks)
	}

	localReader := &unreadableAssetReader{}
	localClient := newUpgradeStore(t).client(t, "")
	_, err = localClient.createBundleRevision(context.Background(), CreateDocumentRevisionInput{
		Target:   "https://plans.example.com/target/index.html",
		Document: oversizedUnreadableDocument(localReader),
	}, MaxDiffSize)
	if err == nil || !strings.Contains(err.Error(), "maximum is 100") {
		t.Fatalf("local error = %v", err)
	}
	if localReader.reads != 0 || localReader.seeks != 0 {
		t.Fatalf("local asset reader calls = %d reads, %d seeks", localReader.reads, localReader.seeks)
	}
}

func TestUploadDocumentPreservesPublicURLWarningAlongsideRenderWarnings(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	client.cfg.PublicBaseURL = ""
	result, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("<main>trusted</main>"), Path: "index.html"},
		Assets:        []AssetInput{{Reader: bytes.NewReader(nil), Path: "empty.bin", Size: 0}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "no <head> tag") ||
		strings.Count(warnings, PublicURLFallbackWarning) != 1 {
		t.Fatalf("warnings = %#v", result.Warnings)
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

func TestCreateDocumentRevisionPreservesAssetReverificationReadError(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	readErr := errors.New("asset re-verification failed")
	body := []byte("evidence")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Entry"), Path: "entry.md",
		},
		Assets: []AssetInput{{
			Reader: bytes.NewReader(body), Path: "evidence.bin",
			Size: int64(len(body)), ContentType: "application/octet-stream",
		}},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &failSecondHashReader{
		reader: bytes.NewReader(body), failErr: readErr,
	}
	_, err = client.CreateDocumentRevision(context.Background(), CreateDocumentRevisionInput{
		Target: first.URL,
		Document: DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("# Revised entry"), Path: "entry.md",
			},
			Assets: []AssetInput{{
				Reader: reader, Path: "evidence.bin", Size: int64(len(body)),
				ContentType: "application/octet-stream",
			}},
			RepositoryURL: "none",
		},
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("error = %v, want wrapped %v", err, readErr)
	}
	if strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("read failure reported as asset mutation: %v", err)
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

func TestCreateDocumentRevisionCompleteReplacementAndBundlePromotion(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	assetBody := []byte("asset")
	newBundle := func(title string, includeMembers bool) DocumentInput {
		input := DocumentInput{
			Entry: PageInput{
				Reader: strings.NewReader("# Plan\n\n[Details](docs/details.md)\n"),
				Path:   "plan.md", Title: title,
			},
			Title: title, RepositoryURL: "none",
		}
		if includeMembers {
			input.Pages = []PageInput{{
				Reader: strings.NewReader("# Details\n"), Path: "docs/details.md",
			}}
			input.Assets = []AssetInput{{
				Reader: bytes.NewReader(assetBody), Path: "asset.bin",
				Size: int64(len(assetBody)),
			}}
		}
		return input
	}

	first, err := client.UploadDocument(
		context.Background(), newBundle("Plan", true),
	)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL, Document: newBundle("Plan", true),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.Unchanged || len(unchanged.Pages) != 2 ||
		len(unchanged.Assets) != 1 {
		t.Fatalf("unchanged bundle result = %+v", unchanged)
	}

	second, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL, Document: newBundle("Renamed plan", false),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.Unchanged || second.Revision != 2 || len(second.Pages) != 1 ||
		len(second.Assets) != 0 || second.Title != "Renamed plan" {
		t.Fatalf("replacement result = %+v", second)
	}

	predecessor, err := client.InspectUpload(context.Background(), first.URL)
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State != UploadComplete || predecessor.Revision != 1 ||
		len(predecessor.Pages) != 2 || len(predecessor.Assets) != 1 {
		t.Fatalf("promoted predecessor = %+v", predecessor)
	}
	details, ok := store.get(first.ID + "/docs/details.html")
	entry, entryOK := store.get(first.Key)
	if !ok || !bytes.Contains(details, []byte(`class="pages-nav"`)) ||
		!entryOK || bytes.Equal(details, entry) {
		t.Fatalf("promoted detail page = %q", details)
	}
	for _, want := range []string{
		`name="airplan-revision" content="1"`,
		`name="airplan-revision-chain"`,
		`name="airplan-page-path" content="docs/details.md"`,
		`name="airplan-entrypoint" content="../plan.html"`,
	} {
		if !bytes.Contains(details, []byte(want)) {
			t.Fatalf("promoted detail page lacks %q", want)
		}
	}
	secondEntry, secondEntryOK := store.get(second.Key)
	diff, diffOK := store.get(second.ID + "/" + DiffFilename)
	if !secondEntryOK || !bytes.Contains(secondEntry, []byte(`data-airplan-all-changes`)) ||
		!diffOK {
		t.Fatal("revision entry or complete diff is missing")
	}
	for _, want := range []string{
		"# airplan page order\n", "# airplan asset order\n",
		"# airplan metadata\n", `# airplan page: "docs/details.md"` + "\npage removed:",
		`# airplan asset: "asset.bin"` + "\nasset removed:",
	} {
		if !bytes.Contains(diff, []byte(want)) {
			t.Fatalf("complete diff lacks %q:\n%s", want, diff)
		}
	}

	noop, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL, Document: newBundle("Renamed plan", false),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !noop.Unchanged || noop.Revision != 2 || noop.LatestRevision != 2 ||
		noop.PreviousURL != first.URL || noop.DiffURL == "" ||
		len(noop.Pages) != 1 || len(noop.Assets) != 0 {
		t.Fatalf("latest no-op result = %+v", noop)
	}
}

func TestCreateDocumentRevisionEmptyEntryPathPreservesLatestLogicalPath(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Plan\n"), Path: "README.md",
		},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	putsBefore := store.puts

	unchanged, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Plan\n"),
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.Unchanged || unchanged.ID != first.ID ||
		unchanged.Revision != first.Revision ||
		unchanged.LatestRevision != first.LatestRevision ||
		len(unchanged.Pages) != 1 || unchanged.Pages[0].Path != "README.md" {
		t.Fatalf("unchanged revision = %+v", unchanged)
	}
	if store.puts != putsBefore {
		t.Fatalf("no-op storage PUTs = %d -> %d", putsBefore, store.puts)
	}

	changed, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Plan\n\nChanged.\n"),
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Unchanged || changed.Revision != 2 || len(changed.Pages) != 1 ||
		changed.Pages[0].Path != "README.md" {
		t.Fatalf("changed revision = %+v", changed)
	}
	markerBody, ok := store.get(changed.MarkerKey)
	if !ok {
		t.Fatalf("marker %q is missing", changed.MarkerKey)
	}
	marker, err := DecodeUploadMarker(markerBody, changed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(marker.Pages) != 1 || marker.Pages[0].Path != "README.md" {
		t.Fatalf("changed marker pages = %+v", marker.Pages)
	}
}

func TestCreateDocumentRevisionOmittedTitlePreservesLatestTitle(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# Inferred heading\n"), Path: "plan.md",
			Title: "Custom title",
		},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	putsBefore := store.puts

	unchanged, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Inferred heading\n"), Path: "plan.md",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !unchanged.Unchanged || unchanged.Title != "Custom title" ||
		unchanged.Revision != first.Revision {
		t.Fatalf("unchanged revision = %+v", unchanged)
	}
	if store.puts != putsBefore {
		t.Fatalf("no-op storage PUTs = %d -> %d", putsBefore, store.puts)
	}

	changed, err := client.CreateDocumentRevision(
		context.Background(), CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Changed heading\n"), Path: "plan.md",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Unchanged || changed.Revision != 2 ||
		changed.Title != "Custom title" {
		t.Fatalf("changed revision = %+v", changed)
	}
	markerBody, ok := store.get(changed.MarkerKey)
	if !ok {
		t.Fatalf("marker %q is missing", changed.MarkerKey)
	}
	marker, err := DecodeUploadMarker(markerBody, changed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Title != "Custom title" || len(marker.Pages) != 1 ||
		marker.Pages[0].Title != "Custom title" {
		t.Fatalf("changed marker = %+v", marker)
	}
}

func TestCreateDocumentRevisionRollbackFailureLeavesDiscoverableManagedCandidate(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.UploadDocument(context.Background(), DocumentInput{
		Entry: PageInput{
			Reader: strings.NewReader("# One\n"), Path: "plan.md",
		},
		RepositoryURL: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutSuffix = ".html"
	store.failDeleteKeys = true
	store.mu.Unlock()
	_, err = client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: first.URL,
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Two\n"), Path: "plan.md",
			}},
		})
	if err == nil || !strings.Contains(err.Error(), "candidate rollback failed") {
		t.Fatalf("revision error = %v", err)
	}
	store.mu.Lock()
	candidateMarkerKey := ""
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) && key != first.MarkerKey {
			candidateMarkerKey = key
			break
		}
	}
	store.mu.Unlock()
	if candidateMarkerKey == "" {
		t.Fatal("rollback failure did not leave a discoverable ownership marker")
	}
	candidateDir := strings.TrimSuffix(candidateMarkerKey, "/"+MarkerFilename)
	if _, err := client.DeleteUpload(
		context.Background(), candidateDir+"/plan.md",
	); !errors.Is(err, errObjectNotFound) {
		t.Fatalf("live-predecessor candidate delete error = %v, want fail closed", err)
	}
	if _, err := client.DeleteUpload(context.Background(), first.URL); err != nil {
		t.Fatalf("delete predecessor: %v", err)
	}
	deleted, err := client.DeleteUpload(
		context.Background(), candidateDir+"/plan.md",
	)
	if err != nil {
		t.Fatalf("recover candidate after predecessor deletion: %v", err)
	}
	if deleted.MarkerKey != candidateMarkerKey || !strings.Contains(
		strings.Join(deleted.Warnings, "\n"), "unannounced revision candidate",
	) {
		t.Fatalf("recovery result = %+v", deleted)
	}
}

func TestCreateDocumentRevisionRetryRecreatesMissingSiblingMetadata(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	input := func(body string) DocumentInput {
		return DocumentInput{Entry: PageInput{
			Reader: strings.NewReader(body), Path: "plan.md",
		}}
	}
	first, err := client.UploadDocument(
		context.Background(), input("# One\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: first.URL, Document: input("# Two\n"),
		})
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: second.URL, Document: input("# Three\n"),
		})
	if err != nil {
		t.Fatal(err)
	}
	firstMetadataKey := first.ID + "/" + VersionsFilename
	store.mu.Lock()
	delete(store.objects, firstMetadataKey)
	delete(store.etags, firstMetadataKey)
	store.mu.Unlock()

	retried, err := client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: third.URL, Document: input("# Three\n"),
		})
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Unchanged {
		t.Fatalf("retry result = %+v", retried)
	}
	body, ok := store.get(firstMetadataKey)
	if !ok {
		t.Fatal("retry did not recreate missing sibling metadata")
	}
	metadata, err := DecodeVersionsMetadata(body, client.cfg, first.Key)
	if err != nil || metadata.CurrentRevision != 1 ||
		metadata.LatestRevision != 3 {
		t.Fatalf("recreated metadata = %+v, err = %v", metadata, err)
	}
}

func TestCreateDocumentRevisionRunsLegacyPrerequisiteUpgrade(t *testing.T) {
	store := newUpgradeStore(t)
	dir := strings.Repeat("v", 26)
	seedV3UpgradeDocument(t, store, dir)
	client := store.client(t, "")
	result, err := client.CreateDocumentRevision(context.Background(),
		CreateDocumentRevisionInput{
			Target: "https://plans.example.com/" + dir + "/plan.html",
			Document: DocumentInput{Entry: PageInput{
				Reader: strings.NewReader("# Plan\n\nRevised.\n"), Path: "plan.md",
			}},
		})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 2 || result.PreviousURL == "" {
		t.Fatalf("revision = %+v", result)
	}
	markerBody, ok := store.get(dir + "/" + MarkerFilename)
	if !ok {
		t.Fatal("upgraded predecessor marker is missing")
	}
	marker, err := DecodeUploadMarker(markerBody, dir)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Version != MarkerVersion || marker.Revision == nil ||
		marker.Revision.Number != 1 {
		t.Fatalf("upgraded predecessor marker = %+v", marker)
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
