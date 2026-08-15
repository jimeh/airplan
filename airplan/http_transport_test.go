package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jimeh/airplan/internal/httpapi"
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

func TestAirplanBackendValidatesManifestInventoryEncoding(t *testing.T) {
	tests := []struct {
		name        string
		inventory   map[string]any
		wantError   bool
		wantObjects int
		wantBytes   int64
	}{
		{name: "omitted inventory"},
		{
			name: "positive inventory",
			inventory: map[string]any{
				"objects": 3, "total_bytes": 42,
			},
			wantObjects: 3, wantBytes: 42,
		},
		{
			name:      "partial inventory",
			inventory: map[string]any{"objects": 3},
			wantError: true,
		},
		{
			name: "explicit zero inventory",
			inventory: map[string]any{
				"objects": 0, "total_bytes": 0,
			},
			wantError: true,
		},
		{
			name: "null inventory",
			inventory: map[string]any{
				"objects": nil, "total_bytes": nil,
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					record := map[string]any{
						"type": "upload", "time": "2026-08-11T00:00:00Z",
						"key": "aaaaaaaaaaaaaaaaaaaaaaaaaa/plan.html",
						"url": "https://plans.example/plan", "bucket": "plans",
						"bytes": 12,
					}
					for name, value := range test.inventory {
						record[name] = value
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"records": []any{record}, "warnings": []string{},
					})
				},
			))
			t.Cleanup(server.Close)
			client, err := New(context.Background(), &Config{
				Backend: BackendAirplan, APIURL: server.URL,
				APIToken:   "01234567890123456789012345678901",
				Repository: "none",
			})
			if err != nil {
				t.Fatal(err)
			}
			listed, err := client.ListManifest(context.Background(),
				ListManifestOptions{Scope: ManifestScopeService})
			if test.wantError {
				if err == nil || !strings.Contains(
					err.Error(), "objects and total_bytes",
				) {
					t.Fatalf("error = %v, want invalid inventory pair", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(listed.Records) != 1 ||
				listed.Records[0].Objects != test.wantObjects ||
				listed.Records[0].TotalBytes != test.wantBytes {
				t.Fatalf("records = %+v, want inventory %d/%d",
					listed.Records, test.wantObjects, test.wantBytes)
			}
		})
	}
}

func TestAirplanBackendMapsProtectedProblemToTypedError(t *testing.T) {
	const token = "01234567890123456789012345678901"
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/uploads/delete" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			var request struct {
				URLOrKey string `json:"url_or_key"`
				Force    bool   `json:"force"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Force {
				t.Fatal("force sent without --force")
			}
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":       "https://airplan.dev/problems/upload-protected",
				"title":      "Upload protected",
				"status":     http.StatusConflict,
				"code":       "upload_protected",
				"request_id": "test",
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
	_, err = client.DeleteUpload(context.Background(), "dir/plan.html")
	var protectedErr *UploadProtectedError
	if !errors.As(err, &protectedErr) ||
		protectedErr.Target != "dir/plan.html" {
		t.Fatalf("error = %v, want UploadProtectedError", err)
	}
}

func TestAirplanBackendForwardsDiffDownloadSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/uploads/get" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			var request httpapi.GetUploadRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.URLOrKey != "dir/plan.html" || !request.Diff || request.Source {
				t.Fatalf("request = %+v", request)
			}
			w.Header().Set("X-Airplan-Object-Key", "dir/"+DiffFilename)
			_, _ = io.WriteString(w, "diff body")
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
	var output bytes.Buffer
	key, err := client.GetUploadTo(context.Background(), "dir/plan.html",
		GetOptions{Diff: true}, &output)
	if err != nil || key != "dir/"+DiffFilename || output.String() != "diff body" {
		t.Fatalf("key = %q, body = %q, error = %v", key, output.String(), err)
	}
}

func TestAirplanBackendDropsInvalidProtectionReasons(t *testing.T) {
	const invalid = "keep\n\x1b[31mPROTECTED no"
	inspection := coreInspection(httpapi.UploadInspection{
		ProtectReason: invalid,
	})
	protection := coreProtectionResult(httpapi.ProtectionResult{
		Reason: invalid,
	})
	record := coreManifestRecord(httpapi.ManifestRecord{
		ProtectReason: invalid,
	})
	if inspection.ProtectReason != "" || protection.Reason != "" ||
		record.ProtectReason != "" {
		t.Fatalf("invalid reasons survived: inspection=%q protection=%q record=%q",
			inspection.ProtectReason, protection.Reason, record.ProtectReason)
	}

	const valid = "README demo link"
	if got := coreInspection(httpapi.UploadInspection{
		ProtectReason: valid,
	}).ProtectReason; got != valid {
		t.Fatalf("valid reason = %q, want %q", got, valid)
	}
}

func TestAirplanBackendNormalizesEmptyPurgeArrays(t *testing.T) {
	const token = "01234567890123456789012345678901"
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/purge/preview" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candidates": []any{}, "protected": []any{},
				"invalid": 0, "warnings": []any{},
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
	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceStorage, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Candidates == nil || plan.Protected == nil {
		t.Fatalf("required arrays are nil: %+v", plan)
	}
}

func TestAirplanBackendPreservesPresentBulkNoOpResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/upgrades" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
  "items": [{
    "plan": {
      "target": "dir/plan.html",
      "state": "current",
      "target_marker_version": 4,
      "target_producer_version": "0.8.0",
      "target_renderer_generation": 1
    },
    "result": {
      "result": {
        "id": "dir",
        "kind": "document",
        "url": "https://plans.example.com/dir/plan.html",
        "key": "dir/plan.html",
        "bucket": "plans",
        "bytes": 0,
        "content_type": "text/html; charset=utf-8",
        "created_at": "2026-08-15T12:00:00Z",
        "marker_version": 4,
        "marker_key": "dir/.airplan.json",
        "warnings": []
      },
      "state": "current",
      "upgraded": false,
      "reason": "already current"
    }
  }],
  "upgraded": 0,
  "failed": 0
}`)
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
	result, err := client.ExecuteBulkUpgrade(context.Background(),
		BulkUpgradeRequest{Items: []UpgradeDocumentPlan{{
			Target: "dir/plan.html", State: UpgradeStateCurrent,
			TargetMarkerVersion:      MarkerVersion,
			TargetProducerVersion:    "0.8.0",
			TargetRendererGeneration: RendererGeneration,
		}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Result == nil ||
		result.Items[0].Result.State != UpgradeStateCurrent ||
		result.Items[0].Result.Reason != "already current" ||
		result.Items[0].Result.Result.URL != "https://plans.example.com/dir/plan.html" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTransportUpgradeConflictPreservesProblemError(t *testing.T) {
	problem := httpapi.NewProblemError(
		http.StatusConflict, "upgrade_conflict", "Upgrade conflict", "stale",
	)
	problem.Problem.RequestID = "request-123"
	err := transportError(problem)
	var got *httpapi.ProblemError
	if !errors.Is(err, ErrConflict) || !errors.As(err, &got) ||
		got.Problem.Status != http.StatusConflict ||
		got.Problem.RequestID != "request-123" {
		t.Fatalf("error = %v, problem = %+v", err, got)
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
