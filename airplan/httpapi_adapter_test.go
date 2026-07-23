package airplan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

func TestHTTPAPIManifestListScopesSharedManifest(t *testing.T) {
	manifestPath := t.TempDir() + "/manifest.jsonl"
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(now, "work", "plans", "team/current", "a"),
		manifestUploadRecord(now, "work", "plans", "team/current", "e"),
		manifestUploadRecord(now, "home", "plans", "team/current", "b"),
		manifestUploadRecord(now, "work", "archive", "team/current", "c"),
		manifestUploadRecord(now, "work", "plans", "team/old", "d"),
	} {
		if err := appendManifestRecord(context.Background(), manifestPath, record); err != nil {
			t.Fatal(err)
		}
	}
	records, _, err := ReadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	records[1].MarkerKey = ""
	if err := rewriteManifest(t, manifestPath, records); err != nil {
		t.Fatal(err)
	}
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, Endpoint: "http://127.0.0.1",
		Bucket: "plans", KeyPrefix: "team/current", Profile: "work",
		ManifestPath: manifestPath, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(&HTTPOperations{
		Client: client, ServerVersion: "test",
	}, httpapi.Options{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/uploads", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var result httpapi.ManifestList
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 || result.Records[0].Title != "a" ||
		result.Records[1].Title != "e" {
		t.Fatalf("records = %+v", result.Records)
	}
}

func TestPlanPurgeScopesPrefixWithoutStorageConfig(t *testing.T) {
	manifestPath := t.TempDir() + "/manifest.jsonl"
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(now, "work", "plans", "team/current", "a"),
		manifestUploadRecord(now, "work", "plans", "team/old", "b"),
	} {
		if err := appendManifestRecord(
			context.Background(), manifestPath, record,
		); err != nil {
			t.Fatal(err)
		}
	}
	client, err := New(context.Background(), &Config{
		Backend: BackendS3, KeyPrefix: "team/current", Profile: "work",
		ManifestPath: manifestPath, Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.PlanPurge(context.Background(), PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Record.Title != "a" {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
}

func TestHTTPOperationsGetUploadStreamsStorageBody(t *testing.T) {
	dir := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: dir, CreatedAt: time.Now().UTC(),
		Kind: UploadKindDocument, Slug: "plan", Format: "md",
		Objects: []MarkerObject{{
			Name: "plan.html", Role: MarkerRolePage,
			Bytes: 7, ContentType: "text/html; charset=utf-8",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	started := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		switch r.URL.Path {
		case "/plans/" + dir + "/" + MarkerFilename:
			w.Header().Set("Content-Type", markerContentType)
			_, _ = w.Write(marker)
		case "/plans/" + dir + "/" + CollectionMarkerFilename:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		case "/plans/" + dir + "/plan.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			<-release
			_, _ = io.WriteString(w, "payload")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		server.Close()
	})
	client := &Client{
		cfg: &Config{
			Backend: BackendS3, Endpoint: server.URL,
			Bucket: "plans", Repository: "none",
		},
		st: newTestStorage(t, server.URL),
	}
	result := make(chan httpapi.Download, 1)
	errs := make(chan error, 1)
	go func() {
		download, getErr := (&HTTPOperations{Client: client}).GetUpload(
			context.Background(), httpapi.GetUploadRequest{URLOrKey: dir},
		)
		if getErr != nil {
			errs <- getErr
			return
		}
		result <- download
	}()

	select {
	case <-started:
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("storage body request did not start")
	}
	var download httpapi.Download
	select {
	case download = <-result:
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("GetUpload buffered the storage body")
	}
	releaseOnce.Do(func() { close(release) })
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := download.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" || !strings.HasSuffix(download.Key, "plan.html") {
		t.Fatalf("download = %q, key = %q", body, download.Key)
	}
}

func rewriteManifest(
	t *testing.T, path string, records []ManifestRecord,
) error {
	t.Helper()
	var body strings.Builder
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(body.String()), 0o600)
}

func manifestUploadRecord(
	when time.Time, profile, bucket, prefix, title string,
) ManifestRecord {
	dir := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	if title != "a" {
		dir = string(title[0]) + dir[1:]
	}
	key := BuildKey(prefix, dir, title+".html")
	return ManifestRecord{
		Type: "upload", Time: when, Key: key,
		MarkerKey: BuildKey(prefix, dir, MarkerFilename),
		URL:       "https://plans.example/" + key, Bucket: bucket,
		Profile: profile, Format: "md", Kind: string(UploadKindDocument),
		Slug: title, Title: title, Bytes: 10, MarkerVersion: MarkerVersion,
	}
}
