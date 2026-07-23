package airplan

import (
	"context"
	"encoding/json"
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
	if result.URL != "https://plans.example/plan" {
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
