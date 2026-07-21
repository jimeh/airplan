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
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 2 || !strings.HasSuffix(res.URL, "/index.html") {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Files[0].URL, "shot%20one.png") {
		t.Fatalf("file URL = %q", res.Files[0].URL)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 4 {
		t.Fatalf("PUTs = %d, want 4", len(puts))
	}
	if !strings.HasSuffix(puts[0].path, "/"+CollectionMarkerFilename) ||
		!strings.HasSuffix(puts[3].path, "/index.html") {
		t.Fatalf("PUT order = %v", []string{puts[0].path, puts[1].path, puts[2].path, puts[3].path})
	}
	dir := strings.Split(strings.Trim(puts[0].path, "/"), "/")[1]
	marker, err := DecodeUploadMarkerForName(puts[0].body, dir, CollectionMarkerFilename)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Kind != UploadKindCollection || marker.Slug != "" || len(marker.Objects) != 3 {
		t.Fatalf("marker = %+v", marker)
	}
	if !bytes.Equal(puts[1].body, image) || !bytes.Equal(puts[2].body, video) {
		t.Fatal("collection bytes changed")
	}
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
