package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func readOpenedPart(t *testing.T, open func() (io.ReadSeekCloser, error)) string {
	t.Helper()
	if open == nil {
		t.Fatal("part has no Open function")
	}
	file, err := open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestServerEnforcesConfiguredFileLimit(t *testing.T) {
	tempDir := t.TempDir()
	collectionCalls := 0
	operations := &stubOperations{
		uploadCollection: func(ctx context.Context, upload CollectionUpload) (UploadResult, error) {
			collectionCalls++
			if limit := ItemLimit(ctx); limit != 3 {
				t.Errorf("operation item limit = %d, want 3", limit)
			}
			for index, file := range upload.Files {
				if file.Reader != nil {
					t.Errorf("collection file %q holds an open reader", file.Name)
				}
				if got := readOpenedPart(t, file.Open); got != fmt.Sprint(index) {
					t.Errorf("collection file %q = %q", file.Name, got)
				}
			}
			return UploadResult{ID: "collection", Warnings: []string{}, Files: []FileResult{}}, nil
		},
	}
	handler := newTestHandler(t, operations, Options{
		TempDir: tempDir, MaxDocumentItems: 3, MaxCollectionFiles: 3,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.DocumentBundle == nil || capabilities.DocumentBundle.MaxItems != 3 {
		t.Fatalf("document bundle capability = %+v", capabilities.DocumentBundle)
	}

	files := func(count int) []CollectionFile {
		result := make([]CollectionFile, count)
		for index := range result {
			body := fmt.Sprint(index)
			result[index] = CollectionFile{
				Name: fmt.Sprintf("file-%d.txt", index), Size: int64(len(body)),
				Reader: strings.NewReader(body),
			}
		}
		return result
	}
	if _, err := client.UploadCollection(context.Background(), CollectionMetadata{}, files(3)); err != nil {
		t.Fatalf("collection at limit: %v", err)
	}
	_, err = client.UploadCollection(context.Background(), CollectionMetadata{}, files(4))
	assertInvalidRequest(t, err)
	if collectionCalls != 1 {
		t.Fatalf("collection operation calls = %d, want 1", collectionCalls)
	}

	document := DocumentUpload{
		Metadata: DocumentMetadata{Name: "plan.md"},
		Document: strings.NewReader("# Plan\n"), DocumentSize: 7,
	}
	for index := range 3 {
		descriptor := DocumentAssetDescriptor{Path: fmt.Sprintf("asset-%d.bin", index), Size: 1}
		document.Metadata.Assets = append(document.Metadata.Assets, descriptor)
		document.Assets = append(document.Assets, DocumentAsset{
			DocumentAssetDescriptor: descriptor, Reader: strings.NewReader("x"),
		})
	}
	// The stub has no document operation, so reaching it would surface as an
	// internal error rather than an invalid request.
	_, err = client.UploadDocumentBundle(context.Background(), document)
	assertInvalidRequest(t, err)
	assertDirEmpty(t, tempDir)
}

func assertInvalidRequest(t *testing.T, err error) {
	t.Helper()
	var problem *ProblemError
	if !errors.As(err, &problem) || problem.Problem.Code != "invalid_request" {
		t.Fatalf("error = %v, want invalid_request problem", err)
	}
}

// A bundle far beyond the former 100-item and 256 KiB metadata limits must
// stream through the typed client and server handler intact.
func TestTypedClientRoundTripsLargeDocumentBundle(t *testing.T) {
	const assets = 2500
	tempDir := t.TempDir()
	path := func(index int) string {
		return fmt.Sprintf("assets/%s/item-%04d.bin", strings.Repeat("d", 120), index)
	}
	operations := &stubOperations{uploadDocument: func(
		ctx context.Context, request DocumentUpload,
	) (UploadResult, error) {
		if limit := ItemLimit(ctx); limit != assets+1 {
			t.Errorf("operation item limit = %d, want %d", limit, assets+1)
		}
		if len(request.Assets) != assets {
			t.Errorf("assets = %d, want %d", len(request.Assets), assets)
			return UploadResult{}, nil
		}
		for index, asset := range request.Assets {
			if asset.Path != path(index) || asset.Reader != nil {
				t.Errorf("asset %d = %+v", index, asset)
				break
			}
			if got := readOpenedPart(t, asset.Open); got != fmt.Sprint(index%10) {
				t.Errorf("asset %q = %q", asset.Path, got)
				break
			}
		}
		return UploadResult{ID: "bundle", Warnings: []string{}, Files: []FileResult{}}, nil
	}}
	handler := newTestHandler(t, operations, Options{
		TempDir: tempDir, MaxDocumentItems: assets + 1,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL)

	upload := DocumentUpload{
		Metadata: DocumentMetadata{Name: "plan.md"},
		Document: strings.NewReader("# Plan\n"), DocumentSize: 7,
	}
	for index := range assets {
		descriptor := DocumentAssetDescriptor{Path: path(index), Size: 1}
		body := fmt.Sprint(index % 10)
		upload.Metadata.Assets = append(upload.Metadata.Assets, descriptor)
		upload.Assets = append(upload.Assets, DocumentAsset{
			DocumentAssetDescriptor: descriptor,
			Open: func() (io.ReadSeekCloser, error) {
				return nopSeekCloser{strings.NewReader(body)}, nil
			},
		})
	}
	encoded, err := json.Marshal(upload.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= 256<<10 {
		t.Fatalf("metadata = %d bytes, want more than the former 256 KiB limit", len(encoded))
	}
	result, err := client.UploadDocumentBundle(context.Background(), upload)
	if err != nil || result.ID != "bundle" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	assertDirEmpty(t, tempDir)
}

type nopSeekCloser struct{ io.ReadSeeker }

func (nopSeekCloser) Close() error { return nil }
