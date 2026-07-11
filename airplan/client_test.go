package airplan

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadInput(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		data, err := readInput(context.Background(), strings.NewReader("hello"), 10)
		if err != nil || string(data) != "hello" {
			t.Errorf("data = %q, err = %v", data, err)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		data, err := readInput(context.Background(), strings.NewReader("0123456789"), 10)
		if err != nil || len(data) != 10 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		_, err := readInput(context.Background(), strings.NewReader("0123456789x"), 10)
		if !errors.Is(err, ErrInputTooLarge) {
			t.Errorf("err = %v, want ErrInputTooLarge", err)
		}
	})

	t.Run("unlimited", func(t *testing.T) {
		data, err := readInput(context.Background(), strings.NewReader("0123456789x"), 0)
		if err != nil || len(data) != 11 {
			t.Errorf("len = %d, err = %v", len(data), err)
		}
	})
}

func TestUploadRejectsOversizedInput(t *testing.T) {
	// Oversized input must fail before any key generation or storage
	// access, so a Client without a live storage backend suffices.
	c := &Client{cfg: &Config{Bucket: "b"}}

	t.Run("default limit", func(t *testing.T) {
		huge := io.LimitReader(zeroReader{}, DefaultMaxInputSize+1)
		_, err := c.Upload(context.Background(), Input{Reader: huge})
		if !errors.Is(err, ErrInputTooLarge) {
			t.Fatalf("err = %v, want ErrInputTooLarge", err)
		}
		if !strings.Contains(err.Error(), "10 MiB") {
			t.Errorf("error does not name the limit: %v", err)
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		_, err := c.Upload(context.Background(), Input{
			Reader:  strings.NewReader("0123456789x"),
			MaxSize: 10,
		})
		if !errors.Is(err, ErrInputTooLarge) {
			t.Fatalf("err = %v, want ErrInputTooLarge", err)
		}
		if !strings.Contains(err.Error(), "10 bytes") {
			t.Errorf("error does not name the limit: %v", err)
		}
	})
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"0", 0, false},
		{"1048576", 1 << 20, false},
		{"512k", 512 << 10, false},
		{"512KB", 512 << 10, false},
		{"10m", 10 << 20, false},
		{"10MB", 10 << 20, false},
		{"10MiB", 10 << 20, false},
		{"1g", 1 << 30, false},
		{"1GiB", 1 << 30, false},
		{" 5m ", 5 << 20, false},
		{"", 0, true},
		{"-1", 0, true},
		{"1.5m", 0, true},
		{"10x", 0, true},
		{"mb", 0, true},
		{"9999999999g", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSize(%q) error = %v, wantErr %v",
				tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{10 << 20, "10 MiB"},
		{512 << 10, "512 KiB"},
		{1 << 30, "1 GiB"},
		{10, "10 bytes"},
		{(1 << 20) + 1, "1048577 bytes"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.in); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
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

	binary := "valid UTF-8\x00with a NUL"
	_, err := c.Upload(context.Background(), Input{
		Reader: strings.NewReader(binary),
		Name:   "data.bin",
	})
	if !errors.Is(err, ErrBinaryInput) {
		t.Fatalf("err = %v, want ErrBinaryInput", err)
	}
}

// TestUploadTextInput drives the full text pipeline against a fake S3
// endpoint: highlighted page plus original source sibling with the
// real extension.
func TestUploadTextInput(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

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
		DisableManifest: true,
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
	if res.Title != "main.go" {
		t.Errorf("Title = %q, want %q", res.Title, "main.go")
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
	if !strings.Contains(page, `aria-label="Download source"`) {
		t.Error("page missing 'Download source' anchor")
	}
	if !strings.Contains(page, "func") {
		t.Error("page missing source content")
	}
	if !strings.Contains(page,
		`<div class="filehead"><code>main.go</code></div>`) {
		t.Error("page missing filename header bar")
	}

	manifest, err := DefaultManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default manifest was touched: %v", err)
	}
}

func TestRenderTextFileNameHeader(t *testing.T) {
	src := []byte("hello\n")

	out, err := RenderText(src, "notes/hello.txt", RenderOptions{
		Title: "hello.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out),
		`<div class="filehead"><code>hello.txt</code></div>`) {
		t.Error("named input missing filename header")
	}

	out, err = RenderText(src, "", RenderOptions{Title: "stdin"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `<div class="filehead">`) {
		t.Error("stdin input should not render a filename header")
	}
}

// blockingReader blocks forever on Read, simulating a stalled stdin.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) {
	select {} // block until the process exits
}

func TestReadInputHonorsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 20*time.Millisecond,
	)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		_, err = readInput(ctx, blockingReader{}, 10)
		close(done)
	}()

	select {
	case <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readInput did not return after context deadline")
	}
}

func TestHighlightSourceLangOverride(t *testing.T) {
	src := []byte("package main\n")

	out, _, err := highlightSource(src, "", "go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<span class="kn">package</span>`) {
		t.Errorf("--lang go did not highlight: %s", out)
	}

	out, _, err = highlightSource(src, "main.go", "notalanguage")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `class="kn"`) {
		t.Error("unrecognized lang should fall back to plain text, " +
			"not the filename lexer")
	}
}
