//go:build integration

package airplan

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jimeh/airplan/internal/httpapi"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

// TestIntegrationRoundTrip uploads through the real pipeline against a
// live S3-compatible server (MinIO, managed by testcontainers) and
// verifies bytes and headers by fetching the objects back. Excluded
// from plain `go test ./...` by the integration build tag; run via
// `mise run test-integration`.
func TestIntegrationRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Minute,
	)
	defer cancel()

	// Keep the reviewed release tag and multi-platform digest together.
	// Refresh both, inspect the image labels, then run this test before
	// accepting a newer MinIO image.
	const minioImage = "minio/minio:RELEASE.2025-09-07T16-13-09Z@" +
		"sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
	minioC, err := tcminio.Run(ctx, minioImage)
	testcontainers.CleanupContainer(t, minioC)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := minioC.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        "http://" + endpoint,
		Bucket:          "airplan-test",
		Region:          "us-east-1",
		AccessKeyID:     minioC.Username,
		SecretAccessKey: minioC.Password,
		DisableManifest: true,
		Repository:      "https://github.com/jimeh/airplan",
	}

	st, err := newStorage(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		t.Fatal(err)
	}

	client, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	src := "# Integration Plan\n\nSome **bold** text.\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n"
	res, err := client.Upload(ctx, Input{
		Reader: strings.NewReader(src),
		Name:   "integration-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Title != "Integration Plan" {
		t.Errorf("Title = %q, want %q", res.Title, "Integration Plan")
	}
	if !strings.HasSuffix(res.Key, "/integration-plan.html") {
		t.Errorf("unexpected page key %q", res.Key)
	}
	if !strings.HasSuffix(res.SourceKey, "/integration-plan.md") {
		t.Errorf("unexpected source key %q", res.SourceKey)
	}

	page := getObject(ctx, t, st, res.Key)
	if page.contentType != "text/html; charset=utf-8" {
		t.Errorf("page Content-Type = %q", page.contentType)
	}
	if page.cacheControl != "no-store" {
		t.Errorf("page Cache-Control = %q", page.cacheControl)
	}
	if page.metaTitle != "Integration Plan" {
		t.Errorf("page x-amz-meta-title = %q", page.metaTitle)
	}
	if int64(len(page.body)) != res.Bytes {
		t.Errorf("page bytes = %d, Result.Bytes = %d",
			len(page.body), res.Bytes)
	}
	if !strings.Contains(string(page.body), "Some <strong>bold</strong>") {
		t.Error("page body missing rendered markdown")
	}
	if !strings.Contains(string(page.body),
		`href="./integration-plan.md" download`) {
		t.Error("page missing download link to sibling source")
	}
	if !strings.Contains(string(page.body),
		`class="raw" href="./integration-plan.md"`) {
		t.Error("page missing raw link to sibling source")
	}

	source := getObject(ctx, t, st, res.SourceKey)
	if string(source.body) != src {
		t.Error("source object bytes differ from input")
	}
	if source.contentType != "text/markdown; charset=utf-8" {
		t.Errorf("source Content-Type = %q", source.contentType)
	}
	if source.cacheControl != "no-store" {
		t.Errorf("source Cache-Control = %q", source.cacheControl)
	}

	dirPrefix, err := uploadDirPrefix(res.Key)
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSuffix(dirPrefix, "/")
	dir = dir[strings.LastIndex(dir, "/")+1:]
	markerKey := dirPrefix + MarkerFilename
	markerObject := getObject(ctx, t, st, markerKey)
	if markerObject.contentType != "application/json" {
		t.Errorf("marker Content-Type = %q", markerObject.contentType)
	}
	if markerObject.cacheControl != "no-store" {
		t.Errorf("marker Cache-Control = %q", markerObject.cacheControl)
	}
	marker, err := DecodeUploadMarker(markerObject.body, dir)
	if err != nil {
		t.Fatal(err)
	}
	if marker.Page != "integration-plan.html" ||
		marker.Source != "integration-plan.md" ||
		marker.Title != "Integration Plan" || marker.Format != "md" ||
		marker.PageBytes != int64(len(page.body)) ||
		marker.Repo != "https://github.com/jimeh/airplan" {
		t.Fatalf("uploaded marker = %+v", marker)
	}

	remote, err := client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexed := remoteByDir(t, remote, dir)
	wantBytes := int64(len(markerObject.body) + len(page.body) + len(source.body))
	if indexed.Slug != "integration-plan" || indexed.Objects != 3 ||
		indexed.Bytes != wantBytes {
		t.Fatalf("indexed upload = %+v, want 3 objects and %d bytes",
			indexed, wantBytes)
	}
	inspection, err := client.InspectUpload(ctx, res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadComplete || inspection.Page == nil ||
		!inspection.Page.Exists || inspection.Source == nil ||
		!inspection.Source.Exists {
		t.Fatalf("complete inspection = %+v", inspection)
	}

	collection, err := client.UploadFiles(ctx, FilesInput{Files: []FileInput{
		{Name: "shot.png", Reader: bytes.NewReader([]byte("png")), Size: 3},
		{Name: "demo.webm", Reader: bytes.NewReader([]byte("video")), Size: 5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	collectionDirPrefix, err := uploadDirPrefix(collection.Key)
	if err != nil {
		t.Fatal(err)
	}
	collectionDir := strings.TrimSuffix(collectionDirPrefix, "/")
	collectionDir = collectionDir[strings.LastIndex(collectionDir, "/")+1:]
	collectionMarker := getObject(ctx, t, st, collection.MarkerKey)
	decodedCollection, err := DecodeUploadMarkerForName(
		collectionMarker.body, collectionDir, CollectionMarkerFilename,
	)
	if err != nil || decodedCollection.Kind != UploadKindCollection ||
		len(decodedCollection.Objects) != 3 {
		t.Fatalf("collection marker = %+v, %v", decodedCollection, err)
	}
	if got := getObject(ctx, t, st, collection.Files[1].Key); string(got.body) != "video" || got.contentType != "video/webm" {
		t.Fatalf("collection video = %+v", got)
	}
	collectionInspection, err := client.InspectUpload(ctx, collection.Files[0].URL)
	if err != nil || collectionInspection.State != UploadComplete || len(collectionInspection.Files) != 2 {
		t.Fatalf("collection inspection = %+v, %v", collectionInspection, err)
	}
	var downloaded bytes.Buffer
	if _, err := client.GetUploadTo(ctx, collection.Files[1].URL, GetOptions{}, &downloaded); err != nil || downloaded.String() != "video" {
		t.Fatalf("streamed collection member = %q, %v", downloaded.String(), err)
	}
	conflictDir := "zzzzzzzzzzzzzzzzzzzzzzzzzz"
	putIntegrationObject(ctx, t, st, object{
		Key:  conflictDir + "/" + MarkerFilename,
		Body: []byte(`{}`), ContentType: markerContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key:  conflictDir + "/" + CollectionMarkerFilename,
		Body: []byte(`{}`), ContentType: markerContentType,
	})
	conflictInspection, err := client.InspectUpload(ctx, conflictDir)
	if err != nil || conflictInspection.Error != MarkerErrorConflictingMarkers {
		t.Fatalf("conflict inspection = %+v, %v", conflictInspection, err)
	}
	if _, err := client.DeleteUpload(ctx, conflictDir); err == nil {
		t.Fatal("delete accepted conflicting ownership markers")
	}
	if err := st.deleteKeys(ctx, []string{
		conflictDir + "/" + MarkerFilename,
		conflictDir + "/" + CollectionMarkerFilename,
	}); err != nil {
		t.Fatal(err)
	}

	partialDir := "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	partialMarker, err := EncodeUploadMarker(UploadMarker{
		Schema: MarkerSchema, Version: 1,
		Directory: partialDir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Format:    "md",
		Page:      "missing.html",
		Source:    "missing.md",
		Title:     "Partial upload",
	})
	if err != nil {
		t.Fatal(err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: partialDir + "/" + MarkerFilename, Body: partialMarker,
		ContentType: "application/json",
	})

	unmarkedDir := "cccccccccccccccccccccccccc"
	putIntegrationObject(ctx, t, st, object{
		Key: unmarkedDir + "/unowned.html", Body: []byte("unowned"),
		ContentType: "text/html; charset=utf-8",
	})

	invalidDir := "dddddddddddddddddddddddddd"
	putIntegrationObject(ctx, t, st, object{
		Key:         invalidDir + "/" + MarkerFilename,
		Body:        []byte(`{"schema":"airplan-upload","version":99}`),
		ContentType: "application/json",
	})

	remote, err = client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote) != 4 {
		t.Fatalf("remote uploads = %+v, want document, collection, partial, and invalid", remote)
	}
	if indexedCollection := remoteByDir(t, remote, collectionDir); indexedCollection.Kind != UploadKindCollection || !strings.HasSuffix(indexedCollection.Key, "/index.html") {
		t.Fatalf("indexed collection = %+v", indexedCollection)
	}
	remoteByDir(t, remote, partialDir)
	remoteByDir(t, remote, invalidDir)
	for _, upload := range remote {
		if upload.Dir == unmarkedDir {
			t.Fatal("markerless directory was remotely discoverable")
		}
	}

	inspection, err = client.InspectUpload(ctx, partialDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadIncomplete || inspection.Page.Exists ||
		inspection.Source == nil || inspection.Source.Exists {
		t.Fatalf("partial inspection = %+v", inspection)
	}
	inspection, err = client.InspectUpload(ctx, invalidDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != UploadInvalid ||
		inspection.Error != MarkerErrorUnsupportedVersion {
		t.Fatalf("invalid inspection = %+v", inspection)
	}
	if _, err := client.DeleteUpload(ctx, invalidDir); err == nil {
		t.Fatal("delete accepted an invalid ownership marker")
	}
	getObject(ctx, t, st, invalidDir+"/"+MarkerFilename)

	syncCfg := *cfg
	syncCfg.DisableManifest = false
	syncCfg.ManifestPath = filepath.Join(t.TempDir(), "manifest.jsonl")
	syncCfg.Profile = "receiver"
	syncClient, err := New(ctx, &syncCfg)
	if err != nil {
		t.Fatal(err)
	}
	synced, err := syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Added) != 2 || synced.Incomplete != 1 ||
		synced.Invalid != 1 {
		t.Fatalf("initial sync = %+v, %v", synced, err)
	}

	deleted, err := client.DeleteUpload(ctx, res.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Keys) != 3 || deleted.Keys[len(deleted.Keys)-1] != markerKey {
		t.Fatalf("delete operation order = %v", deleted.Keys)
	}
	objects, err := st.listKeys(ctx, dirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 0 {
		t.Fatalf("objects remain after delete: %+v", objects)
	}
	synced, err = syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Tombstoned) != 1 {
		t.Fatalf("deletion sync = %+v, %v", synced, err)
	}
	putIntegrationObject(ctx, t, st, object{
		Key: markerKey, Body: markerObject.body,
		ContentType: markerContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key: res.SourceKey, Body: source.body,
		ContentType: sourceContentType,
	})
	putIntegrationObject(ctx, t, st, object{
		Key: res.Key, Body: page.body, ContentType: pageContentType,
	})
	synced, err = syncClient.SyncManifest(ctx, SyncManifestOptions{Prune: true})
	if err != nil || len(synced.Added) != 1 {
		t.Fatalf("restoration sync = %+v, %v", synced, err)
	}
	if _, err := client.DeleteUpload(ctx, res.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUpload(ctx, partialDir); err != nil {
		t.Fatal(err)
	}
	collectionDeleted, err := client.DeleteUpload(ctx, collection.Files[0].URL)
	if err != nil || collectionDeleted.Keys[len(collectionDeleted.Keys)-1] != collection.MarkerKey {
		t.Fatalf("collection delete = %+v, %v", collectionDeleted, err)
	}

	remote, err = client.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote) != 1 || remote[0].Dir != invalidDir {
		t.Fatalf("remote uploads after deletes = %+v", remote)
	}

	testHTTPBackendRoundTrip(ctx, t, cfg)
}

func testHTTPBackendRoundTrip(
	ctx context.Context, t *testing.T, storageConfig *Config,
) {
	t.Helper()
	serverConfig := *storageConfig
	serverConfig.DisableManifest = false
	serverConfig.ManifestPath = filepath.Join(t.TempDir(), "server-manifest.jsonl")
	serverConfig.Profile = "server"
	serverConfig.Repository = "none"
	serverClient, err := New(ctx, &serverConfig)
	if err != nil {
		t.Fatal(err)
	}
	const token = "01234567890123456789012345678901"
	handler, err := httpapi.NewHandler(
		&HTTPOperations{Client: serverClient, ServerVersion: "integration"},
		httpapi.Options{Token: token},
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	remoteClient, err := New(ctx, &Config{
		Backend: BackendAirplan, APIURL: server.URL, APIToken: token,
		Repository: "none",
	})
	if err != nil {
		t.Fatal(err)
	}

	document, err := remoteClient.Upload(ctx, Input{
		Reader: strings.NewReader("# HTTP Integration\n\nRemote body.\n"),
		Name:   "http-integration.md", MaxSize: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := remoteClient.ListManifest(ctx, ListManifestOptions{
		Scope: ManifestScopeService,
	})
	if err != nil || len(manifest.Records) != 1 ||
		manifest.Records[0].MarkerKey != document.MarkerKey {
		t.Fatalf("HTTP manifest = %+v, %v", manifest, err)
	}
	storage, err := remoteClient.ListRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	remoteByDir(t, storage, document.ID)
	inspection, err := remoteClient.InspectUpload(ctx, document.URL)
	if err != nil || inspection.State != UploadComplete {
		t.Fatalf("HTTP inspection = %+v, %v", inspection, err)
	}
	var page bytes.Buffer
	if _, err := remoteClient.GetUploadTo(
		ctx, document.URL, GetOptions{}, &page,
	); err != nil || !strings.Contains(page.String(), "Remote body") {
		t.Fatalf("HTTP download = %q, %v", page.String(), err)
	}
	plan, err := remoteClient.PlanPurge(ctx, PurgePlanOptions{
		Source: UploadSourceManifest, All: true,
	})
	if err != nil || len(plan.Candidates) != 1 ||
		plan.Candidates[0].UploadID != document.ID {
		t.Fatalf("HTTP purge plan = %+v, %v", plan, err)
	}
	purged, err := remoteClient.Purge(ctx, PurgeRequest{
		UploadIDs: []string{document.ID},
	})
	if err != nil || len(purged.Items) != 1 ||
		purged.Items[0].Deleted == nil {
		t.Fatalf("HTTP purge = %+v, %v", purged, err)
	}

	collection, err := remoteClient.UploadFiles(ctx, FilesInput{
		Files: []FileInput{
			{
				Name: "shot.png", Reader: bytes.NewReader([]byte("png")),
				Size: 3,
			},
			{
				Name: "notes.txt", Reader: bytes.NewReader([]byte("notes")),
				Size: 5,
			},
		},
		MaxSize: -1, MaxTotalSize: -1,
	})
	if err != nil || len(collection.Files) != 2 {
		t.Fatalf("HTTP collection = %+v, %v", collection, err)
	}
	var member bytes.Buffer
	if _, err := remoteClient.GetUploadTo(
		ctx, collection.Files[1].URL, GetOptions{}, &member,
	); err != nil || member.String() != "notes" {
		t.Fatalf("HTTP collection member = %q, %v", member.String(), err)
	}
	if _, err := remoteClient.DeleteUpload(
		ctx, collection.Files[0].URL,
	); err != nil {
		t.Fatal(err)
	}
}

type fetchedObject struct {
	body         []byte
	contentType  string
	cacheControl string
	metaTitle    string
}

func getObject(
	ctx context.Context,
	t *testing.T,
	st *storage,
	key string,
) fetchedObject {
	t.Helper()

	out, err := st.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	defer func() { _ = out.Body.Close() }()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}

	return fetchedObject{
		body:         body,
		contentType:  aws.ToString(out.ContentType),
		cacheControl: aws.ToString(out.CacheControl),
		metaTitle:    out.Metadata["title"],
	}
}

func putIntegrationObject(
	ctx context.Context,
	t *testing.T,
	st *storage,
	obj object,
) {
	t.Helper()
	if err := st.put(ctx, obj); err != nil {
		t.Fatal(err)
	}
}

func remoteByDir(
	t *testing.T,
	uploads []RemoteUpload,
	dir string,
) RemoteUpload {
	t.Helper()
	for _, upload := range uploads {
		if upload.Dir == dir {
			return upload
		}
	}
	t.Fatalf("remote upload %q not found in %+v", dir, uploads)
	return RemoteUpload{}
}
