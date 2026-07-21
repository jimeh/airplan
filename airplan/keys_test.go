package airplan

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

const testDir = "vq3nhk2p7r4wzt5c6ydjm3xhqd"

func TestKeyFromURLOrKey(t *testing.T) {
	cfg := &Config{
		Endpoint:      "https://s3.example.com",
		Bucket:        "plans",
		PublicBaseURL: "https://plans.example.com",
	}
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "public URL",
			in:   "https://plans.example.com/" + testDir + "/plan.html",
			want: testDir + "/plan.html",
		},
		{
			name: "path-style URL",
			in:   "https://s3.example.com/plans/" + testDir + "/plan.html",
			want: testDir + "/plan.html",
		},
		{
			name:    "different path-style bucket",
			in:      "https://s3.example.com/archive/" + testDir + "/plan.html",
			wantErr: true,
		},
		{
			name:    "missing path-style bucket",
			in:      "https://s3.example.com/" + testDir + "/plan.html",
			wantErr: true,
		},
		{
			name:    "unconfigured host",
			in:      "https://other.example.com/plans/" + testDir + "/plan.html",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			in:      "ftp://plans.example.com/" + testDir + "/plan.html",
			wantErr: true,
		},
		{
			name: "bare key", in: testDir + "/plan.md",
			want: testDir + "/plan.md",
		},
		{
			name: "prefixed key", in: "team/jimeh/" + testDir + "/plan.html",
			want: "team/jimeh/" + testDir + "/plan.html",
		},
		{name: "bare directory", in: testDir, want: testDir},
		{name: "not airplan key", in: "some/random/object.png", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := KeyFromURLOrKey(cfg, tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("key = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeyFromURLOrKeyBucketOnlyFallback(t *testing.T) {
	cfg := &Config{Bucket: "plans"}
	got, err := KeyFromURLOrKey(cfg,
		"https://s3.example.com/plans/"+testDir+"/plan.html")
	if err != nil || got != testDir+"/plan.html" {
		t.Fatalf("key = %q, error = %v", got, err)
	}
	_, err = KeyFromURLOrKey(cfg,
		"https://s3.example.com/archive/"+testDir+"/plan.html")
	if err == nil {
		t.Fatal("different bucket accepted")
	}
}

func TestUploadDirPrefix(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{testDir + "/plan.html", testDir + "/", false},
		{"pre/fix/" + testDir + "/main.go", "pre/fix/" + testDir + "/", false},
		{testDir, testDir + "/", false},
		{testDir + "/abcdefghijklmnopqrstuvw234.html", testDir + "/", false},
		{"not-a-key.html", "", true},
	}
	for _, tt := range tests {
		got, err := uploadDirPrefix(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Fatalf("uploadDirPrefix(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestUploadDirPrefixForKeyPrefixKeepsRandomLookingMember(t *testing.T) {
	dir := testDir
	member := strings.Repeat("a", 26)
	for _, tt := range []struct {
		key, prefix, want string
	}{
		{dir + "/" + member, "", dir + "/"},
		{"team/" + dir + "/" + member, "team", "team/" + dir + "/"},
	} {
		got, err := uploadDirPrefixForKeyPrefix(tt.key, tt.prefix)
		if err != nil || got != tt.want {
			t.Fatalf("prefix(%q, %q) = %q, %v; want %q",
				tt.key, tt.prefix, got, err, tt.want)
		}
	}
}

func TestKeyFromURLOrKeyPrefixAndEncodedPaths(t *testing.T) {
	cfg := &Config{
		Endpoint:      "https://s3.example.com/api",
		Bucket:        "plans",
		PublicBaseURL: "https://CDN.example.com/shared/plans",
		KeyPrefix:     "team/Jiméh plans",
	}
	key := "team/Jiméh plans/" + testDir + "/plan #1.html"
	public, _, err := PublicURL(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := KeyFromURLOrKey(cfg, public)
	if err != nil || got != key {
		t.Fatalf("public round trip = %q, %v", got, err)
	}
	cfg.PublicBaseURL = ""
	fallback, _, err := PublicURL(cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err = KeyFromURLOrKey(cfg, fallback)
	if err != nil || got != key {
		t.Fatalf("fallback round trip = %q, %v", got, err)
	}
	if _, err := KeyFromURLOrKey(cfg,
		"team/other/"+testDir+"/plan.html"); err == nil {
		t.Fatal("out-of-prefix key accepted")
	}
}

func TestKeyMatchesPrefix(t *testing.T) {
	for _, tt := range []struct {
		key, prefix string
		want        bool
	}{
		{testDir + "/plan.html", "", true},
		{"team/jimeh/" + testDir + "/plan.html", "team/jimeh", true},
		{"team/other/" + testDir + "/plan.html", "team/jimeh", false},
		{"team/jimeh/" + testDir + "/plan.html", "", false},
		{"invalid/plan.html", "", false},
	} {
		if got := KeyMatchesPrefix(tt.key, tt.prefix); got != tt.want {
			t.Errorf("KeyMatchesPrefix(%q, %q) = %v, want %v",
				tt.key, tt.prefix, got, tt.want)
		}
	}
}

func TestDeleteUploadMarkerLast(t *testing.T) {
	marker := testUploadMarker(t, "md", "plan.html", "plan.md")
	fake := newDeleteLifecycleS3(t, marker, []objectInfo{
		{Key: testDir + "/" + MarkerFilename, Size: int64(len(marker))},
		{Key: testDir + "/plan.html", Size: 10},
		{Key: testDir + "/plan.md", Size: 5},
		{Key: testDir + "/deep/extra", Size: 3},
	})
	manifest := t.TempDir() + "/manifest.jsonl"
	client := newDeleteTestClient(t, fake.server.URL, manifest, false)

	res, err := client.DeleteUpload(context.Background(),
		"http://plans.example.com/"+testDir+"/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	if res.PageKey != testDir+"/plan.html" || len(res.Keys) != 4 ||
		res.Keys[len(res.Keys)-1] != testDir+"/"+MarkerFilename {
		t.Fatalf("result = %+v", res)
	}
	if got := fake.operationOrder(); len(got) != 5 ||
		!markerProbes(got[:2]) || strings.Join(got[2:], ",") !=
		"list,payload-delete,marker-delete" {
		t.Fatalf("operations = %v", got)
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 || len(records) != 1 ||
		records[0].Type != "delete" || records[0].Key != res.PageKey {
		t.Fatalf("manifest = %+v, warnings = %v, error = %v",
			records, warnings, err)
	}
}

func TestDeleteUploadResolvesBothMarkersBeforeDecoding(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	fake := newDeleteLifecycleS3(t, []byte(`{"schema":`), nil)
	fake.collectionMarker = []byte(`{}`)
	client := newDeleteTestClient(t, fake.server.URL, "", true)

	_, err := client.DeleteUpload(context.Background(), testDir)
	assertMarkerCode(t, err, MarkerErrorConflictingMarkers)
	operations := fake.operationOrder()
	if len(operations) != 2 || !markerProbes(operations) {
		t.Fatalf("operations = %v, want only both marker probes", operations)
	}
	if fake.deleteCalls() != 0 {
		t.Fatalf("delete calls = %d", fake.deleteCalls())
	}
}

func TestDeleteUploadFailsClosedOnAmbiguousMarkerProbe(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	marker := testUploadMarker(t, "html", "plan.html", "")
	fake := newDeleteLifecycleS3(t, marker, nil)
	fake.collectionStatus = http.StatusInternalServerError
	client := newDeleteTestClient(t, fake.server.URL, "", true)

	_, err := client.DeleteUpload(context.Background(), testDir)
	if err == nil || !strings.Contains(err.Error(), "Internal Server Error") {
		t.Fatalf("error = %v, want marker probe failure", err)
	}
	operations := fake.operationOrder()
	if len(operations) != 2 || !markerProbes(operations) {
		t.Fatalf("operations = %v, want only both marker probes", operations)
	}
	if fake.deleteCalls() != 0 {
		t.Fatalf("delete calls = %d", fake.deleteCalls())
	}
}

func TestDeleteUploadRefusesInvalidAndUndeclaredTargets(t *testing.T) {
	valid := testUploadMarker(t, "html", "plan.html", "")
	for _, tt := range []struct {
		name   string
		marker []byte
		target string
	}{
		{
			name: "malformed marker", marker: []byte(`{"schema":`),
			target: testDir + "/plan.html",
		},
		{
			name: "undeclared sibling", marker: valid,
			target: testDir + "/other.txt",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newDeleteLifecycleS3(t, tt.marker, []objectInfo{
				{Key: testDir + "/" + MarkerFilename, Size: int64(len(tt.marker))},
				{Key: testDir + "/plan.html", Size: 10},
			})
			client := newDeleteTestClient(t, fake.server.URL, "", true)
			if _, err := client.DeleteUpload(context.Background(), tt.target); err == nil {
				t.Fatal("delete succeeded")
			}
			if fake.deleteCalls() != 0 {
				t.Fatalf("delete calls = %d", fake.deleteCalls())
			}
		})
	}
}

func TestDeleteUploadFailuresPreserveMarkerAndManifest(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")

	marker := testUploadMarker(t, "html", "plan.html", "")
	for _, tt := range []struct {
		name        string
		failPayload bool
		failMarker  bool
	}{
		{name: "payload", failPayload: true},
		{name: "marker", failMarker: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newDeleteLifecycleS3(t, marker, []objectInfo{
				{Key: testDir + "/" + MarkerFilename, Size: int64(len(marker))},
				{Key: testDir + "/plan.html", Size: 10},
			})
			fake.failPayload = tt.failPayload
			fake.failMarker = tt.failMarker
			manifest := t.TempDir() + "/manifest.jsonl"
			client := newDeleteTestClient(t, fake.server.URL, manifest, false)
			if _, err := client.DeleteUpload(context.Background(),
				testDir+"/plan.html"); err == nil {
				t.Fatal("delete succeeded")
			}
			if _, err := os.Stat(manifest); !os.IsNotExist(err) {
				t.Fatalf("manifest changed: %v", err)
			}
			operations := strings.Join(fake.operationOrder(), ",")
			if tt.failPayload && strings.Contains(operations, "marker-delete") {
				t.Fatalf("marker delete followed payload failure: %s", operations)
			}
		})
	}
}

func TestDeleteUploadMissingMarkerReconciliation(t *testing.T) {
	fake := newDeleteLifecycleS3(t, nil, nil)
	fake.markerMissing = true
	manifest := t.TempDir() + "/manifest.jsonl"
	if err := os.WriteFile(manifest, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pageKey := testDir + "/plan.html"
	if err := appendManifestRecord(context.Background(), manifest, ManifestRecord{
		Type: "upload", Time: time.Now().UTC(), Key: pageKey,
		URL:    "https://plans.example.com/" + pageKey,
		Bucket: "plans", Profile: "work", Bytes: 1,
		MarkerVersion: MarkerVersion,
	}); err != nil {
		t.Fatal(err)
	}
	client := newDeleteTestClient(t, fake.server.URL, manifest, false)
	client.cfg.Profile = "work"

	res, err := client.DeleteUpload(context.Background(), testDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.PageKey != pageKey || fake.deleteCalls() != 0 ||
		len(res.Warnings) == 0 {
		t.Fatalf("result = %+v, delete calls = %d", res, fake.deleteCalls())
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 1 {
		t.Fatalf("manifest warnings = %v, error = %v", warnings, err)
	}
	if active := ActiveUploads(records); len(active) != 0 {
		t.Fatalf("active uploads = %+v", active)
	}
}

func TestDeleteTombstoneFailureRecoversOnRetry(t *testing.T) {
	marker := testUploadMarker(t, "html", "plan.html", "")
	fake := newDeleteLifecycleS3(t, marker, []objectInfo{
		{Key: testDir + "/" + MarkerFilename, Size: int64(len(marker))},
		{Key: testDir + "/plan.html", Size: 10},
	})
	manifest := t.TempDir() + "/manifest.jsonl"
	pageKey := testDir + "/plan.html"
	if err := appendManifestRecord(context.Background(), manifest, ManifestRecord{
		Type: "upload", Time: time.Now().UTC(), Key: pageKey,
		URL:    "https://plans.example.com/" + pageKey,
		Bucket: "plans", Bytes: 10, MarkerVersion: MarkerVersion,
	}); err != nil {
		t.Fatal(err)
	}

	lock := flock.New(manifest + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	client := newDeleteTestClient(t, fake.server.URL, manifest, false)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res, err := client.DeleteUpload(ctx, pageKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"),
		"tombstone not recorded") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}

	retry, err := client.DeleteUpload(context.Background(), pageKey)
	if err != nil {
		t.Fatal(err)
	}
	if retry.PageKey != pageKey || len(retry.Warnings) == 0 {
		t.Fatalf("retry = %+v", retry)
	}
	if active := mustActiveUploads(t, manifest); len(active) != 0 {
		t.Fatalf("active uploads = %+v", active)
	}
}

func TestDeleteUploadMissingMarkerRequiresExactManifest(t *testing.T) {
	record := func(profile, bucket string, markerVersion int) *ManifestRecord {
		return &ManifestRecord{
			Type: "upload", Time: time.Now().UTC(),
			Key:    testDir + "/plan.html",
			URL:    "https://plans.example.com/" + testDir + "/plan.html",
			Bucket: bucket, Profile: profile, Bytes: 1,
			MarkerVersion: markerVersion,
		}
	}
	for _, tt := range []struct {
		name      string
		record    *ManifestRecord
		target    string
		prefix    string
		wantError string
	}{
		{name: "no record", wantError: "no matching active"},
		{
			name: "invalid manifest", prefix: "not json\n",
			wantError: "local manifest is incomplete",
		},
		{
			name: "legacy record", record: record("", "plans", 0),
			wantError: "no matching active",
		},
		{
			name: "wrong profile", record: record("work", "plans", MarkerVersion),
			wantError: `belongs to profile "work"; active connection uses profile "<root>"`,
		},
		{
			name: "wrong bucket", record: record("", "archive", MarkerVersion),
			wantError: `belongs to bucket "archive"`,
		},
		{
			name: "undeclared target", target: testDir + "/other.txt",
			record:    record("", "plans", MarkerVersion),
			wantError: "is not the directory, marker, or recorded payload",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newDeleteLifecycleS3(t, nil, nil)
			fake.markerMissing = true
			manifest := t.TempDir() + "/manifest.jsonl"
			if tt.prefix != "" {
				if err := os.WriteFile(
					manifest, []byte(tt.prefix), 0o600,
				); err != nil {
					t.Fatal(err)
				}
			}
			if tt.record != nil {
				if err := appendManifestRecord(
					context.Background(), manifest, *tt.record,
				); err != nil {
					t.Fatal(err)
				}
			}
			client := newDeleteTestClient(t, fake.server.URL, manifest, false)
			target := tt.target
			if target == "" {
				target = testDir
			}
			_, err := client.DeleteUpload(context.Background(), target)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
			if fake.deleteCalls() != 0 {
				t.Fatalf("delete calls = %d", fake.deleteCalls())
			}
		})
	}
}

func TestDeleteUploadMissingMarkerReportsManifestAccessFailures(t *testing.T) {
	for _, tt := range []struct {
		name            string
		disableManifest bool
		manifestIsDir   bool
		wantError       string
	}{
		{
			name: "disabled", disableManifest: true,
			wantError: "local manifest is disabled",
		},
		{
			name: "unreadable", manifestIsDir: true,
			wantError: "read local manifest",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fake := newDeleteLifecycleS3(t, nil, nil)
			fake.markerMissing = true
			manifest := t.TempDir() + "/manifest.jsonl"
			if tt.manifestIsDir {
				if err := os.Mkdir(manifest, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			client := newDeleteTestClient(
				t, fake.server.URL, manifest, tt.disableManifest,
			)

			_, err := client.DeleteUpload(context.Background(), testDir)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
			if fake.deleteCalls() != 0 {
				t.Fatalf("delete calls = %d", fake.deleteCalls())
			}
		})
	}
}

func TestActiveUploads(t *testing.T) {
	records := []ManifestRecord{
		{Type: "upload", Key: "a/x.html", MarkerVersion: MarkerVersion},
		{Type: "upload", Key: "b/y.html", MarkerVersion: MarkerVersion},
		{Type: "delete", Key: "a/x.html"},
	}
	got := ActiveUploads(records)
	if len(got) != 1 || got[0].Key != "b/y.html" {
		t.Fatalf("ActiveUploads = %+v", got)
	}
}

type deleteLifecycleS3 struct {
	server           *httptest.Server
	mu               sync.Mutex
	marker           []byte
	collectionMarker []byte
	objects          []objectInfo
	operations       []string
	deletes          int
	failPayload      bool
	failMarker       bool
	markerMissing    bool
	collectionStatus int
}

func newDeleteLifecycleS3(
	t *testing.T, marker []byte, objects []objectInfo,
) *deleteLifecycleS3 {
	t.Helper()
	fake := &deleteLifecycleS3{marker: marker, objects: objects}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *deleteLifecycleS3) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
		f.record("list")
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
			`<IsTruncated>false</IsTruncated>`)
		for _, object := range f.objects {
			fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>"+
				"<LastModified>2026-07-11T09:00:00Z</LastModified></Contents>",
				object.Key, object.Size)
		}
		fmt.Fprintln(w, `</ListBucketResult>`)
	case r.Method == http.MethodGet:
		isCollection := strings.HasSuffix(
			r.URL.Path, "/"+CollectionMarkerFilename,
		)
		operation := "get-document-marker"
		body := f.marker
		status := 0
		if isCollection {
			operation = "get-collection-marker"
			body = f.collectionMarker
			status = f.collectionStatus
		}
		f.record(operation)
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if f.markerMissing || body == nil {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w,
				`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
			return
		}
		_, _ = w.Write(body)
	case r.Method == http.MethodPost:
		f.record("payload-delete")
		f.mu.Lock()
		f.deletes++
		fail := f.failPayload
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w,
			`<?xml version="1.0"?><DeleteResult></DeleteResult>`)
	case r.Method == http.MethodDelete:
		f.record("marker-delete")
		f.mu.Lock()
		f.deletes++
		fail := f.failMarker
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.mu.Lock()
		f.markerMissing = true
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *deleteLifecycleS3) record(operation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations = append(f.operations, operation)
}

func (f *deleteLifecycleS3) operationOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.operations...)
}

func (f *deleteLifecycleS3) deleteCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deletes
}

func markerProbes(operations []string) bool {
	if len(operations) != 2 {
		return false
	}
	return operations[0] != operations[1] &&
		(operations[0] == "get-document-marker" ||
			operations[0] == "get-collection-marker") &&
		(operations[1] == "get-document-marker" ||
			operations[1] == "get-collection-marker")
}

func testUploadMarker(
	t *testing.T, format, page, source string,
) []byte {
	t.Helper()
	body, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: testDir,
		CreatedAt: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		Format:    format, Page: page, Source: source, Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func newDeleteTestClient(
	t *testing.T, endpoint, manifest string, disableManifest bool,
) *Client {
	t.Helper()
	client, err := New(context.Background(), &Config{
		Endpoint: endpoint, Bucket: "plans", AccessKeyID: "test",
		SecretAccessKey: "test", PublicBaseURL: "https://plans.example.com",
		ManifestPath: manifest, DisableManifest: disableManifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func mustActiveUploads(t *testing.T, manifest string) []ManifestRecord {
	t.Helper()
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("manifest warnings = %v, error = %v", warnings, err)
	}
	return ActiveUploads(records)
}
