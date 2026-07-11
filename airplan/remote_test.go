package airplan

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListRemoteIndexesMarkerDirectories(t *testing.T) {
	markerA := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	markerB := markerA.Add(time.Hour)
	prefix := "team/jimeh/"
	dirA := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	dirB := "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	dirMarkerOnly := "cccccccccccccccccccccccccc"
	dirMarkerless := "dddddddddddddddddddddddddd"

	var requests int
	var capturedPrefix string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Method != http.MethodGet || r.URL.Query().Get("list-type") != "2" {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			capturedPrefix = r.URL.Query().Get("prefix")
			writeListXML(t, w, []objectInfo{
				{
					Key: prefix + dirA + "/" + MarkerFilename, Size: 100,
					LastModified: markerA,
				},
				{
					Key: prefix + dirA + "/plan.html", Size: 20,
					LastModified: markerA.Add(time.Minute),
				},
				{
					Key: prefix + dirA + "/plan.md", Size: 5,
					LastModified: markerA,
				},
				{
					Key: prefix + dirA + "/deep/extra.bin", Size: 7,
					LastModified: markerA,
				},
				{
					Key: prefix + dirB + "/" + MarkerFilename, Size: 90,
					LastModified: markerB,
				},
				{
					Key: prefix + dirB + "/alpha.html", Size: 10,
					LastModified: markerB,
				},
				{
					Key: prefix + dirB + "/beta.html", Size: 30,
					LastModified: markerB,
				},
				{
					Key: prefix + dirMarkerOnly + "/" + MarkerFilename, Size: 80,
					LastModified: markerB.Add(time.Hour),
				},
				{
					Key: prefix + dirMarkerless + "/page.html", Size: 1,
					LastModified: markerA,
				},
				{
					Key: prefix + "not-base32/" + MarkerFilename, Size: 1,
					LastModified: markerA,
				},
				{
					Key: prefix + dirMarkerless + "/deep/" + MarkerFilename, Size: 1,
					LastModified: markerA,
				},
				{
					Key: "other/" + dirA + "/" + MarkerFilename, Size: 1,
					LastModified: markerA,
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

	if requests != 1 {
		t.Fatalf("requests = %d, want one LIST", requests)
	}
	if capturedPrefix != prefix {
		t.Fatalf("prefix = %q, want %q", capturedPrefix, prefix)
	}
	if len(uploads) != 3 {
		t.Fatalf("uploads = %+v, want 3 marker directories", uploads)
	}

	first := uploads[0]
	if first.Dir != dirA ||
		first.MarkerKey != prefix+dirA+"/"+MarkerFilename ||
		first.Slug != "plan" ||
		first.Objects != 4 || first.Bytes != 132 ||
		!first.LastModified.Equal(markerA) {
		t.Fatalf("first upload = %+v", first)
	}
	wantKeys := []string{
		prefix + dirA + "/" + MarkerFilename,
		prefix + dirA + "/deep/extra.bin",
		prefix + dirA + "/plan.html",
		prefix + dirA + "/plan.md",
	}
	if strings.Join(first.Keys, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("keys = %v, want %v", first.Keys, wantKeys)
	}
	if uploads[1].Dir != dirB || uploads[1].Slug != "" ||
		uploads[1].Objects != 3 ||
		uploads[1].Bytes != 130 {
		t.Fatalf("ambiguous upload = %+v", uploads[1])
	}
	if uploads[2].Dir != dirMarkerOnly || uploads[2].Objects != 1 ||
		uploads[2].Bytes != 80 || uploads[2].Slug != "" {
		t.Fatalf("marker-only upload = %+v", uploads[2])
	}
}

func TestListRemotePaginatesAndAggregates(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	when := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests++
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("continuation-token") == "next" {
				fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
					`<IsTruncated>false</IsTruncated>`+
					objectXML(objectInfo{
						Key: dir + "/plan.html", Size: 20,
						LastModified: when,
					})+`</ListBucketResult>`)
				return
			}
			fmt.Fprintln(w, `<?xml version="1.0"?><ListBucketResult>`+
				`<IsTruncated>true</IsTruncated>`+
				`<NextContinuationToken>next</NextContinuationToken>`+
				objectXML(objectInfo{
					Key: dir + "/" + MarkerFilename, Size: 10,
					LastModified: when,
				})+`</ListBucketResult>`)
		},
	))
	t.Cleanup(server.Close)

	client := newRemoteTestClient(t, server.URL, "")
	uploads, err := client.ListRemote(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(uploads) != 1 || uploads[0].Objects != 2 ||
		uploads[0].Bytes != 30 || uploads[0].Slug != "plan" {
		t.Fatalf("requests = %d, uploads = %+v", requests, uploads)
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
		fmt.Fprintln(w, objectXML(obj))
	}
	fmt.Fprintln(w, `</ListBucketResult>`)
}

func objectXML(obj objectInfo) string {
	return fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size>"+
		"<LastModified>%s</LastModified></Contents>",
		obj.Key, obj.Size, obj.LastModified.Format(time.RFC3339))
}
