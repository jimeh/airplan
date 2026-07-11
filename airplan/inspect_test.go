package airplan

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestInspectUploadStates(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	createdAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	markerBody, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: createdAt, Format: "md", Page: "plan.html",
		Source: "plan.md", Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		body      []byte
		objects   []objectInfo
		wantState UploadState
		wantCode  MarkerErrorCode
	}{
		{
			name: "complete", body: markerBody,
			objects: []objectInfo{
				{Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody))},
				{Key: dir + "/plan.html", Size: 20},
				{Key: dir + "/plan.md", Size: 5},
				{Key: dir + "/deep/extra", Size: 7},
			},
			wantState: UploadComplete,
		},
		{
			name: "missing source", body: markerBody,
			objects: []objectInfo{
				{Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody))},
				{Key: dir + "/plan.html", Size: 20},
			},
			wantState: UploadIncomplete,
		},
		{
			name: "invalid", body: []byte(`{"schema":`),
			objects: []objectInfo{
				{Key: dir + "/" + MarkerFilename, Size: 10},
				{Key: dir + "/plan.html", Size: 20},
			},
			wantState: UploadInvalid, wantCode: MarkerErrorMalformedJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newInspectServer(t, dir, tt.body, tt.objects, http.StatusOK)
			client := newInspectTestClient(t, server.URL)
			got, err := client.InspectUpload(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tt.wantState || got.Error != tt.wantCode {
				t.Fatalf("inspection = %+v", got)
			}
			if got.Objects != len(tt.objects) {
				t.Fatalf("objects = %d, want %d", got.Objects, len(tt.objects))
			}
			if tt.wantState == UploadInvalid {
				if got.Page != nil || got.Source != nil || got.Title != "" {
					t.Fatalf("invalid inspection trusts marker fields: %+v", got)
				}
				return
			}
			if got.Page == nil || !got.Page.Exists ||
				got.Page.URL != "https://plans.example.com/"+dir+"/plan.html" {
				t.Fatalf("page = %+v", got.Page)
			}
			if got.Source == nil || got.Source.Exists !=
				(tt.wantState == UploadComplete) {
				t.Fatalf("source = %+v", got.Source)
			}
		})
	}
}

func TestInspectUploadRequestFailures(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	dir := "abcdefghijklmnopqrstuvwxyz"

	for _, tt := range []struct {
		name       string
		status     int
		wantSubstr string
	}{
		{
			name: "missing", status: http.StatusNotFound,
			wantSubstr: "ownership marker",
		},
		{
			name: "storage failure", status: http.StatusInternalServerError,
			wantSubstr: "get object",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newInspectServer(t, dir, nil, nil, tt.status)
			client := newInspectTestClient(t, server.URL)
			got, err := client.InspectUpload(context.Background(), dir)
			if err == nil || got != nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("inspection = %+v, error = %v", got, err)
			}
		})
	}
}

func TestInspectUploadWarnsForFallbackPublicURL(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	markerBody, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC(), Format: "html", Page: "plan.html",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newInspectServer(t, dir, markerBody, []objectInfo{
		{Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody))},
		{Key: dir + "/plan.html", Size: 20},
	}, http.StatusOK)
	client := newInspectTestClient(t, server.URL)
	client.cfg.PublicBaseURL = ""

	got, err := client.InspectUpload(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 ||
		!strings.Contains(got.Warnings[0], "public_base_url") {
		t.Fatalf("warnings = %v", got.Warnings)
	}
}

func TestInspectUploadRejectsNestedTargetBeforeRequests(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	client := newInspectTestClient(t, server.URL)
	_, err := client.InspectUpload(context.Background(), dir+"/deep/object")
	if err == nil || requests != 0 {
		t.Fatalf("error = %v, requests = %d", err, requests)
	}
}

func newInspectServer(
	t *testing.T,
	dir string,
	markerBody []byte,
	objects []objectInfo,
	markerStatus int,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("list-type") == "2" {
				writeListXML(t, w, objects)
				return
			}
			if r.Method != http.MethodGet ||
				r.URL.Path != "/plans/"+dir+"/"+MarkerFilename {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			if markerStatus != http.StatusOK {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(markerStatus)
				if markerStatus == http.StatusNotFound {
					_, _ = io.WriteString(w,
						`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
				}
				return
			}
			_, _ = w.Write(markerBody)
		},
	))
	t.Cleanup(server.Close)
	return server
}

func newInspectTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := New(context.Background(), &Config{
		Endpoint:        endpoint,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
		DisableManifest: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
