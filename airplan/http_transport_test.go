package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAirplanBackendUsesHTTPWithoutS3Credentials(t *testing.T) {
	const token = "01234567890123456789012345678901"
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if r.URL.Path != "/api/v1/uploads" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(ManifestList{
				Records: []ManifestRecord{},
			})
		},
	))
	defer server.Close()

	client, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: token,
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ListManifest(context.Background(),
		ListManifestOptions{Scope: ManifestScopeService})
	if err != nil {
		t.Fatal(err)
	}
	if result.Records == nil || len(result.Records) != 0 {
		t.Fatalf("records = %#v", result.Records)
	}
}

func TestAirplanBackendStreamsDocumentMultipart(t *testing.T) {
	const token = "01234567890123456789012345678901"
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(DefaultMaxInputSize + 1<<20); err != nil {
				t.Fatal(err)
			}
			var metadata map[string]json.RawMessage
			if err := json.Unmarshal(
				[]byte(r.FormValue("metadata")), &metadata,
			); err != nil {
				t.Fatal(err)
			}
			if _, exists := metadata["max_size"]; exists {
				t.Fatalf("metadata = %s, want no unlimited lower bound",
					r.FormValue("metadata"))
			}
			file, _, err := r.FormFile("document")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			body := make([]byte, 5)
			if _, err := file.Read(body); err != nil {
				t.Fatal(err)
			}
			if string(body) != "hello" {
				t.Fatalf("document = %q", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Result{
				ID: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "document",
				URL: "https://plans.example/plan", Key: "key",
				MarkerKey: "aaaaaaaaaaaaaaaaaaaaaaaaaa/.airplan.json",
				Format:    "md", Slug: "plan",
			})
		},
	))
	defer server.Close()
	client, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: token,
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("hello"), Name: "plan.md",
		MaxSize: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://plans.example/plan" ||
		result.MarkerKey != "aaaaaaaaaaaaaaaaaaaaaaaaaa/.airplan.json" ||
		result.Format != "md" || result.Slug != "plan" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPortableUploadLimit(t *testing.T) {
	for _, test := range []struct {
		input int64
		want  int64
	}{
		{input: -1, want: 0},
		{input: 0, want: 0},
		{input: 42, want: 42},
	} {
		if got := portableUploadLimit(test.input); got != test.want {
			t.Errorf("portableUploadLimit(%d) = %d, want %d",
				test.input, got, test.want)
		}
	}
}

func TestAirplanBackendPreflightsCollectionFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			file, header, err := r.FormFile("files")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			if header.Filename != "shot.png" ||
				header.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("file header = %+v", header)
			}
			var metadata map[string]any
			if err := json.Unmarshal(
				[]byte(r.FormValue("metadata")), &metadata,
			); err != nil {
				t.Fatal(err)
			}
			if metadata["title"] != "shot.png" {
				t.Fatalf("metadata = %+v", metadata)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Result{
				ID: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "collection",
				URL: "https://plans.example/collection", Key: "index.html",
			})
		},
	))
	t.Cleanup(server.Close)
	client, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL,
		APIToken: "01234567890123456789012345678901", Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.UploadFiles(context.Background(), FilesInput{
		Files: []FileInput{{
			Name:   "screenshots/shot.png",
			Reader: bytes.NewReader([]byte("png")), Size: 3,
		}},
	})
	if err != nil || result.URL != "https://plans.example/collection" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestAirplanBackendRejectsTruncatedCollectionFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
		},
	))
	t.Cleanup(server.Close)
	client, err := New(context.Background(), &Config{
		Backend: BackendAirplan, APIURL: server.URL,
		APIToken: "01234567890123456789012345678901", Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.UploadFiles(context.Background(), FilesInput{
		Files: []FileInput{{
			Name: "short.bin", Reader: bytes.NewReader([]byte("short")), Size: 10,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error = %v, want truncated input", err)
	}
}
