package airplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
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
			"public url",
			"https://plans.example.com/" + testDir + "/plan.html",
			testDir + "/plan.html", false,
		},
		{
			"path-style url strips bucket",
			"https://s3.example.com/plans/" + testDir + "/plan.html",
			testDir + "/plan.html", false,
		},
		{
			"unconfigured host",
			"https://other.example.com/plans/" + testDir + "/plan.html",
			"", true,
		},
		{
			"unsupported scheme",
			"ftp://plans.example.com/" + testDir + "/plan.html",
			"", true,
		},
		{
			"bare key",
			testDir + "/plan.md",
			testDir + "/plan.md", false,
		},
		{
			"prefixed key",
			"team/jimeh/" + testDir + "/plan.html",
			"team/jimeh/" + testDir + "/plan.html", false,
		},
		{
			"bare directory",
			testDir,
			testDir, false,
		},
		{"not an airplan key", "some/random/object.png", "", true},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := KeyFromURLOrKey(cfg, tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeyFromURLOrKeyBucketOnlyFallback(t *testing.T) {
	cfg := &Config{Bucket: "plans"}
	got, err := KeyFromURLOrKey(cfg,
		"https://s3.example.com/plans/"+testDir+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	if got != testDir+"/plan.html" {
		t.Fatalf("got %q, want %q", got, testDir+"/plan.html")
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
		// A slug that happens to be 26 base32-ish chars still carries
		// an extension dot, so it can't be mistaken for the dir.
		{testDir + "/abcdefghijklmnopqrstuvw234.html", testDir + "/", false},
		{"not-a-key.html", "", true},
	}
	for _, tt := range tests {
		got, err := uploadDirPrefix(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("uploadDirPrefix(%q) err = %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("uploadDirPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestDeleteUpload drives the full delete path against a fake S3:
// list the directory, batch-delete every object, tombstone the page
// key in the manifest.
func TestDeleteUpload(t *testing.T) {
	var (
		mu      sync.Mutex
		deleted bool
	)
	listXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>` + testDir + `/plan.html</Key><Size>10</Size>
    <LastModified>2026-07-01T00:00:00Z</LastModified></Contents>
  <Contents><Key>` + testDir + `/plan.md</Key><Size>5</Size>
    <LastModified>2026-07-01T00:00:00Z</LastModified></Contents>
</ListBucketResult>`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "GET":
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(listXML))
			case "POST": // DeleteObjects
				mu.Lock()
				deleted = true
				mu.Unlock()
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write([]byte(
					`<?xml version="1.0"?><DeleteResult></DeleteResult>`))
			default:
				w.WriteHeader(http.StatusOK)
			}
		},
	))
	t.Cleanup(server.Close)

	manifest := t.TempDir() + "/manifest.jsonl"
	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		PublicBaseURL:   "https://plans.example.com",
		ManifestPath:    manifest,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.DeleteUpload(context.Background(),
		"https://other.example.com/plans/"+testDir+"/plan.html")
	if err == nil || !strings.Contains(err.Error(),
		"does not match the configured") {
		t.Fatalf("unconfigured URL error = %v", err)
	}

	// Scheme variants of the configured public host remain equivalent:
	// the target is parsed for its key, not fetched.
	res, err := client.DeleteUpload(context.Background(),
		"http://plans.example.com/"+testDir+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 2 {
		t.Errorf("Keys = %v, want 2 entries", res.Keys)
	}
	if res.PageKey != testDir+"/plan.html" {
		t.Errorf("PageKey = %q", res.PageKey)
	}
	mu.Lock()
	if !deleted {
		t.Error("DeleteObjects never called")
	}
	mu.Unlock()

	records, warns, err := ReadManifest(manifest)
	if err != nil || len(warns) > 0 {
		t.Fatalf("ReadManifest: %v %v", err, warns)
	}
	if len(records) != 1 || records[0].Type != "delete" ||
		records[0].Key != testDir+"/plan.html" {
		t.Errorf("tombstone = %+v", records)
	}
	if len(ActiveUploads(records)) != 0 {
		t.Error("ActiveUploads should be empty")
	}
}

func TestActiveUploads(t *testing.T) {
	recs := []ManifestRecord{
		{Type: "upload", Key: "a/x.html"},
		{Type: "upload", Key: "b/y.html"},
		{Type: "delete", Key: "a/x.html"},
		{Type: "upload", Key: "c/z.html"},
	}
	got := ActiveUploads(recs)
	if len(got) != 2 || got[0].Key != "b/y.html" || got[1].Key != "c/z.html" {
		t.Errorf("ActiveUploads = %+v", got)
	}
	if strings.Contains(got[0].Key, "a/") {
		t.Error("tombstoned upload survived")
	}
}

// TestDeleteUploadEnsureGone: an empty directory tombstones instead
// of failing (SPEC.md §9 — the manifest must converge on
// externally-deleted uploads).
func TestDeleteUploadEnsureGone(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(emptyXML))
		},
	))
	t.Cleanup(server.Close)

	manifest := t.TempDir() + "/manifest.jsonl"
	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		ManifestPath:    manifest,
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.DeleteUpload(context.Background(),
		testDir+"/plan.md")
	if err != nil {
		t.Fatalf("ensure-gone should not error: %v", err)
	}
	if len(res.Keys) != 0 {
		t.Errorf("Keys = %v, want none", res.Keys)
	}
	if len(res.Warnings) == 0 ||
		!strings.Contains(res.Warnings[0], "already deleted") {
		t.Errorf("Warnings = %v", res.Warnings)
	}

	records, _, err := ReadManifest(manifest)
	if err != nil || len(records) != 1 || records[0].Type != "delete" {
		t.Fatalf("tombstone missing: %+v err=%v", records, err)
	}
}

func TestKeyFromURLOrKeyPrefixScoping(t *testing.T) {
	cfg := &Config{Bucket: "plans", KeyPrefix: "team/jimeh"}

	if _, err := KeyFromURLOrKey(cfg,
		"team/jimeh/"+testDir+"/plan.html"); err != nil {
		t.Errorf("in-prefix key rejected: %v", err)
	}

	_, err := KeyFromURLOrKey(cfg, "team/other/"+testDir+"/plan.html")
	if err == nil || !strings.Contains(err.Error(), "outside the configured") {
		t.Errorf("out-of-prefix key not rejected: %v", err)
	}

	// Un-prefixed keys are outside a configured prefix too.
	if _, err := KeyFromURLOrKey(cfg, testDir+"/plan.html"); err == nil {
		t.Error("bare key accepted despite configured key_prefix")
	}
}

func TestKeyFromURLOrKeyBaseURLWithPath(t *testing.T) {
	cfg := &Config{
		Bucket:        "plans",
		PublicBaseURL: "https://cdn.example.com/plans",
	}

	for _, scheme := range []string{"https", "http"} {
		t.Run(scheme, func(t *testing.T) {
			got, err := KeyFromURLOrKey(cfg,
				scheme+"://cdn.example.com/plans/"+
					testDir+"/plan.html")
			if err != nil {
				t.Fatal(err)
			}
			if got != testDir+"/plan.html" {
				t.Errorf("got %q, want %q", got, testDir+"/plan.html")
			}
		})
	}
}

func TestKeyFromURLOrKeyEncodedPathRoundTrip(t *testing.T) {
	cfg := &Config{
		Endpoint:      "https://s3.example.com/api",
		Bucket:        "plans",
		PublicBaseURL: "https://CDN.example.com/shared/plans",
		KeyPrefix:     "team/Jiméh plans",
	}
	key := "team/Jiméh plans/" + testDir + "/plan #1.html"
	public, _ := PublicURL(cfg, key)

	got, err := KeyFromURLOrKey(cfg, public)
	if err != nil {
		t.Fatal(err)
	}
	if got != key {
		t.Fatalf("round trip = %q, want %q", got, key)
	}

	cfg.PublicBaseURL = ""
	fallback, _ := PublicURL(cfg, key)
	got, err = KeyFromURLOrKey(cfg, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if got != key {
		t.Fatalf("fallback round trip = %q, want %q", got, key)
	}
}

func TestDeleteUploadEnsureGoneRejectsWrongConnection(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(emptyXML))
		},
	))
	t.Cleanup(server.Close)

	for _, tt := range []struct {
		name          string
		recordBucket  string
		recordProfile string
		activeBucket  string
		activeProfile string
	}{
		{"different bucket", "archive", "work", "plans", "work"},
		{"different profile in shared bucket", "plans", "home", "plans", "work"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := t.TempDir() + "/manifest.jsonl"
			pageKey := testDir + "/plan.html"
			if err := appendManifestRecord(manifest, ManifestRecord{
				Type: "upload", Key: pageKey, Bucket: tt.recordBucket,
				Profile: tt.recordProfile,
			}); err != nil {
				t.Fatal(err)
			}

			client, err := New(context.Background(), &Config{
				Endpoint: server.URL, Bucket: tt.activeBucket,
				AccessKeyID: "test", SecretAccessKey: "test",
				Profile: tt.activeProfile, ManifestPath: manifest,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.DeleteUpload(context.Background(), pageKey)
			if err == nil || !strings.Contains(err.Error(),
				"retry with the recorded profile") {
				t.Fatalf("wrong connection error = %v", err)
			}

			records, _, readErr := ReadManifest(manifest)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(records) != 1 || records[0].Type != "upload" {
				t.Fatalf("manifest changed despite refusal: %+v", records)
			}
		})
	}
}

func TestEnsureGoneWithoutManifestUsesTarget(t *testing.T) {
	c := &Client{cfg: &Config{DisableManifest: true}}
	key := testDir + "/plan.md"
	got, warnings, err := c.ensureGonePageKey(testDir+"/", key)
	if err != nil || got != key || len(warnings) != 0 {
		t.Fatalf("ensureGonePageKey() = %q, %v, %v", got, warnings, err)
	}
}

func TestDeleteUploadEnsureGoneWithManifestDisabled(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(emptyXML))
		},
	))
	t.Cleanup(server.Close)

	manifest := t.TempDir() + "/manifest.jsonl"
	client, err := New(context.Background(), &Config{
		Endpoint: server.URL, Bucket: "plans",
		AccessKeyID: "test", SecretAccessKey: "test",
		DisableManifest: true, ManifestPath: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	target := testDir + "/plan.md"
	res, err := client.DeleteUpload(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if res.PageKey != target {
		t.Fatalf("PageKey = %q, want %q", res.PageKey, target)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("disabled manifest was written: %v", err)
	}
}

func TestDeleteUploadEnsureGoneReportsManifestProblems(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(emptyXML))
		},
	))
	t.Cleanup(server.Close)

	tests := []struct {
		name         string
		manifestPath func(*testing.T) string
		wantWarning  string
	}{
		{
			name: "unreadable manifest",
			manifestPath: func(*testing.T) string {
				return string([]byte{0})
			},
			wantWarning: "manifest unreadable; skipping bucket/profile check",
		},
		{
			name: "malformed manifest line",
			manifestPath: func(t *testing.T) string {
				path := t.TempDir() + "/manifest.jsonl"
				if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantWarning: "manifest incomplete; bucket/profile check may be " +
				"incomplete: skipping malformed manifest line 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(context.Background(), &Config{
				Endpoint: server.URL, Bucket: "plans",
				AccessKeyID: "test", SecretAccessKey: "test",
				ManifestPath: tt.manifestPath(t),
			})
			if err != nil {
				t.Fatal(err)
			}
			res, err := client.DeleteUpload(context.Background(),
				testDir+"/plan.html")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(res.Warnings, "\n"),
				tt.wantWarning) {
				t.Fatalf("warnings = %v, want %q", res.Warnings,
					tt.wantWarning)
			}
		})
	}
}

// TestDeleteUploadEnsureGoneBareDir: ensure-gone on a bare directory
// target must tombstone the manifest's page key, not the directory
// string, so ActiveUploads deactivates the record.
func TestDeleteUploadEnsureGoneBareDir(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(emptyXML))
		},
	))
	t.Cleanup(server.Close)

	manifest := t.TempDir() + "/manifest.jsonl"
	pageKey := testDir + "/plan.html"
	err := appendManifestRecord(manifest, ManifestRecord{
		Type: "upload", Key: pageKey, Bucket: "plans", Profile: "work",
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        server.URL,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		ManifestPath:    manifest,
		Profile:         "work",
	}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.DeleteUpload(context.Background(), testDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.PageKey != pageKey {
		t.Errorf("PageKey = %q, want %q", res.PageKey, pageKey)
	}

	records, _, err := ReadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if active := ActiveUploads(records); len(active) != 0 {
		t.Errorf("upload still active after ensure-gone: %+v", active)
	}
}
