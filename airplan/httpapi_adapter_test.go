package airplan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jimeh/airplan/internal/httpapi"
)

func TestHTTPAPIManifestListScopesSharedManifest(t *testing.T) {
	manifestPath := t.TempDir() + "/manifest.jsonl"
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ManifestRecord{
		manifestUploadRecord(now, "work", "plans", "team/current", "a"),
		manifestUploadRecord(now, "home", "plans", "team/current", "b"),
		manifestUploadRecord(now, "work", "archive", "team/current", "c"),
		manifestUploadRecord(now, "work", "plans", "team/old", "d"),
	} {
		if err := appendManifestRecord(context.Background(), manifestPath, record); err != nil {
			t.Fatal(err)
		}
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
	if len(result.Records) != 1 || result.Records[0].Title != "a" {
		t.Fatalf("records = %+v", result.Records)
	}
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
