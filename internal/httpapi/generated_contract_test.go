package httpapi

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/jimeh/airplan/internal/httpapi/generated"
)

func TestGeneratedClientInvokesEveryGeneratedServerOperation(t *testing.T) {
	operations := &stubOperations{
		uploadDocument: func(
			_ context.Context, upload DocumentUpload,
		) (UploadResult, error) {
			body, err := io.ReadAll(upload.Document)
			if err != nil {
				t.Fatal(err)
			}
			return UploadResult{
				ID: "doc", Kind: "document", URL: "https://example/doc",
				Key: "doc/plan.html", Bucket: "plans", Bytes: int64(len(body)),
				ContentType: "text/html", Warnings: []string{},
			}, nil
		},
		uploadCollection: func(
			_ context.Context, upload CollectionUpload,
		) (UploadResult, error) {
			return UploadResult{
				ID: "collection", Kind: "collection",
				URL: "https://example/collection", Key: "collection/index.html",
				Bucket: "plans", ContentType: "text/html", Warnings: []string{},
			}, nil
		},
		getUpload: func(
			context.Context, GetUploadRequest,
		) (Download, error) {
			return Download{
				Body: io.NopCloser(strings.NewReader("download")),
				Key:  "doc/plan.html", Filename: "plan.html",
				ContentType: "text/html",
			}, nil
		},
	}
	handler := newTestHandler(t, operations, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := generated.NewClientWithResponses(
		server.URL,
		generated.WithHTTPClient(server.Client()),
		generated.WithRequestEditorFn(func(
			_ context.Context, request *http.Request,
		) error {
			request.Header.Set("Authorization", "Bearer "+testToken)
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	health, err := client.HealthWithResponse(ctx)
	assertGeneratedResponse(t, health, err)
	openAPI, err := client.GetOpenAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = openAPI.Body.Close()
	if openAPI.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI status = %d", openAPI.StatusCode)
	}
	capabilities, err := client.GetCapabilitiesWithResponse(ctx)
	assertGeneratedResponse(t, capabilities, err)

	documentBody, documentType := generatedMultipartBody(t, func(
		writer *multipart.Writer,
	) {
		writeGeneratedPart(t, writer, "metadata", "", "application/json",
			`{"name":"plan.md","format":"md"}`)
		writeGeneratedPart(t, writer, "document", "plan.md",
			"text/markdown", "# Plan")
	})
	document, err := client.UploadDocumentWithBodyWithResponse(
		ctx, documentType, documentBody,
	)
	assertGeneratedResponse(t, document, err)

	collectionBody, collectionType := generatedMultipartBody(t, func(
		writer *multipart.Writer,
	) {
		writeGeneratedPart(t, writer, "metadata", "", "application/json", `{}`)
		writeGeneratedPart(t, writer, "files", "plan.md",
			"text/markdown", "# Plan")
	})
	collection, err := client.UploadCollectionWithBodyWithResponse(
		ctx, collectionType, collectionBody,
	)
	assertGeneratedResponse(t, collection, err)

	target := generated.TargetRequest{URLOrKey: "doc/plan.html"}
	inspection, err := client.InspectUploadWithResponse(ctx, target)
	assertGeneratedResponse(t, inspection, err)
	download, err := client.GetUploadWithResponse(
		ctx, generated.GetUploadRequest{URLOrKey: target.URLOrKey},
	)
	assertGeneratedResponse(t, download, err)
	deleted, err := client.DeleteUploadWithResponse(ctx, target)
	assertGeneratedResponse(t, deleted, err)
	manifest, err := client.ListManifestUploadsWithResponse(ctx)
	assertGeneratedResponse(t, manifest, err)
	storage, err := client.ListStorageUploadsWithResponse(ctx)
	assertGeneratedResponse(t, storage, err)
	syncResult, err := client.SyncManifestWithResponse(
		ctx, generated.SyncRequest{DryRun: true},
	)
	assertGeneratedResponse(t, syncResult, err)
	preview, err := client.PreviewPurgeWithResponse(
		ctx, generated.PurgePreviewRequest{Source: "manifest", All: true},
	)
	assertGeneratedResponse(t, preview, err)
	purged, err := client.ExecutePurgeWithResponse(
		ctx, generated.PurgeRequest{UploadIds: []string{"upload-id"}},
	)
	assertGeneratedResponse(t, purged, err)
}

func TestGeneratedRequestValidationRejectsSchemaViolations(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})
	for _, test := range []struct {
		name string
		path string
		body string
	}{
		{
			name: "unknown property", path: "/api/v1/sync",
			body: `{"unknown":true}`,
		},
		{
			name: "concurrency above maximum", path: "/api/v1/sync",
			body: `{"concurrency":65}`,
		},
		{
			name: "invalid purge source", path: "/api/v1/purge/preview",
			body: `{"source":"elsewhere","all":true}`,
		},
		{
			name: "empty purge IDs", path: "/api/v1/purge",
			body: `{"upload_ids":[]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := authorizedRequest(
				http.MethodPost, test.path, strings.NewReader(test.body),
			)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if problem := decodeProblem(t, recorder); problem.Code != "invalid_request" {
				t.Fatalf("problem = %+v", problem)
			}
		})
	}
}

func TestMultipartAdapterEnforcesSchemaParityWithoutBuffering(t *testing.T) {
	handler := newTestHandler(t, &stubOperations{}, Options{})

	t.Run("media type", func(t *testing.T) {
		request := authorizedRequest(
			http.MethodPost,
			"/api/v1/uploads/documents",
			strings.NewReader(`{"name":"plan.md"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("method", func(t *testing.T) {
		request := authorizedRequest(
			http.MethodGet, "/api/v1/uploads/documents", nil,
		)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code < 400 || recorder.Code >= 500 {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	for _, test := range []struct {
		name     string
		metadata string
	}{
		{name: "format enum", metadata: `{"name":"plan.md","format":"pdf"}`},
		{name: "unknown metadata", metadata: `{"name":"plan.md","extra":true}`},
		{name: "negative size", metadata: `{"name":"plan.md","max_size":-1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, contentType := generatedMultipartBody(t, func(
				writer *multipart.Writer,
			) {
				writeGeneratedPart(t, writer, "metadata", "",
					"application/json", test.metadata)
				writeGeneratedPart(t, writer, "document", "plan.md",
					"text/markdown", "# Plan")
			})
			request := authorizedRequest(
				http.MethodPost, "/api/v1/uploads/documents", body,
			)
			request.Header.Set("Content-Type", contentType)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}
}

func assertGeneratedResponse[T interface{ StatusCode() int }](
	t *testing.T, response T, err error,
) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode() < 200 || response.StatusCode() >= 300 {
		t.Fatalf("generated response status = %d", response.StatusCode())
	}
}

func generatedMultipartBody(
	t *testing.T, write func(*multipart.Writer),
) (io.Reader, string) {
	t.Helper()
	reader, pipe := io.Pipe()
	writer := multipart.NewWriter(pipe)
	contentType := writer.FormDataContentType()
	go func() {
		write(writer)
		_ = writer.Close()
		_ = pipe.Close()
	}()
	return reader, contentType
}

func writeGeneratedPart(
	t *testing.T, writer *multipart.Writer,
	name, filename, contentType, body string,
) {
	t.Helper()
	header := make(textproto.MIMEHeader)
	parameters := map[string]string{"name": name}
	if filename != "" {
		parameters["filename"] = filename
	}
	header.Set("Content-Disposition", mime.FormatMediaType(
		"form-data", parameters,
	))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Error(err)
		return
	}
	if _, err = io.WriteString(part, body); err != nil {
		t.Error(err)
	}
}
