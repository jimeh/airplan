package airplan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetUploadSelectsMarkerManagedObjects(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	markerBody := getTestMarker(t, dir, "plan.md")
	objects := map[string][]byte{
		dir + "/" + MarkerFilename: markerBody,
		dir + "/plan.html":         []byte("page bytes\x00"),
		dir + "/plan.md":           []byte("source bytes\n"),
	}

	for _, tt := range []struct {
		name   string
		target string
		opts   GetOptions
		key    string
		body   []byte
	}{
		{
			name: "directory page", target: dir,
			key: dir + "/plan.html", body: objects[dir+"/plan.html"],
		},
		{
			name: "directory source", target: dir,
			opts: GetOptions{Source: true},
			key:  dir + "/plan.md", body: objects[dir+"/plan.md"],
		},
		{
			name: "explicit page", target: dir + "/plan.html",
			key: dir + "/plan.html", body: objects[dir+"/plan.html"],
		},
		{
			name:   "explicit page URL",
			target: "https://plans.example.com/" + dir + "/plan.html",
			key:    dir + "/plan.html", body: objects[dir+"/plan.html"],
		},
		{
			name: "explicit source", target: dir + "/plan.md",
			key: dir + "/plan.md", body: objects[dir+"/plan.md"],
		},
		{
			name: "explicit marker", target: dir + "/" + MarkerFilename,
			key: dir + "/" + MarkerFilename, body: markerBody,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newGetServer(t, objects)
			client := newInspectTestClient(t, server.URL)
			got, err := client.GetUpload(
				context.Background(), tt.target, tt.opts,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got.Key != tt.key || !bytes.Equal(got.Body, tt.body) {
				t.Fatalf("result = %+v, want key %q and body %q",
					got, tt.key, tt.body)
			}
		})
	}
}

func TestGetUploadRejectsInvalidSelection(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	markerBody := getTestMarker(t, dir, "plan.md")
	objects := map[string][]byte{
		dir + "/" + MarkerFilename: markerBody,
		dir + "/plan.html":         []byte("page"),
		dir + "/plan.md":           []byte("source"),
	}

	for _, tt := range []struct {
		name       string
		target     string
		opts       GetOptions
		wantSubstr string
	}{
		{
			name: "source with explicit page", target: dir + "/plan.html",
			opts: GetOptions{Source: true}, wantSubstr: "explicit child",
		},
		{
			name:   "source with explicit marker",
			target: dir + "/" + MarkerFilename,
			opts:   GetOptions{Source: true}, wantSubstr: "explicit child",
		},
		{
			name: "undeclared sibling", target: dir + "/other.txt",
			wantSubstr: "not the directory, marker, or declared payload",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newGetServer(t, objects)
			client := newInspectTestClient(t, server.URL)
			got, err := client.GetUpload(
				context.Background(), tt.target, tt.opts,
			)
			if err == nil || got != nil ||
				!strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("result = %+v, error = %v", got, err)
			}
		})
	}
}

func TestGetUploadRejectsSourceWhenMarkerDeclaresNone(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	objects := map[string][]byte{
		dir + "/" + MarkerFilename: getTestMarker(t, dir, ""),
		dir + "/plan.html":         []byte("page"),
	}
	server := newGetServer(t, objects)
	client := newInspectTestClient(t, server.URL)

	got, err := client.GetUpload(
		context.Background(), dir, GetOptions{Source: true},
	)
	if err == nil || got != nil ||
		!strings.Contains(err.Error(), "declares no source object") {
		t.Fatalf("result = %+v, error = %v", got, err)
	}
}

func TestGetUploadCollectionMemberMayLookLikeRandomDirectory(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	member := strings.Repeat("a", 26)
	marker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC(), Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{
				Name: "index.html", Role: MarkerRolePage, Bytes: 4,
				ContentType: pageContentType,
			},
			{
				Name: member, Role: MarkerRoleFile, Bytes: 5,
				ContentType: "application/octet-stream",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string][]byte{
		dir + "/" + CollectionMarkerFilename: marker,
		dir + "/index.html":                  []byte("page"),
		dir + "/" + member:                   []byte("bytes"),
	}
	server := newGetServer(t, objects)
	client := newInspectTestClient(t, server.URL)

	got, err := client.GetUpload(
		context.Background(), dir+"/"+member, GetOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != dir+"/"+member || string(got.Body) != "bytes" {
		t.Fatalf("result = %+v", got)
	}
}

func TestGetUploadMarkerAndObjectFailures(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	dir := "abcdefghijklmnopqrstuvwxyz"
	validMarker := getTestMarker(t, dir, "plan.md")

	for _, tt := range []struct {
		name       string
		objects    map[string][]byte
		wantSubstr string
	}{
		{
			name: "missing marker", objects: map[string][]byte{},
			wantSubstr: "ownership marker",
		},
		{
			name: "malformed marker", objects: map[string][]byte{
				dir + "/" + MarkerFilename: []byte(`{"schema":`),
			},
			wantSubstr: "invalid ownership marker",
		},
		{
			name: "missing page", objects: map[string][]byte{
				dir + "/" + MarkerFilename: validMarker,
				dir + "/plan.md":           []byte("source"),
			},
			wantSubstr: `upload object "` + dir + `/plan.html" is missing`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newGetServer(t, tt.objects)
			client := newInspectTestClient(t, server.URL)
			got, err := client.GetUpload(
				context.Background(), dir, GetOptions{},
			)
			if err == nil || got != nil ||
				!strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("result = %+v, error = %v", got, err)
			}
		})
	}
}

func getTestMarker(t *testing.T, dir, source string) []byte {
	t.Helper()
	body, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1, Directory: dir,
		CreatedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
		Format:    "md", Page: "plan.html", Source: source, Title: "Plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func newGetServer(
	t *testing.T, objects map[string][]byte,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
			}
			key := strings.TrimPrefix(r.URL.Path, "/plans/")
			body, ok := objects[key]
			if !ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w,
					`<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
		},
	))
	t.Cleanup(server.Close)
	return server
}
