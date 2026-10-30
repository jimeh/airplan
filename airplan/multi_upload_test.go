package airplan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

// recordingS3 accepts every request and records successful PUT keys in
// arrival order. A PUT whose key ends with failSuffix is rejected with a
// non-retryable status.
type recordingS3 struct {
	mu         sync.Mutex
	requests   int
	puts       []string
	failSuffix string
	server     *httptest.Server
}

func newRecordingS3(t *testing.T, failSuffix string) *recordingS3 {
	t.Helper()
	store := &recordingS3{failSuffix: failSuffix}
	store.server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			key := strings.TrimPrefix(r.URL.Path, "/plans/")
			store.mu.Lock()
			store.requests++
			fail := r.Method == http.MethodPut && failSuffix != "" &&
				strings.HasSuffix(key, failSuffix)
			if r.Method == http.MethodPut && !fail {
				store.puts = append(store.puts, key)
			}
			store.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(store.server.Close)
	return store
}

func (s *recordingS3) client(t *testing.T) *Client {
	t.Helper()
	client, err := New(context.Background(), &Config{
		Endpoint: s.server.URL, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		DisableManifest: true, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (s *recordingS3) snapshot() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, append([]string(nil), s.puts...)
}

func hasKeySuffix(keys []string, suffix string) bool {
	for _, key := range keys {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

// openTracker counts simultaneously open inputs. The first read of every
// opened input waits until at least two inputs are open at once, so a passing
// test proves that preflight overlaps rather than relying on scheduling luck.
type openTracker struct {
	t       *testing.T
	mu      sync.Mutex
	open    int
	peak    int
	opens   int
	overlap chan struct{}
	once    sync.Once
}

func newOpenTracker(t *testing.T) *openTracker {
	return &openTracker{t: t, overlap: make(chan struct{})}
}

func (o *openTracker) opener(body []byte) func() (io.ReadSeekCloser, error) {
	return func() (io.ReadSeekCloser, error) {
		o.mu.Lock()
		o.open++
		o.opens++
		o.peak = max(o.peak, o.open)
		if o.open >= 2 {
			o.once.Do(func() { close(o.overlap) })
		}
		o.mu.Unlock()
		return &trackedInput{Reader: bytes.NewReader(body), tracker: o}, nil
	}
}

func (o *openTracker) stats() (open, peak, opens int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.open, o.peak, o.opens
}

type trackedInput struct {
	*bytes.Reader
	tracker *openTracker
	waited  bool
	closed  bool
}

func (r *trackedInput) Read(buffer []byte) (int, error) {
	if !r.waited {
		r.waited = true
		select {
		case <-r.tracker.overlap:
		case <-time.After(5 * time.Second):
			r.tracker.t.Error("inputs never overlapped: preflight is not concurrent")
		}
	}
	return r.Reader.Read(buffer)
}

func (r *trackedInput) Close() error {
	if !r.closed {
		r.closed = true
		r.tracker.mu.Lock()
		r.tracker.open--
		r.tracker.mu.Unlock()
	}
	return nil
}

func TestUploadFilesBoundsOpenInputsAndKeepsBarriers(t *testing.T) {
	store := newRecordingS3(t, "")
	tracker := newOpenTracker(t)
	const count = 5 * uploadConcurrency
	files := make([]FileInput, count)
	for index := range files {
		body := []byte(fmt.Sprintf("member %d", index))
		files[index] = FileInput{
			Name: fmt.Sprintf("member-%02d.txt", index),
			Open: tracker.opener(body), Size: int64(len(body)),
		}
	}
	result, err := store.client(t).UploadFiles(context.Background(), FilesInput{Files: files})
	if err != nil {
		t.Fatal(err)
	}

	open, peak, opens := tracker.stats()
	if peak > uploadConcurrency || peak < 2 {
		t.Fatalf("peak open inputs = %d, want 2..%d", peak, uploadConcurrency)
	}
	if open != 0 {
		t.Fatalf("inputs still open = %d", open)
	}
	// Each member is opened once for preflight and once for upload.
	if opens != 2*count {
		t.Fatalf("opens = %d, want %d", opens, 2*count)
	}
	for index, file := range result.Files {
		if file.Name != files[index].Name {
			t.Fatalf("result[%d] = %q, want %q", index, file.Name, files[index].Name)
		}
	}
	_, puts := store.snapshot()
	if len(puts) != count+2 ||
		!strings.HasSuffix(puts[0], "/"+CollectionMarkerFilename) ||
		!strings.HasSuffix(puts[len(puts)-1], "/index.html") {
		t.Fatalf("PUT order = %q, want marker first and overview last", puts)
	}
}

func TestUploadFilesMemberFailurePreventsOverview(t *testing.T) {
	store := newRecordingS3(t, "/member-07.txt")
	files := make([]FileInput, 30)
	for index := range files {
		body := []byte(fmt.Sprintf("member %d", index))
		files[index] = FileInput{
			Name:   fmt.Sprintf("member-%02d.txt", index),
			Reader: bytes.NewReader(body), Size: int64(len(body)),
		}
	}
	_, err := store.client(t).UploadFiles(context.Background(), FilesInput{Files: files})
	if err == nil {
		t.Fatal("expected member upload failure")
	}
	_, puts := store.snapshot()
	if len(puts) == 0 || !strings.HasSuffix(puts[0], "/"+CollectionMarkerFilename) {
		t.Fatalf("PUT order = %q, want marker first", puts)
	}
	if hasKeySuffix(puts, "/index.html") {
		t.Fatalf("overview was published after a member failed: %q", puts)
	}
}

func concurrentTestDocument() DocumentInput {
	in := DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("# Entry\n"), Path: "entry.md"},
		RepositoryURL: "none",
	}
	for index := range 3 {
		in.Pages = append(in.Pages, PageInput{
			Reader: strings.NewReader(fmt.Sprintf("# Page %d\n", index)),
			Path:   fmt.Sprintf("docs/page-%d.md", index),
		})
	}
	for index := range 20 {
		body := []byte(fmt.Sprintf("asset %d", index))
		in.Assets = append(in.Assets, AssetInput{
			Reader: bytes.NewReader(body), Size: int64(len(body)),
			Path: fmt.Sprintf("assets/asset-%02d.bin", index),
		})
	}
	return in
}

func TestUploadDocumentKeepsBarriersAndDeclaredOrder(t *testing.T) {
	store := newRecordingS3(t, "")
	in := concurrentTestDocument()
	result, err := store.client(t).UploadDocument(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	_, puts := store.snapshot()
	// Marker, entry page plus three managed pages with sources, and assets.
	if want := 1 + 4*2 + len(in.Assets); len(puts) != want {
		t.Fatalf("PUTs = %d, want %d: %q", len(puts), want, puts)
	}
	if !strings.HasSuffix(puts[0], "/"+MarkerFilename) ||
		!strings.HasSuffix(puts[len(puts)-1], "/entry.html") {
		t.Fatalf("PUT order = %q, want marker first and entry last", puts)
	}
	wantPages := []string{"entry.md", "docs/page-0.md", "docs/page-1.md", "docs/page-2.md"}
	if len(result.Pages) != len(wantPages) {
		t.Fatalf("pages = %+v", result.Pages)
	}
	for index, page := range result.Pages {
		if page.Path != wantPages[index] {
			t.Fatalf("page[%d] = %q, want %q", index, page.Path, wantPages[index])
		}
	}
	if len(result.Assets) != len(in.Assets) {
		t.Fatalf("assets = %+v", result.Assets)
	}
	for index, asset := range result.Assets {
		if asset.Path != in.Assets[index].Path {
			t.Fatalf("asset[%d] = %q, want %q", index, asset.Path, in.Assets[index].Path)
		}
	}
}

func TestUploadDocumentPayloadFailurePreventsEntry(t *testing.T) {
	store := newRecordingS3(t, "/assets/asset-05.bin")
	_, err := store.client(t).UploadDocument(
		context.Background(), concurrentTestDocument(),
	)
	if err == nil {
		t.Fatal("expected asset upload failure")
	}
	_, puts := store.snapshot()
	if len(puts) == 0 || !strings.HasSuffix(puts[0], "/"+MarkerFilename) {
		t.Fatalf("PUT order = %q, want marker first", puts)
	}
	if hasKeySuffix(puts, "/entry.html") {
		t.Fatalf("entry was published after a payload failed: %q", puts)
	}
}

func emptyCollection(count int) []FileInput {
	files := make([]FileInput, count)
	for index := range files {
		files[index] = FileInput{
			Name:   fmt.Sprintf("file-%05d.txt", index),
			Reader: bytes.NewReader(nil),
		}
	}
	return files
}

func emptyAssetDocument(assets int) DocumentInput {
	in := DocumentInput{
		Entry:         PageInput{Reader: strings.NewReader("# Entry\n"), Path: "entry.md"},
		RepositoryURL: "none",
	}
	for index := range assets {
		in.Assets = append(in.Assets, AssetInput{
			Reader: bytes.NewReader(nil),
			Path:   fmt.Sprintf("assets/file-%05d.bin", index),
		})
	}
	return in
}

func TestFileCountLimitDefaultsAndOverrides(t *testing.T) {
	ctx := context.Background()
	collection := func(count, maxFiles int) error {
		_, _, err := RenderCollection(ctx,
			FilesInput{Files: emptyCollection(count), MaxFiles: maxFiles},
			CollectionRenderOptions{Repository: "none"})
		return err
	}
	document := func(assets, maxFiles int) error {
		in := emptyAssetDocument(assets)
		in.MaxFiles = maxFiles
		_, err := RenderDocument(ctx, in, DocumentRenderOptions{
			RenderInputOptions: RenderInputOptions{Repository: "none"},
		})
		return err
	}
	for _, tt := range []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{"collection at default", func() error { return collection(DefaultMaxFiles, 0) }, ""},
		{"collection over default", func() error { return collection(DefaultMaxFiles+1, 0) }, "collection has 1001 files; maximum is 1000"},
		{"collection unlimited", func() error { return collection(DefaultMaxFiles+1, -1) }, ""},
		{"collection custom", func() error { return collection(6, 5) }, "maximum is 5"},
		// The entry counts as one document item.
		{"document at default", func() error { return document(DefaultMaxFiles-1, 0) }, ""},
		{"document over default", func() error { return document(DefaultMaxFiles, 0) }, "document has 1001 items; maximum is 1000"},
		{"document unlimited", func() error { return document(DefaultMaxFiles, -1) }, ""},
		{"document custom", func() error { return document(5, 5) }, "maximum is 5"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestUploadFilesRejectsOversizedMarkerBeforeAnyRequest(t *testing.T) {
	store := newRecordingS3(t, "")
	// Each declared member costs roughly 400 marker bytes, so this inventory
	// exceeds the 4 MiB marker limit while every name stays valid.
	const count = 12000
	prefix := strings.Repeat("n", 200)
	files := make([]FileInput, count)
	for index := range files {
		files[index] = FileInput{
			Name:   fmt.Sprintf("%s-%05d.txt", prefix, index),
			Reader: bytes.NewReader(nil),
		}
	}
	_, err := store.client(t).UploadFiles(context.Background(), FilesInput{
		Files: files, MaxFiles: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "4 MiB marker limit") ||
		!strings.Contains(err.Error(), fmt.Sprintf("%d objects", count+1)) {
		t.Fatalf("error = %v, want marker limit with object count", err)
	}
	if code, ok := MarkerCode(err); !ok || code != MarkerErrorOversized {
		t.Fatalf("marker code = %q, %t", code, ok)
	}
	if requests, _ := store.snapshot(); requests != 0 {
		t.Fatalf("storage requests = %d, want none", requests)
	}
}

func TestLocalFileOpenRejectsChangesAfterPreflight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "member.txt")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := StatLocalFile(path, "collection input")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := file.Open()
	if err != nil {
		t.Fatalf("open unchanged file: %v", err)
	}
	_ = opened.Close()

	t.Run("replaced", func(t *testing.T) {
		replacement := filepath.Join(dir, "replacement.txt")
		if err := os.WriteFile(replacement, []byte("before"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Open(); err == nil ||
			!strings.Contains(err.Error(), "changed after preflight") {
			t.Fatalf("error = %v, want changed after preflight", err)
		}
	})

	t.Run("resized", func(t *testing.T) {
		resizedPath := filepath.Join(dir, "resized.txt")
		if err := os.WriteFile(resizedPath, []byte("before"), 0o600); err != nil {
			t.Fatal(err)
		}
		resized, err := StatLocalFile(resizedPath, "bundle asset")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resizedPath, []byte("before and after"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := resized.Open(); err == nil ||
			!strings.Contains(err.Error(), "changed after preflight") {
			t.Fatalf("error = %v, want changed after preflight", err)
		}
	})
}

func TestHTTPMetadataLimitMatchesMarkerLimit(t *testing.T) {
	if httpapi.MaxMetadataBytes != MaxMarkerSize {
		t.Fatalf("metadata limit = %d, marker limit = %d",
			httpapi.MaxMetadataBytes, MaxMarkerSize)
	}
}

// A client must never reject an upload that the server's request-body
// allowance admits: both scale their multipart overhead with part count.
func TestValidateBundleCapabilityAdmitsLargeItemCounts(t *testing.T) {
	const maxItems = 3000
	capability := httpapi.DocumentBundleCapabilities{
		ManagedPages: true, Assets: true, MaxItems: maxItems,
		MaxPageBytes: DefaultMaxInputSize, MaxTotalPageBytes: DefaultMaxTotalPageSize,
		MaxGeneratedPageBytes: DefaultMaxGeneratedPageSize,
		MaxAssetBytes:         DefaultMaxAssetSize,
		MaxTotalAssetBytes:    DefaultMaxDocumentAssetTotalSize,
		MaxMetadataBytes:      httpapi.MaxMetadataBytes,
		MaxRequestBytes: DefaultMaxTotalPageSize + DefaultMaxDocumentAssetTotalSize +
			httpapi.RequestBodyOverhead(maxItems),
	}
	upload := httpapi.DocumentUpload{
		Metadata: httpapi.DocumentMetadata{Name: "plan.md"}, DocumentSize: 1,
	}
	for index := range maxItems - 1 {
		descriptor := httpapi.DocumentAssetDescriptor{
			Path: fmt.Sprintf("assets/%s-%04d.bin", strings.Repeat("a", 100), index),
		}
		upload.Metadata.Assets = append(upload.Metadata.Assets, descriptor)
		upload.Assets = append(upload.Assets, httpapi.DocumentAsset{
			DocumentAssetDescriptor: descriptor, Reader: bytes.NewReader(nil),
		})
	}
	if err := validateBundleCapability(capability, upload); err != nil {
		t.Fatalf("%d-item upload rejected: %v", maxItems, err)
	}
}
