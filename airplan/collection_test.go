package airplan

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUploadFilesStreamsOneCollection(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var mu sync.Mutex
	var puts []capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		request := captureRequest(r)
		request.body = body
		mu.Lock()
		puts = append(puts, request)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	c, err := New(context.Background(), &Config{
		Endpoint: server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: filepath.Join(t.TempDir(), "manifest.jsonl"),
		Repository:   "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	image := []byte("not really a png")
	video := []byte{0, 1, 2, 3}
	res, err := c.UploadFiles(context.Background(), FilesInput{
		Files: []FileInput{
			{Name: "shot one.png", Reader: bytes.NewReader(image), Size: int64(len(image))},
			{Name: "demo.webm", Reader: bytes.NewReader(video), Size: int64(len(video))},
			{Name: "empty.bin", Reader: bytes.NewReader(nil), Size: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 3 || !strings.HasSuffix(res.URL, "/index.html") {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Files[0].URL, "shot%20one.png") {
		t.Fatalf("file URL = %q", res.Files[0].URL)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 5 {
		t.Fatalf("PUTs = %d, want 5", len(puts))
	}
	if !strings.HasSuffix(puts[0].path, "/"+CollectionMarkerFilename) ||
		!strings.HasSuffix(puts[4].path, "/index.html") {
		t.Fatalf("PUT order = %v", []string{
			puts[0].path, puts[1].path, puts[2].path, puts[3].path, puts[4].path,
		})
	}
	dir := strings.Split(strings.Trim(puts[0].path, "/"), "/")[1]
	marker, err := DecodeUploadMarkerForName(puts[0].body, dir, CollectionMarkerFilename)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Kind != UploadKindCollection || marker.Slug != "" || len(marker.Objects) != 4 {
		t.Fatalf("marker = %+v", marker)
	}
	if !bytes.Equal(puts[1].body, image) || !bytes.Equal(puts[2].body, video) {
		t.Fatal("collection bytes changed")
	}
	if len(puts[3].body) != 0 {
		t.Fatalf("zero-byte member body = %q", puts[3].body)
	}
}

func TestRenderCollectionGolden(t *testing.T) {
	t.Run("release_verification_evidence", func(t *testing.T) {
		paths := []string{
			filepath.Join(
				"testdata", "collection-demo", "verification-summary.svg",
			),
			filepath.Join("testdata", "collection-demo", "checks.json"),
			filepath.Join("testdata", "collection-demo", "release-notes.txt"),
		}
		files := make([]FileInput, 0, len(paths))
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, FileInput{
				Name: filepath.Base(path), Reader: bytes.NewReader(body),
				Size: int64(len(body)),
			})
		}

		body, prepared, err := RenderCollection(
			context.Background(),
			FilesInput{Files: files, Title: "Release verification evidence"},
			CollectionRenderOptions{
				Repository: "https://github.com/jimeh/airplan",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared) != len(files) || prepared[0].MediaKind != "image" {
			t.Fatalf("prepared files = %+v", prepared)
		}
		want := htmlGoldens.Do(t, body)
		if !bytes.Equal(body, want) {
			t.Errorf(
				"rendered output differs from %s "+
					"(set GOLDEN_UPDATE=1 to refresh)",
				htmlGoldens.File(t),
			)
		}
	})
}

func TestRenderCollectionCustomTemplateAndValidation(t *testing.T) {
	tmpl := filepath.Join(t.TempDir(), "collection.tmpl")
	if err := os.WriteFile(tmpl, []byte(`{{range .Files}}{{.Name}}={{.Path}};{{end}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body, files, err := RenderCollection(context.Background(), FilesInput{
		Files: []FileInput{{Name: "a b.svg", Reader: bytes.NewReader(nil), Size: 0}},
	}, CollectionRenderOptions{TemplatePath: tmpl, Repository: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "a b.svg=./a%20b.svg;" || files[0].MediaKind != "image" {
		t.Fatalf("body=%q files=%+v", body, files)
	}
	_, _, err = RenderCollection(context.Background(), FilesInput{Files: []FileInput{
		{Name: "same.txt", Reader: bytes.NewReader(nil), Size: 0},
		{Name: "same.txt", Reader: bytes.NewReader(nil), Size: 0},
	}}, CollectionRenderOptions{Repository: "none"})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderCollectionTotalSizeBoundariesAndOverflow(t *testing.T) {
	inputs := []FileInput{
		{Name: "a.bin", Reader: bytes.NewReader([]byte("a")), Size: 1},
		{Name: "b.bin", Reader: bytes.NewReader([]byte("b")), Size: 1},
	}
	if _, _, err := RenderCollection(context.Background(), FilesInput{
		Files: inputs, MaxTotalSize: 2,
	}, CollectionRenderOptions{Repository: "none"}); err != nil {
		t.Fatalf("exact boundary: %v", err)
	}
	if _, _, err := RenderCollection(context.Background(), FilesInput{
		Files: inputs, MaxTotalSize: 1,
	}, CollectionRenderOptions{Repository: "none"}); err == nil ||
		!strings.Contains(err.Error(), "maximum total size") {
		t.Fatalf("over boundary error = %v", err)
	}
	overflow := []FileInput{
		{Name: "huge.bin", Reader: bytes.NewReader(nil), Size: mathMaxInt64},
		{Name: "one.bin", Reader: bytes.NewReader(nil), Size: 1},
	}
	if _, _, err := RenderCollection(context.Background(), FilesInput{
		Files: overflow, MaxSize: -1, MaxTotalSize: -1,
	}, CollectionRenderOptions{Repository: "none"}); err == nil ||
		!strings.Contains(err.Error(), "out of range") {
		t.Fatalf("overflow error = %v", err)
	}
}
