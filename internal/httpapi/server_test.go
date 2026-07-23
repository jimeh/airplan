package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testToken = "01234567890123456789012345678901"

type stubOperations struct {
	uploadDocument   func(context.Context, DocumentUpload) (UploadResult, error)
	uploadCollection func(context.Context, CollectionUpload) (UploadResult, error)
	getUpload        func(context.Context, GetUploadRequest) (Download, error)
	capabilitiesErr  error
}

func (s *stubOperations) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{
		APIVersion:     "v1",
		ServerVersion:  "test",
		Operations:     []string{"upload_document"},
		UploadFormats:  []CapabilitiesUploadFormats{"md"},
		Limits:         UploadLimits{DocumentBytes: 1024},
		MarkerVersions: []int{3},
	}, s.capabilitiesErr
}

func (s *stubOperations) UploadDocument(
	ctx context.Context,
	request DocumentUpload,
) (UploadResult, error) {
	if s.uploadDocument == nil {
		return UploadResult{}, errors.New("unexpected UploadDocument call")
	}
	return s.uploadDocument(ctx, request)
}

func (s *stubOperations) UploadCollection(
	ctx context.Context,
	request CollectionUpload,
) (UploadResult, error) {
	if s.uploadCollection == nil {
		return UploadResult{}, errors.New("unexpected UploadCollection call")
	}
	return s.uploadCollection(ctx, request)
}

func (s *stubOperations) InspectUpload(
	context.Context,
	TargetRequest,
) (UploadInspection, error) {
	return UploadInspection{State: "complete", ID: "id"}, nil
}

func (s *stubOperations) GetUpload(
	ctx context.Context,
	request GetUploadRequest,
) (Download, error) {
	if s.getUpload == nil {
		return Download{}, errors.New("unexpected GetUpload call")
	}
	return s.getUpload(ctx, request)
}

func (s *stubOperations) DeleteUpload(
	context.Context,
	TargetRequest,
) (DeleteResult, error) {
	return DeleteResult{ID: "id"}, nil
}

func (s *stubOperations) ListManifestUploads(context.Context) (ManifestList, error) {
	return ManifestList{}, nil
}

func (s *stubOperations) ListStorageUploads(context.Context) (StorageList, error) {
	return StorageList{}, nil
}

func (s *stubOperations) SyncManifest(
	context.Context,
	SyncRequest,
) (SyncResult, error) {
	return SyncResult{}, nil
}

func (s *stubOperations) PreviewPurge(
	context.Context,
	PurgePreviewRequest,
) (PurgePreview, error) {
	return PurgePreview{}, nil
}

func (s *stubOperations) ExecutePurge(
	context.Context,
	PurgeRequest,
) (PurgeResult, error) {
	return PurgeResult{}, nil
}

func TestOpenAPIIsPublicAndByteExact(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	request := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	want, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !bytes.Equal(recorder.Body.Bytes(), want) {
		t.Fatal("served schema differs from source bytes")
	}
}

func TestBearerAuthRejectsBeforeReadingBody(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	body := &recordingBody{Reader: strings.NewReader("secret body")}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/uploads/documents",
		body,
	)
	request.Body = body
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if body.reads != 0 {
		t.Fatalf("unauthorized request body read %d times", body.reads)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
	}
	problem := decodeProblem(t, recorder)
	if problem.Code != "unauthorized" || problem.RequestID == "" {
		t.Fatalf("unexpected problem: %+v", problem)
	}
}

func TestBearerAuthRejectsDuplicateAndMalformedValues(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	tests := []struct {
		name   string
		values []string
	}{
		{name: "wrong scheme", values: []string{"Basic abc"}},
		{name: "extra whitespace", values: []string{"Bearer  abc"}},
		{name: "wrong token", values: []string{"Bearer wrong"}},
		{name: "duplicate", values: []string{"Bearer " + testToken, "Bearer " + testToken}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodGet,
				"/api/v1/capabilities",
				nil,
			)
			for _, value := range test.values {
				request.Header.Add("Authorization", value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", recorder.Code)
			}
		})
	}
}

func TestRequestIDAcceptsOnlyConstrainedValues(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	for _, test := range []struct {
		incoming string
		wantSame bool
	}{
		{incoming: "agent.request-42_ok", wantSame: true},
		{incoming: "bad request id", wantSame: false},
		{incoming: strings.Repeat("a", 65), wantSame: false},
	} {
		request := authorizedRequest(
			http.MethodGet,
			"/api/v1/capabilities",
			nil,
		)
		request.Header.Set(requestIDHeader, test.incoming)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		got := recorder.Header().Get(requestIDHeader)
		if got == "" {
			t.Fatal("response request ID is empty")
		}
		if (got == test.incoming) != test.wantSame {
			t.Fatalf("response request ID = %q for incoming %q", got, test.incoming)
		}
	}
}

