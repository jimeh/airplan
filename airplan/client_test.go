package airplan

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestReadInput(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		data, err := readInput(strings.NewReader("hello"), 10)
		if err != nil || string(data) != "hello" {
			t.Errorf("data = %q, err = %v", data, err)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		data, err := readInput(strings.NewReader("0123456789"), 10)
		if err != nil || len(data) != 10 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		_, err := readInput(strings.NewReader("0123456789x"), 10)
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("err = %v, want ErrInputTooLarge", err)
		}
	})

	t.Run("unlimited", func(t *testing.T) {
		data, err := readInput(strings.NewReader("0123456789x"), 0)
		if err != nil || len(data) != 11 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})
}

func TestUploadRejectsOversizedInput(t *testing.T) {
	// Oversized input must fail before any key generation or storage
	// access, so a Client without a live storage backend suffices.
	c := &Client{cfg: &Config{Bucket: "b"}}

	huge := io.LimitReader(zeroReader{}, MaxInputSize+1)
	_, err := c.Upload(context.Background(), Input{Reader: huge})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("err = %v, want ErrInputTooLarge", err)
	}
}

// zeroReader yields zero bytes forever without allocating input data.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { return len(p), nil }

func TestTitleMetadata(t *testing.T) {
	if titleMetadata("") != nil {
		t.Error("empty title should produce nil metadata")
	}

	if got := titleMetadata("Refactor auth")["title"]; got != "Refactor auth" {
		t.Errorf("ASCII title changed: %q", got)
	}

	title := "Ünïcode Tïtle ✨"
	encoded := titleMetadata(title)["title"]
	for i := 0; i < len(encoded); i++ {
		if encoded[i] > 0x7e {
			t.Fatalf("encoded title contains non-ASCII byte: %q", encoded)
		}
	}
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != title {
		t.Errorf("round-trip = %q, want %q", decoded, title)
	}
}

func TestFilenameStem(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"plan.md", "plan"},
		{"/tmp/dir/refactor-auth.html", "refactor-auth"},
		{"noext", "noext"},
		{"archive.tar.gz", "archive.tar"},
	}
	for _, tt := range tests {
		if got := filenameStem(tt.in); got != tt.want {
			t.Errorf("filenameStem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSourceExt(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "txt"},
		{"main.go", "go"},
		{"config.JSON", "json"},
		{"noext", "txt"},
		{"page.html", "txt"},
		{"page.htm", "txt"},
		{"weird.t?x", "tx"},
	}
	for _, tt := range tests {
		if got := sourceExt(tt.in); got != tt.want {
			t.Errorf("sourceExt(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUploadRejectsBinaryInput(t *testing.T) {
	c := &Client{cfg: &Config{Bucket: "b"}}

	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	_, err := c.Upload(context.Background(), Input{
		Reader: strings.NewReader(png),
		Name:   "image.png",
	})
	if !errors.Is(err, ErrBinaryInput) {
		t.Fatalf("err = %v, want ErrBinaryInput", err)
	}
}

// TestUploadTextInput drives the full text pipeline against a fake S3
// endpoint: highlighted page plus original source sibling with the
// real extension.
func TestUploadTextInput(t *testing.T) {
	var (
		mu   sync.Mutex
		puts []capturedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			cr := captureRequest(r)
			cr.body = body
			puts = append(puts, cr)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	src := "package main\n\nfunc main() {}\n"
	res, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader(src),
		Name:   "main.go",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(res.SourceKey, "/main.go") {
		t.Errorf("SourceKey = %q, want */main.go", res.SourceKey)
	}
	if !strings.HasSuffix(res.Key, "/main.html") {
		t.Errorf("Key = %q, want */main.html", res.Key)
	}
	if res.Title != "main" {
		t.Errorf("Title = %q, want %q", res.Title, "main")
	}
	if res.SourceURL == "" {
		t.Error("SourceURL empty for text input")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(puts) != 2 {
		t.Fatalf("got %d puts, want 2 (source first, then page)", len(puts))
	}
	if got := puts[0].header.Get("Content-Type"); got !=
		"text/plain; charset=utf-8" {
		t.Errorf("source Content-Type = %q", got)
	}
	page := string(puts[1].body)
	if !strings.Contains(page, `class="chroma"`) {
		t.Error("page body not chroma-highlighted")
	}
	if !strings.Contains(page, ">Download source<") {
		t.Error("page missing 'Download source' anchor label")
	}
	if !strings.Contains(page, "func") {
		t.Error("page missing source content")
	}
}
