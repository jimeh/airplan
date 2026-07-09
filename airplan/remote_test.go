package airplan

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListRemoteRecognizesUploadKeys(t *testing.T) {
	oldPage := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	newPage := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	prefix := "team/jimeh/"
	dirA := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	dirB := "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	dirNoPage := "cccccccccccccccccccccccccc"
	dirNested := "dddddddddddddddddddddddddd"

	var capturedPrefix string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusOK)
				return
			}
			capturedPrefix = r.URL.Query().Get("prefix")
			writeListXML(t, w, []objectInfo{
				{
					Key: prefix + dirA + "/zeta.html", Size: 20,
					LastModified: newPage,
				},
				{
					Key: prefix + dirA + "/plan.md", Size: 5,
					LastModified: newPage.Add(-time.Minute),
				},
				{
					Key: prefix + dirA + "/main.go", Size: 7,
					LastModified: newPage.Add(-2 * time.Minute),
				},
				{
					Key: prefix + dirB + "/beta.html", Size: 30,
					LastModified: newPage,
				},
				{
					Key: prefix + dirB + "/alpha.html", Size: 10,
					LastModified: oldPage,
				},
				{
					Key: prefix + "not-base32/plan.html", Size: 1,
					LastModified: oldPage,
				},
				{
					Key: prefix + dirNoPage + "/plan.md", Size: 1,
					LastModified: oldPage,
				},
				{
					Key: prefix + dirNested + "/page.html", Size: 1,
					LastModified: oldPage,
				},
				{
					Key: prefix + dirNested + "/deep/page.md", Size: 1,
					LastModified: oldPage,
				},
				{
					Key: "other/" + dirA + "/outside.html", Size: 1,
					LastModified: oldPage,
				},
			})
		},
	))
	t.Cleanup(server.Close)

	client := newRemoteTestClient(t, server.URL, "team/jimeh")
	uploads, err := client.ListRemote(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if capturedPrefix != prefix {
		t.Fatalf("prefix = %q, want %q", capturedPrefix, prefix)
	}
	if len(uploads) != 2 {
		t.Fatalf("uploads = %+v, want 2 uploads", uploads)
	}

	if uploads[0].Dir != prefix+dirB ||
		uploads[0].PageKey != prefix+dirB+"/alpha.html" ||
		uploads[0].Bytes != 10 ||
		!uploads[0].LastModified.Equal(oldPage) {
		t.Fatalf("first upload = %+v", uploads[0])
	}
	if uploads[1].Dir != prefix+dirA ||
		uploads[1].PageKey != prefix+dirA+"/zeta.html" ||
		uploads[1].Bytes != 20 ||
		!uploads[1].LastModified.Equal(newPage) {
		t.Fatalf("second upload = %+v", uploads[1])
	}

	wantKeys := []string{
		prefix + dirA + "/main.go",
		prefix + dirA + "/plan.md",
		prefix + dirA + "/zeta.html",
	}
	if strings.Join(uploads[1].Keys, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("keys = %v, want %v", uploads[1].Keys, wantKeys)
	}
}

func TestListRemoteRootPrefixListsBucketRoot(t *testing.T) {
	var capturedPrefix string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			capturedPrefix = r.URL.Query().Get("prefix")
			writeListXML(t, w, nil)
		},
	))
	t.Cleanup(server.Close)

	client := newRemoteTestClient(t, server.URL, "")
	if _, err := client.ListRemote(context.Background()); err != nil {
		t.Fatal(err)
	}
	if capturedPrefix != "" {
		t.Fatalf("prefix = %q, want bucket root", capturedPrefix)
	}
}

func TestRemoteTitleDecodesMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "HEAD" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("X-Amz-Meta-Title",
				mime.QEncoding.Encode("utf-8", "Plan ✓"))
			w.WriteHeader(http.StatusOK)
		},
	))
	t.Cleanup(server.Close)

	client := newRemoteTestClient(t, server.URL, "")
	title, err := client.RemoteTitle(context.Background(), testDir+"/plan.html")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Plan ✓" {
		t.Fatalf("title = %q, want decoded title", title)
	}
}

func newRemoteTestClient(t *testing.T, endpoint string, keyPrefix string) *Client {
	t.Helper()

	client, err := New(context.Background(), &Config{
		Endpoint:        endpoint,
		Bucket:          "plans",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		KeyPrefix:       keyPrefix,
		DisableManifest: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeListXML(t *testing.T, w http.ResponseWriter, objects []objectInfo) {
	t.Helper()

	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<ListBucketResult><IsTruncated>false</IsTruncated>`)
	for _, obj := range objects {
		fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size>",
			obj.Key, obj.Size)
		fmt.Fprintf(w, "<LastModified>%s</LastModified>",
			obj.LastModified.Format(time.RFC3339))
		fmt.Fprintln(w, "</Contents>")
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}