func TestUnknownErrorsAreRedacted(t *testing.T) {
	operations := &stubOperations{
		capabilitiesErr: errors.New("s3 secret response and /private/path"),
	}
	handler := newTestHandler(t, operations, Options{})
	request := authorizedRequest(
		http.MethodGet,
		"/api/v1/capabilities",
		nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	problem := decodeProblem(t, recorder)
	if problem.Code != "internal_server_error" {
		t.Fatalf("problem code = %q", problem.Code)
	}
	if strings.Contains(recorder.Body.String(), "secret") ||
		strings.Contains(recorder.Body.String(), "/private") {
		t.Fatalf("internal detail leaked: %s", recorder.Body.String())
	}
}

func TestTypedClientStreamsDocumentAndCleansTempFile(t *testing.T) {
	tempDir := t.TempDir()
	operations := &stubOperations{}
	operations.uploadDocument = func(
		_ context.Context,
		request DocumentUpload,
	) (UploadResult, error) {
		file, ok := request.Document.(*os.File)
		if !ok {
			t.Fatalf("document reader type = %T, want *os.File", request.Document)
		}
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("temp mode = %o, want 600", info.Mode().Perm())
		}
		body, err := io.ReadAll(request.Document)
		if err != nil {
			t.Fatal(err)
		}
		if request.Metadata.Name != "plan.md" || string(body) != "# Plan\n" {
			t.Fatalf("unexpected upload: %+v %q", request.Metadata, body)
		}
		return UploadResult{
			ID:       "upload-id",
			Kind:     "document",
			URL:      "https://example.test/upload-id/plan.html",
			Warnings: []string{},
			Files:    []FileResult{},
		}, nil
	}
	handler := newTestHandler(t, operations, Options{TempDir: tempDir})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	result, err := client.UploadDocument(
		context.Background(),
		DocumentMetadata{Name: "plan.md", Format: "md"},
		strings.NewReader("# Plan\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "upload-id" {
		t.Fatalf("upload ID = %q", result.ID)
	}
	assertDirEmpty(t, tempDir)
}

func TestTypedClientAllowsUnnamedStdinDocument(t *testing.T) {
	operations := &stubOperations{}
	operations.uploadDocument = func(
		_ context.Context, request DocumentUpload,
	) (UploadResult, error) {
		if request.Metadata.Name != "" {
			t.Fatalf("name = %q, want empty stdin name", request.Metadata.Name)
		}
		return UploadResult{
			ID: "upload-id", Kind: UploadResultKind("document"),
			URL:      "https://example.test/upload-id/plan.html",
			Warnings: []string{}, Files: []FileResult{},
		}, nil
	}
	handler := newTestHandler(t, operations, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)
	if _, err := client.UploadDocument(
		context.Background(), DocumentMetadata{}, strings.NewReader("# Plan\n"),
	); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentClientLimitRejectsBeforeOperation(t *testing.T) {
	tempDir := t.TempDir()
	called := false
	operations := &stubOperations{
		uploadDocument: func(
			context.Context,
			DocumentUpload,
		) (UploadResult, error) {
			called = true
			return UploadResult{}, nil
		},
	}
	handler := newTestHandler(t, operations, Options{
		TempDir:          tempDir,
		MaxDocumentBytes: 64,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	_, err := client.UploadDocument(
		context.Background(),
		DocumentMetadata{Name: "plan.md", MaxSize: 3},
		strings.NewReader("four"),
	)
	var problem *ProblemError
	if !errors.As(err, &problem) || problem.Problem.Status != 413 {
		t.Fatalf("error = %v, want 413 problem", err)
	}
	if called {
		t.Fatal("operation called for oversized document")
	}
	assertDirEmpty(t, tempDir)
}

func TestCollectionRejectsDuplicateBasenamesAndCleansTempFiles(t *testing.T) {
	tempDir := t.TempDir()
	handler := newTestHandler(t, &stubOperations{}, Options{TempDir: tempDir})
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	if err := writeMetadataPart(writer, CollectionMetadata{}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="files"; filename="same.txt"`)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(part, "content")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := authorizedRequest(
		http.MethodPost,
		"/api/v1/uploads/collections",
		body,
	)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	assertDirEmpty(t, tempDir)
}

func TestTypedClientStreamsDownload(t *testing.T) {
	operations := &stubOperations{
		getUpload: func(
			_ context.Context,
			request GetUploadRequest,
		) (Download, error) {
			if request.URLOrKey != "upload-id" || request.Source {
				t.Fatalf("unexpected get request: %+v", request)
			}
			return Download{
				Body:        io.NopCloser(strings.NewReader("download body")),
				Key:         "prefix/upload-id/plan.html",
				Filename:    "plan.html",
				ContentType: "text/html; charset=utf-8",
			}, nil
		},
	}
	handler := newTestHandler(t, operations, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	download, err := client.GetUpload(
		context.Background(),
		GetUploadRequest{URLOrKey: "upload-id"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = download.Body.Close() }()
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "download body" || download.Filename != "plan.html" ||
		download.ContentType != "text/html; charset=utf-8" {
		t.Fatalf("unexpected download: %+v %q", download, body)
	}
}

func TestOriginGuard(t *testing.T) {
	guard, err := NewOriginGuard([]string{"https://agent.example"})
	if err != nil {
		t.Fatal(err)
	}
	handler := requestIDMiddleware(guard.Wrap(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	)))
	for _, test := range []struct {
		name   string
		origin string
		status int
	}{
		{name: "absent", status: http.StatusNoContent},
		{name: "allowed", origin: "https://agent.example", status: http.StatusNoContent},
		{name: "not allowed", origin: "https://other.example", status: http.StatusForbidden},
		{name: "malformed", origin: "https://agent.example/path", status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestUnknownJSONFieldsAreRejected(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	request := authorizedRequest(
		http.MethodPost,
		"/api/v1/uploads/inspect",
		strings.NewReader(`{"url_or_key":"id","future":true}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

type recordingBody struct {
	io.Reader
	reads int
}

func (r *recordingBody) Read(buffer []byte) (int, error) {
	r.reads++
	return r.Reader.Read(buffer)
}

func (r *recordingBody) Close() error { return nil }

func newTestHandler(
	t *testing.T,
	operations Operations,
	options Options,
) http.Handler {
	t.Helper()
	options.Token = testToken
	handler, err := NewHandler(operations, options)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(baseURL, testToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func authorizedRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Authorization", "Bearer "+testToken)
	return request
}

func decodeProblem(t *testing.T, recorder *httptest.ResponseRecorder) Problem {
	t.Helper()
	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	return problem
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary directory contains %v", entries)
	}
}
