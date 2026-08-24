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
		Schema: MarkerSchema, Version: 1, Directory: dir,
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
				{Key: dir + "/archive/" + CollectionMarkerFilename, Size: 3},
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
			if got.Source.ExpectedKnown {
				t.Fatalf("legacy source size marked known: %+v", got.Source)
			}
		})
	}
}

func TestInspectUploadConflictReportsDeterministicOccupancy(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	objects := []objectInfo{
		{Key: dir + "/" + CollectionMarkerFilename, Size: 11},
		{Key: dir + "/" + MarkerFilename, Size: 7},
		{Key: dir + "/index.html", Size: 13},
	}
	server := newInspectServer(t, dir, nil, objects, http.StatusOK)
	got, err := newInspectTestClient(t, server.URL).InspectUpload(
		context.Background(), dir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != UploadInvalid ||
		got.Error != MarkerErrorConflictingMarkers ||
		got.MarkerKey != dir+"/"+MarkerFilename ||
		got.Objects != 3 || got.Bytes != 31 {
		t.Fatalf("inspection = %+v", got)
	}
}

func TestInspectUploadV2RestoresKnownPageSize(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	markerBody, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 2, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Format:    "md", Page: "plan.html", PageBytes: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newInspectServer(t, dir, markerBody, []objectInfo{
		{Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody))},
		{Key: dir + "/plan.html", Size: 20},
	}, http.StatusOK)
	got, err := newInspectTestClient(t, server.URL).InspectUpload(
		context.Background(), dir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Page == nil || !got.Page.ExpectedKnown || got.Page.ExpectedBytes != 20 {
		t.Fatalf("v2 page inspection = %+v", got.Page)
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
		Schema: MarkerSchema, Version: 1, Directory: dir,
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

func TestInspectUploadCollectionChecksEveryDeclaredFile(t *testing.T) {
	dir := strings.Repeat("k", 26)
	markerBody, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 3, Directory: dir,
		CreatedAt: time.Now().UTC(), Kind: UploadKindCollection,
		Objects: []MarkerObject{
			{Name: "index.html", Role: MarkerRolePage, Bytes: 5, ContentType: pageContentType},
			{Name: "shot.png", Role: MarkerRoleFile, Bytes: 3, ContentType: "image/png"},
			{Name: "empty.bin", Role: MarkerRoleFile, Bytes: 0, ContentType: "application/octet-stream"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	objects := []objectInfo{
		{Key: dir + "/" + CollectionMarkerFilename, Size: int64(len(markerBody))},
		{Key: dir + "/index.html", Size: 5},
		{Key: dir + "/shot.png", Size: 3},
		{Key: dir + "/empty.bin", Size: 0},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			writeListXML(t, w, objects)
			return
		}
		if r.URL.Path != "/plans/"+dir+"/"+CollectionMarkerFilename {
			t.Fatalf("unexpected %s", r.URL)
		}
		_, _ = w.Write(markerBody)
	}))
	t.Cleanup(server.Close)
	got, err := newInspectTestClient(t, server.URL).InspectUpload(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != UploadComplete || got.Kind != UploadKindCollection || len(got.Files) != 2 || got.Files[1].ExpectedBytes != 0 {
		t.Fatalf("inspection = %+v", got)
	}
}

func TestInspectUploadCollectionChecksEntryPageDigest(t *testing.T) {
	dir := strings.Repeat("d", 26)
	pageBody := []byte("actual page")
	fileBody := []byte("valid file")
	marker := validCollectionMarker()
	marker.Directory = dir
	marker.Objects[0].Bytes = int64(len(pageBody))
	marker.Objects[0].SHA256 = contentSHA256([]byte("wrong page!"))
	marker.Objects[1].Bytes = int64(len(fileBody))
	marker.Objects[1].SHA256 = contentSHA256(fileBody)
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	objects := []objectInfo{
		{Key: dir + "/" + CollectionMarkerFilename, Size: int64(len(markerBody))},
		{Key: dir + "/index.html", Size: int64(len(pageBody))},
		{Key: dir + "/screenshot.png", Size: int64(len(fileBody))},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			writeListXML(t, w, objects)
			return
		}
		switch r.URL.Path {
		case "/plans/" + dir + "/" + CollectionMarkerFilename:
			_, _ = w.Write(markerBody)
		case "/plans/" + dir + "/index.html":
			_, _ = w.Write(pageBody)
		case "/plans/" + dir + "/screenshot.png":
			_, _ = w.Write(fileBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := newInspectTestClient(t, server.URL).InspectUpload(
		context.Background(), dir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != UploadIncomplete {
		t.Fatalf("inspection state = %s, want incomplete", got.State)
	}
	if got.Page == nil || got.Page.Bytes != got.Page.ExpectedBytes ||
		got.Page.SHA256 == got.Page.ExpectedSHA256 {
		t.Fatalf("entry page inspection = %+v, want digest mismatch", got.Page)
	}
	if len(got.Files) != 1 ||
		got.Files[0].SHA256 != got.Files[0].ExpectedSHA256 {
		t.Fatalf("collection files = %+v, want valid digest", got.Files)
	}
}

func TestInspectUploadAcceptsDeclaredNestedTarget(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	marker := validBundleMarkerV6()
	marker.Directory = dir
	bodies := make(map[string][]byte, len(marker.Objects))
	for index := range marker.Objects {
		body := []byte("body for " + marker.Objects[index].Name)
		bodies[marker.Objects[index].Name] = body
		marker.Objects[index].Bytes = int64(len(body))
		marker.Objects[index].SHA256 = contentSHA256(body)
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	objects := []objectInfo{
		{Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody))},
	}
	for _, object := range marker.Objects {
		objects = append(objects, objectInfo{
			Key: dir + "/" + object.Name, Size: object.Bytes,
		})
	}
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("list-type") == "2" {
				writeListXML(t, w, objects)
				return
			}
			key := strings.TrimPrefix(r.URL.Path, "/plans/"+dir+"/")
			if key == MarkerFilename {
				_, _ = w.Write(markerBody)
				return
			}
			if body, ok := bodies[key]; ok {
				_, _ = w.Write(body)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	))
	t.Cleanup(server.Close)

	client := newInspectTestClient(t, server.URL)
	inspection, err := client.InspectUpload(
		context.Background(), dir+"/examples/server.go.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadComplete || inspection.Dir != dir {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func TestInspectUploadReportsSameSizeDigestMismatch(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	marker := validBundleMarkerV6()
	marker.Directory = dir
	bodies := make(map[string][]byte, len(marker.Objects))
	objects := make([]objectInfo, 0, len(marker.Objects)+1)
	for index := range marker.Objects {
		body := []byte(strings.Repeat("x", int(marker.Objects[index].Bytes)))
		if len(body) == 0 {
			body = []byte("x")
			marker.Objects[index].Bytes = 1
		}
		marker.Objects[index].SHA256 = contentSHA256([]byte(strings.Repeat(
			"y", len(body),
		)))
		bodies[marker.Objects[index].Name] = body
		objects = append(objects, objectInfo{
			Key:  dir + "/" + marker.Objects[index].Name,
			Size: marker.Objects[index].Bytes,
		})
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	objects = append(objects, objectInfo{
		Key: dir + "/" + MarkerFilename, Size: int64(len(markerBody)),
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			writeListXML(t, w, objects)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/plans/"+dir+"/")
		if name == MarkerFilename {
			_, _ = w.Write(markerBody)
			return
		}
		if body, ok := bodies[name]; ok {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	inspection, err := newInspectTestClient(t, server.URL).InspectUpload(
		context.Background(), dir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadIncomplete {
		t.Fatalf("inspection state = %s, want incomplete", inspection.State)
	}
}

func TestInspectRemoteUploadsDoesNotDownloadPayloads(t *testing.T) {
	dir := "abcdefghijklmnopqrstuvwxyz"
	marker := validBundleMarkerV6()
	marker.Directory = dir
	objects := make(map[string]objectInfo, len(marker.Objects)+1)
	for _, object := range marker.Objects {
		objects[dir+"/"+object.Name] = objectInfo{
			Key: dir + "/" + object.Name, Size: object.Bytes,
		}
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	markerKey := dir + "/" + MarkerFilename
	objects[markerKey] = objectInfo{Key: markerKey, Size: int64(len(markerBody))}
	var payloadGETs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/plans/"+markerKey {
			_, _ = w.Write(markerBody)
			return
		}
		payloadGETs++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := newInspectTestClient(t, server.URL)
	results, err := client.InspectRemoteUploads(context.Background(), []RemoteUpload{{
		Dir: dir, MarkerKey: markerKey, Objects: len(objects), objects: objects,
	}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err != nil ||
		results[0].Inspection.State != UploadComplete {
		t.Fatalf("results = %+v", results)
	}
	if payloadGETs != 0 {
		t.Fatalf("payload GETs = %d, want 0", payloadGETs)
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
